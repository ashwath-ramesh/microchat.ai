package ratelimit

import (
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IPLimiter manages rate limiters for different IP addresses
type IPLimiter struct {
	limiters map[string]*limitEntry
	mu       sync.RWMutex
	rps      rate.Limit
	burst    int
	// Cleanup configuration
	cleanupInterval time.Duration
	expiry          time.Duration
	stopCleanup     chan bool
}

type limitEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPLimiter creates a new IP-based rate limiter
func NewIPLimiter(rps rate.Limit, burst int) *IPLimiter {
	il := &IPLimiter{
		limiters:        make(map[string]*limitEntry),
		rps:            rps,
		burst:          burst,
		cleanupInterval: 10 * time.Minute, // Check every 10 minutes
		expiry:         24 * time.Hour,    // Remove entries not seen for 24 hours
		stopCleanup:    make(chan bool),
	}
	
	// Start cleanup goroutine
	go il.cleanupWorker()
	
	return il
}

// Allow checks if a request from the given IP is allowed
func (il *IPLimiter) Allow(ip string) bool {
	il.mu.Lock()
	defer il.mu.Unlock()
	
	entry, exists := il.limiters[ip]
	if !exists {
		// Create new limiter for this IP
		entry = &limitEntry{
			limiter:  rate.NewLimiter(il.rps, il.burst),
			lastSeen: time.Now(),
		}
		il.limiters[ip] = entry
	} else {
		// Update last seen time
		entry.lastSeen = time.Now()
	}
	
	return entry.limiter.Allow()
}

// cleanupWorker periodically removes stale limiters to prevent memory leaks
func (il *IPLimiter) cleanupWorker() {
	ticker := time.NewTicker(il.cleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			il.cleanup()
		case <-il.stopCleanup:
			return
		}
	}
}

// cleanup removes entries that haven't been seen for the expiry duration
func (il *IPLimiter) cleanup() {
	il.mu.Lock()
	defer il.mu.Unlock()
	
	now := time.Now()
	for ip, entry := range il.limiters {
		if now.Sub(entry.lastSeen) > il.expiry {
			delete(il.limiters, ip)
		}
	}
}

// Stop gracefully stops the cleanup worker
func (il *IPLimiter) Stop() {
	close(il.stopCleanup)
}

// GetActiveCount returns the number of active limiters (for testing/monitoring)
func (il *IPLimiter) GetActiveCount() int {
	il.mu.RLock()
	defer il.mu.RUnlock()
	return len(il.limiters)
}

// ExtractIP extracts the real client IP from various sources
func ExtractIP(remoteAddr string, forwardedFor string) string {
	// First try X-Forwarded-For header (handles proxies/load balancers)
	if forwardedFor != "" {
		// X-Forwarded-For can contain multiple IPs: "client, proxy1, proxy2"
		// The first IP is typically the original client
		ips := strings.Split(forwardedFor, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	
	// Fall back to remote address
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// If we can't split host:port, assume it's just an IP
		return remoteAddr
	}
	return host
}