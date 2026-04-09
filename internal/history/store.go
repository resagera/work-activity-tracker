package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type SessionRecord struct {
	SessionStartedAt time.Time `json:"session_started_at"`
	SessionEndedAt   time.Time `json:"session_ended_at"`
	TotalActive      int64     `json:"total_active"`
	TotalInactive    int64     `json:"total_inactive"`
	TotalAdded       int64     `json:"total_added"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) LoadAll() ([]SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAllLocked()
}

func (s *Store) Last() (*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadAllLocked()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	last := records[len(records)-1]
	return &last, nil
}

func (s *Store) Save(record SessionRecord, replaceLast bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadAllLocked()
	if err != nil {
		return err
	}

	if replaceLast && len(records) > 0 {
		records[len(records)-1] = record
	} else {
		records = append(records, record)
	}

	return s.writeAllLocked(records)
}

func (s *Store) loadAllLocked() ([]SessionRecord, error) {
	if s.path == "" {
		return []SessionRecord{}, nil
	}

	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []SessionRecord{}, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return []SessionRecord{}, nil
	}

	var records []SessionRecord
	if err := json.Unmarshal(b, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) writeAllLocked(records []SessionRecord) error {
	if s.path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, b, 0o644)
}
