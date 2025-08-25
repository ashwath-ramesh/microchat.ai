package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"

	pb "microchat.ai/proto"
)

const quitCommand = "/quit"

type config struct {
	serverAddr string
	model      pb.Model
	sessionID  uint16 // Ultra-low bandwidth: 16-bit value, encodes as ~2 bytes in protobuf
}

type application struct {
	config  config
	logger  *slog.Logger
	conn    *grpc.ClientConn
	grpc    pb.ChatServiceClient
	metrics metrics
}

func main() {
	var cfg config

	flag.StringVar(&cfg.serverAddr, "addr", "localhost:4000", "gRPC server address")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Set defaults
	cfg.model = pb.Model_GPT_4
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

	logger.Info("connected to server", "addr", cfg.serverAddr)

	app.startChat()
}

// Returns uint16 (~2 bytes in protobuf). Uses crypto/rand for secure generation.
// Collision probability ~0.15% with 200 concurrent sessions, ~0.76% with 1000 sessions.
func generateSessionID() uint16 {
	var b [2]byte
	rand.Read(b[:])
	return binary.LittleEndian.Uint16(b[:])
}

func (app *application) connect() error {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
		grpc.WithUnaryInterceptor(app.byteTracker),
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
		SessionId: uint32(app.config.sessionID), // Convert uint16 to uint32 for protobuf
		Model:     app.config.model,
		Message:   message,
	}

	resp, err := app.grpc.Chat(ctx, req)
	if err != nil {
		return err
	}

	// Display session totals only
	totalOut, totalIn := app.metrics.getTotals()
	fmt.Printf("[Total: %s sent, %s received]\n", formatBytes(totalOut), formatBytes(totalIn))
	fmt.Printf("Assistant: %s\n", resp.Reply)
	return nil
}
