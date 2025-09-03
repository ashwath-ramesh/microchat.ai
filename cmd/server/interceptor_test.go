package main

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"microchat.ai/cmd/server/ratelimit"
)

// MockSpendingTracker for testing
type MockSpendingTracker struct {
	canMakeCall  bool
	callRecorded bool
}

func (m *MockSpendingTracker) CanMakeCall(apiKey string) bool {
	return m.canMakeCall
}

func (m *MockSpendingTracker) RecordCall(apiKey string) {
	m.callRecorded = true
}

func TestRateLimitInterceptor(t *testing.T) {
	// Create a limiter with very restrictive limits for testing
	ipLimiter := ratelimit.NewIPLimiter(1, 1) // 1 RPS, burst of 1
	defer ipLimiter.Stop()

	interceptor := RateLimitInterceptor(ipLimiter)

	// Mock handler that just returns success
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// Create a context with peer information
	addr, _ := net.ResolveTCPAddr("tcp", "192.168.1.1:54321")
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: addr,
	})

	// First request should succeed
	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err != nil {
		t.Errorf("expected first request to succeed, got error: %v", err)
	}
	if resp != "success" {
		t.Errorf("expected success response, got: %v", resp)
	}

	// Second request should be rate limited
	resp, err = interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err == nil {
		t.Error("expected second request to be rate limited")
	}

	// Check that it's the correct error code
	st, ok := status.FromError(err)
	if !ok {
		t.Error("expected gRPC status error")
	}
	if st.Code() != codes.ResourceExhausted {
		t.Errorf("expected ResourceExhausted code, got: %v", st.Code())
	}
	if st.Message() != "rate limit exceeded" {
		t.Errorf("expected rate limit message, got: %v", st.Message())
	}
}

func TestRateLimitInterceptorDifferentIPs(t *testing.T) {
	ipLimiter := ratelimit.NewIPLimiter(1, 1) // 1 RPS, burst of 1
	defer ipLimiter.Stop()

	interceptor := RateLimitInterceptor(ipLimiter)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// Create contexts for different IPs
	addr1, _ := net.ResolveTCPAddr("tcp", "192.168.1.1:54321")
	ctx1 := peer.NewContext(context.Background(), &peer.Peer{Addr: addr1})

	addr2, _ := net.ResolveTCPAddr("tcp", "192.168.1.2:54321")
	ctx2 := peer.NewContext(context.Background(), &peer.Peer{Addr: addr2})

	// Both IPs should be able to make one request
	_, err1 := interceptor(ctx1, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err1 != nil {
		t.Errorf("expected request from IP1 to succeed, got: %v", err1)
	}

	_, err2 := interceptor(ctx2, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err2 != nil {
		t.Errorf("expected request from IP2 to succeed, got: %v", err2)
	}

	// Second requests from both should be rate limited
	_, err1 = interceptor(ctx1, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err1 == nil {
		t.Error("expected second request from IP1 to be rate limited")
	}

	_, err2 = interceptor(ctx2, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err2 == nil {
		t.Error("expected second request from IP2 to be rate limited")
	}
}

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name       string
		setupCtx   func() context.Context
		expectedIP string
	}{
		{
			name: "basic peer IP",
			setupCtx: func() context.Context {
				addr, _ := net.ResolveTCPAddr("tcp", "192.168.1.1:54321")
				return peer.NewContext(context.Background(), &peer.Peer{Addr: addr})
			},
			expectedIP: "192.168.1.1",
		},
		{
			name: "no peer context",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expectedIP: "unknown",
		},
		{
			name: "with X-Forwarded-For header",
			setupCtx: func() context.Context {
				addr, _ := net.ResolveTCPAddr("tcp", "10.0.0.1:54321")
				ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: addr})
				md := metadata.Pairs("x-forwarded-for", "203.0.113.1")
				return metadata.NewIncomingContext(ctx, md)
			},
			expectedIP: "203.0.113.1",
		},
		{
			name: "with multiple X-Forwarded-For IPs",
			setupCtx: func() context.Context {
				addr, _ := net.ResolveTCPAddr("tcp", "10.0.0.1:54321")
				ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: addr})
				md := metadata.Pairs("x-forwarded-for", "203.0.113.1, 198.51.100.1")
				return metadata.NewIncomingContext(ctx, md)
			},
			expectedIP: "203.0.113.1",
		},
		{
			name: "IPv6 address",
			setupCtx: func() context.Context {
				addr, _ := net.ResolveTCPAddr("tcp6", "[2001:db8::1]:8080")
				return peer.NewContext(context.Background(), &peer.Peer{Addr: addr})
			},
			expectedIP: "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			ip := extractClientIP(ctx)
			if ip != tt.expectedIP {
				t.Errorf("extractClientIP() = %q, want %q", ip, tt.expectedIP)
			}
		})
	}
}

