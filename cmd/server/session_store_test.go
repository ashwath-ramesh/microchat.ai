package main

import (
	"fmt"
	"regexp"
	"sync"
	"testing"
	"time"
)

func TestSessionStore_AppendMessage(t *testing.T) {
	store := NewSessionStore()

	// Test appending to new session
	store.AppendMessage(1, User, "Hello")
	messages := store.GetMessages(1)

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
	store.AppendMessage(1, Assistant, "Hello")
	messages = store.GetMessages(1)

	if len(messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(messages))
	}

	// Test formatted messages with timestamps
	formattedMessages := store.GetFormattedMessages(1)
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
}

func TestSessionStore_GetMessages_NonExistent(t *testing.T) {
	store := NewSessionStore()

	messages := store.GetMessages(999)
	if len(messages) != 0 {
		t.Errorf("Expected empty slice for non-existent session, got %d messages", len(messages))
	}

	// Test formatted messages for non-existent session
	formattedMessages := store.GetFormattedMessages(999)
	if len(formattedMessages) != 0 {
		t.Errorf("Expected empty slice for non-existent session, got %d formatted messages", len(formattedMessages))
	}
}

func TestSessionStore_GetMessages_ReturnsCopy(t *testing.T) {
	store := NewSessionStore()
	store.AppendMessage(1, User, "test message")

	messages1 := store.GetMessages(1)
	messages2 := store.GetMessages(1)

	// Modify one slice
	messages1[0].Text = "modified"

	// Other slice should be unaffected
	if messages2[0].Text != "test message" {
		t.Errorf("GetMessages should return independent copies")
	}
}

func TestSessionStore_ConcurrentAccess(t *testing.T) {
	store := NewSessionStore()
	var wg sync.WaitGroup
	sessionID := uint32(1)

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
	store := NewSessionStore()

	// Initially empty
	if count := store.GetSessionCount(); count != 0 {
		t.Errorf("Expected 0 sessions, got %d", count)
	}

	// Add messages to different sessions
	store.AppendMessage(1, User, "message 1")
	store.AppendMessage(2, User, "message 2")
	store.AppendMessage(1, Assistant, "message 3") // Same session

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
	store := NewSessionStore()

	before := time.Now()
	store.AppendMessage(1, User, "First message")
	time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	store.AppendMessage(1, Assistant, "Second message")
	after := time.Now()

	messages := store.GetMessages(1)

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
	store := NewSessionStore()
	sessionID := uint32(1)

	store.AppendMessage(sessionID, User, "First message")

	// Sleep to ensure different timestamps
	time.Sleep(10 * time.Millisecond)

	// Record time before second message
	middle := time.Now().UTC()
	store.AppendMessage(sessionID, Assistant, "Second message")
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
	store := NewSessionStore()

	// Create sessions with different ages
	store.AppendMessage(1, User, "Recent message")
	store.AppendMessage(2, User, "Old message")

	// Manually set LastActive to simulate old session
	store.mu.Lock()
	store.sessions[2].LastActive = time.Now().UTC().Add(-3 * time.Hour) // 3 hours ago
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
	messages := store.GetMessages(1)
	if len(messages) == 0 {
		t.Error("Recent session should still exist")
	}

	// Verify old session is gone
	messages = store.GetMessages(2)
	if len(messages) != 0 {
		t.Error("Old session should be cleaned up")
	}
}
