package authflow

import (
	"fmt"
	"net/url"
	"strings"
)

// BrowserSession represents a pending local callback flow.
type BrowserSession[T any] struct {
	URL         string
	ManualURL   string
	CallbackURL string
	wait        func() (T, error)
	submit      func(string) error
}

// Wait blocks until the browser flow completes.
func (s *BrowserSession[T]) Wait() (T, error) {
	var zero T
	if s == nil || s.wait == nil {
		return zero, fmt.Errorf("invalid browser session")
	}
	return s.wait()
}

// SubmitCallback manually fulfills the OAuth callback using a pasted URL or code#state pair.
func (s *BrowserSession[T]) SubmitCallback(raw string) error {
	if s == nil || s.submit == nil {
		return fmt.Errorf("this flow does not support manual callbacks")
	}
	return s.submit(raw)
}

func parseCallbackInput(raw string) (code string, state string, host string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", "", fmt.Errorf("paste the callback URL that includes code and state parameters")
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", "", "", fmt.Errorf("invalid callback URL: %w", err)
		}
		code := u.Query().Get("code")
		state := u.Query().Get("state")
		if code == "" || state == "" {
			return "", "", "", fmt.Errorf("callback URL missing code or state parameters")
		}
		h := strings.ToLower(u.Hostname())
		return code, state, h, nil
	}
	if strings.HasPrefix(trimmed, "?") {
		trimmed = trimmed[1:]
	}
	if strings.Contains(trimmed, "code=") && strings.Contains(trimmed, "state=") {
		values, err := url.ParseQuery(trimmed)
		if err != nil {
			return "", "", "", fmt.Errorf("could not parse query: %w", err)
		}
		code := values.Get("code")
		state := values.Get("state")
		if code == "" || state == "" {
			return "", "", "", fmt.Errorf("query missing code or state parameters")
		}
		return code, state, "", nil
	}
	if strings.Contains(trimmed, "#") {
		parts := strings.SplitN(trimmed, "#", 2)
		if parts[0] == "" || parts[1] == "" {
			return "", "", "", fmt.Errorf("expected format code#state")
		}
		return parts[0], parts[1], "", nil
	}
	return "", "", "", fmt.Errorf("authorization callback missing code and state; paste the full URL or code#state pair")
}
