package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"microchat.ai/cmd/server/llm"
	pb "microchat.ai/proto"
)

// Test helper to create application instance for grpc handler tests
func setupTestApplication(t *testing.T) *application {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := &application{
		logger:       logger,
		sessionStore: NewSessionStore(2*time.Hour, 1000, 100, 100*1024),
	}

	return app
}

// Test helper to create application instance with mock provider
func setupTestApplicationWithMock(t *testing.T) (*application, *llm.MockProvider) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockProvider := llm.NewMockProvider("Mock-Test-Provider")

	app := &application{
		logger:       logger,
		sessionStore: NewSessionStore(2*time.Hour, 1000, 100, 100*1024),
		providerFactory: func(model pb.Model, logger *slog.Logger) llm.Provider {
			return mockProvider
		},
	}

	return app, mockProvider
}

// Layer 4: Happy path - normal delta protocol flow
func TestDeltaProtocol(t *testing.T) {
	app, mockProvider := setupTestApplicationWithMock(t)
	mockProvider.SetResponses("First response", "Second response", "Third response")
	ctx := context.Background()
	sessionID := "550e8400-e29b-41d4-a716-446655440000" // Valid UUID

	// Register the session first
	app.sessionStore.RegisterSession(sessionID)

	// First message: index=0, expect count=2
	req1 := &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "First",
		MessageIndex: 0,
	}
	resp1, _ := app.Chat(ctx, req1)
	if resp1.MessageCount != 2 {
		t.Errorf("First message: expected count=2, got %d", resp1.MessageCount)
	}

	// Second message: index=2, expect count=4
	req2 := &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "Second",
		MessageIndex: 2,
	}
	resp2, _ := app.Chat(ctx, req2)
	if resp2.MessageCount != 4 {
		t.Errorf("Second message: expected count=4, got %d", resp2.MessageCount)
	}

	// Third message: index=4, expect count=6
	req3 := &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "Third",
		MessageIndex: 4,
	}
	resp3, _ := app.Chat(ctx, req3)
	if resp3.MessageCount != 6 {
		t.Errorf("Third message: expected count=6, got %d", resp3.MessageCount)
	}
}

// Edge case: Client sends wrong index
func TestDeltaProtocolWrongIndex(t *testing.T) {
	app, mockProvider := setupTestApplicationWithMock(t)
	mockProvider.SetResponses("First response", "Wrong index response")
	ctx := context.Background()
	sessionID := "550e8400-e29b-41d4-a716-446655440001" // Valid UUID

	// Register the session first
	app.sessionStore.RegisterSession(sessionID)

	// Create session
	app.Chat(ctx, &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "First",
		MessageIndex: 0,
	})

	// Send with wrong index (10 instead of 2)
	req := &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "Wrong index",
		MessageIndex: 10,
	}
	resp, _ := app.Chat(ctx, req)

	// Should still accept and return correct count
	if resp.MessageCount != 4 {
		t.Errorf("Wrong index: expected count=4, got %d", resp.MessageCount)
	}
}

// Edge case: Backward compatibility (no index field)
func TestDeltaProtocolBackwardCompatibility(t *testing.T) {
	app, mockProvider := setupTestApplicationWithMock(t)
	mockProvider.SetResponses("Backward compatibility response")
	ctx := context.Background()
	sessionID := "550e8400-e29b-41d4-a716-446655440002" // Valid UUID

	// Register the session first
	app.sessionStore.RegisterSession(sessionID)

	// Send without MessageIndex (defaults to 0)
	req := &pb.ChatRequest{
		SessionId: sessionID,
		Message:   "No index",
		// MessageIndex omitted
	}
	resp, _ := app.Chat(ctx, req)

	// Should work normally
	if resp.MessageCount != 2 {
		t.Errorf("No index: expected count=2, got %d", resp.MessageCount)
	}
}

