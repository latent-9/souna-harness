package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/latent-9/souna-harness/internal/provider"
)

// Session is one conversation with its message history.
type Session struct {
	ID        string             `json:"id"`
	Title     string             `json:"title"`
	Provider  string             `json:"provider"`
	Model     string             `json:"model"`
	ParentID  string             `json:"parent_id,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	Messages  []provider.Message `json:"messages"`
}

// Store holds sessions in memory and persists them as JSON files.
type Store struct {
	dir      string
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewStore creates a store. When dir is non-empty, sessions are persisted
// there and existing ones are loaded.
func NewStore(dir string) (*Store, error) {
	s := &Store{dir: dir, sessions: make(map[string]*Session)}
	if dir == "" {
		return s, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		s.sessions[sess.ID] = &sess
	}
	return s, nil
}

// Create adds a new session and returns it.
func (s *Store) Create(title, providerName, model, parentID string) (*Session, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}
	sess := &Session{
		ID:        id,
		Title:     title,
		Provider:  providerName,
		Model:     model,
		ParentID:  parentID,
		CreatedAt: time.Now(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = sess
	if err := s.persist(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// Get returns the named session.
func (s *Store) Get(id string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	return sess, nil
}

// Append appends a message to a session and persists it.
func (s *Store) Append(id string, msg provider.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("session %q not found", id)
	}
	sess.Messages = append(sess.Messages, msg)
	return s.persist(sess)
}

// List returns all sessions, oldest first.
func (s *Store) List() []*Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// Depth returns the nesting depth of a session by walking its parent chain.
func (s *Store) Depth(id string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	depth := 0
	current, ok := s.sessions[id]
	if !ok {
		return 0, fmt.Errorf("session %q not found", id)
	}
	for current.ParentID != "" {
		depth++
		current, ok = s.sessions[current.ParentID]
		if !ok {
			return depth, nil
		}
	}
	return depth, nil
}

func (s *Store) persist(sess *Session) error {
	if s.dir == "" {
		return nil
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, sess.ID+".json"), data, 0o600)
}

func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
