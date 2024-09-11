package sesh

import (
	"context"
	"sync"
	"time"
)

func newMemoryStore() *memoryStore {
	return &memoryStore{
		sync.Mutex{},
		map[string]memorySession{},
	}
}

// memoryStore is the default session store
type memoryStore struct {
	mu       sync.Mutex
	sessions map[string]memorySession
}

var _ Store = (*memoryStore)(nil)

type memorySession struct {
	data   []byte
	expiry time.Time
}

func (s *memoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func (s *memoryStore) Find(_ context.Context, id string) (data []byte, expiry time.Time, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil, expiry, nil
	}
	return session.data, session.expiry, nil
}

func (s *memoryStore) Upsert(_ context.Context, id string, data []byte, expiry time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = memorySession{data, expiry}
	return nil
}
