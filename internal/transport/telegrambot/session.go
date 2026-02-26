package telegrambot

import (
	"fmt"
	"sync"
	"time"
)

type sessionStep string

const (
	stepAwaitSymbol sessionStep = "await_symbol"
)

type session struct {
	ChatID    int64
	UserID    int64
	Step      sessionStep
	UpdatedAt time.Time
}

type sessionStore struct {
	mu   sync.Mutex
	ttl  time.Duration
	data map[string]*session
}

func newSessionStore(ttl time.Duration) *sessionStore {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &sessionStore{ttl: ttl, data: make(map[string]*session)}
}

func (s *sessionStore) get(chatID, userID int64) (*session, bool) {
	key := sessionKey(chatID, userID)
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.data[key]
	if !ok {
		return nil, false
	}
	if s.isExpired(sess) {
		delete(s.data, key)
		return nil, false
	}
	return sess, true
}

func (s *sessionStore) save(sess *session) {
	key := sessionKey(sess.ChatID, sess.UserID)
	sess.UpdatedAt = time.Now()
	s.mu.Lock()
	s.data[key] = sess
	s.mu.Unlock()
}

func (s *sessionStore) delete(chatID, userID int64) {
	key := sessionKey(chatID, userID)
	s.mu.Lock()
	delete(s.data, key)
	s.mu.Unlock()
}

func (s *sessionStore) isExpired(sess *session) bool {
	if sess == nil {
		return true
	}
	if sess.UpdatedAt.IsZero() {
		return false
	}
	return time.Since(sess.UpdatedAt) > s.ttl
}

func sessionKey(chatID, userID int64) string {
	return fmt.Sprintf("%d:%d", chatID, userID)
}
