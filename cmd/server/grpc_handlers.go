package main

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "microchat.ai/proto"
)

// validateSessionID checks if session ID is valid UUID format
func validateSessionID(sessionID string) error {
	if sessionID == "" {
		return status.Error(codes.InvalidArgument, "session ID cannot be empty")
	}
	if _, err := uuid.Parse(sessionID); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid session ID format: %v", err)
	}
	return nil
}

// validateMessage checks if message is valid
func validateMessage(message string) error {
	if message == "" {
		return status.Error(codes.InvalidArgument, "message cannot be empty")
	}
	const maxMessageSize = 10 * 1024 // 10KB
	if len(message) > maxMessageSize {
		return status.Errorf(codes.InvalidArgument, "message too large: %d bytes (max %d)", len(message), maxMessageSize)
	}
	return nil
}

// StartSession creates a new session with server-generated UUID
func (app *application) StartSession(ctx context.Context, req *pb.StartSessionRequest) (*pb.StartSessionResponse, error) {
	sessionID := uuid.New().String()
	app.logger.Info("created new session", "session_id", sessionID)

	return &pb.StartSessionResponse{
		SessionId: sessionID,
	}, nil
}

// Implement ChatService interface
func (app *application) Chat(ctx context.Context, req *pb.ChatRequest) (*pb.ChatResponse, error) {
	// Validate input parameters
	if err := validateSessionID(req.SessionId); err != nil {
		app.logger.Warn("invalid session ID", "session_id", req.SessionId, "error", err)
		return nil, err
	}
	
	if err := validateMessage(req.Message); err != nil {
		app.logger.Warn("invalid message", "session_id", req.SessionId, "message_len", len(req.Message), "error", err)
		return nil, err
	}

	app.logger.Info("received chat request",
		"session_id", req.SessionId,
		"model", req.Model,
		"message_len", len(req.Message),
		"message_index", req.MessageIndex)

	// Layer 4: Delta protocol - verify client has correct message count
	currentMessages := app.sessionStore.GetMessages(req.SessionId)
	currentCount := uint32(len(currentMessages))

	// If client's index doesn't match our count, they may be out of sync
	// For now, we'll accept the message anyway, but log the discrepancy
	if req.MessageIndex > 0 && req.MessageIndex != currentCount {
		app.logger.Warn("client message index mismatch",
			"session_id", req.SessionId,
			"client_index", req.MessageIndex,
			"server_count", currentCount)
	}

	// Store user message in session (Layer 2: structured format)
	app.sessionStore.AppendMessage(req.SessionId, User, req.Message)

	// Get LLM provider based on requested model
	provider := app.getProvider(req.Model)
	app.logger.Info("using LLM provider", "provider", provider.Name(), "model", req.Model.String())

	// Get conversation history for LLM
	messages := app.sessionStore.GetMessagesAsLLMFormat(req.SessionId)

	// Generate response using LLM provider
	reply, err := provider.GenerateResponse(ctx, messages)
	if err != nil {
		app.logger.Error("LLM provider error", "error", err, "provider", provider.Name())
		return nil, status.Errorf(codes.Internal, "LLM provider failed: %v", err)
	}

	// Store LLM response in session (Layer 2: structured format)
	app.sessionStore.AppendMessage(req.SessionId, Assistant, reply)

	// Get updated message count after adding both messages
	newCount := currentCount + 2 // Added user message and assistant reply

	resp := &pb.ChatResponse{
		SessionId:    req.SessionId,
		Reply:        reply,
		MessageCount: newCount, // Layer 4: Tell client total message count
	}

	return resp, nil
}

func (app *application) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Ok: true}, nil
}

func (app *application) GetHistory(ctx context.Context, req *pb.GetHistoryRequest) (*pb.GetHistoryResponse, error) {
	// Validate session ID
	if err := validateSessionID(req.SessionId); err != nil {
		app.logger.Warn("invalid session ID in get history", "session_id", req.SessionId, "error", err)
		return nil, err
	}

	app.logger.Info("received get history request", "session_id", req.SessionId)

	messages := app.sessionStore.GetFormattedMessages(req.SessionId)

	resp := &pb.GetHistoryResponse{
		SessionId: req.SessionId,
		Messages:  messages,
	}

	return resp, nil
}
