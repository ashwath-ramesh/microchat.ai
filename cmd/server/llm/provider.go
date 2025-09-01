package llm

import "context"

// Provider defines the interface for LLM providers
type Provider interface {
	GenerateResponse(ctx context.Context, messages []Message) (string, error)
	Name() string
}

// Message represents a single message in the conversation
type Message struct {
	Role string // "user" or "assistant"
	Text string
}
