package main

import (
	"context"
	"fmt"

	pb "microchat.ai/proto"
)

// Implement ChatService interface
func (app *application) Chat(ctx context.Context, req *pb.ChatRequest) (*pb.ChatResponse, error) {
	app.logger.Info("received chat request",
		"session_id", req.SessionId,
		"model", req.Model,
		"message_len", len(req.Message))

	// Store user message in session (Layer 1: string format)
	userMsg := fmt.Sprintf("user: %s", req.Message)
	app.sessionStore.AppendMessage(req.SessionId, userMsg)

	// TODO: Replace with actual LLM integration
	reply := req.Message // Echo back for now

	// Store echo response in session (Layer 1: string format)
	echoMsg := fmt.Sprintf("echo: %s", reply)
	app.sessionStore.AppendMessage(req.SessionId, echoMsg)

	resp := &pb.ChatResponse{
		SessionId: req.SessionId,
		Reply:     reply,
	}

	return resp, nil
}

func (app *application) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Ok: true}, nil
}

func (app *application) GetHistory(ctx context.Context, req *pb.GetHistoryRequest) (*pb.GetHistoryResponse, error) {
	app.logger.Info("received get history request", "session_id", req.SessionId)

	messages := app.sessionStore.GetMessages(req.SessionId)

	resp := &pb.GetHistoryResponse{
		SessionId: req.SessionId,
		Messages:  messages,
	}

	return resp, nil
}
