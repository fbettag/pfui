package authflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/fbettag/pfui/internal/authstore"
)

const (
	anthropicClientID  = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	anthropicRedirect  = "https://console.anthropic.com/oauth/code/callback"
	anthropicTokenURL  = "https://console.anthropic.com/v1/oauth/token"
	anthropicScope     = "org:create_api_key user:profile user:inference"
	anthropicAPIKeyURL = "https://api.anthropic.com/api/oauth/claude_cli/create_api_key"
)

// AnthropicMode describes which entry point is being used.
type AnthropicMode int

const (
	AnthropicModeMax AnthropicMode = iota
	AnthropicModeConsole
)

// AnthropicAuthorize describes the prepared OAuth request.
type AnthropicAuthorize struct {
	URL      string
	Verifier string
	Mode     AnthropicMode
}

// PrepareAnthropicFlow builds the authorization URL for the selected mode.
func PrepareAnthropicFlow(mode AnthropicMode) (*AnthropicAuthorize, error) {
	clientID := os.Getenv("PFUI_ANTHROPIC_CLIENT_ID")
	if strings.TrimSpace(clientID) == "" {
		clientID = anthropicClientID
	}
	pkce, err := generatePKCE()
	if err != nil {
		return nil, err
	}
	host := "claude.ai"
	if mode == AnthropicModeConsole {
		host = "console.anthropic.com"
	}
	authURL := url.URL{}
	authURL.Scheme = "https"
	authURL.Host = host
	authURL.Path = "/oauth/authorize"
	q := authURL.Query()
	q.Set("code", "true")
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", anthropicRedirect)
	q.Set("scope", anthropicScope)
	q.Set("code_challenge", pkce.Challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", pkce.Verifier)
	authURL.RawQuery = q.Encode()

	return &AnthropicAuthorize{URL: authURL.String(), Verifier: pkce.Verifier, Mode: mode}, nil
}

// AnthropicResult captures the output of exchanging a code.
type AnthropicResult struct {
	Type          string
	APIKey        string
	Tokens        authstore.OAuthTokens
	HasMillionCtx bool
}

// CompleteAnthropicFlow exchanges the provided code for tokens or an API key.
func CompleteAnthropicFlow(auth *AnthropicAuthorize, codeInput string) (AnthropicResult, error) {
	if auth == nil {
		return AnthropicResult{}, fmt.Errorf("missing anthropic session")
	}
	clientID := os.Getenv("PFUI_ANTHROPIC_CLIENT_ID")
	if strings.TrimSpace(clientID) == "" {
		clientID = anthropicClientID
	}
	parts := strings.SplitN(strings.TrimSpace(codeInput), "#", 2)
	if len(parts) != 2 {
		return AnthropicResult{}, fmt.Errorf("expected code of the form code#state")
	}
	if parts[1] != auth.Verifier {
		return AnthropicResult{}, fmt.Errorf("anthropic state mismatch; restart the login flow")
	}
	payload := map[string]string{
		"code":          parts[0],
		"state":         parts[1],
		"grant_type":    "authorization_code",
		"client_id":     clientID,
		"redirect_uri":  anthropicRedirect,
		"code_verifier": auth.Verifier,
	}
	body, _ := json.Marshal(payload)
	tokenRes, err := fetchJSON(anthropicTokenURL, body)
	if err != nil {
		return AnthropicResult{}, err
	}
	refresh, ok := tokenRes["refresh_token"].(string)
	if !ok || refresh == "" {
		return AnthropicResult{}, fmt.Errorf("anthropic response missing refresh token")
	}
	access, ok := tokenRes["access_token"].(string)
	if !ok || access == "" {
		return AnthropicResult{}, fmt.Errorf("anthropic response missing access token")
	}
	expires := time.Now().Add(60 * time.Minute).Unix()
	if exp, ok := tokenRes["expires_in"].(float64); ok {
		expires = time.Now().Add(time.Duration(exp) * time.Second).Unix()
	}

	tokens := authstore.OAuthTokens{RefreshToken: refresh, AccessToken: access, ExpiresAt: expires}

	switch auth.Mode {
	case AnthropicModeMax:
		tokens.Extra = map[string]string{"has_1m_context": "true"}
		if err := authstore.SaveOAuthTokens("anthropic", tokens); err != nil {
			return AnthropicResult{}, err
		}
		return AnthropicResult{Type: "oauth", Tokens: tokens, HasMillionCtx: true}, nil
	case AnthropicModeConsole:
		apiKey, err := createAnthropicAPIKey(access)
		if err != nil {
			return AnthropicResult{}, err
		}
		if err := authstore.SaveAPIKey("anthropic", apiKey); err != nil {
			return AnthropicResult{}, err
		}
		return AnthropicResult{Type: "api", APIKey: apiKey}, nil
	default:
		return AnthropicResult{}, fmt.Errorf("unsupported anthropic mode")
	}
}

func createAnthropicAPIKey(access string) (string, error) {
	req := map[string]any{}
	body, _ := json.Marshal(req)
	resp, err := fetchJSONWithAuth(anthropicAPIKeyURL, body, access)
	if err != nil {
		return "", err
	}
	key, _ := resp["raw_key"].(string)
	if key == "" {
		return "", fmt.Errorf("anthropic did not return an API key")
	}
	return key, nil
}

func fetchJSON(endpoint string, body []byte) (map[string]any, error) {
	req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic token exchange failed: %s", data)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func fetchJSONWithAuth(endpoint string, body []byte, token string) (map[string]any, error) {
	req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic API key creation failed: %s", data)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// RefreshAnthropicTokens exchanges the refresh token for a new access token.
func RefreshAnthropicTokens(existing authstore.OAuthTokens) (authstore.OAuthTokens, error) {
	if existing.RefreshToken == "" {
		return authstore.OAuthTokens{}, fmt.Errorf("no Anthropic refresh token available")
	}
	clientID := os.Getenv("PFUI_ANTHROPIC_CLIENT_ID")
	if strings.TrimSpace(clientID) == "" {
		clientID = anthropicClientID
	}
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": existing.RefreshToken,
		"client_id":     clientID,
	})
	resp, err := fetchJSON(anthropicTokenURL, body)
	if err != nil {
		return authstore.OAuthTokens{}, err
	}
	refresh, _ := resp["refresh_token"].(string)
	access, _ := resp["access_token"].(string)
	expires := time.Now().Add(60 * time.Minute).Unix()
	if exp, ok := resp["expires_in"].(float64); ok {
		expires = time.Now().Add(time.Duration(exp) * time.Second).Unix()
	}
	if refresh == "" || access == "" {
		return authstore.OAuthTokens{}, fmt.Errorf("anthropic refresh response missing tokens")
	}
	return authstore.OAuthTokens{
		RefreshToken: refresh,
		AccessToken:  access,
		ExpiresAt:    expires,
		Extra:        existing.Extra,
	}, nil
}
