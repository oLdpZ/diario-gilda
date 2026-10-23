// Package outbox is an on-disk queue of files waiting to be sent.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const tmpPrefix = ".tmp-"

// Put writes data atomically (temp file in dir + rename).
func Put(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("outbox: crea cartella: %w", err)
	}
	f, err := os.CreateTemp(dir, tmpPrefix+"*")
	if err != nil {
		return fmt.Errorf("outbox: file temporaneo: %w", err)
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("outbox: scrittura: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("outbox: rinomina: %w", err)
	}
	return nil
}

// Flush sends queued files in name order, deleting each one only after send
// returns nil. It stops at the first error, leaving the rest queued.
func Flush(ctx context.Context, dir string, send func(name string, data []byte) error) (sent int, err error) {
	entries, err := os.ReadDir(dir) // sorted by name
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("outbox: leggi cartella: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), tmpPrefix) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, n := range names {
		if err := ctx.Err(); err != nil {
			return sent, fmt.Errorf("outbox: %w", err)
		}
		p := filepath.Join(dir, n)
		data, err := os.ReadFile(p)
		if err != nil {
			return sent, fmt.Errorf("outbox: leggi %s: %w", n, err)
		}
		if err := send(n, data); err != nil {
			return sent, fmt.Errorf("outbox: invio %s: %w", n, err)
		}
		if err := os.Remove(p); err != nil {
			return sent, fmt.Errorf("outbox: elimina %s: %w", n, err)
		}
		sent++
	}
	return sent, nil
}
