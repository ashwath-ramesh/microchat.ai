package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"flag"
	"fmt"
	"io/ioutil"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding/gzip"

	pb "microchat.ai/proto"
)

const quitCommand = "/quit"

type config struct {
	serverAddr    string
	model         pb.Model
	modelString   string // String representation of model for flag parsing
	sessionID     uint16 // Ultra-low bandwidth: 16-bit value, encodes as ~2 bytes in protobuf
	metrics       bool   // Show compact session metrics
	metricsDetail bool   // Show detailed metrics
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
	// Load .env file from project root (required)
	if err := godotenv.Load("../../.env"); err != nil {
		logger.Error("failed to load .env file", "error", err)
		return err
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

	// Parse model string to enum
	cfg.model = parseModel(cfg.modelString, logger)
	cfg.sessionID = generateSessionID()

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

	logger.Info("connected to server", "addr", cfg.serverAddr, "model", cfg.modelString)

	app.startChat()
}

// Returns uint16 (~2 bytes in protobuf). Uses crypto/rand for secure generation.
// Collision probability ~0.15% with 200 concurrent sessions, ~0.76% with 1000 sessions.
func generateSessionID() uint16 {
	var b [2]byte
	rand.Read(b[:])
	return binary.LittleEndian.Uint16(b[:])
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

func (app *application) connect() error {
	serverName := os.Getenv("SERVER_NAME")
	if serverName == "" {
		serverName = "localhost"
	}

	// Load CA certificate (required)
	caPath := os.Getenv("CA_CERT_FILE")
	if caPath == "" {
		app.logger.Error("CA_CERT_FILE environment variable is required")
		return fmt.Errorf("CA_CERT_FILE environment variable is required")
	}
	// Resolve path relative to project root (since client runs from cmd/client)
	fullCaPath := "../../" + caPath
	caCert, err := ioutil.ReadFile(fullCaPath)
	if err != nil {
		app.logger.Error("failed to read CA certificate", "path", fullCaPath, "error", err)
		return fmt.Errorf("failed to read CA certificate from %s: %v", fullCaPath, err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		app.logger.Error("failed to append CA certificate", "path", fullCaPath)
		return fmt.Errorf("failed to append CA certificate")
	}

	creds := credentials.NewTLS(&tls.Config{
		ServerName: serverName,
		RootCAs:    caCertPool,
	})

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
			app.logger.Error("failed to send message", "error", err)
		}

		fmt.Print("> ")
	}

	if err := scanner.Err(); err != nil {
		app.logger.Error("error reading input", "error", err)
	}
}

func (app *application) sendMessage(message string) error {
	ctx := context.Background()
	req := &pb.ChatRequest{
		SessionId:    uint32(app.config.sessionID), // Convert uint16 to uint32 for protobuf
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
