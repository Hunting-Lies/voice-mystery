package api

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jenny-zeng/lai-dian-yi-yun/internal/game"
	"github.com/jenny-zeng/lai-dian-yi-yun/internal/mcp"
	"github.com/jenny-zeng/lai-dian-yi-yun/internal/room"
	"github.com/jenny-zeng/lai-dian-yi-yun/internal/tts"
)

type Server struct {
	hub      *room.Hub
	mcp      *mcp.Server
	tts      *tts.Service
	upgrader websocket.Upgrader
	subsMu   sync.RWMutex
	subs     map[string]map[*wsClient]struct{}
}

type wsClient struct {
	roomCode string
	playerID string
	conn     *websocket.Conn
	send     chan []byte
	closed   sync.Once
}

func NewServer(hub *room.Hub, webRoot string) *Server {
	s := &Server{
		hub: hub,
		mcp: mcp.New(hub),
		tts: tts.NewService(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		subs: make(map[string]map[*wsClient]struct{}),
	}
	go s.broadcastLoop()
	_ = webRoot
	return s
}

func (s *Server) TTSEnabled() bool {
	return s.tts != nil && s.tts.Enabled()
}

func (s *Server) Handler(webRoot string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/asr", s.handleASR)
	mux.HandleFunc("/api/tts/status", s.handleTTSStatus)
	mux.HandleFunc("/api/tts/roster", s.handleTTSRoster)
	mux.HandleFunc("/api/tts", s.handleTTSSynthesize)
	mux.HandleFunc("/api/scripts", s.handleScripts)
	mux.HandleFunc("/api/rooms", s.handleRooms)
	mux.HandleFunc("/api/rooms/", s.handleRoomAction)
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/mcp/tools", s.mcp.HandleListTools)
	mux.HandleFunc("/mcp/call", s.mcp.HandleCallTool)
	mux.HandleFunc("/mcp/", s.mcp.HandleSSE)

	fs := http.FileServer(http.Dir(webRoot))
	mux.Handle("/", spaHandler(webRoot, fs))
	return withCORS(mux)
}

func spaHandler(webRoot string, fs http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/ws") || strings.HasPrefix(r.URL.Path, "/mcp") {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(webRoot, filepath.Clean(r.URL.Path))
		if strings.HasSuffix(r.URL.Path, "/") || filepath.Ext(path) == "" {
			http.ServeFile(w, r, filepath.Join(webRoot, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "game": "来电疑云"})
}

func (s *Server) handleScripts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scripts": game.ScriptMetas()})
}

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name     string `json:"name"`
		ScriptID string `json:"scriptId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.ScriptID != "" {
		if _, err := game.ScriptByID(body.ScriptID); err != nil {
			writeErr(w, err)
			return
		}
	}
	roomObj := s.hub.Create(body.ScriptID)
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "房主"
	}
	p, err := roomObj.AddPlayer(name, false)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.broadcastRoom(roomObj.Code)
	writeJSON(w, http.StatusOK, map[string]any{
		"roomCode": roomObj.Code,
		"player":   p,
		"state":    roomObj.Snapshot(),
		"private":  mustPrivate(roomObj, p.ID),
	})
}

func (s *Server) handleRoomAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/rooms/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	code := parts[0]
	roomObj, err := s.hub.Get(code)
	if err != nil {
		writeErr(w, err)
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, roomObj.Snapshot())
		return
	}
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	action := parts[1]
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body map[string]string
	_ = json.NewDecoder(r.Body).Decode(&body)

	switch action {
	case "join":
		p, err := roomObj.AddPlayer(body["name"], false)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.broadcastRoom(code)
		writeJSON(w, http.StatusOK, map[string]any{
			"player":  p,
			"state":   roomObj.Snapshot(),
			"private": mustPrivate(roomObj, p.ID),
		})
	case "fill-bots":
		_, err := roomObj.FillBots(game.MinPlayers)
		if err != nil {
			writeErr(w, err)
			return
		}
		s.broadcastRoom(code)
		writeJSON(w, http.StatusOK, roomObj.Snapshot())
	case "start":
		if err := roomObj.Start(); err != nil {
			writeErr(w, err)
			return
		}
		s.broadcastRoom(code)
		writeJSON(w, http.StatusOK, roomObj.Snapshot())
	case "advance":
		if err := roomObj.AdvanceFromBriefing(); err != nil {
			writeErr(w, err)
			return
		}
		s.broadcastRoom(code)
		writeJSON(w, http.StatusOK, roomObj.Snapshot())
	case "statement":
		if err := roomObj.SubmitStatement(body["playerId"], body["text"]); err != nil {
			writeErr(w, err)
			return
		}
		s.broadcastRoom(code)
		writeJSON(w, http.StatusOK, roomObj.Snapshot())
	case "cross":
		if err := roomObj.SubmitCrossAnswer(body["playerId"], body["text"]); err != nil {
			writeErr(w, err)
			return
		}
		s.broadcastRoom(code)
		writeJSON(w, http.StatusOK, roomObj.Snapshot())
	case "vote":
		if err := roomObj.CastVote(body["playerId"], body["targetId"]); err != nil {
			writeErr(w, err)
			return
		}
		s.broadcastRoom(code)
		writeJSON(w, http.StatusOK, roomObj.Snapshot())
	case "private":
		view, err := roomObj.PrivateView(body["playerId"])
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	default:
		http.NotFound(w, r)
	}
}

func mustPrivate(r *game.Room, playerID string) any {
	v, err := r.PrivateView(playerID)
	if err != nil {
		return nil
	}
	return v
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("room")
	playerID := r.URL.Query().Get("playerId")
	if code == "" {
		http.Error(w, "room required", http.StatusBadRequest)
		return
	}
	roomObj, err := s.hub.Get(code)
	if err != nil {
		writeErr(w, err)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &wsClient{
		roomCode: roomObj.Code,
		playerID: playerID,
		conn:     conn,
		send:     make(chan []byte, 16),
	}
	s.addSub(c)
	defer func() {
		s.removeSub(c)
		_ = conn.Close()
	}()

	// initial push
	s.pushState(c, roomObj)

	go func() {
		for msg := range c.send {
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			TargetID string `json:"targetId"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "statement":
			_ = roomObj.SubmitStatement(playerID, msg.Text)
		case "cross":
			_ = roomObj.SubmitCrossAnswer(playerID, msg.Text)
		case "vote":
			_ = roomObj.CastVote(playerID, msg.TargetID)
		case "start":
			_ = roomObj.Start()
		case "advance":
			_ = roomObj.AdvanceFromBriefing()
		case "fill_bots":
			_, _ = roomObj.FillBots(game.MinPlayers)
		case "ping":
			continue
		}
		s.broadcastRoom(roomObj.Code)
	}
}

