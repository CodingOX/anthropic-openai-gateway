// Package main 网关入口。
// 启动 HTTP 服务，监听 Anthropic 格式请求，按需转换为 OpenAI 格式并调用后端。
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"anthropic-openai-gateway/internal/config"
	"anthropic-openai-gateway/internal/handler"
)

func main() {
	log.Println("")
	log.Println("╔════════════════════════════════════════╗")
	log.Println("║   OpenAI Gateway 启动                  ║")
	log.Println("╚════════════════════════════════════════╝")

	// 加载配置
	log.Println("[MAIN] 📋 正在加载配置...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[MAIN] ❌ 加载配置失败: %v", err)
	}

	// 创建消息处理器
	log.Println("[MAIN] 🚀 初始化HTTP处理器...")
	msgHandler := handler.NewMessagesHandler(cfg)

	// 注册路由
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", msgHandler.HandleMessages)
	mux.HandleFunc("/v1/messages/count_tokens", msgHandler.HandleCountTokens)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	log.Println("[MAIN] ✅ 路由已注册: /v1/messages, /v1/messages/count_tokens, /health")

	// 启动服务
	addr := fmt.Sprintf("0.0.0.0:%d", cfg.ListenPort)
	log.Println("")
	log.Printf("[MAIN] 🌐 网关启动成功，监听 %s", addr)
	log.Printf("[MAIN] 🔗 上游服务: %s", cfg.BaseURL)
	log.Printf("[MAIN] ⏱️  非流式请求超时: %dms", cfg.NonStreamingTimeoutMS)
	log.Println("[MAIN] 📡 等待请求...")
	log.Println("")

	if err := http.ListenAndServe(addr, handler.RecoverMiddleware(mux)); err != nil {
		log.Fatalf("[MAIN] ❌ 服务启动失败: %v", err)
	}
}
