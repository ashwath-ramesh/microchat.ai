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
		"message_len", len(req.Message))

	// TODO: Replace with actual LLM integration
	reply := req.Message // Echo back for now

	resp := &pb.ChatResponse{
		SessionId: req.SessionId,
		Reply:     reply,
	}

	return resp, nil
}

func (app *application) Health(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Ok: true}, nil
}
