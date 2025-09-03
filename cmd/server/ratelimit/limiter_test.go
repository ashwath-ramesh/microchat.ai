package ratelimit

import (
	"testing"
	"time"
)

func TestNewIPLimiter(t *testing.T) {
	limiter := NewIPLimiter(10, 20)
	defer limiter.Stop()

	if limiter.rps != 10 {
		t.Errorf("expected RPS to be 10, got %v", limiter.rps)
	}
	if limiter.burst != 20 {
		t.Errorf("expected burst to be 20, got %d", limiter.burst)
	}
	if len(limiter.limiters) != 0 {
		t.Errorf("expected empty limiters map, got %d entries", len(limiter.limiters))
	}
}

func TestIPLimiterAllow(t *testing.T) {
	// Create a limiter with 2 RPS, burst of 3
	limiter := NewIPLimiter(2, 3)
	defer limiter.Stop()

	ip := "192.168.1.1"

	// Should allow first 3 requests (burst capacity)
	for i := 0; i < 3; i++ {
		if !limiter.Allow(ip) {
			t.Errorf("expected request %d to be allowed", i+1)
		}
	}

	// 4th request should be denied (burst capacity exhausted)
	if limiter.Allow(ip) {
		t.Error("expected 4th request to be denied")
	}

	// Wait for tokens to replenish (at 2 RPS, wait 1 second for 2 tokens)
	time.Sleep(1100 * time.Millisecond)

	// Should allow 2 more requests
	for i := 0; i < 2; i++ {
		if !limiter.Allow(ip) {
			t.Errorf("expected request after wait to be allowed")
		}
	}

	// Next request should be denied
	if limiter.Allow(ip) {
		t.Error("expected request after burst to be denied")
	}
}

func TestIPLimiterMultipleIPs(t *testing.T) {
	limiter := NewIPLimiter(1, 2)
	defer limiter.Stop()

	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Each IP should have its own limit
	if !limiter.Allow(ip1) {
		t.Error("expected first request from IP1 to be allowed")
	}
	if !limiter.Allow(ip2) {
		t.Error("expected first request from IP2 to be allowed")
	}

	// Second burst request for each IP
	if !limiter.Allow(ip1) {
		t.Error("expected second request from IP1 to be allowed")
	}
	if !limiter.Allow(ip2) {
		t.Error("expected second request from IP2 to be allowed")
	}

	// Third request should be denied for both
	if limiter.Allow(ip1) {
		t.Error("expected third request from IP1 to be denied")
	}
	if limiter.Allow(ip2) {
		t.Error("expected third request from IP2 to be denied")
	}

	// Check that we have 2 active limiters
	if count := limiter.GetActiveCount(); count != 2 {
		t.Errorf("expected 2 active limiters, got %d", count)
	}
}

func TestIPLimiterCleanup(t *testing.T) {
	limiter := NewIPLimiter(10, 20)
	// Set a very short expiry for testing
	limiter.expiry = 100 * time.Millisecond
	defer limiter.Stop()

	ip := "192.168.1.1"

	// Make a request to create a limiter entry
	limiter.Allow(ip)

	if count := limiter.GetActiveCount(); count != 1 {
		t.Errorf("expected 1 active limiter, got %d", count)
	}

	// Wait for expiry
	time.Sleep(200 * time.Millisecond)

	// Trigger cleanup manually
	limiter.cleanup()

	if count := limiter.GetActiveCount(); count != 0 {
		t.Errorf("expected 0 active limiters after cleanup, got %d", count)
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name         string
		remoteAddr   string
		forwardedFor string
		expected     string
	}{
		{
			name:         "simple IP with port",
			remoteAddr:   "192.168.1.1:54321",
			forwardedFor: "",
			expected:     "192.168.1.1",
		},
		{
			name:         "IP without port",
			remoteAddr:   "10.0.0.1",
			forwardedFor: "",
			expected:     "10.0.0.1",
		},
		{
			name:         "forwarded header single IP",
			remoteAddr:   "10.0.0.1:12345",
			forwardedFor: "203.0.113.1",
			expected:     "203.0.113.1",
		},
		{
			name:         "forwarded header multiple IPs",
			remoteAddr:   "10.0.0.1:12345",
			forwardedFor: "203.0.113.1, 198.51.100.1, 192.0.2.1",
			expected:     "203.0.113.1",
		},
		{
			name:         "forwarded header with spaces",
			remoteAddr:   "10.0.0.1:12345",
			forwardedFor: " 203.0.113.1 ",
			expected:     "203.0.113.1",
		},
		{
			name:         "invalid forwarded header falls back",
			remoteAddr:   "10.0.0.1:12345",
			forwardedFor: "not-an-ip",
			expected:     "10.0.0.1",
		},
		{
			name:         "IPv6 with port",
			remoteAddr:   "[2001:db8::1]:8080",
			forwardedFor: "",
			expected:     "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractIP(tt.remoteAddr, tt.forwardedFor)
			if result != tt.expected {
				t.Errorf("ExtractIP(%q, %q) = %q, want %q",
					tt.remoteAddr, tt.forwardedFor, result, tt.expected)
			}
		})
	}
}

func TestIPLimiterConcurrency(t *testing.T) {
	limiter := NewIPLimiter(100, 200) // High limits for concurrency test
	defer limiter.Stop()

	ip := "192.168.1.1"

	// Run multiple goroutines concurrently
	results := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			results <- limiter.Allow(ip)
		}()
	}

	// Collect results
	allowed := 0
	for i := 0; i < 10; i++ {
		if <-results {
			allowed++
		}
	}

	// All requests should be allowed with high limits
	if allowed != 10 {
		t.Errorf("expected all 10 concurrent requests to be allowed, got %d", allowed)
	}

	// Should have exactly one limiter entry
	if count := limiter.GetActiveCount(); count != 1 {
		t.Errorf("expected 1 active limiter after concurrent access, got %d", count)
	}
}

func TestIPLimiterStop(t *testing.T) {
	limiter := NewIPLimiter(10, 20)

	// Make a request to ensure cleanup worker is running
	limiter.Allow("192.168.1.1")

	// Stop should not hang
	done := make(chan bool)
	go func() {
		limiter.Stop()
		close(done)
	}()

	// Wait for stop with timeout
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Stop() did not complete within 1 second")
	}
}
