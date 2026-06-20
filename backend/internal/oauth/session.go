package oauth

import (
	"sync"
	"time"
)

// Session holds the transient state of an in-progress OAuth flow, kept between
// the start (authorize/device-code) and finish (exchange/poll) steps. Sessions
// are in-memory and short-lived: they expire after the flow timeout.
type Session struct {
	Provider    string
	Flow        FlowType
	State       string
	Verifier    string
	RedirectURI string

	// Device-code fields.
	DeviceCode string
	Interval   int

	// Kiro (AWS SSO OIDC) device-flow fields. The registered client credentials
	// are needed to poll for and later refresh tokens.
	KiroClientID     string
	KiroClientSecret string
	KiroRegion       string
	KiroStartURL     string
	KiroAuthMethod   string // builder-id | idc

	// Extra carries custom-flow state (qoder nonce/verifier/machine id,
	// codebuddy state, kilocode code, cursor machine id) between steps.
	Extra map[string]string

	ExpiresAt time.Time
}

// sessionTTL bounds how long an in-progress OAuth session is retained.
const sessionTTL = 10 * time.Minute

// SessionStore is a concurrency-safe in-memory map of flow sessions keyed by
// state (auth-code) or a generated session id (device-code).
type SessionStore struct {
	mu sync.Mutex
	m  map[string]*Session
}

// NewSessionStore builds an empty session store.
func NewSessionStore() *SessionStore {
	return &SessionStore{m: make(map[string]*Session)}
}

// Put stores a session under key, stamping its expiry.
func (s *SessionStore) Put(key string, sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.ExpiresAt = time.Now().Add(sessionTTL)
	s.m[key] = sess
	s.gcLocked()
}

// Get returns the session for key, or false if absent/expired.
func (s *SessionStore) Get(key string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.m[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		delete(s.m, key)
		return nil, false
	}
	return sess, true
}

// Delete removes a session (called when a flow completes).
func (s *SessionStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}

// gcLocked drops expired sessions. Caller must hold the lock.
func (s *SessionStore) gcLocked() {
	now := time.Now()
	for k, v := range s.m {
		if now.After(v.ExpiresAt) {
			delete(s.m, k)
		}
	}
}