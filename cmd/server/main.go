package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/reflection"

	pb "microchat.ai/proto"
)

type config struct {
	port int
	env  string
}

type application struct {
	config       config
	logger       *slog.Logger
	sessionStore *SessionStore
	pb.UnimplementedChatServiceServer
}

func main() {
	var cfg config

	flag.IntVar(&cfg.port, "port", 4000, "gRPC server port")
	flag.StringVar(&cfg.env, "env", "development", "environment (development|staging|production)")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := &application{
		config:       cfg,
		logger:       logger,
		sessionStore: NewSessionStore(),
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
	s := grpc.NewServer(grpc.Creds(creds))

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
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			app.sessionStore.CleanupIdleSessions()
		}
	}()

	logger.Info("starting gRPC server", "addr", lis.Addr(), "env", cfg.env)

	if err := s.Serve(lis); err != nil {
		logger.Error("failed to serve", "error", err)
		os.Exit(1)
	}
}
