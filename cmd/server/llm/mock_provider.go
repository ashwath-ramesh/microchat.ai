package llm

import (
	"context"
	"errors"
	"fmt"
)

// MockProvider is a test implementation of the Provider interface
type MockProvider struct {
	name          string
	responses     []string
	responseIndex int
	shouldError   bool
	errorMessage  string
}

// NewMockProvider creates a new mock provider with configurable responses
func NewMockProvider(name string) *MockProvider {
	return &MockProvider{
		name:          name,
		responses:     []string{"Mock response from " + name},
		responseIndex: 0,
		shouldError:   false,
	}
}

// SetResponses configures the mock to return specific responses in sequence
func (m *MockProvider) SetResponses(responses ...string) {
	m.responses = responses
	m.responseIndex = 0
}

// SetError configures the mock to return an error
func (m *MockProvider) SetError(errorMessage string) {
	m.shouldError = true
	m.errorMessage = errorMessage
}

// ClearError configures the mock to stop returning errors
func (m *MockProvider) ClearError() {
	m.shouldError = false
	m.errorMessage = ""
}

// GenerateResponse implements the Provider interface
func (m *MockProvider) GenerateResponse(ctx context.Context, messages []Message) (string, error) {
	if m.shouldError {
		return "", errors.New(m.errorMessage)
	}

	if len(m.responses) == 0 {
		return "Default mock response", nil
	}

	// Cycle through responses
	response := m.responses[m.responseIndex%len(m.responses)]
	m.responseIndex++

	// Add message context to make testing more realistic
	if len(messages) > 0 {
		lastMessage := messages[len(messages)-1]
		response = fmt.Sprintf("Mock response to: '%s' - %s", lastMessage.Text, response)
	}

	return response, nil
}

// Name implements the Provider interface
func (m *MockProvider) Name() string {
	return m.name
}

// Reset resets the mock's state
func (m *MockProvider) Reset() {
	m.responseIndex = 0
	m.shouldError = false
	m.errorMessage = ""
}
