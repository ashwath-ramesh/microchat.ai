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
		Error:     pb.ErrorCode_NO_ERROR, // Success case = 0 bytes
	}

	return resp, nil
}

func (app *application) HealthCheck(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	var env pb.Environment
	switch app.config.env {
	case "development":
		env = pb.Environment_DEVELOPMENT
	case "staging":
		env = pb.Environment_STAGING
	case "production":
		env = pb.Environment_PRODUCTION
	default:
		env = pb.Environment_DEVELOPMENT
	}

	return &pb.HealthResponse{
		Status:      pb.HealthStatus_AVAILABLE,
		Environment: env,
		Version:     version,
	}, nil
}
