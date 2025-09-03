package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
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
	apiKeys                map[string]bool // API keys for authentication
	dailyCallLimit         int             // Daily call limit per API key
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

	// Parse API keys (comma-separated)
	apiKeysStr := os.Getenv("API_KEYS")
	cfg.apiKeys = make(map[string]bool)
	if apiKeysStr != "" {
		keys := strings.Split(apiKeysStr, ",")
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key != "" {
				cfg.apiKeys[key] = true
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

	return cfg, nil
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
		sessionStore:    NewSessionStore(cfg.sessionIdleTimeout),
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

	// Gracefully stop the gRPC server
	s.GracefulStop()
	logger.Info("server stopped")
}
