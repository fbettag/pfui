package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fbettag/pfui/internal/provider"
)

// Client is a placeholder Anthropic provider implementation.
type Client struct {
	host       string
	token      string
	name       string
	httpClient *http.Client
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
	return &Client{
		host:       strings.TrimRight(host, "/"),
		token:      token,
		name:       name,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
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

func (c *Client) StreamChat(ctx context.Context, req provider.ChatCompletionRequest) (<-chan provider.StreamChunk, error) {
	if strings.TrimSpace(c.token) == "" {
		return nil, fmt.Errorf("%s: API key missing; run pfui --configuration", c.name)
	}
	model := req.Model
	if model == "" {
		model = "claude-4.5-sonnet"
	}
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": joinContent(req.Messages)},
		},
		"stream":     true,
		"max_tokens": 1024,
	}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("x-api-key", c.token)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s messages error: %s", c.name, strings.TrimSpace(string(data)))
	}
	ch := make(chan provider.StreamChunk)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					ch <- provider.StreamChunk{Err: err}
				}
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" || payload == "[DONE]" {
				ch <- provider.StreamChunk{Done: true}
				return
			}
			var event anthropicEvent
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				ch <- provider.StreamChunk{Err: err, Done: true}
				return
			}
			switch event.Type {
			case "content_block_delta":
				if event.Delta.Text != "" {
					ch <- provider.StreamChunk{Content: event.Delta.Text}
				}
			case "message_delta":
				if len(event.Delta.StopReason) > 0 {
					ch <- provider.StreamChunk{Done: true}
					return
				}
			case "error":
				ch <- provider.StreamChunk{Err: errors.New(event.Error.Message), Done: true}
				return
			case "message_stop":
				ch <- provider.StreamChunk{Done: true}
				return
			}
		}
	}()
	return ch, nil
}

func joinContent(messages []provider.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}
	var buf strings.Builder
	for _, msg := range messages {
		if msg.Content == "" {
			continue
		}
		buf.WriteString(msg.Content)
		buf.WriteString("\n\n")
	}
	return strings.TrimSpace(buf.String())
}

type anthropicEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Text       string `json:"text"`
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}
