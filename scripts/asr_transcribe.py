#!/usr/bin/env python3
"""本地 Whisper 转写：读取音频文件路径，输出识别文本。"""
from __future__ import annotations

import os
import struct
import sys
import wave


def load_wav_mono_f32(path: str):
    """读取 16-bit PCM wav，返回 float32 单声道样本（绕过 PyAV 版本差异）。"""
    import numpy as np

    with wave.open(path, "rb") as wf:
        channels = wf.getnchannels()
        sample_width = wf.getsampwidth()
        rate = wf.getframerate()
        nframes = wf.getnframes()
        raw = wf.readframes(nframes)

    if sample_width != 2:
        raise ValueError(f"unsupported wav sample width: {sample_width}")

    samples = struct.unpack("<" + "h" * (len(raw) // 2), raw)
    arr = np.array(samples, dtype=np.float32) / 32768.0
    if channels > 1:
        arr = arr.reshape(-1, channels).mean(axis=1)
    return arr, rate


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: asr_transcribe.py <audio-file>", file=sys.stderr)
        return 2
    path = sys.argv[1]
    if not os.path.isfile(path):
        print(f"file not found: {path}", file=sys.stderr)
        return 2

    from faster_whisper import WhisperModel

    model_size = os.environ.get("WHISPER_MODEL", "tiny")
    model = WhisperModel(model_size, device="cpu", compute_type="int8")
    audio, sr = load_wav_mono_f32(path)
    if sr != 16000:
        import numpy as np

        duration = len(audio) / float(sr)
        target_len = int(round(duration * 16000))
        if target_len < 1:
            target_len = 1
        x_old = np.linspace(0.0, duration, num=len(audio), endpoint=False)
        x_new = np.linspace(0.0, duration, num=target_len, endpoint=False)
        audio = np.interp(x_new, x_old, audio).astype(np.float32)
    segments, _info = model.transcribe(
        audio,
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
    sys.stdout.write(text)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as e:
        print(f"asr error: {e}", file=sys.stderr)
        raise SystemExit(1)
