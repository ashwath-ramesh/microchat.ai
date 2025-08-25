package main

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

const kibibyte = 1024

type metrics struct {
	bytesIn  int64
	bytesOut int64
	mu       sync.RWMutex
}

func (m *metrics) addBytes(out, in int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bytesOut += out
	m.bytesIn += in
}

func (m *metrics) getTotals() (int64, int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bytesOut, m.bytesIn
}

func formatBytes(bytes int64) string {
	if bytes < kibibyte {
		return fmt.Sprintf("%d B", bytes)
	}
	kb := float64(bytes) / kibibyte
	return fmt.Sprintf("%.1f KB", kb)
}

func (app *application) byteTracker(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	reqBytes := 0
	respBytes := 0

	if protoMsg, ok := req.(proto.Message); ok {
		reqBytes = proto.Size(protoMsg)
	}

	err := invoker(ctx, method, req, reply, cc, opts...)

	if protoMsg, ok := reply.(proto.Message); ok {
		respBytes = proto.Size(protoMsg)
	}

	app.metrics.addBytes(int64(reqBytes), int64(respBytes))
	return err
}
