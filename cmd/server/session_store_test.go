package main

import (
	"fmt"
	"regexp"
	"sync"
	"testing"
	"time"
)

func TestSessionStore_AppendMessage(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 1000, 100, 100*1024)

	// Register a valid session ID first
	store.RegisterSession("test-session-1")

	// Test appending to new session
	err := store.AppendMessage("test-session-1", User, "Hello")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	messages := store.GetMessages("test-session-1")

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].Role != User {
		t.Errorf("Expected User role, got %v", messages[0].Role)
	}

	if messages[0].Text != "Hello" {
		t.Errorf("Expected 'Hello', got '%s'", messages[0].Text)
	}

	// Test appending to existing session
	err = store.AppendMessage("test-session-1", Assistant, "Hello")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	messages = store.GetMessages("test-session-1")

	if len(messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(messages))
	}

	// Test formatted messages with timestamps
	formattedMessages := store.GetFormattedMessages("test-session-1")
	expected := []string{"user .* UTC.: Hello", "assistant .* UTC.: Hello"}
	for i, msg := range formattedMessages {
		matched, err := regexp.MatchString(expected[i], msg)
		if err != nil {
			t.Errorf("Regex error: %v", err)
		}
		if !matched {
			t.Errorf("Message '%s' doesn't match expected pattern '%s'", msg, expected[i])
		}
	}

	// Test invalid session ID (not registered)
	err = store.AppendMessage("invalid-session", User, "Should fail")
	if err == nil {
		t.Errorf("Expected error for invalid session ID, but got nil")
	}
}

func TestSessionStore_GetMessages_NonExistent(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 1000, 100, 100*1024)

	messages := store.GetMessages("nonexistent-session")
	if len(messages) != 0 {
		t.Errorf("Expected empty slice for non-existent session, got %d messages", len(messages))
	}

	// Test formatted messages for non-existent session
	formattedMessages := store.GetFormattedMessages("nonexistent-session")
	if len(formattedMessages) != 0 {
		t.Errorf("Expected empty slice for non-existent session, got %d formatted messages", len(formattedMessages))
	}
}

func TestSessionStore_GetMessages_ReturnsCopy(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 1000, 100, 100*1024)
	store.RegisterSession("test-session-1")
	err := store.AppendMessage("test-session-1", User, "test message")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	messages1 := store.GetMessages("test-session-1")
	messages2 := store.GetMessages("test-session-1")

	// Modify one slice
	messages1[0].Text = "modified"

	// Other slice should be unaffected
	if messages2[0].Text != "test message" {
		t.Errorf("GetMessages should return independent copies")
	}
}

func TestSessionStore_ConcurrentAccess(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 1000, 100, 100*1024)
	var wg sync.WaitGroup
	sessionID := "concurrent-test-session"
	store.RegisterSession(sessionID)

	// Start multiple goroutines appending messages
	numGoroutines := 10
	messagesPerGoroutine := 10

	for i := range numGoroutines {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := range messagesPerGoroutine {
				msg := fmt.Sprintf("goroutine-%d-message-%d", goroutineID, j)
				store.AppendMessage(sessionID, User, msg)
			}
		}(i)
	}

	wg.Wait()

	// Check that all messages were stored
	messages := store.GetMessages(sessionID)
	expectedCount := numGoroutines * messagesPerGoroutine

	if len(messages) != expectedCount {
		t.Errorf("Expected %d messages, got %d", expectedCount, len(messages))
	}
}

func TestSessionStore_GetSessionCount(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 1000, 100, 100*1024)

	// Initially empty
	if count := store.GetSessionCount(); count != 0 {
		t.Errorf("Expected 0 sessions, got %d", count)
	}

	// Add messages to different sessions
	store.RegisterSession("session-1")
	store.RegisterSession("session-2")
	err := store.AppendMessage("session-1", User, "message 1")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	err = store.AppendMessage("session-2", User, "message 2")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	err = store.AppendMessage("session-1", Assistant, "message 3") // Same session
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if count := store.GetSessionCount(); count != 2 {
		t.Errorf("Expected 2 sessions, got %d", count)
	}
}

func TestMessage_FormattedString(t *testing.T) {
	testCases := []struct {
		role    Role
		text    string
		pattern string
	}{
		{User, "Hello", `user \[\d{2}:\d{2}:\d{2} UTC\]: Hello`},
		{Assistant, "Hi there", `assistant \[\d{2}:\d{2}:\d{2} UTC\]: Hi there`},
		{System, "System message", `system \[\d{2}:\d{2}:\d{2} UTC\]: System message`},
	}

	for _, tc := range testCases {
		msg := Message{
			Role:      tc.role,
			Text:      tc.text,
			Timestamp: time.Now().UTC(),
		}

		result := msg.FormattedString()
		matched, err := regexp.MatchString(tc.pattern, result)
		if err != nil {
			t.Errorf("Regex error: %v", err)
		}
		if !matched {
			t.Errorf("FormattedString '%s' doesn't match expected pattern '%s'", result, tc.pattern)
		}
	}
}

func TestRole_String(t *testing.T) {
	testCases := []struct {
		role     Role
		expected string
	}{
		{User, "user"},
		{Assistant, "assistant"},
		{System, "system"},
		{Role(999), "unknown"},
	}

	for _, tc := range testCases {
		result := tc.role.String()
		if result != tc.expected {
			t.Errorf("Expected '%s', got '%s'", tc.expected, result)
		}
	}
}

