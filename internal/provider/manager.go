package provider

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// AdapterKind enumerates supported provider adapter protocols.
type AdapterKind string

const (
	AdapterOpenAIChat       AdapterKind = "openai-chat"
	AdapterOpenAIResponses  AdapterKind = "openai-responses"
	AdapterAnthropicMessage AdapterKind = "anthropic-messages"
)

// Manifest describes a custom provider connector.
type Manifest struct {
	Name    string      `toml:"name"`
	Adapter AdapterKind `toml:"adapter"`
	Host    string      `toml:"host"`
	Token   string      `toml:"token"`
}

// InitProvider writes a manifest to ~/.pfui/providers/<name>.toml.
func InitProvider(m Manifest) (string, error) {
	if m.Name == "" {
		return "", fmt.Errorf("provider name is required")
	}
	if m.Adapter == "" {
		return "", fmt.Errorf("adapter kind is required")
	}
	dir, err := providerDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ensuring provider dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s.toml", m.Name))
	data, err := toml.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("encoding manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("writing manifest: %w", err)
	}
	return path, nil
}

func providerDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".pfui", "providers"), nil
}

// LoadManifests reads all manifests under ~/.pfui/providers.
func LoadManifests() ([]Manifest, error) {
	dir, err := providerDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading providers dir: %w", err)
	}
	var manifests []Manifest
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading provider manifest %s: %w", path, err)
		}
		var m Manifest
		if err := toml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parsing provider manifest %s: %w", path, err)
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}
