package main

import (
	"sync"
)

// SessionStore provides thread-safe storage for conversation history
// Layer 1: Simple []string storage as specified in the architecture document
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[uint32][]string
}

// NewSessionStore creates a new SessionStore instance
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[uint32][]string),
	}
}

// AppendMessage adds a message to the session history
// Creates session if it doesn't exist
func (s *SessionStore) AppendMessage(sessionID uint32, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sessions[sessionID] == nil {
		s.sessions[sessionID] = make([]string, 0)
	}

	s.sessions[sessionID] = append(s.sessions[sessionID], message)
}

// GetMessages returns all messages for a session
// Returns empty slice if session doesn't exist
func (s *SessionStore) GetMessages(sessionID uint32) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if messages, exists := s.sessions[sessionID]; exists {
		// Return a copy to prevent external modification
		result := make([]string, len(messages))
		copy(result, messages)
		return result
	}

	return []string{}
}

// GetSessionCount returns the number of active sessions
func (s *SessionStore) GetSessionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}
