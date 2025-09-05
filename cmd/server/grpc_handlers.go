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

	// Register the session ID as valid
	app.sessionStore.RegisterSession(sessionID)

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

	// Check if session ID is valid (was created via StartSession)
	if !app.sessionStore.IsValidSession(req.SessionId) {
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
	reply, err := provider.GenerateResponse(ctx, messages)
	if err != nil {
		app.logger.Error("LLM provider error", "error", err, "provider", provider.Name())
		return nil, status.Errorf(codes.Internal, "LLM provider failed: %v", err)
	}

	// Store LLM response in session (Layer 2: structured format)
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

func (app *application) GetMetrics(ctx context.Context, req *pb.MetricsRequest) (*pb.MetricsResponse, error) {
	app.logger.Info("received get metrics request")

	// Get session metrics
	activeSessions := int32(app.sessionStore.GetSessionCount())
	totalSessions := app.sessionStore.GetTotalSessionsCreated()

	// Get detailed session info
	sessionsInfo := app.sessionStore.GetAllSessionsInfo()
	pbSessions := make([]*pb.SessionInfo, len(sessionsInfo))
	for i, info := range sessionsInfo {
		pbSessions[i] = &pb.SessionInfo{
			SessionId:    info.ID,
			MessageCount: int32(info.MessageCount),
			SizeBytes:    int32(info.SizeBytes),
			LastActive:   info.LastActive,
		}
	}

	// Get aggregated API usage stats
	app.spendingTracker.mu.RLock()
	totalApiKeys := int32(len(app.config.apiKeys))
	totalCallsToday := int32(0)
	keysOverLimit := int32(0)

	for _, usage := range app.spendingTracker.usage {
		totalCallsToday += int32(usage.calls)
		if usage.calls >= app.spendingTracker.limit {
			keysOverLimit++
		}
	}

	var averageCallsPerKey int32 = 0
	if totalApiKeys > 0 {
		averageCallsPerKey = totalCallsToday / totalApiKeys
	}

	apiUsageStats := &pb.ApiUsageStats{
		TotalApiKeys:       totalApiKeys,
		TotalCallsToday:    totalCallsToday,
		AverageCallsPerKey: averageCallsPerKey,
		KeysOverLimit:      keysOverLimit,
		DailyLimit:         int32(app.spendingTracker.limit),
	}
	app.spendingTracker.mu.RUnlock()

	// Get server configuration limits
	serverLimits := &pb.ServerLimits{
		SessionCleanupInterval: app.config.sessionCleanupInterval.String(),
		SessionIdleTimeout:     app.config.sessionIdleTimeout.String(),
		MaxSessions:            int32(app.config.maxSessions),
		MaxMessagesPerSession:  int32(app.config.maxMessagesPerSession),
		MaxSessionSizeKb:       int32(app.config.maxSessionSizeBytes / 1024),
		RateLimitRps:           float32(app.config.rateLimitRPS),
		RateLimitBurst:         int32(app.config.rateLimitBurst),
		DailyCallLimit:         int32(app.config.dailyCallLimit),
	}

	resp := &pb.MetricsResponse{
		ActiveSessions:       activeSessions,
		TotalSessionsCreated: totalSessions,
		Sessions:             pbSessions,
		ApiUsageStats:        apiUsageStats,
		ServerLimits:         serverLimits,
	}

	return resp, nil
}
