package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(cfg.Models.Whitelist) != 0 {
		t.Fatalf("expected empty whitelist, got %#v", cfg.Models.Whitelist)
	}
	if !cfg.Providers.OpenAI.Enabled || !cfg.Providers.Anthropic.Enabled {
		t.Fatalf("expected both providers enabled by default: %#v", cfg.Providers)
	}
}

func TestLoadParsesWhitelist(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	err := os.WriteFile(path, []byte(`[models]
whitelist = ["a","b"]
[providers.openai]
enabled = false
`), 0o644)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := len(cfg.Models.Whitelist); got != 2 {
		t.Fatalf("expected 2 models in whitelist, got %d", got)
	}
	if cfg.Providers.OpenAI.Enabled {
		t.Fatalf("expected OpenAI disabled via config")
	}
}

func TestSaveExampleWritesTemplate(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	if err := SaveExample(path); err != nil {
		t.Fatalf("SaveExample: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved example: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("example config is empty")
	}
}
