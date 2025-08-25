package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"

	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/reflection"

	pb "microchat.ai/proto"
)

type config struct {
	port int
	env  string
}

type application struct {
	config config
	logger *slog.Logger
	pb.UnimplementedChatServiceServer
}

func main() {
	var cfg config

	flag.IntVar(&cfg.port, "port", 4000, "gRPC server port")
	flag.StringVar(&cfg.env, "env", "development", "environment (development|staging|production)")
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	app := &application{
		config: cfg,
		logger: logger,
	}

	// create gRPC server with compression
	s := grpc.NewServer()

	// register service
	pb.RegisterChatServiceServer(s, app)

	// Enable reflection
	reflection.Register(s)

	// Listen on TCP
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.port))
	if err != nil {
		logger.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	logger.Info("starting gRPC server", "addr", lis.Addr(), "env", cfg.env)

	if err := s.Serve(lis); err != nil {
		logger.Error("failed to serve", "error", err)
		os.Exit(1)
	}
}
