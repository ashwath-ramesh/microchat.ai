package llm

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"google.golang.org/genai"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGeminiProvider_GenerateResponse_EmptyMessages(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	provider := &GeminiProvider{logger: logger}

	_, err := provider.GenerateResponse(context.Background(), []Message{})

	if err == nil {
		t.Fatal("expected error for empty messages")
	}

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got: %v", status.Code(err))
	}
}

func TestGeminiProvider_GenerateResponse_ContextCancellation(t *testing.T) {
	// Test that cancelled context is handled properly without calling Gemini API
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	provider := &GeminiProvider{logger: logger}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := provider.GenerateResponse(ctx, []Message{{Role: "user", Text: "Hello"}})

	if err == nil {
		t.Fatal("expected error for cancelled context")
	}

	if status.Code(err) != codes.Canceled {
		t.Fatalf("expected Canceled, got: %v", status.Code(err))
	}
}

func TestGeminiProvider_Name(t *testing.T) {
	provider := &GeminiProvider{}

	if provider.Name() != "Gemini-2.5-Flash-Lite" {
		t.Fatalf("unexpected provider name: %s", provider.Name())
	}
}

// MockGenaiClient implements GeminiClient interface for testing
type MockGenaiClient struct {
	shouldFail   bool
	failAttempts int
	responseText string
	callDelay    time.Duration
}

type MockModels struct {
	client *MockGenaiClient
}

func (m *MockGenaiClient) Models() GeminiModels {
	return &MockModels{client: m}
}

func (m *MockModels) GenerateContent(ctx context.Context, model string, content []*genai.Content, opts *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	// Simulate delay if specified
	if m.client.callDelay > 0 {
		select {
		case <-time.After(m.client.callDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Simulate failures for retry testing
	if m.client.failAttempts > 0 {
		m.client.failAttempts--
		return nil, errors.New("simulated Gemini API failure")
	}

	if m.client.shouldFail {
		return nil, errors.New("simulated Gemini API failure")
	}

	// Create mock response
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						genai.NewPartFromText(m.client.responseText),
					},
				},
			},
		},
	}, nil
}

func TestGeminiProvider_GenerateResponse_RetrySuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	mockClient := &MockGenaiClient{
		failAttempts: 2, // Fail first 2 attempts
		responseText: "Success after retries",
	}

	provider := &GeminiProvider{
		client: mockClient,
		logger: logger,
	}

	ctx := context.Background()
	messages := []Message{{Role: "user", Text: "Hello"}}

	start := time.Now()
	response, err := provider.GenerateResponse(ctx, messages)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}

	if response != "Success after retries" {
		t.Fatalf("expected 'Success after retries', got: %s", response)
	}

	// Should have waited for backoff (1s + 2s = 3s minimum)
	if duration < 3*time.Second {
		t.Fatalf("expected at least 3s for backoff, got: %v", duration)
	}
}

func TestGeminiProvider_GenerateResponse_TimeoutWithRetry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	mockClient := &MockGenaiClient{
		callDelay: 35 * time.Second, // Longer than 30s timeout
	}

	provider := &GeminiProvider{
		client: mockClient,
		logger: logger,
	}

	ctx := context.Background()
	messages := []Message{{Role: "user", Text: "Hello"}}

	start := time.Now()
	_, err := provider.GenerateResponse(ctx, messages)
	duration := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	grpcStatus, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC error, got: %v", err)
	}

	if grpcStatus.Code() != codes.DeadlineExceeded && grpcStatus.Code() != codes.Unavailable {
		t.Fatalf("expected DeadlineExceeded or Unavailable, got: %v", grpcStatus.Code())
	}

	// Should complete relatively quickly due to timeouts, not wait full retry duration
	if duration > 2*time.Minute {
		t.Fatalf("took too long, expected timeouts to fail fast: %v", duration)
	}
}