func TestRateLimitInterceptorWithForwardedFor(t *testing.T) {
	ipLimiter := ratelimit.NewIPLimiter(1, 1) // 1 RPS, burst of 1
	defer ipLimiter.Stop()

	interceptor := RateLimitInterceptor(ipLimiter)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// Create context with proxy IP but real client IP in X-Forwarded-For
	addr, _ := net.ResolveTCPAddr("tcp", "10.0.0.1:54321") // Proxy IP
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: addr})
	md := metadata.Pairs("x-forwarded-for", "203.0.113.1") // Real client IP
	ctx = metadata.NewIncomingContext(ctx, md)

	// First request should succeed
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err != nil {
		t.Errorf("expected first request to succeed, got: %v", err)
	}

	// Second request should be rate limited based on the X-Forwarded-For IP
	_, err = interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err == nil {
		t.Error("expected second request to be rate limited")
	}

	// Request from different forwarded IP should succeed
	md2 := metadata.Pairs("x-forwarded-for", "203.0.113.2")
	ctx2 := metadata.NewIncomingContext(peer.NewContext(context.Background(), &peer.Peer{Addr: addr}), md2)

	_, err = interceptor(ctx2, nil, &grpc.UnaryServerInfo{FullMethod: "/test"}, handler)
	if err != nil {
		t.Errorf("expected request from different forwarded IP to succeed, got: %v", err)
	}
}

// Authentication Tests

func TestAuthInterceptor_HealthEndpoint(t *testing.T) {
	// Health endpoint should bypass all auth checks
	apiKeys := map[string]bool{"test-key": true}
	mockTracker := &MockSpendingTracker{canMakeCall: true}
	interceptor := AuthInterceptor(apiKeys, mockTracker)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// No auth header - should still work for health endpoint
	ctx := context.Background()
	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/chat.ChatService/Health"}, handler)

	if err != nil {
		t.Errorf("expected health endpoint to work without auth, got: %v", err)
	}
	if resp != "success" {
		t.Errorf("expected success response, got: %v", resp)
	}
}

func TestAuthInterceptor_MissingAuth(t *testing.T) {
	apiKeys := map[string]bool{"test-key": true}
	mockTracker := &MockSpendingTracker{canMakeCall: true}
	interceptor := AuthInterceptor(apiKeys, mockTracker)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// No metadata at all
	ctx := context.Background()
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/chat.ChatService/Chat"}, handler)

	if err == nil {
		t.Error("expected auth failure for missing metadata")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Error("expected gRPC status error")
	}
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated code, got: %v", st.Code())
	}
	if st.Message() != "missing metadata" {
		t.Errorf("expected missing metadata message, got: %v", st.Message())
	}
}

func TestAuthInterceptor_MissingAuthHeader(t *testing.T) {
	apiKeys := map[string]bool{"test-key": true}
	mockTracker := &MockSpendingTracker{canMakeCall: true}
	interceptor := AuthInterceptor(apiKeys, mockTracker)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// Metadata without authorization header
	md := metadata.Pairs("other-header", "value")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/chat.ChatService/Chat"}, handler)

	if err == nil {
		t.Error("expected auth failure for missing authorization header")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated code, got: %v", st.Code())
	}
	if st.Message() != "missing authorization header" {
		t.Errorf("expected missing authorization header message, got: %v", st.Message())
	}
}

func TestAuthInterceptor_InvalidAuthFormat(t *testing.T) {
	apiKeys := map[string]bool{"test-key": true}
	mockTracker := &MockSpendingTracker{canMakeCall: true}
	interceptor := AuthInterceptor(apiKeys, mockTracker)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// Invalid auth format (not Bearer)
	md := metadata.Pairs("authorization", "Basic dXNlcjpwYXNz")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/chat.ChatService/Chat"}, handler)

	if err == nil {
		t.Error("expected auth failure for invalid format")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated code, got: %v", st.Code())
	}
	if st.Message() != "invalid authorization format" {
		t.Errorf("expected invalid authorization format message, got: %v", st.Message())
	}
}

