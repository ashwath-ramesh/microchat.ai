package main

import (
	"fmt"
	"sync"
	"time"

	"microchat.ai/cmd/server/llm"
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
	mu                    sync.RWMutex
	sessions              map[string]*Session
	validSessions         map[string]bool // Track sessions created via StartSession
	idleTimeout           time.Duration
	maxSessions           int
	maxMessagesPerSession int
	maxSessionSizeBytes   int
	sessionOrder          []string // For LRU eviction
	totalSessionsCreated  int64    // Track total sessions created
}

// NewSessionStore creates a new SessionStore instance
func NewSessionStore(idleTimeout time.Duration, maxSessions, maxMessagesPerSession, maxSessionSizeBytes int) *SessionStore {
	return &SessionStore{
		sessions:              make(map[string]*Session),
		validSessions:         make(map[string]bool),
		idleTimeout:           idleTimeout,
		maxSessions:           maxSessions,
		maxMessagesPerSession: maxMessagesPerSession,
		maxSessionSizeBytes:   maxSessionSizeBytes,
		sessionOrder:          make([]string, 0),
	}
}

// RegisterSession registers a session ID as valid (created via StartSession)
func (s *SessionStore) RegisterSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.validSessions[sessionID] = true
	s.totalSessionsCreated++
}

// IsValidSession checks if a session ID was created via StartSession
func (s *SessionStore) IsValidSession(sessionID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.validSessions[sessionID]
}

// getSessionSize calculates the memory usage of a session in bytes
func (s *SessionStore) getSessionSize(session *Session) int {
	size := 0
	for _, msg := range session.Messages {
		size += len(msg.Text) + len(msg.Role.String()) + 24 // approximate timestamp size
	}
	return size
}

// evictOldestSession removes the oldest session to make room for new ones
func (s *SessionStore) evictOldestSession() {
	if len(s.sessionOrder) == 0 {
		return
	}

	oldestSessionID := s.sessionOrder[0]
	s.sessionOrder = s.sessionOrder[1:]

	delete(s.sessions, oldestSessionID)
	delete(s.validSessions, oldestSessionID)
}

// updateSessionOrder moves a session to the end (most recently used)
func (s *SessionStore) updateSessionOrder(sessionID string) {
	// Remove from current position
	for i, id := range s.sessionOrder {
		if id == sessionID {
			s.sessionOrder = append(s.sessionOrder[:i], s.sessionOrder[i+1:]...)
			break
		}
	}
	// Add to end
	s.sessionOrder = append(s.sessionOrder, sessionID)
}

// AppendMessage adds a structured message to the session history
// Only works with valid session IDs and enforces limits
func (s *SessionStore) AppendMessage(sessionID string, role Role, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if session ID is valid (was created via StartSession)
	if !s.validSessions[sessionID] {
		return fmt.Errorf("invalid session ID: session not found or not properly created")
	}

	now := time.Now().UTC()

	// Create session if it doesn't exist
	if s.sessions[sessionID] == nil {
		// Check if we need to evict sessions to stay under the limit
		for len(s.sessions) >= s.maxSessions {
			s.evictOldestSession()
		}

		s.sessions[sessionID] = &Session{
			Messages:   make([]Message, 0),
			LastActive: now,
		}
		s.sessionOrder = append(s.sessionOrder, sessionID)
	}

	session := s.sessions[sessionID]

	// Check message limit per session
	if len(session.Messages) >= s.maxMessagesPerSession {
		return fmt.Errorf("session message limit exceeded: maximum %d messages per session", s.maxMessagesPerSession)
	}

	// Create new message
	message := Message{
		Role:      role,
		Text:      text,
		Timestamp: now,
	}

	// Check session size limit
	newSessionSize := s.getSessionSize(session) + len(text) + len(role.String()) + 24
	if newSessionSize > s.maxSessionSizeBytes {
		return fmt.Errorf("session size limit exceeded: maximum %d bytes per session", s.maxSessionSizeBytes)
	}

	// Add message to session
	session.Messages = append(session.Messages, message)
	session.LastActive = now

	// Update LRU order
	s.updateSessionOrder(sessionID)

	return nil
}

// GetMessages returns all structured messages for a session
// Returns empty slice if session doesn't exist
func (s *SessionStore) GetMessages(sessionID string) []Message {
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
func (s *SessionStore) GetFormattedMessages(sessionID string) []string {
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

// GetTotalSessionsCreated returns the total number of sessions created
func (s *SessionStore) GetTotalSessionsCreated() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalSessionsCreated
}

// GetAllSessionsInfo returns info about all active sessions
func (s *SessionStore) GetAllSessionsInfo() []struct {
	ID           string
	MessageCount int
	SizeBytes    int
	LastActive   string
} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]struct {
		ID           string
		MessageCount int
		SizeBytes    int
		LastActive   string
	}, 0, len(s.sessions))

	for sessionID, session := range s.sessions {
		result = append(result, struct {
			ID           string
			MessageCount int
			SizeBytes    int
			LastActive   string
		}{
			ID:           sessionID,
			MessageCount: len(session.Messages),
			SizeBytes:    s.getSessionSize(session),
			LastActive:   session.LastActive.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}

	return result
}

// GetMessagesAsLLMFormat returns messages in the format expected by LLM providers
func (s *SessionStore) GetMessagesAsLLMFormat(sessionID string) []llm.Message {
	messages := s.GetMessages(sessionID)
	result := make([]llm.Message, len(messages))

	for i, msg := range messages {
		result[i] = llm.Message{
			Role: msg.Role.String(),
			Text: msg.Text,
		}
	}

	return result
}

// CleanupIdleSessions removes sessions that have been idle for more than the configured timeout
func (s *SessionStore) CleanupIdleSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UTC().Add(-s.idleTimeout)
	toDelete := make([]string, 0)

	for sessionID, session := range s.sessions {
		if session.LastActive.Before(cutoff) {
			toDelete = append(toDelete, sessionID)
		}
	}

	// Remove from all tracking structures
	for _, sessionID := range toDelete {
		delete(s.sessions, sessionID)
		delete(s.validSessions, sessionID)

		// Remove from session order
		for i, id := range s.sessionOrder {
			if id == sessionID {
				s.sessionOrder = append(s.sessionOrder[:i], s.sessionOrder[i+1:]...)
				break
			}
		}
	}
}
