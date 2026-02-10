package proxy

import (
	"sync"
	"time"
)

// PlaygroundChatMessage represents a single message in a chat conversation
type PlaygroundChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PlaygroundSession manages playground state for a user
type PlaygroundSession struct {
	ID       string
	Messages []PlaygroundChatMessage
	Created  time.Time
	Updated  time.Time
}

// PlaygroundSessionManager manages playground sessions
type PlaygroundSessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*PlaygroundSession
}

// NewPlaygroundSessionManager creates a new session manager
func NewPlaygroundSessionManager() *PlaygroundSessionManager {
	manager := &PlaygroundSessionManager{
		sessions: make(map[string]*PlaygroundSession),
	}

	// Start cleanup goroutine
	go manager.cleanupExpiredSessions()

	return manager
}

// GetOrCreateSession gets or creates a playground session
func (m *PlaygroundSessionManager) GetOrCreateSession(sessionID string) *PlaygroundSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		session = &PlaygroundSession{
			ID:       sessionID,
			Messages: make([]PlaygroundChatMessage, 0),
			Created:  time.Now(),
			Updated:  time.Now(),
		}
		m.sessions[sessionID] = session
	} else {
		session.Updated = time.Now()
	}

	return session
}

// AddMessage adds a message to a session
func (m *PlaygroundSessionManager) AddMessage(sessionID string, message PlaygroundChatMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, exists := m.sessions[sessionID]; exists {
		session.Messages = append(session.Messages, message)
		session.Updated = time.Now()
	}
}

// ClearSession clears all messages from a session
func (m *PlaygroundSessionManager) ClearSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, exists := m.sessions[sessionID]; exists {
		session.Messages = make([]PlaygroundChatMessage, 0)
		session.Updated = time.Now()
	}
}

// DeleteSession removes a session
func (m *PlaygroundSessionManager) DeleteSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, sessionID)
}

// cleanupExpiredSessions removes sessions older than 1 hour
func (m *PlaygroundSessionManager) cleanupExpiredSessions() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for id, session := range m.sessions {
			if now.Sub(session.Updated) > 1*time.Hour {
				delete(m.sessions, id)
			}
		}
		m.mu.Unlock()
	}
}

// GetMessages returns all messages for a session
func (m *PlaygroundSessionManager) GetMessages(sessionID string) []PlaygroundChatMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if session, exists := m.sessions[sessionID]; exists {
		// Return a copy
		messages := make([]PlaygroundChatMessage, len(session.Messages))
		copy(messages, session.Messages)
		return messages
	}

	return nil
}
