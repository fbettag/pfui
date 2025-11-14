package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	// DefaultFileName is the expected file name under the config directory.
	DefaultFileName = "config.toml"
)

// Config captures persisted user preferences.
type Config struct {
	Models    ModelConfig     `toml:"models"`
	Providers ProvidersConfig `toml:"providers"`
	Plan      PlanConfig      `toml:"plan"`
}

// ModelConfig governs model discovery/rendering.
type ModelConfig struct {
	// Whitelist applies globally when no provider-specific list is supplied.
	Whitelist []string `toml:"whitelist"`
	// ProviderWhitelist lets administrators scope lists per provider name or kind.
	ProviderWhitelist map[string][]string `toml:"provider_whitelist"`
}

// Default returns config populated with safe defaults.
func Default() Config {
	return Config{
		Models: ModelConfig{
			Whitelist:         nil,
			ProviderWhitelist: map[string][]string{},
		},
		Providers: ProvidersConfig{
			OpenAI:    ProviderToggle{Enabled: true},
			Anthropic: ProviderToggle{Enabled: true},
		},
		Plan: PlanConfig{
			Storage:   "memory",
			FilePath:  "PLAN.md",
			AutoWrite: false,
		},
	}
}

// ProvidersConfig describes provider enablement.
type ProvidersConfig struct {
	OpenAI    ProviderToggle `toml:"openai"`
	Anthropic ProviderToggle `toml:"anthropic"`
}

// ProviderToggle wraps a boolean flag.
type ProviderToggle struct {
	Enabled bool `toml:"enabled"`
}

// PlanConfig controls how plan steps are persisted.
type PlanConfig struct {
	// Storage determines whether plans live only in memory or also sync to disk ("memory" or "file").
	Storage string `toml:"storage"`
	// FilePath controls where plan markdown is written when storage = file.
	FilePath string `toml:"file_path"`
	// AutoWrite toggles automatic PLAN.md updates after every plan mutation.
	AutoWrite bool `toml:"auto_write"`
}

// DefaultPath resolves ~/.pfui/config.toml (creating the directory if necessary).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	dir := filepath.Join(home, ".pfui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ensuring config dir: %w", err)
	}
	return filepath.Join(dir, DefaultFileName), nil
}

// Load reads config from path; when missing, returns defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return cfg, err
		}
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("reading config %s: %w", path, err)
	}
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := toml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.Models.ProviderWhitelist == nil {
		cfg.Models.ProviderWhitelist = map[string][]string{}
	}
	cfg.Plan = normalizePlanConfig(cfg.Plan)
	return cfg, nil
}

// Save writes the provided config to path (defaulting to ~/.pfui/config.toml when empty).
func Save(path string, cfg Config) error {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return err
		}
	}
	cfg.Plan = normalizePlanConfig(cfg.Plan)
	data, err := toml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

func normalizePlanConfig(plan PlanConfig) PlanConfig {
	plan.Storage = strings.ToLower(strings.TrimSpace(plan.Storage))
	switch plan.Storage {
	case "", "memory":
		plan.Storage = "memory"
	default:
		plan.Storage = "file"
	}
	plan.FilePath = strings.TrimSpace(plan.FilePath)
	if plan.FilePath == "" {
		plan.FilePath = "PLAN.md"
	}
	return plan
}

// SaveExample writes a commented example config.
func SaveExample(path string) error {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return err
		}
	}
	content := []byte(exampleConfig)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing example config: %w", err)
	}
	return nil
}

const exampleConfig = `# pfui configuration

# [models.whitelist] lets administrators optionally limit model choices shown in
# the /model picker. Leave it commented to show everything providers report.
# Uncomment and list model identifiers to restrict the popup:
#
# [models]
# whitelist = [
#   "gpt-4.1-mini",
#   "claude-3-5-sonnet",
#   "opus-plan"
# ]
#
# Provider-specific overrides take precedence. Keys can match provider names
# (e.g., "openai", "my-zai-proxy") or kinds ("openai", "anthropic"). Example:
#
# [models.provider_whitelist]
# openai = ["gpt-5.1-codex"]
# "claude" = ["claude-4.5-sonnet"]
# my-custom = ["zai-ultra"]

# Configure how pfui persists plan steps from /plan.
# storage = "memory"  # keep plans in pfui only
# storage = "file"    # also mirror to PLAN.md (defaults to project root)
# file_path = "PLAN.md"
# auto_write = true    # immediately rewrite the markdown file after each edit
#
[plan]
# storage = "memory"
# file_path = "PLAN.md"
# auto_write = false
`
