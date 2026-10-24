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

// RejectedDir is the subfolder where Flush moves files rejected for good.
const RejectedDir = "scartati"

// ErrRejected, wrapped in an error returned by send, means the file will never
// be accepted (e.g. Discord answered 400/413): Flush moves it to RejectedDir
// and goes on with the next one.
var ErrRejected = errors.New("file rifiutato definitivamente")

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
	serr := f.Sync() // su disco prima della rinomina: niente file vuoti dopo un blackout
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
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
// returns nil. A file whose error wraps ErrRejected is moved to RejectedDir
// (logged with logf, which may be nil) and Flush goes on; any other error
// stops it, leaving the rest queued.
func Flush(ctx context.Context, dir string, logf func(format string, args ...any), send func(name string, data []byte) error) (sent int, err error) {
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
			if errors.Is(err, ErrRejected) {
				if merr := reject(dir, n); merr != nil {
					return sent, fmt.Errorf("outbox: %s rifiutato (%v), non spostato: %w", n, err, merr)
				}
				if logf != nil {
					logf("outbox: %s rifiutato (%v): spostato in %s", n, err, RejectedDir)
				}
				continue
			}
			return sent, fmt.Errorf("outbox: invio %s: %w", n, err)
		}
		if err := os.Remove(p); err != nil {
			return sent, fmt.Errorf("outbox: elimina %s: %w", n, err)
		}
		sent++
	}
	return sent, nil
}

func reject(dir, name string) error {
	rd := filepath.Join(dir, RejectedDir)
	if err := os.MkdirAll(rd, 0o755); err != nil {
		return err
	}
	return os.Rename(filepath.Join(dir, name), filepath.Join(rd, name))
}
