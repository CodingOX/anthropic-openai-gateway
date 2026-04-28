// Package config 处理网关配置的加载与合并。
package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

var defaultModelsNeedTransformation = []string{
	"gpt-4.1",
	"gpt-4o",
	"gpt-4o-mini",
	"gpt-4.1-mini",
	"gpt-4.1-nano",
	"gpt-5",
	"o3",
	"o3-mini",
	"o4-mini",
}

// Config 网关配置。
// 两个后端（OpenAI 格式和 Anthropic 格式）均来自同一 opencode go 套餐，共享一个 APIKey。
type Config struct {
	ListenPort               int
	APIKey                   string   // opencode go API 密钥，同时用于两个后端
	BaseURL                  string   // opencode go 的基础端点（两个格式共用）
	NonStreamingTimeoutMS    int      // 非流式 HTTP 请求超时（毫秒）
	ModelsNeedTransformation []string // 需要从 OpenAI 格式转换的模型列表
}

// Load 加载配置：默认值 → .env 文件 → 环境变量。
func Load() (*Config, error) {
	// 尝试加载 .env 文件（非强制，本地开发用）
	if err := godotenv.Load(); err == nil {
		log.Println("[CONFIG] ✅ .env 文件已加载")
	} else {
		log.Println("[CONFIG] ⚠️  .env 未找到，将使用环境变量或默认值")
	}

	cfg := &Config{
		ListenPort:               3456,
		BaseURL:                  "https://api.openai.com/v1",
		NonStreamingTimeoutMS:    120000,
		ModelsNeedTransformation: append([]string(nil), defaultModelsNeedTransformation...),
	}

	overrideFromEnv(cfg)
	logLoadedConfig(cfg)

	return cfg, nil
}

// overrideFromEnv 用环境变量覆盖配置。
func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("LISTEN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.ListenPort = p
		}
	}
	if v := os.Getenv("OPENCODE_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("NON_STREAM_TIMEOUT_MS"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.NonStreamingTimeoutMS = t
		}
	} else if v := os.Getenv("TIMEOUT_MS"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.NonStreamingTimeoutMS = t
		}
	}
	if v := os.Getenv("MODELS_NEED_TRANSFORMATION"); v != "" {
		if models := splitCommaSeparatedValues(v); len(models) > 0 {
			cfg.ModelsNeedTransformation = models
		}
	}
}

// logLoadedConfig 打印已加载的配置。
func logLoadedConfig(cfg *Config) {
	apiKeySuffix := "***"
	if len(cfg.APIKey) > 10 {
		apiKeySuffix = cfg.APIKey[len(cfg.APIKey)-10:]
	}

	log.Println("========================================")
	log.Println("[CONFIG] 网关配置已加载")
	log.Println("========================================")
	log.Printf("[CONFIG] 📌 监听端口: 127.0.0.1:%d", cfg.ListenPort)
	log.Printf("[CONFIG] 🔗 上游端点: %s", cfg.BaseURL)
	log.Printf("[CONFIG] 🔐 API密钥: sk-...%s (最后10位)", apiKeySuffix)
	log.Printf("[CONFIG] ⏱️  非流式请求超时: %dms", cfg.NonStreamingTimeoutMS)
	log.Printf("[CONFIG] 📦 转换模型数: %d", len(cfg.ModelsNeedTransformation))
	if len(cfg.ModelsNeedTransformation) > 0 && len(cfg.ModelsNeedTransformation) <= 10 {
		log.Printf("[CONFIG] 📋 模型列表: %v", cfg.ModelsNeedTransformation)
	}
	log.Println("========================================")
}

func splitCommaSeparatedValues(value string) []string {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
