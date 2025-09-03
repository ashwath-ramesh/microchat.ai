package main

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	pb "microchat.ai/proto"
)

// generateSessionID creates a new UUID session ID for testing
func generateSessionID() string {
	return uuid.New().String()
}

func setupTestApp(t *testing.T) *application {
	// Set required environment variables for testing
	os.Setenv("CA_CERT_FILE", "certs/ca.crt")
	os.Setenv("API_KEY", "test-key")

	cfg := config{
		serverAddr: "localhost:4000",
		model:      pb.Model_GEMINI_2_5_FLASH_LITE,
		sessionID:  generateSessionID(),
		apiKey:     os.Getenv("API_KEY"), // Get API key from environment
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

	// Health endpoint doesn't require authentication
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
	ctx := app.addAuthContext(context.Background())

	req := &pb.ChatRequest{
		SessionId: app.config.sessionID,
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

	if resp.SessionId != app.config.sessionID {
		t.Errorf("Expected session ID %s, got %s", app.config.sessionID, resp.SessionId)
	}

	t.Logf("Chat successful: sent='%s', received='%s'", testMessage, resp.Reply)
}

// Layer 4: Happy path - message index tracking
func TestMessageIndexTracking(t *testing.T) {
	// Use different API key for this test to avoid rate limiting
	os.Setenv("API_KEY", "test-key-2")
	app := setupTestApp(t)
	defer app.conn.Close()

	ctx := app.addAuthContext(context.Background())
	sessionID := app.config.sessionID

	// First message: index=0, expect count=2
	resp1, err := app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "First",
		MessageIndex: 0,
	})
	if err != nil {
		t.Fatalf("First message failed: %v", err)
	}
	if resp1.MessageCount != 2 {
		t.Errorf("First: expected count=2, got %d", resp1.MessageCount)
	}

	// Second message: index=2, expect count=4
	resp2, err := app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "Second",
		MessageIndex: 2,
	})
	if err != nil {
		t.Fatalf("Second message failed: %v", err)
	}
	if resp2.MessageCount != 4 {
		t.Errorf("Second: expected count=4, got %d", resp2.MessageCount)
	}

	// Third message: index=4, expect count=6
	resp3, err := app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId:    sessionID,
		Message:      "Third",
		MessageIndex: 4,
	})
	if err != nil {
		t.Fatalf("Third message failed: %v", err)
	}
	if resp3.MessageCount != 6 {
		t.Errorf("Third: expected count=6, got %d", resp3.MessageCount)
	}
}

// Edge cases: wrong index and backward compatibility
func TestDeltaProtocolEdgeCases(t *testing.T) {
	// Use different API key for this test to avoid rate limiting
	os.Setenv("API_KEY", "test-key-3")
	app := setupTestApp(t)
	defer app.conn.Close()

	ctx := app.addAuthContext(context.Background())

	// Edge case 1: Wrong index (should still work)
	sessionID1 := generateSessionID()
	_, err := app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId:    sessionID1,
		Message:      "First",
		MessageIndex: 0,
	})
	if err != nil {
		t.Fatalf("First message in wrong index test failed: %v", err)
	}

	resp, err := app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId:    sessionID1,
		Message:      "Wrong index",
		MessageIndex: 10, // Wrong: should be 2
	})
	if err != nil {
		t.Fatalf("Wrong index message failed: %v", err)
	}
	if resp.MessageCount != 4 {
		t.Errorf("Wrong index: expected count=4, got %d", resp.MessageCount)
	}

	// Edge case 2: No index field (backward compatibility)
	sessionID2 := generateSessionID()
	resp2, err := app.grpc.Chat(ctx, &pb.ChatRequest{
		SessionId: sessionID2,
		Message:   "No index",
		// MessageIndex omitted
	})
	if err != nil {
		t.Fatalf("No index message failed: %v", err)
	}
	if resp2.MessageCount != 2 {
		t.Errorf("No index: expected count=2, got %d", resp2.MessageCount)
	}
}
