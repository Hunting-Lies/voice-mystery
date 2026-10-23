package tts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

const HostKey = "__host__"

type Service struct {
	client *dashClient

	mu       sync.Mutex
	assign   map[string]Voice // speakerKey -> voice
	playerN  int              // 已分配玩家数（不含主持）
	cache    map[string]cachedAudio
	cacheMax int
}

type cachedAudio struct {
	Bytes       []byte
	ContentType string
}

func NewService() *Service {
	return &Service{
		client:   newDashClientFromEnv(),
		assign:   make(map[string]Voice),
		cache:    make(map[string]cachedAudio),
		cacheMax: 128,
	}
}

func (s *Service) Enabled() bool {
	return s != nil && s.client != nil
}

func (s *Service) Status() map[string]any {
	enabled := s.Enabled()
	model := ""
	if enabled {
		model = s.client.model
	}
	return map[string]any{
		"enabled": enabled,
		"model":   model,
		"provider": "dashscope-cosyvoice",
		"voices":  Catalog(),
		"hostVoice": HostVoice,
		"hint": "设置环境变量 DASHSCOPE_API_KEY 后启用灵矽同款阿里 CosyVoice 音色",
	}
}

// EnsureSpeaker 为 speakerKey 分配固定音色；主持用豪放可爱女，玩家轮换三男声。
func (s *Service) EnsureSpeaker(speakerKey string) Voice {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.assign[speakerKey]; ok {
		return v
	}
	var v Voice
	if speakerKey == HostKey || speakerKey == "" {
		v = HostVoice
	} else {
		v = PlayerPool[s.playerN%len(PlayerPool)]
		s.playerN++
	}
	s.assign[speakerKey] = v
	return v
}

func (s *Service) Roster(playerIDs []string) map[string]Voice {
	out := make(map[string]Voice, len(playerIDs)+1)
	out[HostKey] = s.EnsureSpeaker(HostKey)
	for _, id := range playerIDs {
		if id == "" {
			continue
		}
		out[id] = s.EnsureSpeaker(id)
	}
	return out
}

func (s *Service) ResetAssignments() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assign = make(map[string]Voice)
	s.playerN = 0
}

// Synthesize 合成音频；未启用时返回错误。
func (s *Service) Synthesize(ctx context.Context, speakerKey, text string) (Voice, []byte, string, error) {
	if !s.Enabled() {
		return Voice{}, nil, "", fmt.Errorf("TTS 未启用：请设置 DASHSCOPE_API_KEY")
	}
	voice := s.EnsureSpeaker(speakerKey)
	cacheKey := voice.ID + "|" + hashText(text)

	s.mu.Lock()
	if hit, ok := s.cache[cacheKey]; ok {
		s.mu.Unlock()
		return voice, hit.Bytes, hit.ContentType, nil
	}
	s.mu.Unlock()

	audio, ct, err := s.client.synthesize(ctx, voice.ID, text)
	if err != nil {
		return voice, nil, "", err
	}
	if ct == "" {
		ct = "audio/mpeg"
	}

	s.mu.Lock()
	if len(s.cache) >= s.cacheMax {
		// 简单淘汰：清空一半
		n := 0
		for k := range s.cache {
			delete(s.cache, k)
			n++
			if n >= s.cacheMax/2 {
				break
			}
		}
	}
	s.cache[cacheKey] = cachedAudio{Bytes: audio, ContentType: ct}
	s.mu.Unlock()

	return voice, audio, ct, nil
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:12])
}
