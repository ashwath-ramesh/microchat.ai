package main

import (
	"testing"
	"time"
)

func BenchmarkSessionStore_AppendMessage_Small(b *testing.B) {
	b.ReportAllocs()
	benchmarkAppendMessage(b, "small")
}

func BenchmarkSessionStore_AppendMessage_Medium(b *testing.B) {
	b.ReportAllocs()
	benchmarkAppendMessage(b, "medium")
}

func BenchmarkSessionStore_AppendMessage_Large(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 1024) // Report throughput for large messages (~8KB)
	benchmarkAppendMessage(b, "large")
}

// Helper function for append message benchmarks with concurrent access
func benchmarkAppendMessage(b *testing.B, messageSize string) {
	app, _ := setupBenchApp()
	store := app.sessionStore

	// Pre-create many sessions to distribute load and avoid size limits
	numSessions := 100
	sessions := make([]string, numSessions)
	for i := range numSessions {
		sessionID, err := createSession(app)
		if err != nil {
			b.Fatal(err)
		}
		sessions[i] = sessionID
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		sessionIdx := 0
		messageCount := 0
		maxMessagesPerSession := 10 // Keep sessions small to avoid size limits in concurrent testing

		for pb.Next() {
			currentSession := sessions[sessionIdx%numSessions]

			// Create fresh session if approaching limit
			if messageCount >= maxMessagesPerSession {
				if newSessionID, err := createSession(app); err == nil {
					sessions[sessionIdx%numSessions] = newSessionID
					currentSession = newSessionID
				}
				messageCount = 0
			}

			err := store.AppendMessage(currentSession, User, generateRealisticMessage(messageSize, messageCount))
			if err != nil {
				b.Fatal(err)
			}
			messageCount++
			sessionIdx++
		}
	})
}

func BenchmarkSessionStore_GetMessages_Small(b *testing.B) {
	b.ReportAllocs()
	benchmarkGetMessages(b, "small")
}

func BenchmarkSessionStore_GetMessages_Medium(b *testing.B) {
	b.ReportAllocs()
	benchmarkGetMessages(b, "medium")
}

func BenchmarkSessionStore_GetMessages_Large(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 1024) // Report throughput for large messages (~8KB)
	benchmarkGetMessages(b, "large")
}

// Helper function for get messages benchmarks with concurrent access
func benchmarkGetMessages(b *testing.B, messageSize string) {
	app, _ := setupBenchApp()
	store := app.sessionStore

	// Pre-create sessions and populate them with messages
	numSessions := 100
	sessions := make([]string, numSessions)
	for i := range numSessions {
		sessionID, err := createSession(app)
		if err != nil {
			b.Fatal(err)
		}

		// Populate with fewer messages to avoid size limits
		for j := range 10 {
			msg := generateRealisticMessage(messageSize, j)
			store.AppendMessage(sessionID, User, msg)
		}
		sessions[i] = sessionID
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		sessionIdx := 0
		for pb.Next() {
			currentSession := sessions[sessionIdx%numSessions]
			messages := store.GetMessages(currentSession)
			_ = messages
			sessionIdx++
		}
	})
}

func BenchmarkSessionStore_ConcurrentAccess_Small(b *testing.B) {
	b.ReportAllocs()
	benchmarkConcurrentAccess(b, "small")
}

func BenchmarkSessionStore_ConcurrentAccess_Medium(b *testing.B) {
	b.ReportAllocs()
	benchmarkConcurrentAccess(b, "medium")
}

func BenchmarkSessionStore_ConcurrentAccess_Large(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 1024) // Report throughput for large messages (~8KB)
	benchmarkConcurrentAccess(b, "large")
}

// Helper function for concurrent access benchmarks
func benchmarkConcurrentAccess(b *testing.B, messageSize string) {
	app, _ := setupBenchApp()
	store := app.sessionStore
	numSessions := 100
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
	b.RunParallel(func(pb *testing.PB) {
		sessionIdx := 0
		for pb.Next() {
			sessionID := sessions[sessionIdx%numSessions]
			sessionIdx++

			msg := generateRealisticMessage(messageSize, sessionIdx)
			store.AppendMessage(sessionID, User, msg)
			store.GetMessages(sessionID)
		}
	})
}

func BenchmarkSessionStore_MultipleSessionsWrite_Small(b *testing.B) {
	b.ReportAllocs()
	benchmarkMultipleSessionsWrite(b, "small")
}

func BenchmarkSessionStore_MultipleSessionsWrite_Medium(b *testing.B) {
	b.ReportAllocs()
	benchmarkMultipleSessionsWrite(b, "medium")
}

func BenchmarkSessionStore_MultipleSessionsWrite_Large(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 1024) // Report throughput for large messages (~8KB)
	benchmarkMultipleSessionsWrite(b, "large")
}

