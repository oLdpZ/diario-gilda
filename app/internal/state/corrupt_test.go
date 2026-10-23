package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCorruptIsErrCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{rotto"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAt(dir, 0); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("atteso ErrCorrupt, ottenuto %v", err)
	}
}

func TestLoadReadErrorIsNotErrCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "state.json"), 0o755); err != nil { // illeggibile come file
		t.Fatal(err)
	}
	_, err := LoadAt(dir, 0)
	if err == nil || errors.Is(err, ErrCorrupt) {
		t.Fatalf("atteso errore di lettura non ErrCorrupt, ottenuto %v", err)
	}
}
