package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jenny-zeng/lai-dian-yi-yun/internal/api"
	"github.com/jenny-zeng/lai-dian-yi-yun/internal/room"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	web := flag.String("web", "", "web static root (default: ./web)")
	flag.Parse()

	webRoot := *web
	if webRoot == "" {
		webRoot = os.Getenv("WEB_ROOT")
	}
	if webRoot == "" {
		exe, _ := os.Executable()
		webRoot = filepath.Join(filepath.Dir(exe), "web")
		if _, err := os.Stat(webRoot); err != nil {
			webRoot = "web"
		}
	}

	hub := room.NewHub()
	srv := api.NewServer(hub, webRoot)
	handler := srv.Handler(webRoot)

	fmt.Printf("《来电疑云》已启动\n")
	fmt.Printf("  Web  : http://127.0.0.1%s/\n", *addr)
	fmt.Printf("  MCP  : http://127.0.0.1%s/mcp/tools\n", *addr)
	fmt.Printf("  ASR  : http://127.0.0.1%s/api/asr  (本地 Whisper)\n", *addr)
	fmt.Printf("  TTS  : http://127.0.0.1%s/api/tts/status\n", *addr)
	if srv.TTSEnabled() {
		fmt.Printf("  音色 : 灵矽 CosyVoice 已启用（内勤温暖播报 + 四款角色音色）\n")
	} else {
		fmt.Printf("  音色 : 未设置 DASHSCOPE_API_KEY，Web Demo 回退浏览器 TTS\n")
	}
	fmt.Printf("  Health: http://127.0.0.1%s/api/health\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
