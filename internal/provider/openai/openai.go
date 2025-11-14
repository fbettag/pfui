package openai

import (
	"context"

	"github.com/fbettag/pfui/internal/provider"
)

// Client is a placeholder OpenAI provider implementation.
type Client struct {
	host  string
	token string
	name  string
}

// New creates a client pointed at the provided host/token.
func New(host, token string) *Client {
	return newClient(host, token, "OpenAI")
}

// NewWithName lets callers override the display name (used for custom adapters).
func NewWithName(host, token, name string) *Client {
	return newClient(host, token, name)
}

func newClient(host, token, name string) *Client {
	if host == "" {
		host = "https://api.openai.com"
	}
	if name == "" {
		name = "OpenAI"
	}
	return &Client{host: host, token: token, name: name}
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) Kind() provider.Kind {
	return provider.KindOpenAI
}

func (c *Client) ListModels(ctx context.Context) ([]provider.Model, error) {
	_ = ctx
	return []provider.Model{
		{
			Name:        "gpt-5",
			Description: "General-purpose GPT-5 model for broad coding sessions.",
			Capabilities: []string{
				"chat",
				"code",
				"plan",
				"tools",
			},
		},
		{
			Name:        "gpt-5.1",
			Description: "Updated GPT-5.1 with improved reasoning and tool reliability.",
			Capabilities: []string{
				"chat",
				"code",
				"plan",
				"tools",
			},
		},
		{
			Name:        "gpt-5.1-codex",
			Description: "GPT-5.1 Codex variant optimized for developer workflows and reasoning chains.",
			Capabilities: []string{
				"chat",
				"code",
				"plan",
				"tools",
			},
			Tags: map[string]string{"mode": "codex"},
		},
	}, nil
}

func (c *Client) StartChat(ctx context.Context, opts provider.StartChatOptions) (provider.Session, error) {
	_ = ctx
	return provider.NewSession("openai", opts.SessionID), nil
}
