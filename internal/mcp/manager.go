package mcp

import (
	"fmt"
	"os"
	"path/filepath"
)

// Scope defines where an MCP server is registered.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// Server describes an MCP endpoint.
type Server struct {
	Name string `toml:"name"`
	URL  string `toml:"url"`
}

// AddServer stores metadata under the scope directory.
func AddServer(scope Scope, server Server) (string, error) {
	if server.Name == "" {
		return "", fmt.Errorf("server name is required")
	}
	if server.URL == "" {
		return "", fmt.Errorf("server url is required")
	}
	dir, err := scopeDir(scope)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ensuring scope dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s.toml", server.Name))
	content := fmt.Sprintf("name = %q\nurl = %q\n", server.Name, server.URL)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing mcp server: %w", err)
	}
	return path, nil
}

func scopeDir(scope Scope) (string, error) {
	switch scope {
	case ScopeUser, "":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolving home dir: %w", err)
		}
		return filepath.Join(home, ".pfui", "mcp.d"), nil
	case ScopeProject:
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolving cwd: %w", err)
		}
		return filepath.Join(cwd, ".pfui", "mcp.d"), nil
	default:
		return "", fmt.Errorf("unknown scope %q", scope)
	}
}
