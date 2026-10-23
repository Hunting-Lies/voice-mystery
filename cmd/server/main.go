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
	fmt.Printf("  Health: http://127.0.0.1%s/api/health\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
