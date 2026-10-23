package room

import (
	"errors"
	"sync"
	"time"

	"github.com/jenny-zeng/lai-dian-yi-yun/internal/game"
)

var ErrRoomNotFound = errors.New("房间不存在")

type Hub struct {
	mu    sync.RWMutex
	rooms map[string]*game.Room
}

func NewHub() *Hub {
	h := &Hub{rooms: make(map[string]*game.Room)}
	go h.loopBots()
	return h
}

func (h *Hub) Create(scriptID string) *game.Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := 0; i < 20; i++ {
		code := game.RandomRoomCode()
		if _, ok := h.rooms[code]; !ok {
			r := game.NewRoom(code, scriptID)
			h.rooms[code] = r
			return r
		}
	}
	code := game.RandomRoomCode() + game.RandomRoomCode()[:2]
	r := game.NewRoom(code, scriptID)
	h.rooms[code] = r
	return r
}

func (h *Hub) Get(code string) (*game.Room, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	r, ok := h.rooms[normalize(code)]
	if !ok {
		return nil, ErrRoomNotFound
	}
	return r, nil
}

func (h *Hub) loopBots() {
	t := time.NewTicker(800 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		h.mu.RLock()
		rooms := make([]*game.Room, 0, len(h.rooms))
		for _, r := range h.rooms {
			rooms = append(rooms, r)
		}
		h.mu.RUnlock()
		for _, r := range rooms {
			for i := 0; i < 4; i++ {
				if !r.AutoActBots() {
					break
				}
			}
		}
	}
}

func normalize(code string) string {
	out := make([]rune, 0, len(code))
	for _, c := range code {
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out = append(out, c)
		}
	}
	return string(out)
}
