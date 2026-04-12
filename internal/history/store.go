package history

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	MetadataWindowUsageKey = "window_usage_ns"
	MetadataAppUsageKey    = "app_usage_ns"
)

type SessionPeriod struct {
	Kind      string    `json:"kind"`
	Type      string    `json:"type"`
	Color     string    `json:"color"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

type ActivityStat struct {
	Name     string  `json:"name"`
	ActiveNS int64   `json:"active_ns"`
	Percent  float64 `json:"percent"`
}

type SessionRecord struct {
	Version          int             `json:"version,omitempty"`
	SessionStartedAt time.Time       `json:"session_started_at"`
	SessionEndedAt   time.Time       `json:"session_ended_at"`
	TotalActive      int64           `json:"total_active"`
	TotalInactive    int64           `json:"total_inactive"`
	TotalAdded       int64           `json:"total_added"`
	WindowCount      int             `json:"window_count,omitempty"`
	AppCount         int             `json:"app_count,omitempty"`
	TopWindows       []ActivityStat  `json:"top_windows,omitempty"`
	TopApps          []ActivityStat  `json:"top_apps,omitempty"`
	Periods          []SessionPeriod `json:"periods,omitempty"`
	Metadata         map[string]any  `json:"metadata,omitempty"`
}

func MetadataUsageMap(metadata map[string]any, key string) map[string]time.Duration {
	raw, ok := metadata[key]
	if !ok {
		return map[string]time.Duration{}
	}
	items, ok := raw.(map[string]any)
	if !ok {
		return map[string]time.Duration{}
	}

	out := make(map[string]time.Duration, len(items))
	for name, value := range items {
		switch v := value.(type) {
		case float64:
			out[name] = time.Duration(int64(v))
		case int64:
			out[name] = time.Duration(v)
		case int:
			out[name] = time.Duration(v)
		}
	}
	return out
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
