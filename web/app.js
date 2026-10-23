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
    speechRec: null,       // Web Speech API 兜底
    speechRecMode: "",     // hold | toggle
    speechDraft: "",
    recordEngine: "",      // media | speech
    speakingHost: false,
    submitting: false,
    asrBusy: false,
    // TTS：灵矽 CosyVoice 优先，浏览器兜底；一人一音色 + 串行队列
    lingxiTTS: false,
    voiceRoster: Object.create(null), // speakerKey -> {id,name,gender}
    cachedVoices: [],
    voiceAssign: Object.create(null),
    ttsQueue: [],
    ttsPlaying: false,
    ttsGeneration: 0,
    currentAudio: null,
    spokenStmtCount: 0,
    spokenCrossKeys: Object.create(null),
  };

  const HOST_VOICE_KEY = "__host__";

  refreshLingxiTTSStatus();
  // 预加载浏览器音色（无密钥时兜底）
  warmUpVoices();
  if (window.speechSynthesis) {
    window.speechSynthesis.addEventListener("voiceschanged", () => {
      state.cachedVoices = window.speechSynthesis.getVoices() || [];
      Object.keys(state.voiceAssign).forEach((k) => {
        if (!state.voiceAssign[k]?.voice) delete state.voiceAssign[k];
      });
    });
  }

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
    clearTTS();
    await advanceFromBriefing();
  };

  $("btnSubmitSpeak").onclick = (e) => {
    e.preventDefault();
    submitSpeak();
  };
  $("btnSpeakHost").onclick = () => {
    enqueueTTS({
      text: state.snapshot?.hostLine || "",
      profileKey: HOST_VOICE_KEY,
      forceClear: true,
    });
  };
  $("btnSkipHost").onclick = async () => {
    clearTTS();
    // 播报阶段：跳过朗读并直接进入盘问；其他阶段仅打断当前播报
    if (state.snapshot?.phase === "briefing") {
      await advanceFromBriefing();
    }
  };

  async function advanceFromBriefing() {
    if (!state.roomCode) return;
    if (state.snapshot?.phase !== "briefing") return;
    await api("POST", `/api/rooms/${state.roomCode}/advance`, {});
  }
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
    state.spokenStmtCount = 0;
    state.spokenCrossKeys = Object.create(null);
    state.voiceAssign = Object.create(null);
    state.voiceRoster = Object.create(null);
    state.lastHost = "";
    clearTTS();
    $("landing").classList.add("hidden");
    $("table").classList.remove("hidden");
    $("roomCode").textContent = state.roomCode;
    connectWS();
    const snap = res.state || state.snapshot;
    syncVoiceRoster(snap, true).finally(() => render(snap));
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
    state.snapshot = snap;
    $("meta").textContent = `${phaseLabel[snap.phase] || snap.phase} · ${snap.players.length} 人`;
    $("phasePill").textContent = phaseLabel[snap.phase] || snap.phase;
    $("hostLine").textContent = snap.hostLine || "";
    $("roomCode").textContent = snap.roomCode;

    // 同步灵矽音色座位表（一人一音）；无密钥时走浏览器兜底档案
    syncVoiceRoster(snap, false);
    voiceProfileFor(HOST_VOICE_KEY);
    (snap.players || []).forEach((p) => voiceProfileFor(p.id));

    // 先入队他人发言（盘问 / 质询回答），再播主持词，保证「先听人话再听主持」
    enqueueNewPlayerReplies(snap);

    if (snap.hostLine && snap.hostLine !== state.lastHost) {
      state.lastHost = snap.hostLine;
      document.querySelector(".phone").classList.add("live");
      setTimeout(() => document.querySelector(".phone").classList.remove("live"), 1800);

      if (snap.phase === "briefing") {
        enqueueTTS({ text: snap.hostLine, profileKey: HOST_VOICE_KEY, forceClear: true });
      } else if (["statement", "cross_exam", "vote", "end"].includes(snap.phase) && snap.hostLine.length < 90) {
        // 不 forceClear：与刚入队的玩家发言串行，避免「发言完毕」盖掉正文
        enqueueTTS({
          text: snap.hostLine,
          profileKey: HOST_VOICE_KEY,
          forceClear: false,
        });
      }
    }

    const list = $("playerList");
    list.innerHTML = "";
    snap.players.forEach((p) => {
      const li = document.createElement("li");
      if (p.id === snap.currentSpeakerId) li.classList.add("speaking");
      const voiceName = state.voiceRoster[p.id]?.name || "";
      const tags = [
        p.isBot ? "BOT" : "HUMAN",
        voiceName,
        p.id === snap.currentSpeakerId ? "发言中" : "",
      ].filter(Boolean).join(" · ");
      li.innerHTML = `<span>${escapeHtml(p.name)}${p.id === state.playerId ? "（我）" : ""}</span><span class="tag">${escapeHtml(tags)}</span>`;
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
    btn.disabled = false;
    btn.textContent = briefing && state.speakingHost
      ? "跳过朗读，进入盘问"
      : "听完了，进入盘问";
    const skip = $("btnSkipHost");
    if (skip) skip.disabled = false;
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
    if (state.asrBusy || state.mediaRecorder || state.speechRec) return;
    clearTTS();

    // 1) 优先 MediaRecorder + getUserMedia（兼容前缀 / 宽松约束）
    try {
      await startMediaRecording(mode);
      return;
    } catch (err) {
      console.warn("[mic] media path failed:", err);
      cleanupStream();
      state.mediaRecorder = null;
      // 2) 回退：浏览器内置语音识别（Chrome/Edge/Safari 等）
      if (startSpeechRecognition(mode)) return;
      setSpeakStatus(micErrorHint(err), "err");
      resetMicUI();
    }
  }

  async function startMediaRecording(mode) {
    if (typeof MediaRecorder === "undefined") {
      throw Object.assign(new Error("MediaRecorder unsupported"), { name: "NotSupportedError" });
    }
    const stream = await requestMicStream();
    state.mediaStream = stream;
    state.recordChunks = [];
    const mime = pickRecorderMime();
    let rec;
    try {
      rec = mime ? new MediaRecorder(stream, { mimeType: mime }) : new MediaRecorder(stream);
    } catch (e) {
      // Safari 等对 mimeType 挑剔：再试无类型
      rec = new MediaRecorder(stream);
    }
    state.mediaRecorder = rec;
    state.recordEngine = "media";
    rec.ondataavailable = (ev) => {
      if (ev.data && ev.data.size > 0) state.recordChunks.push(ev.data);
    };
    // timeslice：部分 Safari 对 timeslice 不稳，失败则改一次性录
    try {
      rec.start(250);
    } catch (_) {
      rec.start();
    }
    markMicActive(mode);
    setSpeakStatus("录音中…请说话", "ok");
    clearInterval(state.recordTimer);
    state.recordTimer = setInterval(() => {
      const sec = ((Date.now() - state.recordStartedAt) / 1000).toFixed(1);
      setSpeakStatus(`录音中 ${sec}s…${state.holdingMic ? "松开后识别" : "再点一次结束"}`, "ok");
    }, 200);
  }

  function startSpeechRecognition(mode) {
    const SR = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SR) return false;
    let rec;
    try {
      rec = new SR();
    } catch (_) {
      return false;
    }
    rec.lang = "zh-CN";
    rec.continuous = true;
    rec.interimResults = true;
    rec.maxAlternatives = 1;
    state.speechRec = rec;
    state.speechRecMode = mode;
    state.speechDraft = "";
    state.recordEngine = "speech";

    rec.onresult = (ev) => {
      let finalText = "";
      let interim = "";
      for (let i = 0; i < ev.results.length; i++) {
        const r = ev.results[i];
        if (r.isFinal) finalText += r[0].transcript;
        else interim += r[0].transcript;
      }
      state.speechDraft = `${finalText}${interim}`.trim();
      if (state.speechDraft) {
        setSpeakStatus(`识别中：${state.speechDraft.slice(0, 36)}${state.speechDraft.length > 36 ? "…" : ""}`, "ok");
      }
    };
    rec.onerror = (ev) => {
      console.warn("[speechRec]", ev.error);
      if (ev.error === "not-allowed" || ev.error === "service-not-allowed") {
        setSpeakStatus("麦克风或语音识别权限被拒绝，请到浏览器设置允许后重试，或直接打字。", "err");
      }
    };
    rec.onend = () => {
      // toggle/hold 结束时由 stopRecording 收口；若被系统中断则尝试续听
      if ((state.holdingMic || state.toggleRecording) && state.speechRec === rec) {
        try { rec.start(); } catch (_) { /* ignore */ }
      }
    };

    try {
      rec.start();
    } catch (err) {
      console.warn(err);
      state.speechRec = null;
      return false;
    }
    markMicActive(mode);
    setSpeakStatus("浏览器语音识别中…请说话（松开后写入输入框）", "ok");
    clearInterval(state.recordTimer);
    state.recordTimer = setInterval(() => {
      const sec = ((Date.now() - state.recordStartedAt) / 1000).toFixed(1);
      setSpeakStatus(`浏览器识别中 ${sec}s…${state.speechDraft ? "已听到内容" : "请说话"}`, "ok");
    }, 200);
    return true;
  }

  function markMicActive(mode) {
    state.holdingMic = mode === "hold";
    state.toggleRecording = mode === "toggle";
    state.recordStartedAt = Date.now();
    $("btnMic").classList.toggle("holding", state.holdingMic);
    $("btnMicToggle").classList.toggle("mic-on", state.toggleRecording);
    $("btnMic").textContent = state.holdingMic ? "松开结束" : "按住说话";
    $("btnMicToggle").textContent = state.toggleRecording ? "停止录音" : "点按录音";
  }

  /** 兼容旧前缀 + 非标准环境；先完整约束，失败再 {audio:true} */
  async function requestMicStream() {
    ensureMediaDevicesPolyfill();
    const gum = navigator.mediaDevices?.getUserMedia?.bind(navigator.mediaDevices);
    if (!gum) {
      const err = new Error("getUserMedia unavailable");
      err.name = window.isSecureContext === false ? "SecurityError" : "NotSupportedError";
      throw err;
    }
    try {
      return await gum({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          channelCount: 1,
        },
      });
    } catch (e1) {
      // 约束过严（部分国产浏览器 / 旧 Safari）
      try {
        return await gum({ audio: true });
      } catch (e2) {
        throw e2 || e1;
      }
    }
  }

  function ensureMediaDevicesPolyfill() {
    if (!navigator.mediaDevices) {
      try {
        navigator.mediaDevices = {};
      } catch (_) {
        return;
      }
    }
    if (typeof navigator.mediaDevices.getUserMedia === "function") return;
    const legacy =
      navigator.getUserMedia ||
      navigator.webkitGetUserMedia ||
      navigator.mozGetUserMedia ||
      navigator.msGetUserMedia;
    if (!legacy) return;
    navigator.mediaDevices.getUserMedia = (constraints) =>
      new Promise((resolve, reject) => {
        legacy.call(navigator, constraints, resolve, reject);
      });
  }

  function pickRecorderMime() {
    if (typeof MediaRecorder === "undefined" || !MediaRecorder.isTypeSupported) return "";
    const candidates = [
      "audio/webm;codecs=opus",
      "audio/webm",
      "audio/mp4",
      "audio/aac",
      "audio/mpeg",
      "audio/ogg;codecs=opus",
      "audio/wav",
    ];
    for (const m of candidates) {
      try {
        if (MediaRecorder.isTypeSupported(m)) return m;
      } catch (_) { /* ignore */ }
    }
    return "";
  }

  function micErrorHint(err) {
    const name = err?.name || "";
    const msg = String(err?.message || err || "");
    if (window.isSecureContext === false || name === "SecurityError" || /secure|https|insecure/i.test(msg)) {
      return "当前页面不是安全上下文，浏览器禁止录音。请用 http://127.0.0.1:8080 打开，或配置 HTTPS；也可直接打字。";
    }
    if (name === "NotAllowedError" || name === "PermissionDeniedError") {
      return "麦克风权限被拒绝，请在地址栏允许麦克风后重试，或直接打字。";
    }
    if (name === "NotFoundError" || name === "DevicesNotFoundError") {
      return "未检测到麦克风设备，请检查接入后重试，或直接打字。";
    }
    if (name === "NotSupportedError" || name === "TypeError") {
      return "此浏览器录音能力不完整。已尝试兼容模式仍失败，请换 Chrome/Edge，或直接打字。";
    }
    return "无法打开麦克风，请允许权限后重试，或直接打字。";
  }

  function stopRecording({ skipAsr = false } = {}) {
    return new Promise((resolve) => {
      clearInterval(state.recordTimer);
      state.recordTimer = null;
      const wasHolding = state.holdingMic;
      const wasToggle = state.toggleRecording;
      state.holdingMic = false;
      state.toggleRecording = false;

      if (state.speechRec) {
        const rec = state.speechRec;
        const draft = state.speechDraft;
        state.speechRec = null;
        state.speechRecMode = "";
        try {
          rec.onend = null;
          rec.stop();
        } catch (_) { /* ignore */ }
        resetMicUI();
        if (!skipAsr) {
          const text = (draft || "").trim();
          if (text) {
            const prev = ($("speakText").value || "").trim();
            $("speakText").value = prev ? `${prev} ${text}` : text;
            setSpeakStatus(`识别成功：${text.slice(0, 40)}${text.length > 40 ? "…" : ""}，可修改后提交。`, "ok");
            $("speakText").focus();
          } else if (wasHolding || wasToggle) {
            setSpeakStatus("没有识别到文字，请再说一次或直接打字。", "err");
          }
        }
        state.speechDraft = "";
        state.recordEngine = "";
        resolve();
        return;
      }

      const rec = state.mediaRecorder;
      if (!rec || rec.state === "inactive") {
        cleanupStream();
        state.mediaRecorder = null;
        state.recordEngine = "";
        resetMicUI();
        resolve();
        return;
      }

      rec.onstop = async () => {
        const chunks = state.recordChunks.slice();
        const mime = rec.mimeType || "audio/webm";
        cleanupStream();
        state.mediaRecorder = null;
        state.recordEngine = "";
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
        if (blob.size < 400) {
          setSpeakStatus("录音太短，请按住多说一会儿，或直接打字。", "err");
          resolve();
          return;
        }
        await transcribeBlob(blob);
        resolve();
      };
      try {
        if (rec.state === "recording") rec.requestData?.();
        rec.stop();
      } catch (_) {
        cleanupStream();
        state.mediaRecorder = null;
        state.recordEngine = "";
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

  /** 盘问 / 质询：把新增的「其他人」回复按一人一音色入队朗读 */
  function enqueueNewPlayerReplies(snap) {
    if (!snap) return;
    const statements = snap.statements || [];
    for (let i = state.spokenStmtCount; i < statements.length; i++) {
      const s = statements[i];
      if (!s || s.playerId === state.playerId) continue;
      enqueueTTS({
        text: s.text,
        profileKey: s.playerId,
        prefix: `${s.name}说：`,
      });
    }
    state.spokenStmtCount = statements.length;

    (snap.crossQuestions || []).forEach((q, i) => {
      if (!q?.answer) return;
      const key = `${i}:${q.answer}`;
      if (state.spokenCrossKeys[key]) return;
      state.spokenCrossKeys[key] = true;
      if (q.targetPlayerId === state.playerId) return;
      const name = snap.players.find((p) => p.id === q.targetPlayerId)?.name || "玩家";
      enqueueTTS({
        text: q.answer,
        profileKey: q.targetPlayerId,
        prefix: `${name}回答：`,
      });
    });
  }

  function warmUpVoices() {
    if (!window.speechSynthesis) return;
    const load = () => {
      state.cachedVoices = window.speechSynthesis.getVoices() || [];
    };
    load();
    setTimeout(load, 200);
    setTimeout(load, 800);
  }

  function hashStr(s) {
    let h = 2166136261;
    const str = String(s || "");
    for (let i = 0; i < str.length; i++) {
      h ^= str.charCodeAt(i);
      h = Math.imul(h, 16777619);
    }
    return h >>> 0;
  }

  function listSpeechVoices() {
    const voices = state.cachedVoices?.length
      ? state.cachedVoices
      : (window.speechSynthesis?.getVoices() || []);
    if (voices.length) state.cachedVoices = voices;
    return voices;
  }

  /** 难听 / 停顿感强 / 搞怪音色：一律不用 */
  function isBadVoice(v) {
    const s = `${v.name} ${v.lang} ${v.voiceURI || ""}`;
    return /google|eloquence|novelty|whisper|zarvox|trinoids|boing|bubbles|cellos|deranged|bad news|good news|pipe organ|albert|bahh|bells|boing|jester|organ|kathy \(compact\)|compact|enhanced|robot|dummy|sampler|speechify|microsoft server/i.test(s);
  }

  function isChineseVoice(v) {
    const s = `${v.lang} ${v.name}`;
    return /zh[-_]|chinese|中文|普通话|国语|taiwan|hong.?kong|yue|cantonese|meijia|mei-?jia|tingting|ting-?ting|sinji|sin-?ji|xiaoxiao|yunxi|yunyang|xiaoyi|晓|云/i.test(s);
  }

  function isTaiwanVoice(v) {
    const s = `${v.lang} ${v.name}`;
    return /zh[-_]?tw|taiwan|meijia|mei-?jia|美佳|國語|国语.*(台|灣|湾)/i.test(s);
  }

  /** 好听中文音色池：台湾腔优先，再大陆/港，排除难听音色；按品质排序 */
  function curatedVoicePool() {
    const voices = listSpeechVoices().filter((v) => !isBadVoice(v));
    const zh = voices.filter(isChineseVoice);
    const uniq = [];
    const seen = new Set();
    for (const v of zh) {
      const id = v.voiceURI || `${v.name}|${v.lang}`;
      if (seen.has(id)) continue;
      seen.add(id);
      uniq.push(v);
    }

    const score = (v) => {
      let s = 0;
      if (isTaiwanVoice(v)) s += 100;
      if (/meijia|mei-?jia/i.test(v.name)) s += 40;
      if (/tingting|ting-?ting|xiao|yun|hui|yao|晓|云/i.test(v.name)) s += 20;
      if (/zh[-_]?hk|sinji|sin-?ji/i.test(`${v.lang} ${v.name}`)) s += 10;
      if (/compact|enhanced|eloquence/i.test(v.name)) s -= 50;
      return s;
    };
    uniq.sort((a, b) => score(b) - score(a) || a.name.localeCompare(b.name));
    return uniq;
  }

  function baseVoiceId(v) {
    return v ? (v.voiceURI || v.name) : "";
  }

  /** 已占用的「基础音色」集合（不含 #变体），保证先一人一声 */
  function usedBaseVoiceIds() {
    const used = new Set();
    Object.keys(state.voiceAssign).forEach((k) => {
      const p = state.voiceAssign[k];
      if (p?.baseId) used.add(p.baseId);
    });
    return used;
  }

  function reuseCountForBase(baseId) {
    let n = 0;
    Object.keys(state.voiceAssign).forEach((k) => {
      if (state.voiceAssign[k]?.baseId === baseId) n += 1;
    });
    return n;
  }

  /**
   * 一人一音色：优先台湾腔（Meijia），再其他好听中文声。
   * 系统中文音色不够时，在同一声线上做轻微 pitch 区分（不做夸张变调）。
   */
  function voiceProfileFor(key) {
    const cached = state.voiceAssign[key];
    if (cached?.voice) return cached;
    if (cached && !curatedVoicePool().length) return cached;
    if (cached) delete state.voiceAssign[key];

    const pool = curatedVoicePool();
    const used = usedBaseVoiceIds();
    const h = hashStr(key);

    let voice = null;
    if (key === HOST_VOICE_KEY) {
      voice = pool.find(isTaiwanVoice) || pool[0] || null;
    } else {
      voice = pool.find((v) => !used.has(baseVoiceId(v)));
      if (!voice && pool.length) {
        // 按池顺序轮转复用，保证每人都有独立 profile
        const slot = Object.keys(state.voiceAssign).filter((k) => k !== HOST_VOICE_KEY).length;
        voice = pool[slot % pool.length];
      }
    }

    const baseId = voice ? baseVoiceId(voice) : `fallback-${key}`;
    const reuseIndex = reuseCountForBase(baseId);

    let pitch = 1.0;
    let rate = 1.06; // 略快，减轻系统 TTS 拖沓
    if (reuseIndex > 0) {
      const variants = [
        { pitch: 0.95, rate: 1.07 },
        { pitch: 1.05, rate: 1.05 },
        { pitch: 0.92, rate: 1.08 },
        { pitch: 1.08, rate: 1.04 },
        { pitch: 0.98, rate: 1.09 },
      ];
      const v = variants[(h + reuseIndex) % variants.length];
      pitch = v.pitch;
      rate = v.rate;
    }

    const profile = {
      voice,
      baseId,
      voiceId: reuseIndex > 0 ? `${baseId}#${reuseIndex}` : baseId,
      pitch,
      rate,
      lang: voice?.lang || "zh-TW",
    };
    state.voiceAssign[key] = profile;
    return profile;
  }

  async function refreshLingxiTTSStatus() {
    try {
      const res = await fetch("/api/tts/status");
      const data = await res.json();
      state.lingxiTTS = !!data.enabled;
      const meta = $("meta");
      if (meta && !state.roomCode) {
        meta.textContent = data.enabled
          ? "灵矽 CosyVoice 音色已启用"
          : "浏览器 TTS（可设 DASHSCOPE_API_KEY）";
      }
    } catch (_) {
      state.lingxiTTS = false;
    }
  }

  let rosterSyncToken = 0;
  let lastRosterKey = "";
  async function syncVoiceRoster(snap, reset) {
    if (!snap?.players) return;
    const playerIds = snap.players.map((p) => p.id);
    const key = playerIds.join(",");
    if (!reset && key === lastRosterKey && Object.keys(state.voiceRoster).length) return;
    lastRosterKey = key;
    const token = ++rosterSyncToken;
    try {
      const res = await fetch("/api/tts/roster", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ playerIds, reset: !!reset }),
      });
      const data = await res.json();
      if (token !== rosterSyncToken) return;
      state.lingxiTTS = !!data.enabled;
      state.voiceRoster = data.roster || Object.create(null);
    } catch (_) { /* ignore */ }
  }

  function clearTTS() {
    state.ttsGeneration += 1;
    state.ttsQueue = [];
    state.ttsPlaying = false;
    state.speakingHost = false;
    if (state.currentAudio) {
      try {
        state.currentAudio.pause();
        state.currentAudio.src = "";
      } catch (_) { /* ignore */ }
      state.currentAudio = null;
    }
    try { window.speechSynthesis?.cancel(); } catch (_) { /* ignore */ }
    const btn = $("btnSpeakHost");
    if (btn) {
      btn.disabled = false;
      btn.textContent = "朗读主持词";
    }
    updateAdvanceButton();
  }

  function enqueueTTS({ text, profileKey, prefix = "", forceClear = false }) {
    if (!text || !window.speechSynthesis) return;
    if (state.holdingMic || state.toggleRecording || state.asrBusy) return;
    if (forceClear) clearTTS();
    state.ttsQueue.push({
      text: String(text),
      profileKey: profileKey || HOST_VOICE_KEY,
      prefix: prefix || "",
    });
    pumpTTS();
  }

  function pumpTTS() {
    if (state.ttsPlaying) return;
    const item = state.ttsQueue.shift();
    if (!item) {
      state.speakingHost = false;
      const btn = $("btnSpeakHost");
      if (btn) {
        btn.disabled = false;
        btn.textContent = "朗读主持词";
      }
      updateAdvanceButton();
      return;
    }

    const gen = state.ttsGeneration;
    state.ttsPlaying = true;
    const isHost = item.profileKey === HOST_VOICE_KEY;
    state.speakingHost = isHost;
    updateAdvanceButton();
    if (isHost) {
      $("btnSpeakHost").disabled = true;
      $("btnSpeakHost").textContent = "朗读中…";
    }

    const full = `${item.prefix || ""}${item.text}`.trim();
    speakChunks(full, item.profileKey, gen).finally(() => {
      if (gen !== state.ttsGeneration) return;
      state.ttsPlaying = false;
      pumpTTS();
    });
  }

  async function speakChunks(text, profileKey, gen) {
    if (gen !== state.ttsGeneration) return;
    // 优先灵矽同款 CosyVoice（温暖元气男 / 实力歌手男 / 智慧青年男 / 豪放可爱女）
    if (state.lingxiTTS) {
      const ok = await playLingxiAudio(text, profileKey, gen);
      if (ok || gen !== state.ttsGeneration) return;
    }
    await speakBrowserChunks(text, profileKey, gen);
  }

  async function playLingxiAudio(text, profileKey, gen) {
    try {
      const res = await fetch("/api/tts", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text, speakerKey: profileKey }),
      });
      const ct = res.headers.get("content-type") || "";
      if (!res.ok || !ct.includes("audio")) {
        if (res.status === 503) state.lingxiTTS = false;
        return false;
      }
      if (gen !== state.ttsGeneration) return true;
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const audio = new Audio(url);
      state.currentAudio = audio;
      const voiceName = res.headers.get("X-Voice-Name");
      if (voiceName && profileKey !== HOST_VOICE_KEY) {
        state.voiceRoster[profileKey] = {
          ...(state.voiceRoster[profileKey] || {}),
          name: voiceName,
          id: res.headers.get("X-Voice-Id") || "",
        };
      }
      await new Promise((resolve) => {
        const done = () => {
          URL.revokeObjectURL(url);
          if (state.currentAudio === audio) state.currentAudio = null;
          resolve();
        };
        audio.onended = done;
        audio.onerror = done;
        audio.play().catch(done);
      });
      return gen === state.ttsGeneration;
    } catch (_) {
      return false;
    }
  }

  function speakBrowserChunks(text, profileKey, gen) {
    return new Promise((resolve) => {
      if (gen !== state.ttsGeneration || !window.speechSynthesis) {
        resolve();
        return;
      }
      const profile = voiceProfileFor(profileKey);
      const chunks = splitSpeech(text);
      let idx = 0;
      const speakNext = () => {
        if (gen !== state.ttsGeneration) {
          resolve();
          return;
        }
        if (idx >= chunks.length) {
          resolve();
          return;
        }
        const u = new SpeechSynthesisUtterance(chunks[idx++]);
        u.lang = profile.lang || "zh-TW";
        u.rate = profile.rate;
        u.pitch = profile.pitch;
        if (profile.voice) {
          u.voice = profile.voice;
          if (profile.voice.lang) u.lang = profile.voice.lang;
        }
        u.onend = speakNext;
        u.onerror = speakNext;
        window.speechSynthesis.speak(u);
      };
      speakNext();
    });
  }

  /** 仅超长主持词才切片；玩家短发言整段朗读，消除句间停顿 */
  function splitSpeech(text) {
    const raw = String(text || "").replace(/\s*\n+\s*/g, "，").trim();
    if (!raw) return [""];
    const max = 280;
    if (raw.length <= max) return [raw];

    const parts = [];
    const sentences = raw.split(/(?<=[。！？；;])/);
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
    return parts.length ? parts : [raw];
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
