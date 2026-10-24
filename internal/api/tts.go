package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jenny-zeng/lai-dian-yi-yun/internal/tts"
)

func (s *Server) handleTTSStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.tts.Status())
}

func (s *Server) handleTTSRoster(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		PlayerIDs []string `json:"playerIds"`
		Reset     bool     `json:"reset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效 JSON"})
		return
	}
	if body.Reset {
		s.tts.ResetAssignments()
	}
	roster := s.tts.Roster(body.PlayerIDs)
	// 转成前端友好结构
	out := make(map[string]any, len(roster))
	for k, v := range roster {
		out[k] = v
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": s.tts.Enabled(),
		"roster":  out,
	})
}

func (s *Server) handleTTSSynthesize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Text       string `json:"text"`
		SpeakerKey string `json:"speakerKey"` // __host__ 或 playerId
		PlayerID   string `json:"playerId"`
		Prefix     string `json:"prefix"`
		VoiceID    string `json:"voiceId"` // 可选，与 roster 一致
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效 JSON", "fallback": true})
		return
	}
	text := strings.TrimSpace(body.Prefix + body.Text)
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "text 为空", "fallback": true})
		return
	}
	key := strings.TrimSpace(body.SpeakerKey)
	if key == "" {
		key = strings.TrimSpace(body.PlayerID)
	}
	if key == "" {
		key = tts.HostKey
	}

	if !s.tts.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":    "未配置 DASHSCOPE_API_KEY，将使用浏览器 TTS",
			"fallback": true,
		})
		return
	}

	voice, audio, ct, err := s.tts.Synthesize(r.Context(), key, text, strings.TrimSpace(body.VoiceID))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":    err.Error(),
			"fallback": true,
			"voice":    voice,
		})
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Voice-Id", voice.ID)
	// 中文名仅通过 JSON roster 下发；响应头非 ASCII 易被浏览器误解析为乱码
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
}
