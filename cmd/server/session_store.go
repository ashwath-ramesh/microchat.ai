package main

import (
	"fmt"
	"sync"
	"time"
)

// Role represents the role of a message sender
type Role int

const (
	User Role = iota
	Assistant
	System
)

// String returns the string representation of a Role
func (r Role) String() string {
	switch r {
	case User:
		return "user"
	case Assistant:
		return "assistant"
	case System:
		return "system"
	default:
		return "unknown"
	}
}

// Message represents a structured message with role, text, and timestamp
// Layer 2: Proper message structure as specified in the architecture document
type Message struct {
	Role      Role      `json:"role"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// FormattedString returns the message with UTC timestamp for debugging/testing
func (m Message) FormattedString() string {
	return fmt.Sprintf("%s [%s UTC]: %s",
		m.Role.String(),
		m.Timestamp.UTC().Format("15:04:05"),
		m.Text)
}

// Session represents a conversation session with messages and last activity timestamp
// Layer 3: Session management as specified in the architecture document
type Session struct {
	Messages   []Message `json:"messages"`
	LastActive time.Time `json:"last_active"`
}

// SessionStore provides thread-safe storage for conversation history
// Layer 3: Session management as specified in the architecture document
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[uint32]*Session
}

// NewSessionStore creates a new SessionStore instance
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[uint32]*Session),
	}
}

// AppendMessage adds a structured message to the session history
// Creates session if it doesn't exist
func (s *SessionStore) AppendMessage(sessionID uint32, role Role, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()

	if s.sessions[sessionID] == nil {
		s.sessions[sessionID] = &Session{
			Messages:   make([]Message, 0),
			LastActive: now,
		}
	}

	message := Message{
		Role:      role,
		Text:      text,
		Timestamp: now,
	}

	session := s.sessions[sessionID]
	session.Messages = append(session.Messages, message)
	session.LastActive = now
}

// GetMessages returns all structured messages for a session
// Returns empty slice if session doesn't exist
func (s *SessionStore) GetMessages(sessionID uint32) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if session, exists := s.sessions[sessionID]; exists {
		// Return a copy to prevent external modification
		result := make([]Message, len(session.Messages))
		copy(result, session.Messages)
		return result
	}

	return []Message{}
}

// GetFormattedMessages returns all messages for a session as formatted strings
// For backward compatibility with Layer 1 format
func (s *SessionStore) GetFormattedMessages(sessionID uint32) []string {
	messages := s.GetMessages(sessionID)
	result := make([]string, len(messages))
	for i, msg := range messages {
		result[i] = msg.FormattedString()
	}
	return result
}

// GetSessionCount returns the number of active sessions
func (s *SessionStore) GetSessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// CleanupIdleSessions removes sessions that have been idle for more than 2 hours
func (s *SessionStore) CleanupIdleSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UTC().Add(-2 * time.Hour)

	for sessionID, session := range s.sessions {
		if session.LastActive.Before(cutoff) {
			delete(s.sessions, sessionID)
		}
	}
}