// Test input validation
func TestChatValidation(t *testing.T) {
	app := setupTestApplication(t)
	ctx := context.Background()

	// Test empty session ID
	req := &pb.ChatRequest{
		SessionId: "",
		Message:   "Hello",
	}
	_, err := app.Chat(ctx, req)
	if err == nil {
		t.Error("Expected error for empty session ID")
	}
	if !strings.Contains(err.Error(), "session ID cannot be empty") {
		t.Errorf("Expected empty session ID error, got: %v", err)
	}

	// Test invalid session ID format
	req = &pb.ChatRequest{
		SessionId: "invalid-uuid",
		Message:   "Hello",
	}
	_, err = app.Chat(ctx, req)
	if err == nil {
		t.Error("Expected error for invalid session ID format")
	}
	if !strings.Contains(err.Error(), "invalid session ID format") {
		t.Errorf("Expected invalid session ID format error, got: %v", err)
	}

	// Test empty message
	req = &pb.ChatRequest{
		SessionId: "550e8400-e29b-41d4-a716-446655440000", // Valid UUID
		Message:   "",
	}
	_, err = app.Chat(ctx, req)
	if err == nil {
		t.Error("Expected error for empty message")
	}
	if !strings.Contains(err.Error(), "message cannot be empty") {
		t.Errorf("Expected empty message error, got: %v", err)
	}

	// Test oversized message (over 10KB)
	largeMessage := strings.Repeat("a", 10*1024+1) // 10KB + 1 byte
	req = &pb.ChatRequest{
		SessionId: "550e8400-e29b-41d4-a716-446655440000", // Valid UUID
		Message:   largeMessage,
	}
	_, err = app.Chat(ctx, req)
	if err == nil {
		t.Error("Expected error for oversized message")
	}
	if !strings.Contains(err.Error(), "message too large") {
		t.Errorf("Expected message too large error, got: %v", err)
	}

	// Test valid input should work - use mock for deterministic behavior
	app2, mockProvider := setupTestApplicationWithMock(t)
	mockProvider.SetResponses("Valid response", "Unicode response")

	// Register the session first
	sessionID := "550e8400-e29b-41d4-a716-446655440000"
	app2.sessionStore.RegisterSession(sessionID)

	req = &pb.ChatRequest{
		SessionId: sessionID, // Valid UUID
		Message:   "Hello, this is a valid message!",
	}
	_, err = app2.Chat(ctx, req)
	if err != nil {
		t.Errorf("Valid input should not produce error, got: %v", err)
	}

	// Test Unicode and special characters
	req = &pb.ChatRequest{
		SessionId: sessionID, // Valid UUID
		Message:   "Hello 世界! Special chars: @#$%^&*()_+{}|:<>?[]\\;'\",./ 🚀",
	}
	_, err = app2.Chat(ctx, req)
	if err != nil {
		t.Errorf("Unicode and special characters should be valid, got: %v", err)
	}
}

// Test GetHistory validation
func TestGetHistoryValidation(t *testing.T) {
	app := setupTestApplication(t)
	ctx := context.Background()

	// Test empty session ID
	req := &pb.GetHistoryRequest{
		SessionId: "",
	}
	_, err := app.GetHistory(ctx, req)
	if err == nil {
		t.Error("Expected error for empty session ID")
	}
	if !strings.Contains(err.Error(), "session ID cannot be empty") {
		t.Errorf("Expected empty session ID error, got: %v", err)
	}

	// Test invalid session ID format
	req = &pb.GetHistoryRequest{
		SessionId: "not-a-uuid",
	}
	_, err = app.GetHistory(ctx, req)
	if err == nil {
		t.Error("Expected error for invalid session ID format")
	}
	if !strings.Contains(err.Error(), "invalid session ID format") {
		t.Errorf("Expected invalid session ID format error, got: %v", err)
	}

	// Test valid session ID should work
	req = &pb.GetHistoryRequest{
		SessionId: "550e8400-e29b-41d4-a716-446655440000", // Valid UUID
	}
	_, err = app.GetHistory(ctx, req)
	if err != nil {
		t.Errorf("Valid session ID should not produce error, got: %v", err)
	}
}

