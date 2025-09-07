package main

import (
	"context"
	"testing"

	pb "microchat.ai/proto"
)

func BenchmarkChat_SingleMessage_Small(b *testing.B) {
	b.ReportAllocs()
	benchmarkChatSingleMessage(b, "small")
}

func BenchmarkChat_SingleMessage_Medium(b *testing.B) {
	b.ReportAllocs()
	benchmarkChatSingleMessage(b, "medium")
}

func BenchmarkChat_SingleMessage_Large(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 1024) // Report throughput for large messages (~8KB)
	benchmarkChatSingleMessage(b, "large")
}

// Helper function for single message benchmarks
func benchmarkChatSingleMessage(b *testing.B, messageSize string) {
	app, mockProvider := setupBenchApp()

	// Set realistic response size that matches the request payload for realistic serialization testing
	response := generateRealisticMessage(messageSize, 100) // Use different index for response variation
	mockProvider.SetResponses(response)

	// Create a real session
	sessionID, err := createSession(app)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	messageVariant := 0
	for b.Loop() {
		// Vary the message content to test different serialization patterns
		req := &pb.ChatRequest{
			SessionId: sessionID,
			Model:     pb.Model_ECHO,
			Message:   generateRealisticMessage(messageSize, messageVariant%10), // Cycle through 10 different messages
		}

		_, err := app.Chat(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
		messageVariant++
	}
}

func BenchmarkChat_ConversationFlow_Mixed(b *testing.B) {
	b.ReportAllocs()
	app, mockProvider := setupBenchApp()

	// Use realistic responses that match the input sizes for proper serialization testing
	responses := []string{
		generateRealisticMessage("small", 200),  // Response to initial question
		generateRealisticMessage("medium", 201), // Response to follow-up
		generateRealisticMessage("large", 202),  // Response to code example
		generateRealisticMessage("small", 203),  // Response to clarification
		generateRealisticMessage("medium", 204), // Response to final thoughts
	}
	mockProvider.SetResponses(responses...)

	// Realistic conversation flow with varying message sizes
	messages := []string{
		generateRealisticMessage("small", 1),  // Initial question
		generateRealisticMessage("medium", 2), // Follow-up with details
		generateRealisticMessage("large", 3),  // Code example
		generateRealisticMessage("small", 4),  // Clarification
		generateRealisticMessage("medium", 5), // Final thoughts
	}

	messageCount := 0
	maxMessagesPerSession := 200 // Realistic conversation length (research shows 10-500 typical)
	var sessionID string
	var err error

	// Create initial session
	sessionID, err = createSession(app)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		for _, msg := range messages {
			// Create fresh session if approaching limit
			if messageCount >= maxMessagesPerSession {
				sessionID, err = createSession(app)
				if err != nil {
					b.Fatal(err)
				}
				messageCount = 0
			}

			req := &pb.ChatRequest{
				SessionId: sessionID,
				Model:     pb.Model_ECHO,
				Message:   msg,
			}
			_, err := app.Chat(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}
			messageCount++
		}
	}
}

func BenchmarkChat_ConversationFlow_Small(b *testing.B) {
	b.ReportAllocs()
	benchmarkChatConversationFlow(b, "small")
}

func BenchmarkChat_ConversationFlow_Medium(b *testing.B) {
	b.ReportAllocs()
	benchmarkChatConversationFlow(b, "medium")
}

func BenchmarkChat_ConversationFlow_Large(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 1024) // Report throughput for large messages (~8KB)
	benchmarkChatConversationFlow(b, "large")
}

// Helper function for conversation flow benchmarks with specific message size
func benchmarkChatConversationFlow(b *testing.B, messageSize string) {
	app, mockProvider := setupBenchApp()

	// Use realistic responses that match the input size for proper serialization testing
	responses := []string{
		generateRealisticMessage(messageSize, 300),
		generateRealisticMessage(messageSize, 301),
		generateRealisticMessage(messageSize, 302),
		generateRealisticMessage(messageSize, 303),
		generateRealisticMessage(messageSize, 304),
	}
	mockProvider.SetResponses(responses...)

	// Conversation flow with consistent message size
	messages := make([]string, 5)
	for i := range 5 {
		messages[i] = generateRealisticMessage(messageSize, i+1)
	}

	messageCount := 0
	maxMessagesPerSession := 200 // Realistic conversation length (research shows 10-500 typical)
	var sessionID string
	var err error

	// Create initial session
	sessionID, err = createSession(app)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		for _, msg := range messages {
			// Create fresh session if approaching limit
			if messageCount >= maxMessagesPerSession {
				sessionID, err = createSession(app)
				if err != nil {
					b.Fatal(err)
				}
				messageCount = 0
			}

			req := &pb.ChatRequest{
				SessionId: sessionID,
				Model:     pb.Model_ECHO,
				Message:   msg,
			}
			_, err := app.Chat(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}
			messageCount++
		}
	}
}

func BenchmarkChat_ConcurrentSessions_Small(b *testing.B) {
	b.ReportAllocs()
	benchmarkChatConcurrentSessions(b, "small")
}

func BenchmarkChat_ConcurrentSessions_Medium(b *testing.B) {
	b.ReportAllocs()
	benchmarkChatConcurrentSessions(b, "medium")
}

func BenchmarkChat_ConcurrentSessions_Large(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 1024) // Report throughput for large messages (~8KB)
	benchmarkChatConcurrentSessions(b, "large")
}

// Helper function for concurrent sessions benchmarks
func benchmarkChatConcurrentSessions(b *testing.B, messageSize string) {
	app, mockProvider := setupBenchApp()

	// Use realistic response that matches input size for proper serialization testing
	response := generateRealisticMessage(messageSize, 400)
	mockProvider.SetResponses(response)

	numSessions := 50
	sessions := make([]string, numSessions)

	// Create real sessions
	for i := range numSessions {
		sessionID, err := createSession(app)
		if err != nil {
			b.Fatal(err)
		}
		sessions[i] = sessionID
	}

	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		sessionIdx := 0
		messageVariant := 0
		for p.Next() {
			sessionID := sessions[sessionIdx%numSessions]
			sessionIdx++

			// Vary message content to test different serialization patterns
			req := &pb.ChatRequest{
				SessionId: sessionID,
				Model:     pb.Model_ECHO,
				Message:   generateRealisticMessage(messageSize, messageVariant%20), // Cycle through 20 variants
			}
			_, err := app.Chat(context.Background(), req)
			if err != nil {
				b.Fatal(err)
			}
			messageVariant++
		}
	})
}

func BenchmarkChat_LargeMessage(b *testing.B) {
	b.ReportAllocs()
	app, mockProvider := setupBenchApp()

	// Use realistic large response instead of repeated characters
	largeResponse := generateRealisticMessage("large", 500)
	mockProvider.SetResponses(largeResponse)

	// Report throughput for large messages (approx 8KB per request+response)
	b.SetBytes(8 * 1024)

	b.ResetTimer()
	messageVariant := 0
	for b.Loop() {
		// Create a fresh session for each iteration to avoid size limits
		sessionID, err := createSession(app)
		if err != nil {
			b.Fatal(err)
		}

		// Vary message content to test different serialization patterns
		req := &pb.ChatRequest{
			SessionId: sessionID,
			Model:     pb.Model_ECHO,
			Message:   generateRealisticMessage("large", messageVariant%5), // Cycle through 5 large message variants
		}

		_, err = app.Chat(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
		messageVariant++
	}
}
