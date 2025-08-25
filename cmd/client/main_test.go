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