// Test with mock provider - success scenarios
func TestChatWithMockProvider(t *testing.T) {
	app, mockProvider := setupTestApplicationWithMock(t)
	ctx := context.Background()
	sessionID := "550e8400-e29b-41d4-a716-446655440000"

	// Register the session first
	app.sessionStore.RegisterSession(sessionID)

	// Configure mock responses
	mockProvider.SetResponses("Mocked response 1", "Mocked response 2")

	// First chat request
	req1 := &pb.ChatRequest{
		SessionId: sessionID,
		Message:   "Hello",
		Model:     pb.Model_GEMINI_2_5_FLASH_LITE,
	}
	resp1, err := app.Chat(ctx, req1)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify mock was called and response matches
	if !strings.Contains(resp1.Reply, "Mock response to: 'Hello'") {
		t.Errorf("Expected mocked response containing 'Hello', got: %s", resp1.Reply)
	}
	if !strings.Contains(resp1.Reply, "Mocked response 1") {
		t.Errorf("Expected first mocked response, got: %s", resp1.Reply)
	}

	// Second chat request
	req2 := &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "How are you?",
		Model:        pb.Model_GEMINI_2_5_FLASH_LITE,
		MessageIndex: 2, // After first exchange
	}
	resp2, err := app.Chat(ctx, req2)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify second mock response
	if !strings.Contains(resp2.Reply, "Mock response to: 'How are you?'") {
		t.Errorf("Expected mocked response containing 'How are you?', got: %s", resp2.Reply)
	}
	if !strings.Contains(resp2.Reply, "Mocked response 2") {
		t.Errorf("Expected second mocked response, got: %s", resp2.Reply)
	}

	// Verify message counts are correct
	if resp1.MessageCount != 2 {
		t.Errorf("Expected first response count=2, got %d", resp1.MessageCount)
	}
	if resp2.MessageCount != 4 {
		t.Errorf("Expected second response count=4, got %d", resp2.MessageCount)
	}
}

// Test with mock provider - error scenarios
func TestChatWithMockProviderError(t *testing.T) {
	app, mockProvider := setupTestApplicationWithMock(t)
	ctx := context.Background()
	sessionID := "550e8400-e29b-41d4-a716-446655440000"

	// Register the session first
	app.sessionStore.RegisterSession(sessionID)

	// Configure mock to return an error
	mockProvider.SetError("Mock LLM provider timeout")

	req := &pb.ChatRequest{
		SessionId: sessionID,
		Message:   "This should fail",
		Model:     pb.Model_GEMINI_2_5_FLASH_LITE,
	}

	_, err := app.Chat(ctx, req)
	if err == nil {
		t.Fatal("Expected error from mock provider, got nil")
	}

	// Verify error contains expected message
	if !strings.Contains(err.Error(), "LLM provider failed") {
		t.Errorf("Expected LLM provider error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Mock LLM provider timeout") {
		t.Errorf("Expected mock error message, got: %v", err)
	}

	// Verify error code is Internal
	if !strings.Contains(err.Error(), "code = Internal") {
		t.Errorf("Expected Internal error code, got: %v", err)
	}
}

// Test that mocked tests run without live dependencies
func TestMockedTestsRunInIsolation(t *testing.T) {
	// This test verifies that we can run tests without any external dependencies
	// by ensuring our mock provider works without network calls

	app, mockProvider := setupTestApplicationWithMock(t)
	ctx := context.Background()
	sessionID := "550e8400-e29b-41d4-a716-446655440000"

	// Register the session first
	app.sessionStore.RegisterSession(sessionID)

	mockProvider.SetResponses("Isolated test response")

	req := &pb.ChatRequest{
		SessionId: sessionID,
		Message:   "Test isolation",
		Model:     pb.Model_ECHO, // Even though we request ECHO, mock should handle it
	}

	// This should complete quickly and deterministically
	start := time.Now()
	resp, err := app.Chat(ctx, req)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Isolated test failed: %v", err)
	}

	// Should be very fast since no network calls
	if duration > time.Second {
		t.Errorf("Test took too long (%v), suggesting network calls were made", duration)
	}

	// Should contain our mock response
	if !strings.Contains(resp.Reply, "Isolated test response") {
		t.Errorf("Expected isolated mock response, got: %s", resp.Reply)
	}
}