func TestAuthInterceptor_InvalidAPIKey(t *testing.T) {
	apiKeys := map[string]bool{"valid-key": true}
	mockTracker := &MockSpendingTracker{canMakeCall: true}
	interceptor := AuthInterceptor(apiKeys, mockTracker)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// Invalid API key
	md := metadata.Pairs("authorization", "Bearer invalid-key")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/chat.ChatService/Chat"}, handler)

	if err == nil {
		t.Error("expected auth failure for invalid API key")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated code, got: %v", st.Code())
	}
	if st.Message() != "invalid API key" {
		t.Errorf("expected invalid API key message, got: %v", st.Message())
	}
}

func TestAuthInterceptor_DailyLimitExceeded(t *testing.T) {
	apiKeys := map[string]bool{"test-key": true}
	mockTracker := &MockSpendingTracker{canMakeCall: false} // Over limit
	interceptor := AuthInterceptor(apiKeys, mockTracker)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// Valid API key but over spending limit
	md := metadata.Pairs("authorization", "Bearer test-key")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/chat.ChatService/Chat"}, handler)

	if err == nil {
		t.Error("expected auth failure for daily limit exceeded")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.ResourceExhausted {
		t.Errorf("expected ResourceExhausted code, got: %v", st.Code())
	}
	if st.Message() != "daily call limit exceeded" {
		t.Errorf("expected daily call limit exceeded message, got: %v", st.Message())
	}
}

func TestAuthInterceptor_Success(t *testing.T) {
	apiKeys := map[string]bool{"test-key": true}
	mockTracker := &MockSpendingTracker{canMakeCall: true}
	interceptor := AuthInterceptor(apiKeys, mockTracker)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		// Check that API key was added to context
		if apiKey := ctx.Value("api_key"); apiKey != "test-key" {
			t.Errorf("expected api_key in context to be 'test-key', got: %v", apiKey)
		}
		return "success", nil
	}

	// Valid API key and under limit
	md := metadata.Pairs("authorization", "Bearer test-key")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/chat.ChatService/Chat"}, handler)

	if err != nil {
		t.Errorf("expected successful auth, got: %v", err)
	}
	if resp != "success" {
		t.Errorf("expected success response, got: %v", resp)
	}
	if !mockTracker.callRecorded {
		t.Error("expected call to be recorded in spending tracker")
	}
}

func TestAuthInterceptor_NoAPIKeys(t *testing.T) {
	apiKeys := map[string]bool{} // No keys configured
	mockTracker := &MockSpendingTracker{canMakeCall: true}
	interceptor := AuthInterceptor(apiKeys, mockTracker)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "success", nil
	}

	// No auth header when no API keys configured
	ctx := context.Background()
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/chat.ChatService/Chat"}, handler)

	if err == nil {
		t.Error("expected auth failure when no API keys configured")
	}

	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated code, got: %v", st.Code())
	}
	if st.Message() != "no API keys configured - authentication required" {
		t.Errorf("expected no API keys configured message, got: %v", st.Message())
	}
}

// Spending Tracker Tests

func TestSpendingTracker_NewKey(t *testing.T) {
	tracker := NewSpendingTracker(5) // 5 calls per day

	// New key should be able to make calls
	if !tracker.CanMakeCall("new-key") {
		t.Error("expected new key to be able to make calls")
	}

	// Record a call
	tracker.RecordCall("new-key")

	// Should still be able to make calls (1/5 used)
	if !tracker.CanMakeCall("new-key") {
		t.Error("expected key to still be under limit")
	}
}

func TestSpendingTracker_HitLimit(t *testing.T) {
	tracker := NewSpendingTracker(2) // 2 calls per day
	apiKey := "test-key"

	// Should be able to make calls initially
	if !tracker.CanMakeCall(apiKey) {
		t.Error("expected to be under limit initially")
	}

	// Use up the daily limit
	tracker.RecordCall(apiKey) // Call 1
	if !tracker.CanMakeCall(apiKey) {
		t.Error("expected to be under limit after 1 call")
	}

	tracker.RecordCall(apiKey) // Call 2 - now at limit
	if tracker.CanMakeCall(apiKey) {
		t.Error("expected to be at limit after 2 calls")
	}
}

func TestSpendingTracker_DifferentKeys(t *testing.T) {
	tracker := NewSpendingTracker(1) // 1 call per day

	// Each key should have independent limits
	tracker.RecordCall("key1")
	tracker.RecordCall("key2")

	// Both keys should now be at their limit
	if tracker.CanMakeCall("key1") {
		t.Error("expected key1 to be at limit")
	}
	if tracker.CanMakeCall("key2") {
		t.Error("expected key2 to be at limit")
	}

	// New key should still work
	if !tracker.CanMakeCall("key3") {
		t.Error("expected key3 to be under limit")
	}
}
