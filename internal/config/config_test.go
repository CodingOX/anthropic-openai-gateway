package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadUsesEnvOnlyAndIgnoresConfigFile(t *testing.T) {
	badConfigPath := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(badConfigPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("CONFIG_FILE", badConfigPath)
	t.Setenv("LISTEN_HOST", "0.0.0.0")
	t.Setenv("LISTEN_PORT", "8080")
	t.Setenv("OPENAI_BASE_URL", "https://example.com/v1")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_TIMEOUT_MS", "180000")
	t.Setenv("MODELS_NEED_TRANSFORMATION", "gpt-5,o3-mini")
	t.Setenv("LOG_PROMPT_PREVIEW_ON_ERROR", "true")
	t.Setenv("PROMPT_PREVIEW_MAX_CHARS", "96")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.ListenHost != "0.0.0.0" {
		t.Fatalf("ListenHost = %q, want %q", cfg.ListenHost, "0.0.0.0")
	}
	if cfg.ListenPort != 8080 {
		t.Fatalf("ListenPort = %d, want %d", cfg.ListenPort, 8080)
	}
	if cfg.OpenAI.BaseURL != "https://example.com/v1" {
		t.Fatalf("OpenAI.BaseURL = %q, want %q", cfg.OpenAI.BaseURL, "https://example.com/v1")
	}
	if cfg.OpenAI.APIKey != "sk-test" {
		t.Fatalf("OpenAI.APIKey = %q, want %q", cfg.OpenAI.APIKey, "sk-test")
	}
	if cfg.OpenAI.TimeoutMS != 180000 {
		t.Fatalf("OpenAI.TimeoutMS = %d, want %d", cfg.OpenAI.TimeoutMS, 180000)
	}
	if !reflect.DeepEqual(cfg.ModelsNeedTransformation, []string{"gpt-5", "o3-mini"}) {
		t.Fatalf("ModelsNeedTransformation = %v, want %v", cfg.ModelsNeedTransformation, []string{"gpt-5", "o3-mini"})
	}
	if !cfg.LogPromptPreviewOnError {
		t.Fatalf("LogPromptPreviewOnError = false, want true")
	}
	if cfg.PromptPreviewMaxChars != 96 {
		t.Fatalf("PromptPreviewMaxChars = %d, want %d", cfg.PromptPreviewMaxChars, 96)
	}
}

func TestLoadUsesDefaultModelsWhenEnvUnset(t *testing.T) {
	t.Setenv("CONFIG_FILE", filepath.Join(t.TempDir(), "missing.json"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	want := []string{"gpt-4.1", "gpt-4o", "gpt-4o-mini", "gpt-4.1-mini", "gpt-4.1-nano", "gpt-5", "o3", "o3-mini", "o4-mini"}
	if !reflect.DeepEqual(cfg.ModelsNeedTransformation, want) {
		t.Fatalf("ModelsNeedTransformation = %v, want %v", cfg.ModelsNeedTransformation, want)
	}
}

func TestLoadKeepsDefaultModelsWhenEnvIsBlankList(t *testing.T) {
	t.Setenv("MODELS_NEED_TRANSFORMATION", " , , ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if !reflect.DeepEqual(cfg.ModelsNeedTransformation, defaultModelsNeedTransformation) {
		t.Fatalf("ModelsNeedTransformation = %v, want %v", cfg.ModelsNeedTransformation, defaultModelsNeedTransformation)
	}
}

func TestOverrideFromEnvSetsPromptPreviewFlags(t *testing.T) {
	t.Setenv("LOG_PROMPT_PREVIEW_ON_ERROR", "true")
	t.Setenv("PROMPT_PREVIEW_MAX_CHARS", "96")

	cfg := &Config{}
	overrideFromEnv(cfg)

	if !cfg.LogPromptPreviewOnError {
		t.Fatalf("LogPromptPreviewOnError = false, want true")
	}
	if cfg.PromptPreviewMaxChars != 96 {
		t.Fatalf("PromptPreviewMaxChars = %d, want 96", cfg.PromptPreviewMaxChars)
	}
}
