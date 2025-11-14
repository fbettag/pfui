package authstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var mu sync.Mutex

// Credentials captures persisted secrets.
type Credentials struct {
	APIKeys   map[string]string         `json:"api_keys"`
	AuthCodes map[string]string         `json:"auth_codes"`
	OAuth     map[string]OAuthTokens    `json:"oauth"`
	Metadata  map[string]map[string]any `json:"metadata,omitempty"`
}

// OAuthTokens captures refresh/access tokens for OAuth flows.
type OAuthTokens struct {
	RefreshToken string            `json:"refresh_token"`
	AccessToken  string            `json:"access_token"`
	ExpiresAt    int64             `json:"expires_at"`
	Extra        map[string]string `json:"extra,omitempty"`
}

func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	dir := filepath.Join(home, ".pfui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("ensuring dir: %w", err)
	}
	return filepath.Join(dir, "credentials.json"), nil
}

func load() (Credentials, string, error) {
	path, err := defaultPath()
	if err != nil {
		return Credentials{}, "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Credentials{
			APIKeys:   map[string]string{},
			AuthCodes: map[string]string{},
			OAuth:     map[string]OAuthTokens{},
			Metadata:  map[string]map[string]any{},
		}, path, nil
	}
	if err != nil {
		return Credentials{}, "", fmt.Errorf("reading credentials: %w", err)
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{}, "", fmt.Errorf("parsing credentials: %w", err)
	}
	if creds.APIKeys == nil {
		creds.APIKeys = map[string]string{}
	}
	if creds.AuthCodes == nil {
		creds.AuthCodes = map[string]string{}
	}
	if creds.OAuth == nil {
		creds.OAuth = map[string]OAuthTokens{}
	}
	if creds.Metadata == nil {
		creds.Metadata = map[string]map[string]any{}
	}
	return creds, path, nil
}

func save(creds Credentials, path string) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}
	return nil
}

// SaveAPIKey stores an API key for provider (e.g., "openai" or "anthropic").
func SaveAPIKey(provider string, key string) error {
	mu.Lock()
	defer mu.Unlock()
	creds, path, err := load()
	if err != nil {
		return err
	}
	creds.APIKeys[provider] = key
	return save(creds, path)
}

// SaveAuthCode stores the last OAuth authorization code for a provider.
func SaveAuthCode(provider, code string) error {
	mu.Lock()
	defer mu.Unlock()
	creds, path, err := load()
	if err != nil {
		return err
	}
	creds.AuthCodes[provider] = code
	return save(creds, path)
}

// SaveOAuthTokens persists refresh/access tokens for a provider.
func SaveOAuthTokens(provider string, tokens OAuthTokens) error {
	mu.Lock()
	defer mu.Unlock()
	creds, path, err := load()
	if err != nil {
		return err
	}
	creds.OAuth[provider] = tokens
	return save(creds, path)
}

// Snapshot returns all stored credentials.
func Snapshot() (Credentials, error) {
	mu.Lock()
	defer mu.Unlock()
	creds, _, err := load()
	return creds, err
}

// GetAPIKey fetches the stored API key for a provider.
func GetAPIKey(provider string) (string, bool, error) {
	creds, err := Snapshot()
	if err != nil {
		return "", false, err
	}
	key, ok := creds.APIKeys[provider]
	return key, ok, nil
}

// GetOAuthTokens fetches OAuth tokens for a provider.
func GetOAuthTokens(provider string) (OAuthTokens, bool, error) {
	creds, err := Snapshot()
	if err != nil {
		return OAuthTokens{}, false, err
	}
	tokens, ok := creds.OAuth[provider]
	return tokens, ok, nil
}
