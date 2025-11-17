package provider

import (
	"context"
	"fmt"
)

// Kind identifies the provider family.
type Kind string

const (
	KindOpenAI    Kind = "openai"
	KindAnthropic Kind = "anthropic"
	KindCustom    Kind = "custom"
)

// Model describes a model returned by a provider.
type Model struct {
	Name         string
	Description  string
	Capabilities []string
	Tags         map[string]string
}

// ChatMessage models a basic role/content pair.
type ChatMessage struct {
	Role    string
	Content string
}

// ChatCompletionRequest describes a streaming completion.
type ChatCompletionRequest struct {
	Model    string
	Messages []ChatMessage
}

// StreamChunk is emitted while a provider streams a response.
type StreamChunk struct {
	Content string
	Err     error
	Done    bool
}

// StartChatOptions configure new sessions.
type StartChatOptions struct {
	SessionID string
	PlanMode  string
}

// Session represents an active provider chat.
type Session interface {
	ID() string
	Close() error
}

// Provider is the contract each backend must implement.
type Provider interface {
	Name() string
	Kind() Kind
	ListModels(ctx context.Context) ([]Model, error)
	StartChat(ctx context.Context, opts StartChatOptions) (Session, error)
	StreamChat(ctx context.Context, req ChatCompletionRequest) (<-chan StreamChunk, error)
}

// Registry stores available providers (built-in + custom).
type Registry struct {
	providers []Provider
}

// NewRegistry registers the supplied providers.
func NewRegistry(providers ...Provider) Registry {
	return Registry{providers: providers}
}

// Providers returns all registered providers.
func (r Registry) Providers() []Provider {
	return append([]Provider(nil), r.providers...)
}

// ProviderByKind returns the first provider matching kind.
func (r Registry) ProviderByKind(kind Kind) (Provider, error) {
	for _, p := range r.providers {
		if p.Kind() == kind {
			return p, nil
		}
	}
	return nil, fmt.Errorf("provider %s not registered", kind)
}
