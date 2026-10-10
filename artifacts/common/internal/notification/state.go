package notification

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const maxStateSize = 4 << 20

type stateFile struct {
	Initialized bool                 `json:"initialized"`
	Sent        map[string]time.Time `json:"sent"`
}

type StateStore struct {
	mu   sync.RWMutex
	path string
	data stateFile
}

func OpenState(path string) (*StateStore, error) {
	store := &StateStore{
		path: path,
		data: stateFile{Sent: make(map[string]time.Time)},
	}

	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect notifier state: %w", err)
	}
	if info.Size() > maxStateSize {
		return nil, errors.New("notifier state exceeds 4 MiB")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read notifier state: %w", err)
	}
	if err := json.Unmarshal(raw, &store.data); err != nil {
		return nil, fmt.Errorf("decode notifier state: %w", err)
	}
	if store.data.Sent == nil {
		store.data.Sent = make(map[string]time.Time)
	}
	return store, nil
}

func (s *StateStore) Initialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Initialized
}

func (s *StateStore) Contains(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data.Sent[key]
	return ok
}

func (s *StateStore) Mark(keys ...string) error {
	return s.record(true, keys...)
}

func (s *StateStore) Bootstrap(keys ...string) error {
	return s.record(true, keys...)
}

func (s *StateStore) record(initialized bool, keys ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Initialized = initialized
	now := time.Now().UTC()
	for index, key := range keys {
		if key == "" {
			continue
		}
		s.data.Sent[key] = now.Add(time.Duration(index))
	}
	s.prune()
	return s.persist()
}

func (s *StateStore) prune() {
	if len(s.data.Sent) <= 2000 {
		return
	}

	type entry struct {
		key  string
		time time.Time
	}
	entries := make([]entry, 0, len(s.data.Sent))
	for key, recordedAt := range s.data.Sent {
		entries = append(entries, entry{key: key, time: recordedAt})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].time.Equal(entries[j].time) {
			return entries[i].key > entries[j].key
		}
		return entries[i].time.After(entries[j].time)
	})
	for _, old := range entries[1000:] {
		delete(s.data.Sent, old.key)
	}
}

func (s *StateStore) persist() error {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create notifier state directory: %w", err)
	}

	temporary, err := os.CreateTemp(directory, ".state-*.json")
	if err != nil {
		return fmt.Errorf("create temporary notifier state: %w", err)
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary notifier state: %w", err)
	}
	if err := json.NewEncoder(temporary).Encode(s.data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode notifier state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync notifier state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close notifier state: %w", err)
	}
	if err := os.Rename(temporaryName, s.path); err != nil {
		return fmt.Errorf("replace notifier state: %w", err)
	}
	removeTemporary = false
	return nil
}
