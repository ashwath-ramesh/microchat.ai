package main

import (
	"context"
	"log/slog"
	"os"
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
	sessionID := uint32(1000)

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
	sessionID := uint32(2000)

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
	sessionID := uint32(3000)

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
