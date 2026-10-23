package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultModel   = "cosyvoice-v2"
	defaultAPIURL  = "https://dashscope.aliyuncs.com/api/v1/services/audio/tts/SpeechSynthesizer"
	defaultFormat  = "mp3"
	defaultSample  = 22050
	maxAudioBytes  = 20 << 20
)

type dashClient struct {
	apiKey string
	model  string
	apiURL string
	http   *http.Client
}

func newDashClientFromEnv() *dashClient {
	key := strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("ALIYUN_TTS_API_KEY"))
	}
	if key == "" {
		return nil
	}
	model := strings.TrimSpace(os.Getenv("COSYVOICE_MODEL"))
	if model == "" {
		model = defaultModel
	}
	apiURL := strings.TrimSpace(os.Getenv("COSYVOICE_API_URL"))
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	return &dashClient{
		apiKey: key,
		model:  model,
		apiURL: apiURL,
		http:   &http.Client{Timeout: 45 * time.Second},
	}
}

type synthesizeReq struct {
	Model string `json:"model"`
	Input struct {
		Text       string `json:"text"`
		Voice      string `json:"voice"`
		Format     string `json:"format"`
		SampleRate int    `json:"sample_rate"`
	} `json:"input"`
}

type synthesizeResp struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Output  struct {
		Audio struct {
			Data string `json:"data"`
			URL  string `json:"url"`
		} `json:"audio"`
	} `json:"output"`
}

func (c *dashClient) synthesize(ctx context.Context, voiceID, text string) (audio []byte, contentType string, err error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, "", fmt.Errorf("text empty")
	}
	reqBody := synthesizeReq{Model: c.model}
	reqBody.Input.Text = text
	reqBody.Input.Voice = voiceID
	reqBody.Input.Format = defaultFormat
	reqBody.Input.SampleRate = defaultSample

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAudioBytes))
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("dashscope HTTP %d: %s", resp.StatusCode, truncate(string(body), 240))
	}

	// 部分账号会直接返回二进制音频
	if strings.Contains(ct, "audio/") || looksLikeAudio(body) {
		if ct == "" || strings.Contains(ct, "octet-stream") {
			ct = "audio/mpeg"
		}
		return body, ct, nil
	}

	var parsed synthesizeResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", fmt.Errorf("unexpected TTS response: %s", truncate(string(body), 240))
	}
	if parsed.Code != "" && !strings.EqualFold(parsed.Code, "success") {
		return nil, "", fmt.Errorf("dashscope %s: %s", parsed.Code, parsed.Message)
	}
	if data := strings.TrimSpace(parsed.Output.Audio.Data); data != "" {
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, "", fmt.Errorf("decode audio data: %w", err)
		}
		return raw, "audio/mpeg", nil
	}
	if u := strings.TrimSpace(parsed.Output.Audio.URL); u != "" {
		return c.download(ctx, u)
	}
	return nil, "", fmt.Errorf("TTS 响应无音频: %s", truncate(string(body), 240))
}

func (c *dashClient) download(ctx context.Context, audioURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download audio HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAudioBytes))
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "audio/mpeg"
	}
	return body, ct, nil
}

func looksLikeAudio(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	// ID3 / MP3 frame / RIFF wav / Ogg
	if b[0] == 'I' && b[1] == 'D' && b[2] == '3' {
		return true
	}
	if b[0] == 0xff && (b[1]&0xe0) == 0xe0 {
		return true
	}
	if string(b[0:4]) == "RIFF" || string(b[0:4]) == "OggS" {
		return true
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
