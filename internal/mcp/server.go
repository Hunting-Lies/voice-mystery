package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jenny-zeng/lai-dian-yi-yun/internal/game"
	"github.com/jenny-zeng/lai-dian-yi-yun/internal/room"
)

// Server 暴露《来电疑云》工具面，供灵矽自定义 MCP / 智能体 Function Call 对接。
type Server struct {
	hub *room.Hub
}

func New(hub *room.Hub) *Server {
	return &Server{hub: hub}
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func (s *Server) Tools() []Tool {
	return []Tool{
		{
			Name:        "create_room",
			Description: "创建一局《来电疑云》房间，返回房间号。可选 host_name、script_id（剧本，见 GET /api/scripts）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host_name":  map[string]any{"type": "string", "description": "房主昵称"},
					"script_id":  map[string]any{"type": "string", "description": "剧本 ID，默认 midnight-bank"},
				},
			},
		},
		{
			Name:        "join_room",
			Description: "加入已有房间。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"room_code": map[string]any{"type": "string"},
					"name":      map[string]any{"type": "string"},
				},
				"required": []string{"room_code", "name"},
			},
		},
		{
			Name:        "fill_bots",
			Description: "用机器人补齐到至少3人，便于单人演示。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"room_code": map[string]any{"type": "string"},
				},
				"required": []string{"room_code"},
			},
		},
		{
			Name:        "start_game",
			Description: "开始对局：分配骗子身份，进入来电播报阶段。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"room_code": map[string]any{"type": "string"},
				},
				"required": []string{"room_code"},
			},
		},
		{
			Name:        "advance_phase",
			Description: "从播报阶段进入盘问阶段。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"room_code": map[string]any{"type": "string"},
				},
				"required": []string{"room_code"},
			},
		},
		{
			Name:        "submit_statement",
			Description: "当前发言玩家提交盘问陈述。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"room_code": map[string]any{"type": "string"},
					"player_id": map[string]any{"type": "string"},
					"text":      map[string]any{"type": "string"},
				},
				"required": []string{"room_code", "player_id", "text"},
			},
		},
		{
			Name:        "submit_cross_answer",
			Description: "被质询玩家回答交叉问题。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"room_code": map[string]any{"type": "string"},
					"player_id": map[string]any{"type": "string"},
					"text":      map[string]any{"type": "string"},
				},
				"required": []string{"room_code", "player_id", "text"},
			},
		},
		{
			Name:        "vote",
			Description: "投票指认来电骗子。可用 target_name 或 target_player_id。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"room_code":         map[string]any{"type": "string"},
					"player_id":         map[string]any{"type": "string"},
					"target_player_id":  map[string]any{"type": "string"},
					"target_name":       map[string]any{"type": "string"},
				},
				"required": []string{"room_code", "player_id"},
			},
		},
		{
			Name:        "get_state",
			Description: "获取房间公开状态与主持人台词，用于灵矽播报。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"room_code": map[string]any{"type": "string"},
					"player_id": map[string]any{"type": "string", "description": "若提供则附带私密身份卡"},
				},
				"required": []string{"room_code"},
			},
		},
	}
}

func (s *Server) HandleListTools(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"tools": s.Tools()})
}

func (s *Server) HandleCallTool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := s.Call(req.Name, req.Arguments)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "result": result})
}

// HandleSSE 提供极简 MCP SSE 兼容入口：GET 建立事件流，POST ?message= 也可走 /mcp/call。
func (s *Server) HandleSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "stream unsupported", http.StatusInternalServerError)
			return
		}
		endpoint := fmt.Sprintf(`{"endpoint":"/mcp/call"}`)
		fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpoint)
		flusher.Flush()
		// keep-alive briefly then end; 灵矽云端 MCP 通常会轮询 tools
		fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/tools/list_changed\"}\n\n")
		flusher.Flush()
		return
	}
	http.NotFound(w, r)
}

func (s *Server) Call(name string, args map[string]any) (any, error) {
	str := func(k string) string {
		v, _ := args[k].(string)
		return strings.TrimSpace(v)
	}
	switch name {
	case "create_room":
		scriptID := str("script_id")
		if scriptID != "" {
			if _, err := game.ScriptByID(scriptID); err != nil {
				return nil, err
			}
		}
		r := s.hub.Create(scriptID)
		host := str("host_name")
		if host == "" {
			host = "灵矽主持"
		}
		p, err := r.AddPlayer(host, false)
		if err != nil {
			return nil, err
		}
		return map[string]any{"room_code": r.Code, "player_id": p.ID, "state": r.Snapshot()}, nil
	case "join_room":
		r, err := s.hub.Get(str("room_code"))
		if err != nil {
			return nil, err
		}
		p, err := r.AddPlayer(str("name"), false)
		if err != nil {
			return nil, err
		}
		return map[string]any{"player_id": p.ID, "state": r.Snapshot()}, nil
	case "fill_bots":
		r, err := s.hub.Get(str("room_code"))
		if err != nil {
			return nil, err
		}
		if _, err := r.FillBots(game.MinPlayers); err != nil {
			return nil, err
		}
		return r.Snapshot(), nil
	case "start_game":
		r, err := s.hub.Get(str("room_code"))
		if err != nil {
			return nil, err
		}
		if err := r.Start(); err != nil {
			return nil, err
		}
		return map[string]any{"state": r.Snapshot(), "host_line": r.Snapshot().HostLine}, nil
	case "advance_phase":
		r, err := s.hub.Get(str("room_code"))
		if err != nil {
			return nil, err
		}
		if err := r.AdvanceFromBriefing(); err != nil {
			return nil, err
		}
		return r.Snapshot(), nil
	case "submit_statement":
		r, err := s.hub.Get(str("room_code"))
		if err != nil {
			return nil, err
		}
		if err := r.SubmitStatement(str("player_id"), str("text")); err != nil {
			return nil, err
		}
		return r.Snapshot(), nil
	case "submit_cross_answer":
		r, err := s.hub.Get(str("room_code"))
		if err != nil {
			return nil, err
		}
		if err := r.SubmitCrossAnswer(str("player_id"), str("text")); err != nil {
			return nil, err
		}
		return r.Snapshot(), nil
	case "vote":
		r, err := s.hub.Get(str("room_code"))
		if err != nil {
			return nil, err
		}
		target := str("target_player_id")
		if target == "" {
			if p := r.FindPlayerByName(str("target_name")); p != nil {
				target = p.ID
			}
		}
		if err := r.CastVote(str("player_id"), target); err != nil {
			return nil, err
		}
		return r.Snapshot(), nil
	case "get_state":
		r, err := s.hub.Get(str("room_code"))
		if err != nil {
			return nil, err
		}
		out := map[string]any{"state": r.Snapshot()}
		if pid := str("player_id"); pid != "" {
			if priv, err := r.PrivateView(pid); err == nil {
				out["private"] = priv
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
