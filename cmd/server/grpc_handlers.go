package main

import (
	"context"

	pb "microchat.ai/proto"
)

// Implement ChatService interface
func (app *application) Chat(ctx context.Context, req *pb.ChatRequest) (*pb.ChatResponse, error) {
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

	// TODO: Replace with actual LLM integration
	// When connected to LLM, we'll send full history from currentMessages
	reply := req.Message // Echo back for now

	// Store echo response in session (Layer 2: structured format)
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
	app.logger.Info("received get history request", "session_id", req.SessionId)

	messages := app.sessionStore.GetFormattedMessages(req.SessionId)

	resp := &pb.GetHistoryResponse{
		SessionId: req.SessionId,
		Messages:  messages,
	}

	return resp, nil
}
