package main

import (
	"context"
	"log/slog"
	"os"
	"testing"

	pb "microchat.ai/proto"
)

func setupTestApp(t *testing.T) *application {
	cfg := config{
		serverAddr: "localhost:4000",
		model:      pb.Model_GPT_4,
		sessionID:  generateSessionID(),
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := &application{
		config: cfg,
		logger: logger,
	}

	if err := app.connect(); err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}

	return app
}

func TestHealth(t *testing.T) {
	app := setupTestApp(t)
	defer app.conn.Close()

	ctx := context.Background()
	resp, err := app.grpc.Health(ctx, &pb.HealthRequest{})
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	if !resp.Ok {
		t.Error("Expected health check to return ok=true")
	}
}

func TestChatMessage(t *testing.T) {
	app := setupTestApp(t)
	defer app.conn.Close()

	testMessage := "Hello, server!"
	ctx := context.Background()

	req := &pb.ChatRequest{
		SessionId: uint32(app.config.sessionID),
		Model:     app.config.model,
		Message:   testMessage,
	}

	resp, err := app.grpc.Chat(ctx, req)
	if err != nil {
		t.Fatalf("Chat request failed: %v", err)
	}

	if resp.Reply == "" {
		t.Error("Expected non-empty reply")
	}

	if resp.SessionId != uint32(app.config.sessionID) {
		t.Errorf("Expected session ID %d, got %d", app.config.sessionID, resp.SessionId)
	}

	t.Logf("Chat successful: sent='%s', received='%s'", testMessage, resp.Reply)
}

// Layer 4: Happy path - message index tracking
func TestMessageIndexTracking(t *testing.T) {
	app := setupTestApp(t)
	defer app.conn.Close()

	ctx := context.Background()
	sessionID := uint32(app.config.sessionID)

	// First message: index=0, expect count=2
	resp1, _ := app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "First",
		MessageIndex: 0,
	})
	if resp1.MessageCount != 2 {
		t.Errorf("First: expected count=2, got %d", resp1.MessageCount)
	}

	// Second message: index=2, expect count=4
	resp2, _ := app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "Second",
		MessageIndex: 2,
	})
	if resp2.MessageCount != 4 {
		t.Errorf("Second: expected count=4, got %d", resp2.MessageCount)
	}

	// Third message: index=4, expect count=6
	resp3, _ := app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "Third",
		MessageIndex: 4,
	})
	if resp3.MessageCount != 6 {
		t.Errorf("Third: expected count=6, got %d", resp3.MessageCount)
	}
}

// Edge cases: wrong index and backward compatibility
func TestDeltaProtocolEdgeCases(t *testing.T) {
	app := setupTestApp(t)
	defer app.conn.Close()

	ctx := context.Background()

	// Edge case 1: Wrong index (should still work)
	sessionID1 := generateSessionID()
	app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId:    uint32(sessionID1),
		Message:      "First",
		MessageIndex: 0,
	})

	resp, _ := app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId:    uint32(sessionID1),
		Message:      "Wrong index",
		MessageIndex: 10, // Wrong: should be 2
	})
	if resp.MessageCount != 4 {
		t.Errorf("Wrong index: expected count=4, got %d", resp.MessageCount)
	}

	// Edge case 2: No index field (backward compatibility)
	sessionID2 := generateSessionID()
	resp2, _ := app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId: uint32(sessionID2),
		Message:   "No index",
		// MessageIndex omitted
	})
	if resp2.MessageCount != 2 {
		t.Errorf("No index: expected count=2, got %d", resp2.MessageCount)
	}
}
