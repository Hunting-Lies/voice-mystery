package tts

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
)

const (
	HostKey     = "__host__"
	NarratorKey = "__narrator__"
)

type Service struct {
	client *dashClient

	mu       sync.Mutex
	assign map[string]Voice // speakerKey -> voice
	cache  map[string]cachedAudio
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

// EnsureSpeaker 为 speakerKey 分配固定音色；主持单独音色，玩家按 id 稳定哈希到 PlayerPool。
func (s *Service) EnsureSpeaker(speakerKey string) Voice {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.assign[speakerKey]; ok {
		return v
	}
	var v Voice
	switch speakerKey {
	case HostKey, "":
		v = HostVoice
	case NarratorKey:
		v = NarratorVoice
	default:
		v = PlayerPool[voiceSlotIndex(speakerKey, len(PlayerPool))]
	}
	s.assign[speakerKey] = v
	return v
}

func voiceSlotIndex(speakerKey string, n int) int {
	if n <= 0 {
		return 0
	}
	sum := sha256.Sum256([]byte(speakerKey))
	return int(binary.BigEndian.Uint32(sum[:4]) % uint32(n))
}

func (s *Service) Roster(playerIDs []string) map[string]Voice {
	out := make(map[string]Voice, len(playerIDs)+2)
	out[HostKey] = s.EnsureSpeaker(HostKey)
	out[NarratorKey] = s.EnsureSpeaker(NarratorKey)
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
}

// Synthesize 合成音频；voiceIDOverride 非空时强制使用该 CosyVoice 音色（与 roster 展示一致）。
func (s *Service) Synthesize(ctx context.Context, speakerKey, text, voiceIDOverride string) (Voice, []byte, string, error) {
	if !s.Enabled() {
		return Voice{}, nil, "", fmt.Errorf("TTS 未启用：请设置 DASHSCOPE_API_KEY")
	}
	voice := s.EnsureSpeaker(speakerKey)
	if voiceIDOverride != "" {
		if v, ok := Lookup(voiceIDOverride); ok {
			voice = v
		}
	}
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
