package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	pb "microchat.ai/proto"
)

// Test helper to create application instance for grpc handler tests
func setupTestApplication(t *testing.T) *application {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := &application{
		logger:       logger,
		sessionStore: NewSessionStore(2 * time.Hour),
	}

	return app
}

// Layer 4: Happy path - normal delta protocol flow
func TestDeltaProtocol(t *testing.T) {
	app := setupTestApplication(t)
	ctx := context.Background()
	sessionID := "550e8400-e29b-41d4-a716-446655440000" // Valid UUID

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
	app := setupTestApplication(t)
	ctx := context.Background()
	sessionID := "550e8400-e29b-41d4-a716-446655440001" // Valid UUID

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
	app := setupTestApplication(t)
	ctx := context.Background()
	sessionID := "550e8400-e29b-41d4-a716-446655440002" // Valid UUID

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

	// Test valid input should work
	req = &pb.ChatRequest{
		SessionId: "550e8400-e29b-41d4-a716-446655440000", // Valid UUID
		Message:   "Hello, this is a valid message!",
	}
	_, err = app.Chat(ctx, req)
	if err != nil {
		t.Errorf("Valid input should not produce error, got: %v", err)
	}

	// Test Unicode and special characters
	req = &pb.ChatRequest{
		SessionId: "550e8400-e29b-41d4-a716-446655440000", // Valid UUID
		Message:   "Hello 世界! Special chars: @#$%^&*()_+{}|:<>?[]\\;'\",./ 🚀",
	}
	_, err = app.Chat(ctx, req)
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
