package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"log/slog"
	"math"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "microchat.ai/proto"
)

const quitCommand = "/quit"

type config struct {
	serverAddr    string
	model         pb.Model
	modelString   string // String representation of model for flag parsing
	sessionID     string // Server-generated UUID session ID
	metrics       bool   // Show compact session metrics
	metricsDetail bool   // Show detailed metrics
	apiKey        string // API key for authentication
}

type application struct {
	config       config
	logger       *slog.Logger
	conn         *grpc.ClientConn
	grpc         pb.ChatServiceClient
	metrics      metrics
	messageIndex uint32 // Layer 4: Track message count for delta protocol
}

// loadEnv loads environment variables from .env file
func loadEnv(logger *slog.Logger) error {
	// Load .env file - check current directory first, then project root
	if err := godotenv.Load(".env"); err != nil {
		if err := godotenv.Load("../../.env"); err != nil {
			logger.Warn("no .env file found, using environment variables only")
		}
	}
	return nil
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Load environment variables
	if err := loadEnv(logger); err != nil {
		os.Exit(1)
	}

	var cfg config

	flag.StringVar(&cfg.serverAddr, "addr", "localhost:4000", "gRPC server address")
	flag.StringVar(&cfg.modelString, "model", "gemini", "LLM model to use (echo, gemini)")
	flag.BoolVar(&cfg.metrics, "metrics", false, "show compact session metrics")
	flag.BoolVar(&cfg.metricsDetail, "metrics-detail", false, "show detailed message and session metrics")
	flag.Parse()

	// Get API key from environment
	cfg.apiKey = os.Getenv("MICROCHAT_API_KEY")
	if cfg.apiKey == "" {
		logger.Error("MICROCHAT_API_KEY environment variable is required")
		os.Exit(1)
	}

	// Parse model string to enum
	cfg.model = parseModel(cfg.modelString, logger)

	app := &application{
		config: cfg,
		logger: logger,
	}

	// Connect to server
	if err := app.connect(); err != nil {
		logger.Error("failed to connect", "error", err)
		os.Exit(1)
	}
	defer app.conn.Close()

	// Start session and get server-generated session ID
	if err := app.startSession(); err != nil {
		logger.Error("failed to start session", "error", err)
		os.Exit(1)
	}

	logger.Info("connected to server", "addr", cfg.serverAddr, "model", cfg.modelString, "session_id", app.config.sessionID)

	app.startChat()
}

// parseModel converts string model name to protobuf Model enum
func parseModel(modelStr string, logger *slog.Logger) pb.Model {
	switch strings.ToLower(modelStr) {
	case "gemini":
		return pb.Model_GEMINI_2_5_FLASH_LITE
	case "echo":
		return pb.Model_ECHO
	default:
		logger.Warn("unknown model, using default", "requested", modelStr, "default", "gemini")
		return pb.Model_GEMINI_2_5_FLASH_LITE // Default to gemini
	}
}

// isProductionServer determines if the server address is a production domain
func isProductionServer(serverAddr string) bool {
	host, _, err := net.SplitHostPort(serverAddr)
	if err != nil {
		// If we can't parse the address, assume development
		return false
	}

	// Check if it's localhost, 127.0.0.1, or similar development addresses
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return false
	}

	// If it contains a dot and isn't an IP address, it's likely a production domain
	return strings.Contains(host, ".") && net.ParseIP(host) == nil
}

func (app *application) connect() error {
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			app.logger.Info("retrying connection", "attempt", attempt+1, "delay", delay)
			time.Sleep(delay)
		}

		err := app.attemptConnect()
		if err == nil {
			return nil
		}

		if attempt == maxRetries {
			return fmt.Errorf("failed to connect after %d attempts: %v", maxRetries+1, err)
		}
	}
	return nil
}

