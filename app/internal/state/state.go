// Package state keeps which sessions were already queued/sent.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	fileName = "state.json"
	maxAge   = 60 * 24 * time.Hour
)

type State struct {
	Sent            map[string]int64 `json:"sent"` // session id -> epoch queued/sent
	LastUpdateCheck int64            `json:"lastUpdateCheck"`
}

func (s *State) MarkQueued(id string, now int64) {
	if s.Sent == nil {
		s.Sent = map[string]int64{}
	}
	s.Sent[id] = now
}

func (s *State) Has(id string) bool { _, ok := s.Sent[id]; return ok }

// Load reads dir/state.json pruning entries older than 60 days from time.Now().
func Load(dir string) (*State, error) { return LoadAt(dir, time.Now().Unix()) }

// LoadAt is Load with an explicit "now" (epoch seconds), for testing.
// A missing file yields an empty State with a non-nil map.
func LoadAt(dir string, now int64) (*State, error) {
	st := &State{Sent: map[string]int64{}}
	b, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return st, nil
		}
		return nil, fmt.Errorf("state: leggi: %w", err)
	}
	if err := json.Unmarshal(b, st); err != nil {
		return nil, fmt.Errorf("state: formato non valido: %w", err)
	}
	if st.Sent == nil {
		st.Sent = map[string]int64{}
	}
	cutoff := now - int64(maxAge/time.Second)
	for id, ts := range st.Sent {
		if ts < cutoff {
			delete(st.Sent, id)
		}
	}
	return st, nil
}

// Save writes state.json atomically.
func Save(dir string, st *State) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("state: crea cartella: %w", err)
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("state: serializza: %w", err)
	}
	f, err := os.CreateTemp(dir, ".tmp-state-*")
	if err != nil {
		return fmt.Errorf("state: file temporaneo: %w", err)
	}
	tmp := f.Name()
	_, werr := f.Write(b)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("state: scrittura: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, fileName)); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("state: rinomina: %w", err)
	}
	return nil
}
