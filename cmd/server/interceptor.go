package main

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"microchat.ai/cmd/server/ratelimit"
)

// SpendingLimiter interface for dependency injection
type SpendingLimiter interface {
	CanMakeCall(apiKey string) bool
	RecordCall(apiKey string)
}

// AuthInterceptor creates a gRPC unary server interceptor for API key authentication
func AuthInterceptor(apiKeys map[string]string, spendingTracker SpendingLimiter) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip auth for Health endpoint only
		if info.FullMethod == "/chat.ChatService/Health" {
			return handler(ctx, req)
		}

		// Require authentication for all other endpoints
		if len(apiKeys) == 0 {
			return nil, status.Error(codes.Unauthenticated, "no API keys configured - authentication required")
		}

		// Extract authorization header from metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		auth := md.Get("authorization")
		if len(auth) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}

		// Check Bearer token format
		token := auth[0]
		if !strings.HasPrefix(token, "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "invalid authorization format")
		}

		// Extract and validate API key
		apiKey := strings.TrimPrefix(token, "Bearer ")
		role, exists := apiKeys[apiKey]
		if !exists {
			return nil, status.Error(codes.Unauthenticated, "invalid API key")
		}

		// Check if admin endpoint requires admin role
		if info.FullMethod == "/chat.ChatService/GetMetrics" && role != "admin" {
			return nil, status.Error(codes.PermissionDenied, "admin access required")
		}

		// Check daily spending limit
		if !spendingTracker.CanMakeCall(apiKey) {
			return nil, status.Error(codes.ResourceExhausted, "daily call limit exceeded")
		}

		// Record this call
		spendingTracker.RecordCall(apiKey)

		// Add API key and role to context
		ctx = context.WithValue(ctx, "api_key", apiKey)
		ctx = context.WithValue(ctx, "user_role", role)

		// Continue with the request
		return handler(ctx, req)
	}
}

// RateLimitInterceptor creates a gRPC unary server interceptor for rate limiting
func RateLimitInterceptor(ipLimiter *ratelimit.IPLimiter) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Use API key for rate limiting (auth interceptor runs first)
		var limitKey string
		if apiKey := ctx.Value("api_key"); apiKey != nil {
			limitKey = "api_key:" + apiKey.(string)
		} else {
			// This should only happen for Health endpoint
			limitKey = "ip:" + extractClientIP(ctx)
		}

		// Check rate limit using the appropriate key
		if !ipLimiter.Allow(limitKey) {
			incrementRateLimitExceeded()
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}

		// Continue with the request
		return handler(ctx, req)
	}
}

// extractClientIP extracts the client IP from the gRPC context
func extractClientIP(ctx context.Context) string {
	// Default fallback IP
	defaultIP := "unknown"

	// First, try to get the peer information
	p, ok := peer.FromContext(ctx)
	if !ok {
		return defaultIP
	}

	remoteAddr := p.Addr.String()

	// Check for X-Forwarded-For header in metadata (for proxy situations)
	var forwardedFor string
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if xff := md.Get("x-forwarded-for"); len(xff) > 0 {
			forwardedFor = xff[0]
		}
	}

	// Use the ratelimit package's IP extraction logic
	return ratelimit.ExtractIP(remoteAddr, forwardedFor)
}
