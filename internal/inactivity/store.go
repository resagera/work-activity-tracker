package inactivity

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

const (
	TypeIdle          = "бездействие"
	TypeManualPause   = "ручная пауза"
	TypeLocked        = "экран заблокирован"
	TypeBlockedWindow = "исключенное окно"
)

var BuiltinTypes = []string{
	TypeManualPause,
	TypeIdle,
	TypeLocked,
	TypeBlockedWindow,
}

type Store struct {
	path string
	mu   sync.Mutex
}

func New(path string) *Store {
	return &Store{path: path}
}

func NormalizeName(name string) string {
	return strings.TrimSpace(name)
}

func All(custom []string) []string {
	out := append([]string{}, BuiltinTypes...)
	for _, name := range custom {
		name = NormalizeName(name)
		if name == "" || slices.Contains(out, name) {
			continue
		}
		out = append(out, name)
	}
	return out
}

func (s *Store) LoadAll() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAllLocked()
}

func (s *Store) Add(name string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name = NormalizeName(name)
	if name == "" {
		return nil, errors.New("name is required")
	}

	items, err := s.loadAllLocked()
	if err != nil {
		return nil, err
	}
	if !slices.Contains(items, name) && !slices.Contains(BuiltinTypes, name) {
		items = append(items, name)
	}
	if err := s.writeAllLocked(items); err != nil {
		return nil, err
	}
	return All(items), nil
}

func (s *Store) loadAllLocked() ([]string, error) {
	if s.path == "" {
		return []string{}, nil
	}

	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return []string{}, nil
	}

	var items []string
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) writeAllLocked(items []string) error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}
