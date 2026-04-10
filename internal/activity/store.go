package activity

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

const TypeWork = "работа"

type TypeDefinition struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

var BuiltinTypes = []TypeDefinition{
	{Name: TypeWork, Color: "#20256a"},
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

func NormalizeColor(color string) string {
	return strings.TrimSpace(color)
}

func DefaultType(name string) string {
	name = NormalizeName(name)
	if name == "" {
		return TypeWork
	}
	return name
}

func builtinByName(name string) (TypeDefinition, bool) {
	for _, item := range BuiltinTypes {
		if item.Name == name {
			return item, true
		}
	}
	return TypeDefinition{}, false
}

func Merge(custom []TypeDefinition) []TypeDefinition {
	out := append([]TypeDefinition{}, BuiltinTypes...)
	for _, item := range custom {
		item.Name = NormalizeName(item.Name)
		item.Color = NormalizeColor(item.Color)
		if item.Name == "" {
			continue
		}

		idx := slices.IndexFunc(out, func(existing TypeDefinition) bool {
			return existing.Name == item.Name
		})
		if idx >= 0 {
			if item.Color != "" {
				out[idx].Color = item.Color
			}
			continue
		}
		out = append(out, item)
	}
	return out
}

func Names(items []TypeDefinition) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item.Name != "" {
			out = append(out, item.Name)
		}
	}
	return out
}

func FindColor(items []TypeDefinition, name string) string {
	name = NormalizeName(name)
	for _, item := range items {
		if item.Name == name {
			return item.Color
		}
	}
	return ""
}

func (s *Store) LoadAll() ([]TypeDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAllLocked()
}

func (s *Store) Add(name, color string) ([]TypeDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name = NormalizeName(name)
	color = NormalizeColor(color)
	if name == "" {
		return nil, errors.New("name is required")
	}

	items, err := s.loadAllLocked()
	if err != nil {
		return nil, err
	}

	if builtin, ok := builtinByName(name); ok {
		if color != "" {
			items = upsertCustom(items, TypeDefinition{Name: name, Color: color}, builtin.Color)
		}
		if err := s.writeAllLocked(items); err != nil {
			return nil, err
		}
		return Merge(items), nil
	}

	items = upsertCustom(items, TypeDefinition{Name: name, Color: color}, "")
	if err := s.writeAllLocked(items); err != nil {
		return nil, err
	}
	return Merge(items), nil
}

func (s *Store) SetColor(name, color string) ([]TypeDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name = NormalizeName(name)
	color = NormalizeColor(color)
	if name == "" {
		return nil, errors.New("name is required")
	}
	if color == "" {
		return nil, errors.New("color is required")
	}

	items, err := s.loadAllLocked()
	if err != nil {
		return nil, err
	}

	builtinColor := ""
	if builtin, ok := builtinByName(name); ok {
		builtinColor = builtin.Color
	}
	items = upsertCustom(items, TypeDefinition{Name: name, Color: color}, builtinColor)
	if err := s.writeAllLocked(items); err != nil {
		return nil, err
	}
	return Merge(items), nil
}

func upsertCustom(items []TypeDefinition, item TypeDefinition, builtinColor string) []TypeDefinition {
	idx := slices.IndexFunc(items, func(existing TypeDefinition) bool {
		return existing.Name == item.Name
	})
	if idx >= 0 {
		if item.Color != "" {
			items[idx].Color = item.Color
		}
		return items
	}
	if item.Color == builtinColor {
		return items
	}
	return append(items, item)
}

func (s *Store) loadAllLocked() ([]TypeDefinition, error) {
	if s.path == "" {
		return []TypeDefinition{}, nil
	}

	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []TypeDefinition{}, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return []TypeDefinition{}, nil
	}

	var defs []TypeDefinition
	if err := json.Unmarshal(b, &defs); err == nil {
		for i := range defs {
			defs[i].Name = NormalizeName(defs[i].Name)
			defs[i].Color = NormalizeColor(defs[i].Color)
		}
		return defs, nil
	}

	var names []string
	if err := json.Unmarshal(b, &names); err != nil {
		return nil, err
	}
	defs = make([]TypeDefinition, 0, len(names))
	for _, name := range names {
		name = NormalizeName(name)
		if name == "" {
			continue
		}
		defs = append(defs, TypeDefinition{Name: name})
	}
	return defs, nil
}

func (s *Store) writeAllLocked(items []TypeDefinition) error {
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
