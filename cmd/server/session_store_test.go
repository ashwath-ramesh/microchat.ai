package main

import (
	"fmt"
	"sync"
	"testing"
)

func TestSessionStore_AppendMessage(t *testing.T) {
	store := NewSessionStore()

	// Test appending to new session
	store.AppendMessage(1, "user: Hello")
	messages := store.GetMessages(1)

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	if messages[0] != "user: Hello" {
		t.Errorf("Expected 'user: Hello', got '%s'", messages[0])
	}

	// Test appending to existing session
	store.AppendMessage(1, "echo: Hello")
	messages = store.GetMessages(1)

	if len(messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(messages))
	}

	expected := []string{"user: Hello", "echo: Hello"}
	for i, msg := range messages {
		if msg != expected[i] {
			t.Errorf("Expected '%s', got '%s'", expected[i], msg)
		}
	}
}

func TestSessionStore_GetMessages_NonExistent(t *testing.T) {
	store := NewSessionStore()

	messages := store.GetMessages(999)
	if len(messages) != 0 {
		t.Errorf("Expected empty slice for non-existent session, got %d messages", len(messages))
	}
}

func TestSessionStore_GetMessages_ReturnsCopy(t *testing.T) {
	store := NewSessionStore()
	store.AppendMessage(1, "test message")

	messages1 := store.GetMessages(1)
	messages2 := store.GetMessages(1)

	// Modify one slice
	messages1[0] = "modified"

	// Other slice should be unaffected
	if messages2[0] != "test message" {
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
				store.AppendMessage(sessionID, msg)
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
	store.AppendMessage(1, "message 1")
	store.AppendMessage(2, "message 2")
	store.AppendMessage(1, "message 3") // Same session

	if count := store.GetSessionCount(); count != 2 {
		t.Errorf("Expected 2 sessions, got %d", count)
	}
}
