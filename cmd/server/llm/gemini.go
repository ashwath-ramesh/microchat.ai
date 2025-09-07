package llm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"google.golang.org/genai"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GeminiClient interface for testing
type GeminiClient interface {
	Models() GeminiModels
}

type GeminiModels interface {
	GenerateContent(ctx context.Context, model string, content []*genai.Content, opts *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
}

// GeminiProvider implements Provider interface using Google's Gemini API
type GeminiProvider struct {
	client GeminiClient
	logger *slog.Logger
}

// NewGeminiProvider creates a new Gemini provider
func NewGeminiProvider(logger *slog.Logger) (Provider, error) {
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

	return &GeminiProvider{client: &genaiClientWrapper{client: client}, logger: logger}, nil
}

// genaiClientWrapper adapts the real genai.Client to our interface
type genaiClientWrapper struct {
	client *genai.Client
}

func (w *genaiClientWrapper) Models() GeminiModels {
	return &genaiModelsWrapper{models: w.client.Models}
}

type genaiModelsWrapper struct {
	models *genai.Models
}

func (w *genaiModelsWrapper) GenerateContent(ctx context.Context, model string, content []*genai.Content, opts *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return w.models.GenerateContent(ctx, model, content, opts)
}

// GenerateResponse sends the conversation history to Gemini and returns the response
func (g *GeminiProvider) GenerateResponse(ctx context.Context, messages []Message) (string, error) {
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.5-flash-lite" // default
	}

	// Configure safety settings for content filtering
	safetySettings := []*genai.SafetySetting{
		{
			Category:  genai.HarmCategoryHarassment,
			Threshold: genai.HarmBlockThresholdBlockMediumAndAbove,
		},
		{
			Category:  genai.HarmCategoryHateSpeech,
			Threshold: genai.HarmBlockThresholdBlockMediumAndAbove,
		},
		{
			Category:  genai.HarmCategorySexuallyExplicit,
			Threshold: genai.HarmBlockThresholdBlockMediumAndAbove,
		},
		{
			Category:  genai.HarmCategoryDangerousContent,
			Threshold: genai.HarmBlockThresholdBlockMediumAndAbove,
		},
	}

	// Configure max output tokens (default: 2048 tokens ≈ 1500 words)
	maxTokens := int32(2048)
	if maxTokensEnv := os.Getenv("GEMINI_MAX_OUTPUT_TOKENS"); maxTokensEnv != "" {
		if parsed, err := strconv.Atoi(maxTokensEnv); err == nil && parsed > 0 && parsed <= 8192 {
			maxTokens = int32(parsed)
		}
	}

	generateConfig := &genai.GenerateContentConfig{
		SafetySettings:  safetySettings,
		MaxOutputTokens: maxTokens,
	}

	// Convert our messages to Gemini format
	var parts []*genai.Part
	for _, msg := range messages {
		parts = append(parts, genai.NewPartFromText(fmt.Sprintf("%s: %s", msg.Role, msg.Text)))
	}

	// If no messages, return error
	if len(parts) == 0 {
		return "", status.Error(codes.InvalidArgument, "no messages to process")
	}

	// Create content with parts
	content := []*genai.Content{{Parts: parts}}

	// Retry with exponential backoff
	var lastErr error
	backoffDurations := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	for attempt := 0; attempt < 3; attempt++ {
		// Check if context is already cancelled before attempting
		if ctx.Err() == context.Canceled {
			return "", status.Error(codes.Canceled, "request cancelled")
		}

		if attempt > 0 {
			g.logger.Warn("retrying Gemini API call", "attempt", attempt+1, "backoff", backoffDurations[attempt-1])
			time.Sleep(backoffDurations[attempt-1])
		}

		// Create timeout context (30 seconds)
		timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

		// Generate content using Gemini with safety settings and token limits
		result, err := g.client.Models().GenerateContent(timeoutCtx, model, content, generateConfig)
		cancel() // Always cancel the timeout context

		if err != nil {
			lastErr = err
			g.logger.Warn("Gemini API call failed", "attempt", attempt+1, "error", err)

			// Check if this is a timeout or context cancellation
			if timeoutCtx.Err() == context.DeadlineExceeded {
				lastErr = status.Error(codes.DeadlineExceeded, "Gemini API timeout")
			} else if ctx.Err() == context.Canceled {
				// Don't retry if the original context was cancelled
				return "", status.Error(codes.Canceled, "request cancelled")
			}

			// Continue to next attempt
			continue
		}

		// Extract text from response
		text := result.Text()
		if text == "" {
			lastErr = fmt.Errorf("Gemini returned empty response")
			g.logger.Warn("Gemini returned empty response", "attempt", attempt+1)
			continue
		}

		g.logger.Info("Gemini API call successful", "attempt", attempt+1)
		return text, nil
	}

	// All attempts failed
	g.logger.Error("all Gemini API attempts failed", "error", lastErr)

	// Return appropriate gRPC status code
	if grpcStatus, ok := status.FromError(lastErr); ok {
		return "", grpcStatus.Err()
	}

	// Default to unavailable for unknown errors
	return "", status.Error(codes.Unavailable, fmt.Sprintf("Gemini API failed after 3 attempts: %v", lastErr))
}

// Name returns the provider name
func (g *GeminiProvider) Name() string {
	return "Gemini-2.5-Flash-Lite"
}

// Close closes the Gemini client (if needed)
// Note: genai.Client may not have a Close method - removing for now
