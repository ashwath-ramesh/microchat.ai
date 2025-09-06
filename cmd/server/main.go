package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/reflection"

	"microchat.ai/cmd/server/llm"
	"microchat.ai/cmd/server/ratelimit"
	pb "microchat.ai/proto"
)

type config struct {
	port                   int
	env                    string
	sessionCleanupInterval time.Duration
	sessionIdleTimeout     time.Duration
	rateLimitRPS           rate.Limit
	rateLimitBurst         int
	apiKeys                map[string]string // API keys for authentication (key -> role)
	dailyCallLimit         int               // Daily call limit per API key
	maxSessions            int               // Maximum number of concurrent sessions
	maxMessagesPerSession  int               // Maximum messages per session
	maxSessionSizeBytes    int               // Maximum memory per session in bytes
	pprofPort              int               // Port for pprof profiling server (localhost only)
	metricsPort            int               // Port for Prometheus metrics server (network accessible)
}

// SpendingTracker tracks daily usage per API key
type SpendingTracker struct {
	mu    sync.RWMutex
	usage map[string]keyUsage // API key -> usage data
	limit int                 // Daily call limit
}

type keyUsage struct {
	date  string // YYYY-MM-DD format
	calls int    // Number of calls today
}

type application struct {
	config          config
	logger          *slog.Logger
	sessionStore    *SessionStore
	ipLimiter       *ratelimit.IPLimiter
	spendingTracker *SpendingTracker
	providerFactory func(pb.Model, *slog.Logger) llm.Provider // For dependency injection in tests
	pb.UnimplementedChatServiceServer
}

// getProvider returns the appropriate LLM provider for the requested model
func (app *application) getProvider(model pb.Model) llm.Provider {
	if app.providerFactory != nil {
		return app.providerFactory(model, app.logger)
	}
	return llm.NewProvider(model, app.logger)
}

// NewSpendingTracker creates a new spending tracker
func NewSpendingTracker(dailyLimit int) *SpendingTracker {
	return &SpendingTracker{
		usage: make(map[string]keyUsage),
		limit: dailyLimit,
	}
}

// CanMakeCall checks if API key can make another call today
func (st *SpendingTracker) CanMakeCall(apiKey string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	usage, exists := st.usage[apiKey]

	if !exists || usage.date != today {
		// New day or new key - can make call
		return true
	}

	return usage.calls < st.limit
}

// RecordCall records a call for an API key
func (st *SpendingTracker) RecordCall(apiKey string) {
	st.mu.Lock()
	defer st.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	usage, exists := st.usage[apiKey]

	if !exists || usage.date != today {
		// New day or new key - reset usage
		st.usage[apiKey] = keyUsage{date: today, calls: 1}
		return
	}

	// Increment call count
	usage.calls++
	st.usage[apiKey] = usage
}

