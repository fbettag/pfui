package authflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fbettag/pfui/internal/authflow/successpage"
	"github.com/fbettag/pfui/internal/authstore"
)

const (
	openAIAuthURL    = "https://auth.openai.com/oauth/authorize"
	openAITokenURL   = "https://auth.openai.com/oauth/token"
	openAIClientID   = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAIScope      = "openid profile email offline_access"
	openAIOriginator = "codex_cli_rs"
	openAIUserAgent  = "pfui/0.1 (codex_cli_rs compatible)"
)

// StartOpenAICodexFlow launches a localhost callback server and builds the Codex-style OAuth URL.
func StartOpenAICodexFlow(ctx context.Context) (*BrowserSession[string], error) {
	clientID := os.Getenv("PFUI_OPENAI_CLIENT_ID")
	if strings.TrimSpace(clientID) == "" {
		clientID = openAIClientID
	}
	listener, err := net.Listen("tcp", "127.0.0.1:1455")
	if err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			return nil, fmt.Errorf("port 1455 is already in use. Close other pfui/codex sessions or forward ssh -L 1455:localhost:1455 when remote")
		}
		return nil, fmt.Errorf("creating callback listener: %w", err)
	}

	redirectURL := "http://localhost:1455/auth/callback"
	state := uuid.New().String()
	pkce, err := generatePKCE()
	if err != nil {
		listener.Close()
		return nil, fmt.Errorf("generating PKCE: %w", err)
	}

	authURL := buildOpenAIURL(clientID, redirectURL, state, pkce)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/auth/callback" && r.URL.Path != "/callback" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.URL.Query().Get("state") != state {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, "State mismatch")
				return
			}
			code := r.URL.Query().Get("code")
			if code == "" {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, "Missing code parameter")
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, successpage.HTML())
			select {
			case codeCh <- code:
			default:
			}
		}),
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	return &BrowserSession[string]{
		URL:         authURL,
		CallbackURL: redirectURL,
		wait: func() (string, error) {
			defer server.Shutdown(context.Background())
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case err := <-errCh:
				return "", err
			case code := <-codeCh:
				note, err := completeOpenAIAuthorization(clientID, redirectURL, code, pkce)
				return note, err
			}
		},
		submit: func(raw string) error {
			code, providedState, _, err := parseCallbackInput(raw)
			if err != nil {
				return err
			}
			if providedState != state {
				return fmt.Errorf("OpenAI state mismatch; restart the login flow")
			}
			select {
			case codeCh <- code:
				return nil
			default:
				return fmt.Errorf("OpenAI authorization already completed")
			}
		},
	}, nil
}

// AttemptBrowserOpen tries to open the user's default browser, falling back silently on failure.
func AttemptBrowserOpen(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}

func buildOpenAIURL(clientID, redirectURL, state string, pkce pkceCodes) string {
	values := url.Values{}
	values.Set("response_type", "code")
	values.Set("client_id", clientID)
	values.Set("redirect_uri", redirectURL)
	values.Set("scope", openAIScope)
	values.Set("code_challenge", pkce.Challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("id_token_add_organizations", "true")
	values.Set("codex_cli_simplified_flow", "true")
	values.Set("originator", openAIOriginator)
	values.Set("state", state)
	return fmt.Sprintf("%s?%s", openAIAuthURL, values.Encode())
}

type pkceCodes struct {
	Verifier  string
	Challenge string
}

func generatePKCE() (pkceCodes, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return pkceCodes{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	h := sha256Sum([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h)
	return pkceCodes{Verifier: verifier, Challenge: challenge}, nil
}

func sha256Sum(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

type openAITokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	IDToken      string `json:"id_token"`
}

func completeOpenAIAuthorization(clientID, redirectURL, code string, pkce pkceCodes) (string, error) {
	resp, err := exchangeOpenAITokens(clientID, redirectURL, code, pkce)
	if err != nil {
		return "", err
	}
	if resp.RefreshToken == "" || resp.AccessToken == "" {
		return "", fmt.Errorf("openai response missing refresh/access token")
	}
	tokens := authstore.OAuthTokens{
		RefreshToken: resp.RefreshToken,
		AccessToken:  resp.AccessToken,
		ExpiresAt:    time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second).Unix(),
	}
	if err := authstore.SaveOAuthTokens("openai", tokens); err != nil {
		return "", err
	}
	apiKey, err := createOpenAIAPIKey(clientID, resp.IDToken)
	if err != nil {
		note := "OpenAI linked successfully, but the platform couldn’t mint an API key automatically. If you recently created or changed your workspace, finish onboarding at https://platform.openai.com/org-setup and rerun `pfui auth refresh --provider openai` afterwards."
		return note, nil
	}
	if err := authstore.SaveAPIKey("openai", apiKey); err != nil {
		return "", err
	}
	return "", nil
}

func exchangeOpenAITokens(clientID, redirectURL, code string, pkce pkceCodes) (openAITokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURL)
	form.Set("client_id", clientID)
	form.Set("code_verifier", pkce.Verifier)
	body, err := doFormRequest(openAITokenURL, form)
	if err != nil {
		return openAITokenResponse{}, err
	}
	var resp openAITokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return openAITokenResponse{}, err
	}
	return resp, nil
}

func createOpenAIAPIKey(clientID, idToken string) (string, error) {
	if idToken == "" {
		return "", fmt.Errorf("missing id_token in OpenAI response")
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
	form.Set("client_id", clientID)
	form.Set("requested_token", "openai-api-key")
	form.Set("subject_token", idToken)
	form.Set("subject_token_type", "urn:ietf:params:oauth:token-type:id_token")
	form.Set("name", generateKeyName())
	body, err := doFormRequest(openAITokenURL, form)
	if err != nil {
		return "", err
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("OpenAI token exchange did not return an API key")
	}
	return resp.AccessToken, nil
}

func doFormRequest(endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Originator", openAIOriginator)
	req.Header.Set("User-Agent", openAIUserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openai oauth error: %s", strings.TrimSpace(string(body)))
	}
	return body, nil
}

func generateKeyName() string {
	date := time.Now().Format("2006-01-02")
	return fmt.Sprintf("pfui (%s) [%s]", date, randomHex(6))
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "pfui"
	}
	hex := make([]byte, n*2)
	encoding := "0123456789abcdef"
	for i, b := range buf {
		hex[i*2] = encoding[b>>4]
		hex[i*2+1] = encoding[b&0x0f]
	}
	return string(hex)
}

// RefreshOpenAITokens exchanges the stored refresh token for new tokens and a fresh API key.
func RefreshOpenAITokens(existing authstore.OAuthTokens) (authstore.OAuthTokens, string, error) {
	if existing.RefreshToken == "" {
		return authstore.OAuthTokens{}, "", fmt.Errorf("no OpenAI refresh token available")
	}
	clientID := os.Getenv("PFUI_OPENAI_CLIENT_ID")
	if strings.TrimSpace(clientID) == "" {
		clientID = openAIClientID
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", existing.RefreshToken)
	form.Set("client_id", clientID)
	body, err := doFormRequest(openAITokenURL, form)
	if err != nil {
		return authstore.OAuthTokens{}, "", err
	}
	var resp openAITokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return authstore.OAuthTokens{}, "", err
	}
	tokens := authstore.OAuthTokens{
		RefreshToken: resp.RefreshToken,
		AccessToken:  resp.AccessToken,
		ExpiresAt:    time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second).Unix(),
	}
	apiKey, err := createOpenAIAPIKey(clientID, resp.IDToken)
	if err != nil {
		return authstore.OAuthTokens{}, "", err
	}
	return tokens, apiKey, nil
}
