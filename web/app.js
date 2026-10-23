(() => {
  const $ = (id) => document.getElementById(id);

  const state = {
    roomCode: null,
    playerId: null,
    playerName: null,
    ws: null,
    snapshot: null,
    private: null,
    lastHost: "",
    holdingMic: false,
    toggleRecording: false,
    mediaRecorder: null,
    mediaStream: null,
    recordChunks: [],
    recordTimer: null,
    recordStartedAt: 0,
    speakingHost: false,
    hostSpeakToken: 0,
    submitting: false,
    asrBusy: false,
  };

  const phaseLabel = {
    lobby: "大厅",
    briefing: "来电播报",
    statement: "盘问发言",
    cross_exam: "交叉质询",
    vote: "投票指认",
    reveal: "揭晓",
    end: "结算",
  };

  $("mcpUrl").textContent = `${location.origin}/mcp/tools`;

  $("btnCreate").onclick = async () => {
    const name = ($("nameInput").value || "接线员").trim();
    const res = await api("POST", "/api/rooms", { name });
    enterRoom(res);
  };

  $("btnJoin").onclick = () => {
    $("joinRow").classList.toggle("hidden");
  };

  $("btnJoinConfirm").onclick = async () => {
    const name = ($("nameInput").value || "玩家").trim();
    const code = ($("codeInput").value || "").trim();
    const res = await api("POST", `/api/rooms/${code}/join`, { name });
    enterRoom({ ...res, roomCode: code.toUpperCase() });
  };

  $("btnSolo").onclick = async () => {
    const name = ($("nameInput").value || "接线员").trim();
    const res = await api("POST", "/api/rooms", { name });
    await api("POST", `/api/rooms/${res.roomCode}/fill-bots`, {});
    enterRoom(res);
  };

  $("btnFillBots").onclick = async () => {
    await api("POST", `/api/rooms/${state.roomCode}/fill-bots`, {});
  };

  $("btnStart").onclick = async () => {
    await api("POST", `/api/rooms/${state.roomCode}/start`, {});
  };

  $("btnAdvance").onclick = async () => {
    if (state.speakingHost) return;
    window.speechSynthesis?.cancel();
    await api("POST", `/api/rooms/${state.roomCode}/advance`, {});
  };

  $("btnSubmitSpeak").onclick = (e) => {
    e.preventDefault();
    submitSpeak();
  };
  $("btnSpeakHost").onclick = () => {
    speakHost(state.snapshot?.hostLine || "", { force: true });
  };
  $("btnReplay").onclick = () => location.reload();
  $("btnMicToggle").onclick = () => toggleClickToTalk();

  bindHoldToTalk($("btnMic"));

  function setSpeakStatus(msg, kind = "") {
    const el = $("speakStatus");
    if (!el) return;
    el.textContent = msg || "";
    el.classList.remove("err", "ok");
    if (kind) el.classList.add(kind);
  }

  function enterRoom(res) {
    state.roomCode = res.roomCode || state.roomCode;
    state.playerId = res.player.id;
    state.playerName = res.player.name;
    state.private = res.private;
    $("landing").classList.add("hidden");
    $("table").classList.remove("hidden");
    $("roomCode").textContent = state.roomCode;
    connectWS();
    render(res.state || state.snapshot);
  }

  function connectWS() {
    if (state.ws && state.ws.readyState <= 1) {
      try { state.ws.close(); } catch (_) { /* ignore */ }
    }
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const url = `${proto}://${location.host}/ws?room=${encodeURIComponent(state.roomCode)}&playerId=${encodeURIComponent(state.playerId)}`;
    state.ws = new WebSocket(url);
    state.ws.onmessage = (ev) => {
      const msg = JSON.parse(ev.data);
      if (msg.type === "state") {
        if (msg.private) state.private = msg.private;
        render(msg.state);
      }
    };
    state.ws.onclose = () => {
      setTimeout(() => {
        if (state.roomCode) connectWS();
      }, 1200);
    };
  }

  function render(snap) {
    if (!snap) return;
    const prevPhase = state.snapshot?.phase;
    state.snapshot = snap;
    $("meta").textContent = `${phaseLabel[snap.phase] || snap.phase} · ${snap.players.length} 人`;
    $("phasePill").textContent = phaseLabel[snap.phase] || snap.phase;
    $("hostLine").textContent = snap.hostLine || "";
    $("roomCode").textContent = snap.roomCode;

    if (snap.hostLine && snap.hostLine !== state.lastHost) {
      state.lastHost = snap.hostLine;
      document.querySelector(".phone").classList.add("live");
      setTimeout(() => document.querySelector(".phone").classList.remove("live"), 1800);

      if (snap.phase === "briefing") {
        speakHost(snap.hostLine, { force: true });
      } else if (["statement", "cross_exam", "vote", "end"].includes(snap.phase) && snap.hostLine.length < 90) {
        speakHost(snap.hostLine, { force: prevPhase !== snap.phase });
      }
    }

    const list = $("playerList");
    list.innerHTML = "";
    snap.players.forEach((p) => {
      const li = document.createElement("li");
      if (p.id === snap.currentSpeakerId) li.classList.add("speaking");
      li.innerHTML = `<span>${escapeHtml(p.name)}${p.id === state.playerId ? "（我）" : ""}</span><span class="tag">${p.isBot ? "BOT" : "HUMAN"}${p.id === snap.currentSpeakerId ? " · 发言中" : ""}</span>`;
      list.appendChild(li);
    });

    const roleMap = { citizen: "市民", scammer: "来电骗子（隐藏）" };
    if (state.private && snap.phase !== "lobby") {
      $("secretRole").textContent = roleMap[state.private.role] || state.private.role || "未分配";
      $("secretCard").textContent = state.private.secretCard || "";
    } else {
      $("secretRole").textContent = "对局开始后揭晓";
      $("secretCard").textContent = "请保守你的秘密身份卡，不要让它直接出现在公开发言里。";
    }

    $("lobbyControls").classList.toggle("hidden", snap.phase !== "lobby");
    $("btnStart").disabled = !snap.canStart;

    $("briefingActions").classList.toggle("hidden", snap.phase !== "briefing");
    updateAdvanceButton();

    const myTurn = snap.currentSpeakerId === state.playerId;
    const needSpeak = (snap.phase === "statement" || snap.phase === "cross_exam") && myTurn;
    const wasHidden = $("speakActions").classList.contains("hidden");
    $("speakActions").classList.toggle("hidden", !needSpeak);
    if (needSpeak && wasHidden) {
      state.committedSpeech = ($("speakText").value || "").trim();
      setSpeakStatus("轮到你发言了：可打字，或按住/点击听写，然后提交。", "ok");
      $("speakText").focus();
    }
    if (!needSpeak && (state.holdingMic || state.toggleRecording)) {
      stopRecording({ skipAsr: true });
      setSpeakStatus("已过发言轮次");
    }

    $("voteActions").classList.toggle("hidden", snap.phase !== "vote");
    $("endActions").classList.toggle("hidden", snap.phase !== "end");

    if (snap.phase === "vote") {
      const grid = $("voteTargets");
      grid.innerHTML = "";
      snap.players.forEach((p) => {
        if (p.id === state.playerId) return;
        const b = document.createElement("button");
        b.className = "btn ghost";
        b.type = "button";
        b.textContent = p.name;
        b.onclick = async () => {
          try {
            await api("POST", `/api/rooms/${state.roomCode}/vote`, {
              playerId: state.playerId,
              targetId: p.id,
            });
          } catch (_) { /* api already alerts */ }
        };
        grid.appendChild(b);
      });
    }

    const log = $("log");
    const parts = [];
    (snap.statements || []).forEach((s) => {
      parts.push(`<div class="item"><div class="name">${escapeHtml(s.name)} · 盘问</div>${escapeHtml(s.text)}</div>`);
    });
    (snap.crossQuestions || []).forEach((q) => {
      const target = snap.players.find((p) => p.id === q.targetPlayerId);
      parts.push(`<div class="item"><div class="name">质询 ${escapeHtml(target?.name || "?")}</div>Q: ${escapeHtml(q.question)}${q.answer ? `<br/>A: ${escapeHtml(q.answer)}` : ""}</div>`);
    });
    if (snap.phase === "end") {
      parts.push(`<div class="item"><div class="name">结算</div>${escapeHtml(snap.hostLine)}（胜方：${snap.winner === "citizens" ? "市民" : "骗子"}）</div>`);
    }
    log.innerHTML = parts.join("") || "<div class='item'>等待对局开始…</div>";
  }

  function updateAdvanceButton() {
    const btn = $("btnAdvance");
    if (!btn) return;
    const briefing = state.snapshot?.phase === "briefing";
    btn.disabled = briefing && state.speakingHost;
    btn.textContent = briefing && state.speakingHost
      ? "主持词朗读中…"
      : "听完了，进入盘问";
  }

  async function submitSpeak() {
    if (state.submitting || state.asrBusy) return;
    if (state.holdingMic || state.toggleRecording) {
      await stopRecording();
    }
    const text = ($("speakText").value || "").trim();
    if (!text) {
      setSpeakStatus("请先按住说话，或直接打字，再提交。", "err");
      $("speakText").focus();
      return;
    }
    const snap = state.snapshot;
    if (!snap || (snap.phase !== "statement" && snap.phase !== "cross_exam")) {
      setSpeakStatus("当前不在发言阶段。", "err");
      return;
    }
    if (snap.currentSpeakerId !== state.playerId) {
      setSpeakStatus("还没轮到你发言。", "err");
      return;
    }

    state.submitting = true;
    $("btnSubmitSpeak").disabled = true;
    setSpeakStatus("正在提交…");
    const path = snap.phase === "cross_exam" ? "cross" : "statement";
    try {
      await api("POST", `/api/rooms/${state.roomCode}/${path}`, {
        playerId: state.playerId,
        text,
      });
      $("speakText").value = "";
      setSpeakStatus("提交成功，等待下一位。", "ok");
    } catch (err) {
      const msg = (err && err.error) || (err && err.message) || "提交失败，请重试";
      setSpeakStatus(msg, "err");
    } finally {
      state.submitting = false;
      $("btnSubmitSpeak").disabled = false;
    }
  }

  function bindHoldToTalk(btn) {
    if (!btn) return;
    let activePointer = null;
    const start = (e) => {
      if (e.button != null && e.button !== 0) return;
      e.preventDefault();
      activePointer = e.pointerId;
      try { btn.setPointerCapture(e.pointerId); } catch (_) { /* ignore */ }
      startRecording({ mode: "hold" });
    };
    const end = (e) => {
      if (activePointer != null && e.pointerId != null && e.pointerId !== activePointer) return;
      activePointer = null;
      if (state.holdingMic) stopRecording();
    };
    btn.addEventListener("pointerdown", start);
    btn.addEventListener("pointerup", end);
    btn.addEventListener("pointercancel", () => {
      /* 权限弹窗会 cancel；继续录，直到 pointerup 或再次点击 */
    });
    btn.addEventListener("contextmenu", (e) => e.preventDefault());
  }

  function toggleClickToTalk() {
    if (state.toggleRecording || state.holdingMic) {
      stopRecording();
      return;
    }
    startRecording({ mode: "toggle" });
  }

  async function startRecording({ mode }) {
    if (state.asrBusy || state.mediaRecorder) return;
    if (!navigator.mediaDevices?.getUserMedia) {
      setSpeakStatus("浏览器不支持录音，请直接打字。", "err");
      return;
    }

    window.speechSynthesis?.cancel();
    state.speakingHost = false;
    updateAdvanceButton();

    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          channelCount: 1,
        },
      });
      state.mediaStream = stream;
      state.recordChunks = [];
      const mime = MediaRecorder.isTypeSupported("audio/webm;codecs=opus")
        ? "audio/webm;codecs=opus"
        : (MediaRecorder.isTypeSupported("audio/mp4") ? "audio/mp4" : "");
      const rec = mime ? new MediaRecorder(stream, { mimeType: mime }) : new MediaRecorder(stream);
      state.mediaRecorder = rec;
      rec.ondataavailable = (ev) => {
        if (ev.data && ev.data.size > 0) state.recordChunks.push(ev.data);
      };
      rec.start(200);
      state.holdingMic = mode === "hold";
      state.toggleRecording = mode === "toggle";
      state.recordStartedAt = Date.now();
      $("btnMic").classList.toggle("holding", state.holdingMic);
      $("btnMicToggle").classList.toggle("mic-on", state.toggleRecording);
      $("btnMic").textContent = state.holdingMic ? "松开结束" : "按住说话";
      $("btnMicToggle").textContent = state.toggleRecording ? "停止录音" : "点按录音";
      setSpeakStatus("录音中…请说话", "ok");
      clearInterval(state.recordTimer);
      state.recordTimer = setInterval(() => {
        const sec = ((Date.now() - state.recordStartedAt) / 1000).toFixed(1);
        setSpeakStatus(`录音中 ${sec}s…${state.holdingMic ? "松开后识别" : "再点一次结束"}`, "ok");
      }, 200);
    } catch (err) {
      console.warn(err);
      setSpeakStatus("无法打开麦克风，请允许权限后重试，或直接打字。", "err");
      resetMicUI();
    }
  }

  function stopRecording({ skipAsr = false } = {}) {
    return new Promise((resolve) => {
      const rec = state.mediaRecorder;
      state.holdingMic = false;
      state.toggleRecording = false;
      clearInterval(state.recordTimer);
      state.recordTimer = null;

      if (!rec || rec.state === "inactive") {
        cleanupStream();
        resetMicUI();
        resolve();
        return;
      }

      rec.onstop = async () => {
        const chunks = state.recordChunks.slice();
        const mime = rec.mimeType || "audio/webm";
        cleanupStream();
        state.mediaRecorder = null;
        resetMicUI();
        if (skipAsr) {
          resolve();
          return;
        }
        if (!chunks.length) {
          setSpeakStatus("没有录到声音，请按住再说一次，或直接打字。", "err");
          resolve();
          return;
        }
        const blob = new Blob(chunks, { type: mime });
        if (blob.size < 800) {
          setSpeakStatus("录音太短，请按住多说一会儿，或直接打字。", "err");
          resolve();
          return;
        }
        await transcribeBlob(blob);
        resolve();
      };
      try {
        rec.stop();
      } catch (_) {
        cleanupStream();
        state.mediaRecorder = null;
        resetMicUI();
        resolve();
      }
    });
  }

  function cleanupStream() {
    if (state.mediaStream) {
      state.mediaStream.getTracks().forEach((t) => t.stop());
      state.mediaStream = null;
    }
  }

  function resetMicUI() {
    $("btnMic").classList.remove("holding");
    $("btnMicToggle").classList.remove("mic-on");
    $("btnMic").textContent = "按住说话";
    $("btnMicToggle").textContent = "点按录音";
  }

  async function transcribeBlob(blob) {
    state.asrBusy = true;
    setSpeakStatus("正在本地识别语音，请稍候（首次会下载模型）…", "ok");
    $("btnSubmitSpeak").disabled = true;
    try {
      const fd = new FormData();
      const ext = (blob.type || "").includes("mp4") ? "mp4" : "webm";
      fd.append("audio", blob, `speech.${ext}`);
      const res = await fetch("/api/asr", { method: "POST", body: fd });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        setSpeakStatus(data.error || "识别失败，请直接打字提交。", "err");
        return;
      }
      const text = (data.text || "").trim();
      if (!text) {
        setSpeakStatus(data.message || "没有识别到文字，请再说一次或打字。", "err");
        return;
      }
      const prev = ($("speakText").value || "").trim();
      $("speakText").value = prev ? `${prev} ${text}` : text;
      setSpeakStatus(`识别成功：${text.slice(0, 40)}${text.length > 40 ? "…" : ""}，可修改后提交。`, "ok");
      $("speakText").focus();
    } catch (err) {
      console.warn(err);
      setSpeakStatus("识别请求失败，请直接打字提交。", "err");
    } finally {
      state.asrBusy = false;
      $("btnSubmitSpeak").disabled = false;
    }
  }

  function speakHost(text, { force = false } = {}) {
    if (!text || !window.speechSynthesis) return Promise.resolve();
    if (state.holdingMic || state.toggleRecording || state.asrBusy) return Promise.resolve();
    if (state.speakingHost && !force) return Promise.resolve();

    const token = ++state.hostSpeakToken;
    window.speechSynthesis.cancel();
    state.speakingHost = true;
    updateAdvanceButton();
    $("btnSpeakHost").disabled = true;
    $("btnSpeakHost").textContent = "朗读中…";

    return new Promise((resolve) => {
      const finish = () => {
        if (token !== state.hostSpeakToken) return;
        state.speakingHost = false;
        $("btnSpeakHost").disabled = false;
        $("btnSpeakHost").textContent = "朗读主持词";
        updateAdvanceButton();
        resolve();
      };

      const chunks = splitSpeech(text);
      let idx = 0;
      const speakNext = () => {
        if (token !== state.hostSpeakToken) return;
        if (idx >= chunks.length) {
          finish();
          return;
        }
        const u = new SpeechSynthesisUtterance(chunks[idx++]);
        u.lang = "zh-CN";
        u.rate = 1.0;
        u.onend = speakNext;
        u.onerror = speakNext;
        window.speechSynthesis.speak(u);
      };
      speakNext();
    });
  }

  function splitSpeech(text) {
    const max = 120;
    const parts = [];
    const sentences = String(text).split(/(?<=[。！？\n])/);
    let buf = "";
    for (const s of sentences) {
      if ((buf + s).length > max && buf) {
        parts.push(buf);
        buf = s;
      } else {
        buf += s;
      }
    }
    if (buf.trim()) parts.push(buf);
    return parts.length ? parts : [text];
  }

  async function api(method, path, body) {
    const res = await fetch(path, {
      method,
      headers: { "Content-Type": "application/json" },
      body: body ? JSON.stringify(body) : undefined,
    });
    let data = {};
    try {
      data = await res.json();
    } catch (_) {
      data = { error: `服务返回异常 (${res.status})` };
    }
    if (!res.ok) {
      const err = new Error(data.error || "请求失败");
      err.error = data.error || "请求失败";
      alert(err.error);
      throw err;
    }
    return data;
  }

  function escapeHtml(s) {
    return String(s || "")
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;");
  }
})();
