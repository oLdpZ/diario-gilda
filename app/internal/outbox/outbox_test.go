package outbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func names(t *testing.T, dir string) []string {
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func TestFlushStopsAtFirstError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "outbox")
	for _, n := range []string{"c.gz", "a.gz", "b.gz"} {
		if err := Put(dir, n, []byte(n)); err != nil {
			t.Fatal(err)
		}
	}
	boom := errors.New("boom")
	var order []string
	sent, err := Flush(context.Background(), dir, nil, func(n string, d []byte) error {
		order = append(order, n)
		if n == "b.gz" {
			return boom
		}
		return nil
	})
	if sent != 1 || !errors.Is(err, boom) {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
	if got := names(t, dir); len(got) != 2 || got[0] != "b.gz" || got[1] != "c.gz" {
		t.Fatalf("left %v", got)
	}
	sent, err = Flush(context.Background(), dir, nil, func(string, []byte) error { return nil })
	if sent != 2 || err != nil || len(names(t, dir)) != 0 {
		t.Fatalf("sent=%d err=%v left=%v", sent, err, names(t, dir))
	}
}

func TestFlushMissingDirAndTempFiles(t *testing.T) {
	dir := t.TempDir()
	if n, err := Flush(context.Background(), filepath.Join(dir, "no"), nil, nil); n != 0 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
	_ = os.WriteFile(filepath.Join(dir, ".tmp-x"), []byte("x"), 0o644)
	_ = os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	n, err := Flush(context.Background(), dir, nil, func(string, []byte) error { t.Error("called"); return nil })
	if n != 0 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
}

func TestFlushCanceled(t *testing.T) {
	dir := t.TempDir()
	_ = Put(dir, "a", []byte("a"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err := Flush(ctx, dir, nil, func(string, []byte) error { return nil })
	if n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("%d %v", n, err)
	}
}

func TestFlushRejectedMovedAside(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "outbox")
	for _, n := range []string{"a.gz", "b.gz", "c.gz"} {
		if err := Put(dir, n, []byte(n)); err != nil {
			t.Fatal(err)
		}
	}
	var logs []string
	logf := func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	var order []string
	sent, err := Flush(context.Background(), dir, logf, func(n string, d []byte) error {
		order = append(order, n)
		if n == "b.gz" {
			return fmt.Errorf("discord: HTTP 413: troppo grande: %w", ErrRejected)
		}
		return nil
	})
	if sent != 2 || err != nil {
		t.Fatalf("sent=%d err=%v", sent, err)
	}
	if len(order) != 3 {
		t.Fatalf("Flush non e' andato avanti dopo il rifiuto: %v", order)
	}
	if got := names(t, dir); len(got) != 1 || got[0] != RejectedDir {
		t.Fatalf("rimasti in coda: %v", got)
	}
	if got := names(t, filepath.Join(dir, RejectedDir)); len(got) != 1 || got[0] != "b.gz" {
		t.Fatalf("scartati: %v", got)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "b.gz") || !strings.Contains(logs[0], RejectedDir) {
		t.Fatalf("log: %q", logs)
	}
	// la cartella degli scartati non si reinvia
	if n, err := Flush(context.Background(), dir, nil, func(string, []byte) error { t.Error("reinviato"); return nil }); n != 0 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
}
