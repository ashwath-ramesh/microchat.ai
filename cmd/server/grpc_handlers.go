package main

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

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

// sanitizeForTerminal removes potentially dangerous control characters
// that could manipulate terminal display or execute commands
func sanitizeForTerminal(text string) string {
	// Remove ANSI escape sequences (could manipulate terminal)
	ansiEscapeRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	text = ansiEscapeRegex.ReplaceAllString(text, "")

	// Remove other control characters except safe ones (newline, tab, carriage return)
	var result strings.Builder
	for _, r := range text {
		// Allow printable characters, newlines, tabs, and carriage returns
		if r >= 32 || r == '\n' || r == '\t' || r == '\r' {
			result.WriteRune(r)
		}
		// Skip other control characters (0-31, 127)
	}

	return result.String()
}

// validateResponse checks if LLM response is safe and reasonable
func validateResponse(response string, sessionID string, logger interface {
	Warn(msg string, args ...interface{})
}) error {
	// Configure max response size (default: 50KB)
	maxResponseSize := 50 * 1024 // 50KB default
	if maxSizeEnv := os.Getenv("MAX_RESPONSE_SIZE_KB"); maxSizeEnv != "" {
		if parsed, err := strconv.Atoi(maxSizeEnv); err == nil && parsed > 0 && parsed <= 1024 {
			maxResponseSize = parsed * 1024 // Convert KB to bytes
		}
	}

	// Check response size
	if len(response) > maxResponseSize {
		logger.Warn("response too large, truncating", "session_id", sessionID,
			"original_size", len(response), "max_size", maxResponseSize)
		return status.Errorf(codes.ResourceExhausted, "response too large: %d bytes (max %d)",
			len(response), maxResponseSize)
	}

	// Log warning for suspiciously large responses (>20% of max size)
	warningThreshold := maxResponseSize / 5 // 20% of max size
	if len(response) > warningThreshold {
		logger.Warn("large response detected", "session_id", sessionID, "size", len(response), "max_size", maxResponseSize)
	}

	return nil
}

// StartSession creates a new session with server-generated UUID
func (app *application) StartSession(ctx context.Context, req *pb.StartSessionRequest) (*pb.StartSessionResponse, error) {
	start := time.Now()
	defer func() {
		recordRequestDuration("StartSession", time.Since(start).Seconds())
	}()

	sessionID := uuid.New().String()

	// Register the session ID as valid
	app.sessionStore.RegisterSession(sessionID)

	// Update metrics
	incrementSessionsCreated()
	updateActiveSessions(app.sessionStore.GetSessionCount())

	app.logger.Info("created new session", "session_id", sessionID)

	return &pb.StartSessionResponse{
		SessionId: sessionID,
	}, nil
}

// Implement ChatService interface
func (app *application) Chat(ctx context.Context, req *pb.ChatRequest) (*pb.ChatResponse, error) {
	start := time.Now()
	defer func() {
		recordRequestDuration("Chat", time.Since(start).Seconds())
	}()

	recordRequestSize("Chat", len(req.Message))
	// Validate input parameters
	if err := validateSessionID(req.SessionId); err != nil {
		incrementGRPCError("Chat", "InvalidArgument")
		app.logger.Warn("invalid session ID", "session_id", req.SessionId, "error", err)
		return nil, err
	}

	if err := validateMessage(req.Message); err != nil {
		incrementGRPCError("Chat", "InvalidArgument")
		app.logger.Warn("invalid message", "session_id", req.SessionId, "message_len", len(req.Message), "error", err)
		return nil, err
	}

	// Check if session ID is valid (was created via StartSession)
	if !app.sessionStore.IsValidSession(req.SessionId) {
		incrementGRPCError("Chat", "NotFound")
		app.logger.Warn("invalid session ID", "session_id", req.SessionId, "error", "session not created via StartSession")
		return nil, status.Error(codes.NotFound, "session not found or not properly created")
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
	if err := app.sessionStore.AppendMessage(req.SessionId, User, req.Message); err != nil {
		app.logger.Warn("failed to append user message", "session_id", req.SessionId, "error", err)
		return nil, status.Errorf(codes.ResourceExhausted, "failed to store message: %v", err)
	}

	// Get LLM provider based on requested model
	provider := app.getProvider(req.Model)
	app.logger.Info("using LLM provider", "provider", provider.Name(), "model", req.Model.String())

	// Get conversation history for LLM
	messages := app.sessionStore.GetMessagesAsLLMFormat(req.SessionId)

	// Generate response using LLM provider
	llmStart := time.Now()
	reply, err := provider.GenerateResponse(ctx, messages)
	recordLLMCallDuration(provider.Name(), time.Since(llmStart).Seconds())
	if err != nil {
		incrementLLMError(provider.Name(), "api_error")
		incrementGRPCError("Chat", "Internal")
		app.logger.Error("LLM provider error", "error", err, "provider", provider.Name())
		return nil, status.Errorf(codes.Internal, "LLM provider failed: %v", err)
	}

	// Validate response size and content
	if err := validateResponse(reply, req.SessionId, app.logger); err != nil {
		incrementGRPCError("Chat", "ResourceExhausted")
		return nil, err
	}

	// Sanitize response for terminal safety
	sanitizedReply := sanitizeForTerminal(reply)
	if len(sanitizedReply) != len(reply) {
		app.logger.Warn("sanitized response contained control characters",
			"session_id", req.SessionId, "original_len", len(reply), "sanitized_len", len(sanitizedReply))
	}
	reply = sanitizedReply

	// Store sanitized LLM response in session (Layer 2: structured format)
	if err := app.sessionStore.AppendMessage(req.SessionId, Assistant, reply); err != nil {
		app.logger.Warn("failed to append assistant message", "session_id", req.SessionId, "error", err)
		return nil, status.Errorf(codes.ResourceExhausted, "failed to store response: %v", err)
	}

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
