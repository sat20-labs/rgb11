package storage

import (
	"errors"
	"sync"
)

// MemoryStore is a deterministic transactional store for tests, native tools
// and WASM hosts that provide persistence above the engine boundary.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{data: make(map[string][]byte)} }

func (s *MemoryStore) Get(key []byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.data[string(key)]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *MemoryStore) Begin() (Tx, error) {
	if s == nil {
		return nil, errors.New("nil RGB11 memory store")
	}
	return &memoryTx{store: s, writes: make(map[string]*[]byte)}, nil
}

type memoryTx struct {
	store  *MemoryStore
	writes map[string]*[]byte
	done   bool
}

func (tx *memoryTx) Put(key, value []byte) error {
	if tx.done || len(key) == 0 {
		return errors.New("invalid RGB11 memory transaction")
	}
	copyValue := append([]byte(nil), value...)
	tx.writes[string(key)] = &copyValue
	return nil
}

func (tx *memoryTx) Delete(key []byte) error {
	if tx.done || len(key) == 0 {
		return errors.New("invalid RGB11 memory transaction")
	}
	tx.writes[string(key)] = nil
	return nil
}

func (tx *memoryTx) Commit() error {
	if tx.done {
		return errors.New("RGB11 memory transaction already closed")
	}
	tx.store.mu.Lock()
	defer tx.store.mu.Unlock()
	for key, value := range tx.writes {
		if value == nil {
			delete(tx.store.data, key)
		} else {
			tx.store.data[key] = append([]byte(nil), (*value)...)
		}
	}
	tx.done = true
	return nil
}

func (tx *memoryTx) Rollback() error {
	if tx.done {
		return nil
	}
	tx.done = true
	tx.writes = nil
	return nil
}
