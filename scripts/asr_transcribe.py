#!/usr/bin/env python3
"""本地 Whisper 转写：读取音频文件路径，输出识别文本。"""
from __future__ import annotations

import os
import sys

def main() -> int:
    if len(sys.argv) < 2:
        print("usage: asr_transcribe.py <audio-file>", file=sys.stderr)
        return 2
    path = sys.argv[1]
    if not os.path.isfile(path):
        print(f"file not found: {path}", file=sys.stderr)
        return 2

    # 延迟导入，便于先看到进程起来
    from faster_whisper import WhisperModel

    model_size = os.environ.get("WHISPER_MODEL", "tiny")
    # tiny/base 对中文可用；cpu int8 在 Mac 上足够演示
    model = WhisperModel(model_size, device="cpu", compute_type="int8")
    segments, info = model.transcribe(
        path,
        language="zh",
        beam_size=1,
        vad_filter=True,
    )
    parts = []
    for seg in segments:
        t = (seg.text or "").strip()
        if t:
            parts.append(t)
    text = "".join(parts).strip()
    # 统一输出一行文本
    sys.stdout.write(text)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as e:
        print(f"asr error: {e}", file=sys.stderr)
        raise SystemExit(1)
