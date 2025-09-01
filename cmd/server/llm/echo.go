package llm

import (
	"context"
	"fmt"
)

// EchoProvider implements Provider interface with simple echo functionality
type EchoProvider struct{}

// NewEchoProvider creates a new echo provider
func NewEchoProvider() Provider {
	return &EchoProvider{}
}

// GenerateResponse returns the last user message with "Echo: " prefix
func (e *EchoProvider) GenerateResponse(ctx context.Context, messages []Message) (string, error) {
	if len(messages) == 0 {
		return "Echo: No message to echo", nil
	}

	// Find the last user message
	var lastUserMessage string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserMessage = messages[i].Text
			break
		}
	}

	if lastUserMessage == "" {
		return "Echo: No user message found", nil
	}

	return fmt.Sprintf("Echo: %s", lastUserMessage), nil
}

// Name returns the provider name
func (e *EchoProvider) Name() string {
	return "Echo"
}
