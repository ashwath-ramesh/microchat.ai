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

func TestHealthCheck(t *testing.T) {
	app := setupTestApp(t)
	defer app.conn.Close()

	ctx := context.Background()
	resp, err := app.grpc.HealthCheck(ctx, &pb.HealthRequest{})
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}

	if resp.Status != pb.HealthStatus_AVAILABLE {
		t.Errorf("Expected status AVAILABLE, got %v", resp.Status)
	}

	if resp.Version == "" {
		t.Error("Expected version to be set")
	}

	t.Logf("Health check successful: status=%s, env=%s, version=%s",
		resp.Status, resp.Environment, resp.Version)
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

	if resp.Error != pb.ErrorCode_NO_ERROR {
		t.Errorf("Expected NO_ERROR, got %v", resp.Error)
	}

	if resp.Reply == "" {
		t.Error("Expected non-empty reply")
	}

	if resp.SessionId != uint32(app.config.sessionID) {
		t.Errorf("Expected session ID %d, got %d", app.config.sessionID, resp.SessionId)
	}

	t.Logf("Chat successful: sent='%s', received='%s'", testMessage, resp.Reply)
}

func TestMultipleMessages(t *testing.T) {
	app := setupTestApp(t)
	defer app.conn.Close()

	messages := []string{
		"First message",
		"Second message",
		"Third message",
	}

	ctx := context.Background()

	for i, message := range messages {
		req := &pb.ChatRequest{
			SessionId: uint32(app.config.sessionID),
			Model:     app.config.model,
			Message:   message,
		}

		resp, err := app.grpc.Chat(ctx, req)
		if err != nil {
			t.Fatalf("Chat request %d failed: %v", i+1, err)
		}

		if resp.Error != pb.ErrorCode_NO_ERROR {
			t.Errorf("Message %d: Expected NO_ERROR, got %v", i+1, resp.Error)
		}

		t.Logf("Message %d successful: sent='%s', received='%s'", i+1, message, resp.Reply)
	}
}

func TestDifferentModels(t *testing.T) {
	app := setupTestApp(t)
	defer app.conn.Close()

	models := []pb.Model{
		pb.Model_GPT_4,
		pb.Model_GPT_3_5,
		pb.Model_CLAUDE_4,
		pb.Model_GEMINI_2_5_PRO,
	}

	ctx := context.Background()

	for _, model := range models {
		req := &pb.ChatRequest{
			SessionId: uint32(app.config.sessionID),
			Model:     model,
			Message:   "Test message",
		}

		resp, err := app.grpc.Chat(ctx, req)
		if err != nil {
			t.Fatalf("Chat request with model %v failed: %v", model, err)
		}

		if resp.Error != pb.ErrorCode_NO_ERROR {
			t.Errorf("Model %v: Expected NO_ERROR, got %v", model, resp.Error)
		}

		t.Logf("Model %v successful: received='%s'", model, resp.Reply)
	}
}