// loadConfig loads configuration from environment variables
func loadConfig(logger *slog.Logger) (config, error) {
	cfg := config{}

	// Load .env file - check current directory first, then project root
	if err := godotenv.Load(".env"); err != nil {
		if err := godotenv.Load("../../.env"); err != nil {
			logger.Warn("no .env file found, using environment variables only")
		}
	}

	// Parse port (required)
	portStr := os.Getenv("PORT")
	if portStr == "" {
		logger.Error("PORT environment variable is required")
		return cfg, fmt.Errorf("PORT environment variable is required")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		logger.Error("invalid PORT value", "value", portStr, "error", err)
		return cfg, fmt.Errorf("invalid PORT: %w", err)
	}
	cfg.port = port

	// Get environment (required)
	cfg.env = os.Getenv("APP_ENV")
	if cfg.env == "" {
		logger.Error("APP_ENV environment variable is required")
		return cfg, fmt.Errorf("APP_ENV environment variable is required")
	}

	// Parse session cleanup interval (required)
	cleanupStr := os.Getenv("SESSION_CLEANUP_INTERVAL")
	if cleanupStr == "" {
		logger.Error("SESSION_CLEANUP_INTERVAL environment variable is required")
		return cfg, fmt.Errorf("SESSION_CLEANUP_INTERVAL environment variable is required")
	}
	interval, err := time.ParseDuration(cleanupStr)
	if err != nil {
		logger.Error("invalid SESSION_CLEANUP_INTERVAL value", "value", cleanupStr, "error", err)
		return cfg, fmt.Errorf("invalid SESSION_CLEANUP_INTERVAL: %w", err)
	}
	cfg.sessionCleanupInterval = interval

	// Parse session idle timeout (required)
	timeoutStr := os.Getenv("SESSION_IDLE_TIMEOUT")
	if timeoutStr == "" {
		logger.Error("SESSION_IDLE_TIMEOUT environment variable is required")
		return cfg, fmt.Errorf("SESSION_IDLE_TIMEOUT environment variable is required")
	}
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		logger.Error("invalid SESSION_IDLE_TIMEOUT value", "value", timeoutStr, "error", err)
		return cfg, fmt.Errorf("invalid SESSION_IDLE_TIMEOUT: %w", err)
	}
	cfg.sessionIdleTimeout = timeout

	// Parse rate limiting configuration
	rpsStr := os.Getenv("RATE_LIMIT_RPS")
	if rpsStr == "" {
		rpsStr = "10" // Default to 10 RPS
	}
	rpsFloat, err := strconv.ParseFloat(rpsStr, 64)
	if err != nil || rpsFloat <= 0 {
		logger.Error("invalid RATE_LIMIT_RPS value", "value", rpsStr, "error", err)
		return cfg, fmt.Errorf("invalid RATE_LIMIT_RPS: %w", err)
	}
	cfg.rateLimitRPS = rate.Limit(rpsFloat)

	burstStr := os.Getenv("RATE_LIMIT_BURST")
	if burstStr == "" {
		burstStr = "20" // Default to 20 burst
	}
	burstInt, err := strconv.Atoi(burstStr)
	if err != nil || burstInt <= 0 {
		logger.Error("invalid RATE_LIMIT_BURST value", "value", burstStr, "error", err)
		return cfg, fmt.Errorf("invalid RATE_LIMIT_BURST: %w", err)
	}
	cfg.rateLimitBurst = burstInt

	// Parse API keys (comma-separated, with optional :admin suffix)
	apiKeysStr := os.Getenv("API_KEYS")
	cfg.apiKeys = make(map[string]string)
	if apiKeysStr != "" {
		keys := strings.Split(apiKeysStr, ",")
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key != "" {
				// Check for admin role suffix
				if strings.HasSuffix(key, ":admin") {
					keyPart := strings.TrimSuffix(key, ":admin")
					cfg.apiKeys[keyPart] = "admin"
				} else {
					cfg.apiKeys[key] = "user"
				}
			}
		}
	}

	// Parse daily call limit (with default)
	limitStr := os.Getenv("DAILY_CALL_LIMIT")
	if limitStr == "" {
		limitStr = "100" // Default to 100 calls per day
	}
	limitInt, err := strconv.Atoi(limitStr)
	if err != nil || limitInt <= 0 {
		logger.Error("invalid DAILY_CALL_LIMIT value", "value", limitStr, "error", err)
		return cfg, fmt.Errorf("invalid DAILY_CALL_LIMIT: %w", err)
	}
	cfg.dailyCallLimit = limitInt

	// Parse session limits (with defaults)
	maxSessionsStr := os.Getenv("MAX_SESSIONS")
	if maxSessionsStr == "" {
		maxSessionsStr = "1000" // Default to 1000 sessions
	}
	maxSessionsInt, err := strconv.Atoi(maxSessionsStr)
	if err != nil || maxSessionsInt <= 0 {
		logger.Error("invalid MAX_SESSIONS value", "value", maxSessionsStr, "error", err)
		return cfg, fmt.Errorf("invalid MAX_SESSIONS: %w", err)
	}
	cfg.maxSessions = maxSessionsInt

	maxMessagesStr := os.Getenv("MAX_MESSAGES_PER_SESSION")
	if maxMessagesStr == "" {
		maxMessagesStr = "100" // Default to 100 messages per session
	}
	maxMessagesInt, err := strconv.Atoi(maxMessagesStr)
	if err != nil || maxMessagesInt <= 0 {
		logger.Error("invalid MAX_MESSAGES_PER_SESSION value", "value", maxMessagesStr, "error", err)
		return cfg, fmt.Errorf("invalid MAX_MESSAGES_PER_SESSION: %w", err)
	}
	cfg.maxMessagesPerSession = maxMessagesInt

	maxSizeStr := os.Getenv("MAX_SESSION_SIZE_KB")
	if maxSizeStr == "" {
		maxSizeStr = "100" // Default to 100KB per session
	}
	maxSizeInt, err := strconv.Atoi(maxSizeStr)
	if err != nil || maxSizeInt <= 0 {
		logger.Error("invalid MAX_SESSION_SIZE_KB value", "value", maxSizeStr, "error", err)
		return cfg, fmt.Errorf("invalid MAX_SESSION_SIZE_KB: %w", err)
	}
	cfg.maxSessionSizeBytes = maxSizeInt * 1024 // Convert KB to bytes

	// Parse pprof port (with default)
	pprofPortStr := os.Getenv("PPROF_PORT")
	if pprofPortStr == "" {
		pprofPortStr = "6060" // Default to 6060
	}
	pprofPortInt, err := strconv.Atoi(pprofPortStr)
	if err != nil || pprofPortInt <= 0 || pprofPortInt > 65535 {
		logger.Error("invalid PPROF_PORT value", "value", pprofPortStr, "error", err)
		return cfg, fmt.Errorf("invalid PPROF_PORT: %w", err)
	}
	cfg.pprofPort = pprofPortInt

	// Parse metrics port (with default)
	metricsPortStr := os.Getenv("METRICS_PORT")
	if metricsPortStr == "" {
		metricsPortStr = "9090" // Default to 9090 (standard Prometheus port)
	}
	metricsPortInt, err := strconv.Atoi(metricsPortStr)
	if err != nil || metricsPortInt <= 0 || metricsPortInt > 65535 {
		logger.Error("invalid METRICS_PORT value", "value", metricsPortStr, "error", err)
		return cfg, fmt.Errorf("invalid METRICS_PORT: %w", err)
	}
	cfg.metricsPort = metricsPortInt

	return cfg, nil
}

