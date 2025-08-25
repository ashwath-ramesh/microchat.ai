package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"

	pb "microchat.ai/proto"
)

const version = "1.0.0"

type config struct {
	serverAddr string
	model      pb.Model
	sessionID  uint16 // Ultra-low bandwidth: 16-bit value, encodes as ~2 bytes in protobuf
}

type application struct {
	config config
	logger *slog.Logger
	conn   *grpc.ClientConn
	grpc   pb.ChatServiceClient
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

func (app *application) connect() error {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
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
	fmt.Println("Commands: 'quit' to exit, Ctrl+C to quit")
	fmt.Print("> ")

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			fmt.Print("> ")
			continue
		}

		if input == "quit" || input == "exit" {
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

	if resp.Error != pb.ErrorCode_NO_ERROR {
		fmt.Printf("Error: %v\n", resp.Error)
		return nil
	}

	fmt.Printf("Assistant: %s\n", resp.Reply)
	return nil
}

// Generate session ID
// Returns uint16 (~2 bytes in protobuf). Collision probability <1% with <200 concurrent sessions
func generateSessionID() uint16 {
	return uint16(time.Now().UnixNano() & 0xFFFF)
}
