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
		name         string
		setupCtx     func() context.Context
		expectedIP   string
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