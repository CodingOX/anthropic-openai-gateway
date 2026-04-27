// Package config 处理网关配置的加载与合并。
// 支持 JSON 配置文件 + 环境变量覆盖，环境变量优先级更高。
package config

import (
	"encoding/json"
	"os"
	"strconv"
)

// Config 网关全部配置。
type Config struct {
	ListenHost               string       `json:"listen_host"`
	ListenPort               int          `json:"listen_port"`
	OpenAI                   OpenAIConfig `json:"openai"`
	ModelsNeedTransformation []string     `json:"models_need_transformation"`
	LogPromptPreviewOnError  bool         `json:"log_prompt_preview_on_error"`
	PromptPreviewMaxChars    int          `json:"prompt_preview_max_chars"`
}

// OpenAIConfig OpenAI 后端配置。
type OpenAIConfig struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	TimeoutMS int    `json:"timeout_ms"`
}

// Load 按优先级加载配置：默认值 → 配置文件 → 环境变量。
func Load() (*Config, error) {
	cfg := &Config{
		ListenHost:            "127.0.0.1",
		ListenPort:            3456,
		PromptPreviewMaxChars: 240,
		OpenAI: OpenAIConfig{
			BaseURL:   "https://api.openai.com/v1",
			TimeoutMS: 120000,
		},
	}

	// 1. 加载配置文件
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "./configs/config.json"
	}
	if data, err := os.ReadFile(configFile); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// 2. 环境变量覆盖（优先级最高）
	overrideFromEnv(cfg)

	return cfg, nil
}

// overrideFromEnv 用环境变量覆盖配置。
func overrideFromEnv(cfg *Config) {
	if v := os.Getenv("LISTEN_HOST"); v != "" {
		cfg.ListenHost = v
	}
	if v := os.Getenv("LISTEN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.ListenPort = p
		}
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		cfg.OpenAI.BaseURL = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.OpenAI.APIKey = v
	}
	if v := os.Getenv("OPENAI_TIMEOUT_MS"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.OpenAI.TimeoutMS = t
		}
	}
	if v := os.Getenv("LOG_PROMPT_PREVIEW_ON_ERROR"); v != "" {
		if enabled, err := strconv.ParseBool(v); err == nil {
			cfg.LogPromptPreviewOnError = enabled
		}
	}
	if v := os.Getenv("PROMPT_PREVIEW_MAX_CHARS"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil {
			cfg.PromptPreviewMaxChars = limit
		}
	}
}
