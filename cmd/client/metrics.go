package main

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/proto"
)

const kibibyte = 1024

type metrics struct {
	// Session totals (reset on /clear)
	sessionPayloadBytesIn  int64
	sessionPayloadBytesOut int64
	sessionWireBytesIn     int64
	sessionWireBytesOut    int64

	// Lifetime totals (never reset)
	lifetimePayloadBytesIn  int64
	lifetimePayloadBytesOut int64
	lifetimeWireBytesIn     int64
	lifetimeWireBytesOut    int64

	// Per-message tracking (reset after each message)
	msgPayloadBytesIn  int64
	msgPayloadBytesOut int64
	msgWireBytesIn     int64
	msgWireBytesOut    int64

	mu sync.RWMutex
}

func (m *metrics) addPayloadBytes(out, in int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionPayloadBytesOut += out
	m.sessionPayloadBytesIn += in
	m.lifetimePayloadBytesOut += out
	m.lifetimePayloadBytesIn += in
	m.msgPayloadBytesOut += out
	m.msgPayloadBytesIn += in
}

func (m *metrics) addWireBytes(out, in int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionWireBytesOut += out
	m.sessionWireBytesIn += in
	m.lifetimeWireBytesOut += out
	m.lifetimeWireBytesIn += in
	m.msgWireBytesOut += out
	m.msgWireBytesIn += in
}

func (m *metrics) getSessionPayloadTotals() (int64, int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionPayloadBytesOut, m.sessionPayloadBytesIn
}

func (m *metrics) getLifetimePayloadTotals() (int64, int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lifetimePayloadBytesOut, m.lifetimePayloadBytesIn
}

func (m *metrics) getSessionWireTotals() (int64, int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionWireBytesOut, m.sessionWireBytesIn
}

func (m *metrics) getLifetimeWireTotals() (int64, int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lifetimeWireBytesOut, m.lifetimeWireBytesIn
}

func (m *metrics) getSessionTotals() (int64, int64, int64, int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionPayloadBytesOut, m.sessionPayloadBytesIn, m.sessionWireBytesOut, m.sessionWireBytesIn
}

func (m *metrics) getLifetimeTotals() (int64, int64, int64, int64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lifetimePayloadBytesOut, m.lifetimePayloadBytesIn, m.lifetimeWireBytesOut, m.lifetimeWireBytesIn
}

func (m *metrics) getMessageTotalsAndReset() (int64, int64, int64, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get current message totals
	msgPayloadOut := m.msgPayloadBytesOut
	msgPayloadIn := m.msgPayloadBytesIn
	msgWireOut := m.msgWireBytesOut
	msgWireIn := m.msgWireBytesIn

	// Reset for next message
	m.msgPayloadBytesOut = 0
	m.msgPayloadBytesIn = 0
	m.msgWireBytesOut = 0
	m.msgWireBytesIn = 0

	return msgPayloadOut, msgPayloadIn, msgWireOut, msgWireIn
}

func (m *metrics) resetSessionMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionPayloadBytesOut = 0
	m.sessionPayloadBytesIn = 0
	m.sessionWireBytesOut = 0
	m.sessionWireBytesIn = 0
	m.msgPayloadBytesOut = 0
	m.msgPayloadBytesIn = 0
	m.msgWireBytesOut = 0
	m.msgWireBytesIn = 0
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

	app.metrics.addPayloadBytes(int64(reqBytes), int64(respBytes))
	return err
}

// statsHandler implements grpc/stats.Handler to track wire-level bytes
type statsHandler struct {
	metrics *metrics
}

func (h *statsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return ctx
}

func (h *statsHandler) HandleRPC(ctx context.Context, s stats.RPCStats) {
	switch stat := s.(type) {
	case *stats.OutPayload:
		// Track bytes going out (includes gRPC framing)
		h.metrics.addWireBytes(int64(stat.WireLength), 0)
	case *stats.InPayload:
		// Track bytes coming in (includes gRPC framing)
		h.metrics.addWireBytes(0, int64(stat.WireLength))
	case *stats.InHeader:
		// Track inbound headers
		if stat.WireLength > 0 {
			h.metrics.addWireBytes(0, int64(stat.WireLength))
		}
	case *stats.InTrailer:
		// Track inbound trailers
		if stat.WireLength > 0 {
			h.metrics.addWireBytes(0, int64(stat.WireLength))
		}
	}
}

func (h *statsHandler) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return ctx
}

func (h *statsHandler) HandleConn(ctx context.Context, s stats.ConnStats) {
	// We can track connection-level events here if needed
}