func TestSessionStore_MessageTimestamps(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 1000, 100, 100*1024)

	store.RegisterSession("timestamp-test-session")
	before := time.Now()
	err := store.AppendMessage("timestamp-test-session", User, "First message")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	err = store.AppendMessage("timestamp-test-session", Assistant, "Second message")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	after := time.Now()

	messages := store.GetMessages("timestamp-test-session")

	if len(messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(messages))
	}

	// Check that timestamps are reasonable
	for i, msg := range messages {
		if msg.Timestamp.Before(before) || msg.Timestamp.After(after) {
			t.Errorf("Message %d timestamp %v is outside expected range [%v, %v]",
				i, msg.Timestamp, before, after)
		}
	}

	// Check that second message timestamp is after first
	if !messages[1].Timestamp.After(messages[0].Timestamp) {
		t.Errorf("Second message timestamp should be after first message timestamp")
	}
}

func TestSessionStore_LastActiveTimestamp(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 1000, 100, 100*1024)
	sessionID := "last-active-test-session"

	store.RegisterSession(sessionID)
	err := store.AppendMessage(sessionID, User, "First message")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Sleep to ensure different timestamps
	time.Sleep(10 * time.Millisecond)

	// Record time before second message
	middle := time.Now().UTC()
	err = store.AppendMessage(sessionID, Assistant, "Second message")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	after := time.Now().UTC()

	// Access session directly to check LastActive
	store.mu.RLock()
	session := store.sessions[sessionID]
	store.mu.RUnlock()

	if session == nil {
		t.Fatal("Expected session to exist")
	}

	// LastActive should be after the second message was added
	if session.LastActive.Before(middle) {
		t.Errorf("LastActive should be updated after second message")
	}

	if session.LastActive.After(after) {
		t.Errorf("LastActive should not be after test completion")
	}
}

func TestSessionStore_CleanupIdleSessions(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 1000, 100, 100*1024)

	// Create sessions with different ages
	store.RegisterSession("recent-session")
	store.RegisterSession("old-session")
	err := store.AppendMessage("recent-session", User, "Recent message")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	err = store.AppendMessage("old-session", User, "Old message")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Manually set LastActive to simulate old session
	store.mu.Lock()
	store.sessions["old-session"].LastActive = time.Now().UTC().Add(-3 * time.Hour) // 3 hours ago
	store.mu.Unlock()

	// Verify both sessions exist
	if count := store.GetSessionCount(); count != 2 {
		t.Errorf("Expected 2 sessions before cleanup, got %d", count)
	}

	// Run cleanup
	store.CleanupIdleSessions()

	// Only recent session should remain
	if count := store.GetSessionCount(); count != 1 {
		t.Errorf("Expected 1 session after cleanup, got %d", count)
	}

	// Verify the correct session remains
	messages := store.GetMessages("recent-session")
	if len(messages) == 0 {
		t.Error("Recent session should still exist")
	}

	// Verify old session is gone
	messages = store.GetMessages("old-session")
	if len(messages) != 0 {
		t.Error("Old session should be cleaned up")
	}
}

// New tests for session limits functionality

func TestSessionStore_SessionValidation(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 1000, 100, 100*1024)

	// Test invalid session (not registered)
	err := store.AppendMessage("invalid-session", User, "Should fail")
	if err == nil {
		t.Error("Expected error for invalid session ID")
	}

	// Test valid session
	store.RegisterSession("valid-session")
	err = store.AppendMessage("valid-session", User, "Should work")
	if err != nil {
		t.Errorf("Unexpected error for valid session: %v", err)
	}
}

func TestSessionStore_MessageLimits(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 1000, 3, 100*1024) // Max 3 messages per session

	store.RegisterSession("test-session")

	// Should allow up to 3 messages
	for i := 0; i < 3; i++ {
		err := store.AppendMessage("test-session", User, fmt.Sprintf("Message %d", i+1))
		if err != nil {
			t.Errorf("Unexpected error for message %d: %v", i+1, err)
		}
	}

	// 4th message should fail
	err := store.AppendMessage("test-session", User, "Should fail")
	if err == nil {
		t.Error("Expected error for exceeding message limit")
	}
}

func TestSessionStore_SessionSizeLimits(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 1000, 100, 100) // Max 100 bytes per session

	store.RegisterSession("test-session")

	// Add a large message that exceeds size limit
	largeMessage := make([]byte, 200)
	for i := range largeMessage {
		largeMessage[i] = 'A'
	}

	err := store.AppendMessage("test-session", User, string(largeMessage))
	if err == nil {
		t.Error("Expected error for exceeding session size limit")
	}
}

func TestSessionStore_MaxSessionsWithEviction(t *testing.T) {
	store := NewSessionStore(2*time.Hour, 2, 100, 100*1024) // Max 2 sessions

	// Create first two sessions
	store.RegisterSession("session-1")
	store.RegisterSession("session-2")

	err := store.AppendMessage("session-1", User, "Message 1")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	err = store.AppendMessage("session-2", User, "Message 2")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if count := store.GetSessionCount(); count != 2 {
		t.Errorf("Expected 2 sessions, got %d", count)
	}

	// Create third session - should evict oldest
	store.RegisterSession("session-3")
	err = store.AppendMessage("session-3", User, "Message 3")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Should still have 2 sessions but session-1 should be evicted
	if count := store.GetSessionCount(); count != 2 {
		t.Errorf("Expected 2 sessions after eviction, got %d", count)
	}

	// session-1 should be gone
	messages := store.GetMessages("session-1")
	if len(messages) != 0 {
		t.Error("session-1 should have been evicted")
	}

	// session-2 and session-3 should still exist
	messages = store.GetMessages("session-2")
	if len(messages) == 0 {
		t.Error("session-2 should still exist")
	}
	messages = store.GetMessages("session-3")
	if len(messages) == 0 {
		t.Error("session-3 should still exist")
	}
}
