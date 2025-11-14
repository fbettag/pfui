package anthropic

import (
	"context"

	"github.com/fbettag/pfui/internal/provider"
)

// Client is a placeholder Anthropic provider implementation.
type Client struct {
	host  string
	token string
	name  string
}

// New builds a Client for the provided host/token.
func New(host, token string) *Client {
	return newClient(host, token, "Claude")
}

// NewWithName lets callers override the provider label (e.g., for adapters).
func NewWithName(host, token, name string) *Client {
	return newClient(host, token, name)
}

func newClient(host, token, name string) *Client {
	if host == "" {
		host = "https://api.anthropic.com"
	}
	if name == "" {
		name = "Claude"
	}
	return &Client{host: host, token: token, name: name}
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) Kind() provider.Kind {
	return provider.KindAnthropic
}

func (c *Client) ListModels(ctx context.Context) ([]provider.Model, error) {
	_ = ctx
	return []provider.Model{
		{
			Name:        "claude-4.5-sonnet",
			Description: "Claude 4.5 Sonnet – balanced plan/execution default.",
			Capabilities: []string{
				"chat",
				"code",
				"plan",
				"tools",
			},
			Tags: map[string]string{"mode": "plan"},
		},
		{
			Name:        "claude-4.5-haiku",
			Description: "Claude 4.5 Haiku – fast exploration and search sub-agent.",
			Capabilities: []string{
				"chat",
				"code",
			},
			Tags: map[string]string{"mode": "execution"},
		},
		{
			Name:        "claude-4.1-opus",
			Description: "Claude 4.1 Opus – highest reasoning tier for planning bursts.",
			Capabilities: []string{
				"chat",
				"code",
				"plan",
				"tools",
			},
			Tags: map[string]string{"tier": "opus"},
		},
	}, nil
}

func (c *Client) StartChat(ctx context.Context, opts provider.StartChatOptions) (provider.Session, error) {
	_ = ctx
	return provider.NewSession("claude", opts.SessionID), nil
}
