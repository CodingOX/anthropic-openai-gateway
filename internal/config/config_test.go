package config

import (
	"testing"
)

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
