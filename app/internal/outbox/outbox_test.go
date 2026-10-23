package outbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	sent, err := Flush(context.Background(), dir, func(n string, d []byte) error {
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
	sent, err = Flush(context.Background(), dir, func(string, []byte) error { return nil })
	if sent != 2 || err != nil || len(names(t, dir)) != 0 {
		t.Fatalf("sent=%d err=%v left=%v", sent, err, names(t, dir))
	}
}

func TestFlushMissingDirAndTempFiles(t *testing.T) {
	dir := t.TempDir()
	if n, err := Flush(context.Background(), filepath.Join(dir, "no"), nil); n != 0 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
	_ = os.WriteFile(filepath.Join(dir, ".tmp-x"), []byte("x"), 0o644)
	_ = os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	n, err := Flush(context.Background(), dir, func(string, []byte) error { t.Error("called"); return nil })
	if n != 0 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
}

func TestFlushCanceled(t *testing.T) {
	dir := t.TempDir()
	_ = Put(dir, "a", []byte("a"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err := Flush(ctx, dir, func(string, []byte) error { return nil })
	if n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("%d %v", n, err)
	}
}
