package main

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	
	"microchat.ai/cmd/server/ratelimit"
)

// RateLimitInterceptor creates a gRPC unary server interceptor for rate limiting
func RateLimitInterceptor(ipLimiter *ratelimit.IPLimiter) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Extract client IP address
		clientIP := extractClientIP(ctx)
		
		// Check rate limit
		if !ipLimiter.Allow(clientIP) {
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