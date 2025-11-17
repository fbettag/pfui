package openai

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

// Client is a placeholder OpenAI provider implementation.
type Client struct {
	host       string
	token      string
	name       string
	adapter    provider.AdapterKind
	httpClient *http.Client
}

// New creates a client pointed at the provided host/token.
func New(host, token string) *Client {
	return newClient(host, token, "OpenAI", provider.AdapterOpenAIChat)
}

// NewWithName lets callers override the display name (used for custom adapters).
func NewWithName(host, token, name string) *Client {
	return newClient(host, token, name, provider.AdapterOpenAIChat)
}

// NewWithAdapter allows custom manifests to choose the API style.
func NewWithAdapter(host, token, name string, adapter provider.AdapterKind) *Client {
	return newClient(host, token, name, adapter)
}

func newClient(host, token, name string, adapter provider.AdapterKind) *Client {
	if host == "" {
		host = "https://api.openai.com"
	}
	if name == "" {
		name = "OpenAI"
	}
	if adapter == "" {
		adapter = provider.AdapterOpenAIChat
	}
	return &Client{
		host:       strings.TrimRight(host, "/"),
		token:      token,
		name:       name,
		adapter:    adapter,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
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

func (c *Client) StreamChat(ctx context.Context, req provider.ChatCompletionRequest) (<-chan provider.StreamChunk, error) {
	if strings.TrimSpace(c.token) == "" {
		return nil, fmt.Errorf("%s: API key missing; run pfui --configuration", c.name)
	}
	switch c.adapter {
	case provider.AdapterOpenAIResponses:
		return c.streamResponses(ctx, req)
	default:
		return c.streamChatCompletions(ctx, req)
	}
}

func (c *Client) streamChatCompletions(ctx context.Context, req provider.ChatCompletionRequest) (<-chan provider.StreamChunk, error) {
	model := req.Model
	if model == "" {
		model = "gpt-5.1-codex"
	}
	payload := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "user", "content": joinContent(req.Messages)},
		},
		"stream": true,
	}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s chat error: %s", c.name, strings.TrimSpace(string(data)))
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
			if strings.HasPrefix(line, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if payload == "[DONE]" {
					ch <- provider.StreamChunk{Done: true}
					return
				}
				var chunk openAIChatChunk
				if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
					ch <- provider.StreamChunk{Err: err, Done: true}
					return
				}
				for _, choice := range chunk.Choices {
					if text := choice.Delta.Content; text != "" {
						ch <- provider.StreamChunk{Content: text}
					}
					if choice.FinishReason != "" {
						ch <- provider.StreamChunk{Done: true}
						return
					}
				}
			}
		}
	}()
	return ch, nil
}

func (c *Client) streamResponses(ctx context.Context, req provider.ChatCompletionRequest) (<-chan provider.StreamChunk, error) {
	model := req.Model
	if model == "" {
		model = "gpt-5.1-codex"
	}
	content := joinContent(req.Messages)
	payload := map[string]any{
		"model": model,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]string{
					{"type": "text", "text": content},
				},
			},
		},
		"stream": true,
	}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/v1/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s responses error: %s", c.name, strings.TrimSpace(string(data)))
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
			if strings.HasPrefix(line, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if payload == "[DONE]" {
					ch <- provider.StreamChunk{Done: true}
					return
				}
				var event openAIResponseEvent
				if err := json.Unmarshal([]byte(payload), &event); err != nil {
					ch <- provider.StreamChunk{Err: err, Done: true}
					return
				}
				if event.Error.Message != "" {
					ch <- provider.StreamChunk{Err: errors.New(event.Error.Message), Done: true}
					return
				}
				for _, delta := range event.Delta.Content {
					if delta.Text != "" {
						ch <- provider.StreamChunk{Content: delta.Text}
					}
				}
				if event.Type == "response.completed" {
					ch <- provider.StreamChunk{Done: true}
					return
				}
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

type openAIChatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

type openAIResponseEvent struct {
	Type  string `json:"type"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
	Delta struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"delta"`
}
