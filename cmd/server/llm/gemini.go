package llm

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/genai"
)

// GeminiProvider implements Provider interface using Google's Gemini API
type GeminiProvider struct {
	client *genai.Client
}

// NewGeminiProvider creates a new Gemini provider
func NewGeminiProvider() (Provider, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable not set")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &GeminiProvider{client: client}, nil
}

// GenerateResponse sends the conversation history to Gemini and returns the response
func (g *GeminiProvider) GenerateResponse(ctx context.Context, messages []Message) (string, error) {
	model := "gemini-2.5-flash-lite"

	// Convert our messages to Gemini format
	var parts []*genai.Part
	for _, msg := range messages {
		parts = append(parts, genai.NewPartFromText(fmt.Sprintf("%s: %s", msg.Role, msg.Text)))
	}

	// If no messages, return error
	if len(parts) == 0 {
		return "", fmt.Errorf("no messages to process")
	}

	// Create content with parts
	content := []*genai.Content{{Parts: parts}}

	// Generate content using Gemini
	result, err := g.client.Models.GenerateContent(ctx, model, content, nil)
	if err != nil {
		return "", fmt.Errorf("Gemini API error: %w", err)
	}

	// Extract text from response
	text := result.Text()
	if text == "" {
		return "", fmt.Errorf("Gemini returned empty response")
	}

	return text, nil
}

// Name returns the provider name
func (g *GeminiProvider) Name() string {
	return "Gemini-2.5-Flash-Lite"
}

// Close closes the Gemini client (if needed)
// Note: genai.Client may not have a Close method - removing for now
