package desktop

import (
	"errors"
	"testing"
)

func TestLock(t *testing.T) {
	dir := t.TempDir()
	l1, err := Acquire(dir)
	if err != nil {
		t.Fatalf("primo Acquire: %v", err)
	}
	if _, err := Acquire(dir); !errors.Is(err, ErrLocked) {
		t.Fatalf("secondo Acquire: err = %v, voglio ErrLocked", err)
	}
	if err := l1.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	l2, err := Acquire(dir)
	if err != nil {
		t.Fatalf("Acquire dopo Release: %v", err)
	}
	if err := l2.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := l2.Release(); err != nil {
		t.Errorf("secondo Release: %v", err)
	}
}

func TestLockOtherDir(t *testing.T) {
	a, err := Acquire(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	b, err := Acquire(t.TempDir())
	if err != nil {
		t.Fatalf("cartella diversa: %v", err)
	}
	b.Release()
}

// TestLockStaleFile: un file di lock rimasto da un crash non blocca.
func TestLockStaleFile(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	l.Release() // il file resta su disco, come dopo un crash
	l, err = Acquire(dir)
	if err != nil {
		t.Fatalf("file di lock rimasto blocca: %v", err)
	}
	l.Release()
}
