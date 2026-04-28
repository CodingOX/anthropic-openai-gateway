package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func runWithoutDotEnv(t *testing.T) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("os.Chdir(%q) error = %v", tmpDir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("os.Chdir(%q) cleanup error = %v", wd, err)
		}
	})
}

func TestLoadUsesEnvAndDefaults(t *testing.T) {
	runWithoutDotEnv(t)

	badConfigPath := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(badConfigPath, []byte("{"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("LISTEN_PORT", "8080")
	t.Setenv("OPENCODE_API_KEY", "sk-test")
	t.Setenv("BASE_URL", "https://example.com/v1")
	t.Setenv("NON_STREAM_TIMEOUT_MS", "180000")
	t.Setenv("MODELS_NEED_TRANSFORMATION", "gpt-5,o3-mini")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.ListenPort != 8080 {
		t.Fatalf("ListenPort = %d, want %d", cfg.ListenPort, 8080)
	}
	if cfg.APIKey != "sk-test" {
		t.Fatalf("APIKey = %q, want sk-test", cfg.APIKey)
	}
	if cfg.BaseURL != "https://example.com/v1" {
		t.Fatalf("BaseURL = %q, want https://example.com/v1", cfg.BaseURL)
	}
	if cfg.NonStreamingTimeoutMS != 180000 {
		t.Fatalf("NonStreamingTimeoutMS = %d, want 180000", cfg.NonStreamingTimeoutMS)
	}
	if !reflect.DeepEqual(cfg.ModelsNeedTransformation, []string{"gpt-5", "o3-mini"}) {
		t.Fatalf("ModelsNeedTransformation = %v, want %v", cfg.ModelsNeedTransformation, []string{"gpt-5", "o3-mini"})
	}
}

func TestLoadUsesDefaultModelsWhenEnvUnset(t *testing.T) {
	runWithoutDotEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.NonStreamingTimeoutMS != 120000 {
		t.Fatalf("NonStreamingTimeoutMS = %d, want 120000", cfg.NonStreamingTimeoutMS)
	}

	want := []string{"gpt-4.1", "gpt-4o", "gpt-4o-mini", "gpt-4.1-mini", "gpt-4.1-nano", "gpt-5", "o3", "o3-mini", "o4-mini"}
	if !reflect.DeepEqual(cfg.ModelsNeedTransformation, want) {
		t.Fatalf("ModelsNeedTransformation = %v, want %v", cfg.ModelsNeedTransformation, want)
	}
}

func TestLoadKeepsDefaultModelsWhenEnvIsBlankList(t *testing.T) {
	runWithoutDotEnv(t)

	t.Setenv("MODELS_NEED_TRANSFORMATION", " , , ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if !reflect.DeepEqual(cfg.ModelsNeedTransformation, defaultModelsNeedTransformation) {
		t.Fatalf("ModelsNeedTransformation = %v, want %v", cfg.ModelsNeedTransformation, defaultModelsNeedTransformation)
	}
}

func TestLoadUsesLegacyTimeoutEnvWhenNewNameUnset(t *testing.T) {
	runWithoutDotEnv(t)

	t.Setenv("TIMEOUT_MS", "9000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.NonStreamingTimeoutMS != 9000 {
		t.Fatalf("NonStreamingTimeoutMS = %d, want 9000", cfg.NonStreamingTimeoutMS)
	}
}

func TestLoadPrefersNewTimeoutEnvOverLegacyAlias(t *testing.T) {
	runWithoutDotEnv(t)

	t.Setenv("TIMEOUT_MS", "9000")
	t.Setenv("NON_STREAM_TIMEOUT_MS", "12000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if cfg.NonStreamingTimeoutMS != 12000 {
		t.Fatalf("NonStreamingTimeoutMS = %d, want 12000", cfg.NonStreamingTimeoutMS)
	}
}
