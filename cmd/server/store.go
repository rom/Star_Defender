package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"stardefender/internal/level"
)

const (
	maxScores       = 100
	maxCustomLevels = 500
	maxScoreValue   = 50_000_000
)

type Score struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
	Level int    `json:"level"`
	Time  int64  `json:"time"`
}

var (
	errNotFound = errors.New("level not found")
	errBuiltin  = errors.New("built-in levels cannot be changed")
	errFull     = errors.New("the server's level library is full")
)

// store keeps everything in memory and mirrors it to two JSON files.
type store struct {
	mu     sync.Mutex
	dir    string
	scores []Score
	levels map[string]level.Level
}

func openStore(dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &store{dir: dir, levels: map[string]level.Level{}}
	if err := readJSON(filepath.Join(dir, "scores.json"), &s.scores); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(dir, "levels.json"), &s.levels); err != nil {
		return nil, err
	}
	return s, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// writeJSON replaces path atomically so a crash never leaves half a file.
func writeJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *store) topScores(n int) []Score {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.scores) {
		n = len(s.scores)
	}
	return append([]Score{}, s.scores[:n]...)
}

// addScore inserts sc and returns its 1-based rank (0 if it did not make the table).
func (s *store) addScore(sc Score) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := sort.Search(len(s.scores), func(i int) bool { return s.scores[i].Score < sc.Score })
	if i >= maxScores {
		return 0, nil
	}
	s.scores = append(s.scores, Score{})
	copy(s.scores[i+1:], s.scores[i:])
	s.scores[i] = sc
	if len(s.scores) > maxScores {
		s.scores = s.scores[:maxScores]
	}
	return i + 1, writeJSON(filepath.Join(s.dir, "scores.json"), s.scores)
}

func builtinByID(id string) *level.Level {
	for i := range level.Builtin() {
		if level.Builtin()[i].ID == id {
			return &level.Builtin()[i]
		}
	}
	return nil
}

func (s *store) listLevels() []level.Summary {
	out := []level.Summary{}
	for i := range level.Builtin() {
		out = append(out, level.Builtin()[i].Summary())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	custom := make([]level.Level, 0, len(s.levels))
	for _, l := range s.levels {
		custom = append(custom, l)
	}
	sort.Slice(custom, func(i, j int) bool {
		if custom[i].Updated != custom[j].Updated {
			return custom[i].Updated > custom[j].Updated
		}
		return custom[i].ID < custom[j].ID
	})
	for i := range custom {
		out = append(out, custom[i].Summary())
	}
	return out
}

func (s *store) getLevel(id string) (level.Level, error) {
	if b := builtinByID(id); b != nil {
		return *b, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.levels[id]; ok {
		return l, nil
	}
	return level.Level{}, errNotFound
}

// saveLevel stores l. A level without an id, or carrying the id of a
// built-in or unknown level, is saved as a new custom level.
func (s *store) saveLevel(l level.Level) (level.Level, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.levels[l.ID]; !exists {
		if len(s.levels) >= maxCustomLevels {
			return l, errFull
		}
		var b [6]byte
		if _, err := rand.Read(b[:]); err != nil {
			return l, err
		}
		l.ID = "c-" + hex.EncodeToString(b[:])
	}
	l.Builtin = false
	l.Updated = time.Now().Unix()
	s.levels[l.ID] = l
	return l, writeJSON(filepath.Join(s.dir, "levels.json"), s.levels)
}

func (s *store) deleteLevel(id string) error {
	if builtinByID(id) != nil {
		return errBuiltin
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.levels[id]; !ok {
		return errNotFound
	}
	delete(s.levels, id)
	return writeJSON(filepath.Join(s.dir, "levels.json"), s.levels)
}