func (s *Server) addSub(c *wsClient) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	if s.subs[c.roomCode] == nil {
		s.subs[c.roomCode] = make(map[*wsClient]struct{})
	}
	s.subs[c.roomCode][c] = struct{}{}
}

func (s *Server) removeSub(c *wsClient) {
	s.subsMu.Lock()
	if m, ok := s.subs[c.roomCode]; ok {
		delete(m, c)
		if len(m) == 0 {
			delete(s.subs, c.roomCode)
		}
	}
	s.subsMu.Unlock()
	c.closed.Do(func() { close(c.send) })
}

func (s *Server) broadcastLoop() {
	t := time.NewTicker(700 * time.Millisecond)
	defer t.Stop()
	lastHost := map[string]string{}
	for range t.C {
		s.subsMu.RLock()
		codes := make([]string, 0, len(s.subs))
		for code := range s.subs {
			codes = append(codes, code)
		}
		s.subsMu.RUnlock()
		for _, code := range codes {
			roomObj, err := s.hub.Get(code)
			if err != nil {
				continue
			}
			snap := roomObj.Snapshot()
			if lastHost[code] != snap.HostLine+string(snap.Phase) {
				lastHost[code] = snap.HostLine + string(snap.Phase)
				s.broadcastRoom(code)
			} else {
				// still push light sync for vote counts / deadlines
				s.broadcastRoom(code)
			}
		}
	}
}

func (s *Server) broadcastRoom(code string) {
	roomObj, err := s.hub.Get(code)
	if err != nil {
		return
	}
	s.subsMu.RLock()
	clients := make([]*wsClient, 0)
	for c := range s.subs[code] {
		clients = append(clients, c)
	}
	s.subsMu.RUnlock()
	for _, c := range clients {
		s.pushState(c, roomObj)
	}
}

func (s *Server) pushState(c *wsClient, roomObj *game.Room) {
	payload := map[string]any{
		"type":  "state",
		"state": roomObj.Snapshot(),
	}
	if c.playerID != "" {
		if priv, err := roomObj.PrivateView(c.playerID); err == nil {
			payload["private"] = priv
		}
	}
	data, _ := json.Marshal(payload)
	defer func() { _ = recover() }()
	select {
	case c.send <- data:
	default:
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	log.Printf("api error: %v", err)
	status := http.StatusBadRequest
	if err == room.ErrRoomNotFound {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