// adminAuthWrapper wraps HTTP handlers with admin authentication
func adminAuthWrapper(next http.HandlerFunc, apiKeys map[string]string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract Bearer token from Authorization header
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}
		
		// Check Bearer token format
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(auth, bearerPrefix) {
			http.Error(w, "Authorization must use Bearer token", http.StatusUnauthorized)
			return
		}
		
		// Extract and validate API key
		apiKey := strings.TrimPrefix(auth, bearerPrefix)
		role, exists := apiKeys[apiKey]
		if !exists || role != "admin" {
			http.Error(w, "Admin access required", http.StatusForbidden)
			return
		}
		
		// Admin authenticated - proceed
		next(w, r)
	})
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := loadConfig(logger)
	if err != nil {
		os.Exit(1)
	}

	app := &application{
		config:          cfg,
		logger:          logger,
		sessionStore:    NewSessionStore(cfg.sessionIdleTimeout, cfg.maxSessions, cfg.maxMessagesPerSession, cfg.maxSessionSizeBytes),
		ipLimiter:       ratelimit.NewIPLimiter(cfg.rateLimitRPS, cfg.rateLimitBurst),
		spendingTracker: NewSpendingTracker(cfg.dailyCallLimit),
	}

	// create gRPC server with compression and TLS
	certFile := os.Getenv("TLS_CERT_FILE")
	if certFile == "" {
		certFile = "certs/server.crt"
	}
	keyFile := os.Getenv("TLS_KEY_FILE")
	if keyFile == "" {
		keyFile = "certs/server.key"
	}

	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		logger.Error("failed to load TLS credentials", "error", err)
		os.Exit(1)
	}

	// Create gRPC server with auth and rate limiting interceptors
	s := grpc.NewServer(
		grpc.Creds(creds),
		grpc.ChainUnaryInterceptor(
			AuthInterceptor(cfg.apiKeys, app.spendingTracker),
			RateLimitInterceptor(app.ipLimiter),
		),
	)

	// register service
	pb.RegisterChatServiceServer(s, app)

	// Enable reflection in development only
	if cfg.env == "development" {
		reflection.Register(s)
	}

	// Listen on TCP
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.port))
	if err != nil {
		logger.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	// Start cleanup goroutine for session management
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(cfg.sessionCleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				app.sessionStore.CleanupIdleSessions()
			case <-done:
				return
			}
		}
	}()

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start pprof HTTP server for profiling with admin authentication (localhost only)
	pprofAddr := fmt.Sprintf("127.0.0.1:%d", cfg.pprofPort)
	pprofMux := http.NewServeMux()
	
	// Register single pprof handler - DefaultServeMux handles all sub-routes
	pprofMux.Handle("/debug/pprof/", adminAuthWrapper(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.DefaultServeMux.ServeHTTP(w, r)
	}), cfg.apiKeys))
	
	pprofServer := &http.Server{
		Addr:    pprofAddr,
		Handler: pprofMux,
	}
	
	go func() {
		logger.Info("starting pprof server", "addr", pprofAddr)
		if err := pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("failed to serve pprof", "error", err)
		}
	}()

	// Start separate Prometheus metrics HTTP server (network accessible)
	metricsAddr := fmt.Sprintf(":%d", cfg.metricsPort)
	metricsMux := http.NewServeMux()
	
	// Register Prometheus metrics endpoint with admin authentication
	metricsMux.Handle("/metrics", adminAuthWrapper(promhttp.Handler().ServeHTTP, cfg.apiKeys))
	
	metricsServer := &http.Server{
		Addr:    metricsAddr,
		Handler: metricsMux,
	}
	
	go func() {
		logger.Info("starting metrics server", "addr", metricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("failed to serve metrics", "error", err)
		}
	}()

	// Start metrics updater
	startMetricsUpdater(app)
	
	// Start server in goroutine
	go func() {
		logger.Info("starting gRPC server", "addr", lis.Addr(), "env", cfg.env)
		if err := s.Serve(lis); err != nil {
			logger.Error("failed to serve", "error", err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	logger.Info("shutting down gracefully...")

	// Stop cleanup goroutine
	close(done)

	// Stop rate limiter cleanup
	app.ipLimiter.Stop()

	// Gracefully stop both HTTP servers
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Stop pprof server
	if err := pprofServer.Shutdown(ctx); err != nil {
		logger.Error("failed to shutdown pprof server", "error", err)
	}
	
	// Stop metrics server
	if err := metricsServer.Shutdown(ctx); err != nil {
		logger.Error("failed to shutdown metrics server", "error", err)
	}

	// Gracefully stop the gRPC server
	s.GracefulStop()
	logger.Info("server stopped")
}
