// Package config 处理网关配置的加载与合并。
// 纯 ENV 模式下，仅使用默认值和环境变量。
package config

import (
	"os"
	"strconv"
	"strings"
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

// Config 网关全部配置。
type Config struct {
	ListenHost               string          `json:"listen_host"`
	ListenPort               int             `json:"listen_port"`
	OpenAI                   OpenAIConfig    `json:"openai"`
	Anthropic                AnthropicConfig `json:"anthropic"`
	ModelsNeedTransformation []string        `json:"models_need_transformation"`
	LogPromptPreviewOnError  bool            `json:"log_prompt_preview_on_error"`
	PromptPreviewMaxChars    int             `json:"prompt_preview_max_chars"`
}

// OpenAIConfig OpenAI 后端配置。
type OpenAIConfig struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	TimeoutMS int    `json:"timeout_ms"`
}

// AnthropicConfig Anthropic 后端配置（用于透传模式）。
type AnthropicConfig struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	TimeoutMS int    `json:"timeout_ms"`
}

// Load 加载配置：默认值 → 环境变量。
func Load() (*Config, error) {
	cfg := &Config{
		ListenHost:               "127.0.0.1",
		ListenPort:               3456,
		ModelsNeedTransformation: append([]string(nil), defaultModelsNeedTransformation...),
		PromptPreviewMaxChars:    240,
		OpenAI: OpenAIConfig{
			BaseURL:   "https://api.openai.com/v1",
			TimeoutMS: 120000,
		},
		Anthropic: AnthropicConfig{
			BaseURL:   "https://api.anthropic.com/v1",
			TimeoutMS: 120000,
		},
	}

	// 纯 ENV 模式下，只接受环境变量覆盖，避免部署时维护两套配置来源。
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
	if v := os.Getenv("ANTHROPIC_BASE_URL"); v != "" {
		cfg.Anthropic.BaseURL = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.Anthropic.APIKey = v
	}
	if v := os.Getenv("ANTHROPIC_TIMEOUT_MS"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.Anthropic.TimeoutMS = t
		}
	}
	if v := os.Getenv("MODELS_NEED_TRANSFORMATION"); v != "" {
		if models := splitCommaSeparatedValues(v); len(models) > 0 {
			cfg.ModelsNeedTransformation = models
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