// Helper function for multiple sessions write benchmarks
func benchmarkMultipleSessionsWrite(b *testing.B, messageSize string) {
	app, _ := setupBenchApp()
	store := app.sessionStore
	numSessions := 100
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

	b.RunParallel(func(pb *testing.PB) {
		sessionIdx := 0
		messageCount := 0
		maxMessages := 10 // Keep sessions small to avoid size limits

		for pb.Next() {
			currentSessionIdx := sessionIdx % numSessions
			currentSession := sessions[currentSessionIdx]

			// Create new session if approaching limit
			if messageCount >= maxMessages {
				if newSessionID, err := createSession(app); err == nil {
					sessions[currentSessionIdx] = newSessionID
					currentSession = newSessionID
				}
				messageCount = 0
			}

			msg := generateRealisticMessage(messageSize, messageCount)
			store.AppendMessage(currentSession, User, msg)
			messageCount++
			sessionIdx++
		}
	})
}

func BenchmarkSessionStore_CleanupIdleSessions_Small(b *testing.B) {
	b.ReportAllocs()
	benchmarkCleanupIdleSessions(b, "small")
}

func BenchmarkSessionStore_CleanupIdleSessions_Medium(b *testing.B) {
	b.ReportAllocs()
	benchmarkCleanupIdleSessions(b, "medium")
}

func BenchmarkSessionStore_CleanupIdleSessions_Large(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 1024) // Report throughput for large messages (~8KB)
	benchmarkCleanupIdleSessions(b, "large")
}

// High contention benchmarks - multiple goroutines hitting the same session
func BenchmarkSessionStore_HighContention_Read_Small(b *testing.B) {
	b.ReportAllocs()
	benchmarkHighContentionRead(b, "small")
}

func BenchmarkSessionStore_HighContention_Read_Medium(b *testing.B) {
	b.ReportAllocs()
	benchmarkHighContentionRead(b, "medium")
}

func BenchmarkSessionStore_HighContention_Read_Large(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 1024) // Report throughput for large messages (~8KB)
	benchmarkHighContentionRead(b, "large")
}

// benchmarkHighContentionRead tests lock contention with many readers on the same session
func benchmarkHighContentionRead(b *testing.B, messageSize string) {
	app, _ := setupBenchApp()
	store := app.sessionStore

	// Create a single session and populate it
	sessionID, err := createSession(app)
	if err != nil {
		b.Fatal(err)
	}

	// Populate with realistic messages
	for i := range 100 {
		msg := generateRealisticMessage(messageSize, i)
		store.AppendMessage(sessionID, User, msg)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// All goroutines reading from the same session - high lock contention
			messages := store.GetMessages(sessionID)
			_ = messages
		}
	})
}

func BenchmarkSessionStore_HighContention_Write_Small(b *testing.B) {
	b.ReportAllocs()
	benchmarkHighContentionWrite(b, "small")
}

func BenchmarkSessionStore_HighContention_Write_Medium(b *testing.B) {
	b.ReportAllocs()
	benchmarkHighContentionWrite(b, "medium")
}

func BenchmarkSessionStore_HighContention_Write_Large(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(8 * 1024) // Report throughput for large messages (~8KB)
	benchmarkHighContentionWrite(b, "large")
}

// benchmarkHighContentionWrite tests lock contention with many writers on few sessions
func benchmarkHighContentionWrite(b *testing.B, messageSize string) {
	app, _ := setupBenchApp()
	store := app.sessionStore

	// Create a small number of sessions for high contention
	numSessions := 4 // Few sessions = high contention per session
	sessions := make([]string, numSessions)
	for i := range numSessions {
		sessionID, err := createSession(app)
		if err != nil {
			b.Fatal(err)
		}
		sessions[i] = sessionID
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		sessionIdx := 0
		messageCount := map[int]int{} // Track message count per session
		maxMessages := 10             // Small limit for frequent session cycling

		for pb.Next() {
			currentSessionIdx := sessionIdx % numSessions
			currentSession := sessions[currentSessionIdx]

			// Create new session if approaching limit
			if messageCount[currentSessionIdx] >= maxMessages {
				if newSessionID, err := createSession(app); err == nil {
					sessions[currentSessionIdx] = newSessionID
					currentSession = newSessionID
					messageCount[currentSessionIdx] = 0
				}
			}

			err := store.AppendMessage(currentSession, User, generateRealisticMessage(messageSize, messageCount[currentSessionIdx]))
			if err != nil {
				b.Fatal(err)
			}
			messageCount[currentSessionIdx]++
			sessionIdx++
		}
	})
}

// Helper function for cleanup idle sessions benchmarks
func benchmarkCleanupIdleSessions(b *testing.B, messageSize string) {
	app, _ := setupBenchApp()
	// Use shorter idle timeout for faster benchmark
	app.sessionStore = NewSessionStore(100*time.Millisecond, 1000, 100, 1024*1024)
	store := app.sessionStore

	// Create many real sessions with specified message size
	for i := range 500 {
		sessionID, err := createSession(app)
		if err != nil {
			b.Fatal(err)
		}
		msg := generateRealisticMessage(messageSize, i)
		store.AppendMessage(sessionID, User, msg)
	}

	// Let some sessions become idle
	time.Sleep(150 * time.Millisecond)

	b.ResetTimer()
	for b.Loop() {
		store.CleanupIdleSessions()
	}
}