func (app *application) attemptConnect() error {
	isProduction := isProductionServer(app.config.serverAddr)

	var creds credentials.TransportCredentials

	if isProduction {
		// Production: Use system CA certificates for valid certificates
		host, _, err := net.SplitHostPort(app.config.serverAddr)
		if err != nil {
			return fmt.Errorf("failed to parse server address: %v", err)
		}

		creds = credentials.NewTLS(&tls.Config{
			ServerName: host,
		})
		app.logger.Info("using system CA certificates for production server", "host", host)
	} else {
		// Development: Use self-signed certificates
		serverName := os.Getenv("SERVER_NAME")
		if serverName == "" {
			serverName = "localhost"
		}

		// Load CA certificate (with default)
		caPath := os.Getenv("CA_CERT_FILE")
		if caPath == "" {
			caPath = "certs/ca.crt"
		}

		// Try multiple possible locations for the certificate
		var fullCaPath string
		var caCert []byte
		var err error

		// First try relative to current working directory
		if _, err := os.Stat(caPath); err == nil {
			fullCaPath = caPath
			caCert, err = ioutil.ReadFile(fullCaPath)
		} else {
			// Try relative to project root (backwards compatibility)
			fullCaPath = "../../" + caPath
			caCert, err = ioutil.ReadFile(fullCaPath)
			if err != nil {
				// Try absolute path based on executable location
				if execPath, execErr := os.Executable(); execErr == nil {
					execDir := filepath.Dir(execPath)
					fullCaPath = filepath.Join(execDir, caPath)
					caCert, err = ioutil.ReadFile(fullCaPath)
				}
			}
		}

		if err != nil {
			app.logger.Error("failed to read CA certificate", "path", fullCaPath, "error", err)
			return fmt.Errorf("failed to read CA certificate from %s: %v", fullCaPath, err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			app.logger.Error("failed to append CA certificate", "path", fullCaPath)
			return fmt.Errorf("failed to append CA certificate")
		}

		creds = credentials.NewTLS(&tls.Config{
			ServerName: serverName,
			RootCAs:    caCertPool,
		})
		app.logger.Info("using self-signed CA certificate for development server", "path", fullCaPath, "server_name", serverName)
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
		grpc.WithUnaryInterceptor(app.byteTracker),
		grpc.WithStatsHandler(&statsHandler{metrics: &app.metrics}),
	}

	conn, err := grpc.NewClient(app.config.serverAddr, opts...)
	if err != nil {
		return err
	}

	app.conn = conn
	app.grpc = pb.NewChatServiceClient(conn)
	return nil
}

// addAuthContext adds API key to gRPC context
func (app *application) addAuthContext(ctx context.Context) context.Context {
	md := metadata.Pairs("authorization", "Bearer "+app.config.apiKey)
	return metadata.NewOutgoingContext(ctx, md)
}

func (app *application) startSession() error {
	ctx := app.addAuthContext(context.Background())
	req := &pb.StartSessionRequest{}

	resp, err := app.grpc.StartSession(ctx, req)
	if err != nil {
		return err
	}

	app.config.sessionID = resp.SessionId
	return nil
}

func (app *application) startChat() {
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		app.logger.Info("shutting down...")
		app.conn.Close()
		os.Exit(0)
	}()

	app.logger.Info("starting interactive chat - type 'quit' to exit")
	fmt.Println("microchat.ai client - type your message and press Enter")
	fmt.Printf("Commands: '%s' to exit, Ctrl+C to quit\n", quitCommand)
	fmt.Println("[Starting session - 0 B sent, 0 B received]")
	fmt.Print("> ")

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			fmt.Print("> ")
			continue
		}

		if input == quitCommand {
			app.logger.Info("goodbye!")
			break
		}

		if err := app.sendMessage(input); err != nil {
			grpcStatus, ok := status.FromError(err)
			if ok {
				switch grpcStatus.Code() {
				case codes.Internal, codes.Unavailable:
					fmt.Printf("Error: %s (server is experiencing issues)\n", grpcStatus.Message())
				default:
					fmt.Printf("Error: %s\n", grpcStatus.Message())
				}
			} else {
				app.logger.Error("failed to send message", "error", err)
				fmt.Printf("Error: Connection failed. Please try again.\n")
			}
		}

		fmt.Print("> ")
	}

	if err := scanner.Err(); err != nil {
		app.logger.Error("error reading input", "error", err)
	}
}

func (app *application) sendMessage(message string) error {
	ctx := app.addAuthContext(context.Background())
	req := &pb.ChatRequest{
		SessionId:    app.config.sessionID, // Server-generated UUID session ID
		Model:        app.config.model,
		Message:      message,
		MessageIndex: app.messageIndex, // Layer 4: Include our message index
	}

	resp, err := app.grpc.Chat(ctx, req)
	if err != nil {
		return err
	}

	// Layer 4: Update our message index from server's response
	app.messageIndex = resp.MessageCount

	fmt.Printf("Assistant: %s\n", resp.Reply)
	app.displayMetrics()

	// Layer 4: Log delta protocol info when detailed metrics enabled
	if app.config.metricsDetail {
		fmt.Printf("Delta: Client index=%d, Server count=%d\n",
			req.MessageIndex, resp.MessageCount)
	}

	return nil
}

func (app *application) displayMetrics() {
	if !app.config.metrics && !app.config.metricsDetail {
		return // No metrics to display
	}

	if app.config.metricsDetail {
		// NOTE: Wire bytes can be less than payload bytes due to gzip compression.
		// Payload = uncompressed protobuf size, Wire = compressed data + gRPC protocol overhead.
		// Show detailed metrics with arrow format
		msgPayloadOut, msgPayloadIn, msgWireOut, msgWireIn := app.metrics.getMessageTotalsAndReset()
		totalPayloadOut, totalPayloadIn, totalWireOut, totalWireIn := app.metrics.getAllTotals()

		fmt.Println()
		fmt.Printf("Message: [Payload: ↑%s ↓%s] [Wire (gzip): ↑%s ↓%s]\n",
			formatBytes(msgPayloadOut), formatBytes(msgPayloadIn),
			formatBytes(msgWireOut), formatBytes(msgWireIn))
		fmt.Printf("Session: [Payload: ↑%s ↓%s] [Wire (gzip): ↑%s ↓%s]\n",
			formatBytes(totalPayloadOut), formatBytes(totalPayloadIn),
			formatBytes(totalWireOut), formatBytes(totalWireIn))
		fmt.Println()
	} else if app.config.metrics {
		// Show compact session metrics (wire level only)
		_, _, totalWireOut, totalWireIn := app.metrics.getAllTotals()
		fmt.Printf("[↑%s ↓%s]\n", formatBytes(totalWireOut), formatBytes(totalWireIn))

		// Reset message counters even though we don't display them
		app.metrics.getMessageTotalsAndReset()
	}
}
