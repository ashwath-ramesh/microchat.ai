package main

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "microchat_request_duration_seconds",
			Help:    "Duration of gRPC requests in seconds",
			Buckets: []float64{0.001, 0.01, 0.1, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"method"},
	)

	llmCallDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "microchat_llm_call_duration_seconds",
			Help:    "Duration of LLM provider calls in seconds",
			Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 20.0, 30.0},
		},
		[]string{"provider"},
	)

	activeSessions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "microchat_active_sessions",
			Help: "Number of currently active sessions",
		},
	)

	sessionsCreatedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "microchat_sessions_created_total",
			Help: "Total number of sessions created",
		},
	)

	rateLimitExceededTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "microchat_rate_limit_exceeded_total",
			Help: "Total number of rate limit exceeded responses",
		},
	)

	requestBytes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "microchat_request_bytes",
			Help:    "Size of request payloads in bytes",
			Buckets: []float64{100, 500, 1000, 5000, 10000, 50000},
		},
		[]string{"method"},
	)

	// Business metrics - API usage
	apiKeysTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "microchat_api_keys_total",
			Help: "Total number of configured API keys",
		},
	)

	apiCallsToday = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "microchat_api_calls_today",
			Help: "Number of API calls made today by key",
		},
		[]string{"key_hash"},
	)

	apiKeysOverLimit = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "microchat_api_keys_over_limit",
			Help: "Number of API keys that have exceeded their daily limit",
		},
	)

	dailyCallLimit = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "microchat_daily_call_limit",
			Help: "Configured daily call limit per API key",
		},
	)

	// Session memory tracking (aggregate only - no per-session labels to avoid unbounded cardinality)

	totalSessionMemoryBytes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "microchat_total_session_memory_bytes",
			Help: "Total memory usage across all sessions in bytes",
		},
	)

	// Error tracking
	grpcErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "microchat_grpc_errors_total",
			Help: "Total number of gRPC errors by method and code",
		},
		[]string{"method", "grpc_code"},
	)

	llmErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "microchat_llm_errors_total",
			Help: "Total number of LLM provider errors",
		},
		[]string{"provider", "error_type"},
	)

	// Server configuration info metrics
	serverConfigInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "microchat_server_config_info",
			Help: "Server configuration information as labels",
		},
		[]string{"max_sessions", "max_messages_per_session", "max_session_size_kb", "rate_limit_rps", "rate_limit_burst"},
	)

	serverStartTime = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "microchat_server_start_time_seconds",
			Help: "Unix timestamp when the server started",
		},
	)
)

func updateActiveSessions(count int) {
	activeSessions.Set(float64(count))
}

func incrementSessionsCreated() {
	sessionsCreatedTotal.Inc()
}

func recordRequestDuration(method string, seconds float64) {
	requestDuration.WithLabelValues(method).Observe(seconds)
}

func recordLLMCallDuration(provider string, seconds float64) {
	llmCallDuration.WithLabelValues(provider).Observe(seconds)
}

func incrementRateLimitExceeded() {
	rateLimitExceededTotal.Inc()
}

func recordRequestSize(method string, bytes int) {
	requestBytes.WithLabelValues(method).Observe(float64(bytes))
}

// Business metrics functions
func updateAPIKeyMetrics(totalKeys int, usage map[string]int, limit int, keysOverLimit int) {
	apiKeysTotal.Set(float64(totalKeys))
	dailyCallLimit.Set(float64(limit))
	apiKeysOverLimit.Set(float64(keysOverLimit))

	// Update per-key usage (using hash of key for privacy)
	for keyHash, calls := range usage {
		apiCallsToday.WithLabelValues(keyHash).Set(float64(calls))
	}
}

// recordSessionMemory removed - per-session tracking causes unbounded cardinality memory leak
// Use totalSessionMemoryBytes gauge for aggregate monitoring instead

func updateTotalSessionMemory(bytes int) {
	totalSessionMemoryBytes.Set(float64(bytes))
}

func incrementGRPCError(method string, grpcCode string) {
	grpcErrors.WithLabelValues(method, grpcCode).Inc()
}

func incrementLLMError(provider string, errorType string) {
	llmErrors.WithLabelValues(provider, errorType).Inc()
}

// hashAPIKey creates a privacy-preserving hash of an API key for metrics
func hashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes for short hash
}

// updateBusinessMetrics collects and updates all business metrics
func updateBusinessMetrics(app *application) {
	// Update session metrics
	updateActiveSessions(app.sessionStore.GetSessionCount())

	// Update API key metrics
	app.spendingTracker.mu.RLock()
	totalKeys := len(app.config.apiKeys)
	keysOverLimit := 0
	usage := make(map[string]int)

	for key, usageData := range app.spendingTracker.usage {
		keyHash := hashAPIKey(key)
		usage[keyHash] = usageData.calls
		if usageData.calls >= app.spendingTracker.limit {
			keysOverLimit++
		}
	}
	app.spendingTracker.mu.RUnlock()

	updateAPIKeyMetrics(totalKeys, usage, app.spendingTracker.limit, keysOverLimit)

	// Update session memory metrics (aggregate only - no per-session tracking)
	sessionsInfo := app.sessionStore.GetAllSessionsInfo()
	totalMemory := 0
	for _, info := range sessionsInfo {
		totalMemory += info.SizeBytes
	}
	updateTotalSessionMemory(totalMemory)
}

// initializeServerMetrics sets up one-time server configuration metrics
func initializeServerMetrics(cfg config) {
	// Set server start time
	serverStartTime.Set(float64(time.Now().Unix()))

	// Set server configuration as labels
	serverConfigInfo.WithLabelValues(
		fmt.Sprintf("%d", cfg.maxSessions),
		fmt.Sprintf("%d", cfg.maxMessagesPerSession),
		fmt.Sprintf("%d", cfg.maxSessionSizeBytes/1024),
		fmt.Sprintf("%.1f", float64(cfg.rateLimitRPS)),
		fmt.Sprintf("%d", cfg.rateLimitBurst),
	).Set(1)
}

// startMetricsUpdater starts a goroutine that periodically updates business metrics
func startMetricsUpdater(app *application) {
	// Initialize configuration metrics once
	initializeServerMetrics(app.config)

	go func() {
		ticker := time.NewTicker(30 * time.Second) // Update every 30 seconds
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				updateBusinessMetrics(app)
			}
		}
	}()
}
