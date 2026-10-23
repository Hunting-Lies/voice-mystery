package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var asrMu sync.Mutex

func (s *Server) handleASR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无法读取音频：" + err.Error()})
		return
	}
	file, hdr, err := r.FormFile("audio")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 audio 字段"})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if ext == "" {
		ext = ".webm"
	}
	tmpDir, err := os.MkdirTemp("", "lai-dian-asr-*")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "临时目录创建失败"})
		return
	}
	defer os.RemoveAll(tmpDir)

	inPath := filepath.Join(tmpDir, "src"+ext)
	outPath := filepath.Join(tmpDir, "out.wav")
	f, err := os.Create(inPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存音频失败"})
		return
	}
	if _, err := io.Copy(f, file); err != nil {
		f.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "写入音频失败"})
		return
	}
	f.Close()

	// 转 16k mono wav，兼容 Whisper
	ffmpeg := "ffmpeg"
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		ffmpeg = p
	}
	conv := exec.Command(ffmpeg, "-y", "-i", inPath, "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", outPath)
	var convErr bytes.Buffer
	conv.Stderr = &convErr
	if err := conv.Run(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "音频转码失败，请重试或直接打字：" + shortErr(convErr.String(), err),
		})
		return
	}

	text, err := runLocalWhisper(outPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "本地识别失败：" + err.Error() + "。请直接打字提交。",
		})
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"text":    "",
			"message": "没有识别到有效语音，请靠近麦克风再说一次，或直接打字。",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text})
}

func runLocalWhisper(wavPath string) (string, error) {
	asrMu.Lock()
	defer asrMu.Unlock()

	script := os.Getenv("ASR_SCRIPT")
	if script == "" {
		// 相对项目根
		candidates := []string{
			"scripts/asr_transcribe.py",
			filepath.Join(filepath.Dir(os.Args[0]), "scripts/asr_transcribe.py"),
		}
		for _, c := range candidates {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				script = c
				break
			}
		}
	}
	if script == "" {
		return "", fmt.Errorf("找不到 scripts/asr_transcribe.py")
	}

	py := "python3"
	if p, err := exec.LookPath("python3"); err == nil {
		py = p
	}
	ctxCmd := exec.Command(py, script, wavPath)
	ctxCmd.Env = append(os.Environ(), "WHISPER_MODEL="+getenv("WHISPER_MODEL", "tiny"))
	var stdout, stderr bytes.Buffer
	ctxCmd.Stdout = &stdout
	ctxCmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- ctxCmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return "", fmt.Errorf("%s", msg)
		}
		return strings.TrimSpace(stdout.String()), nil
	case <-time.After(120 * time.Second):
		_ = ctxCmd.Process.Kill()
		return "", fmt.Errorf("识别超时")
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func shortErr(stderr string, err error) string {
	s := strings.TrimSpace(stderr)
	if s == "" {
		return err.Error()
	}
	if len(s) > 180 {
		return s[len(s)-180:]
	}
	return s
}
