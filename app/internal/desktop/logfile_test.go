package desktop

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLogFileRotates(t *testing.T) {
	p := filepath.Join(t.TempDir(), "diario.log")
	w, err := OpenLog(p, 100)
	if err != nil {
		t.Fatal(err)
	}
	line := append(bytes.Repeat([]byte("a"), 39), '\n') // 40 byte
	for i := 0; i < 2; i++ {
		if _, err := w.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(p + ".1"); !os.IsNotExist(err) {
		t.Fatalf("ruotato troppo presto: %v", err)
	}
	third := "terza riga che supera il limite\n" // 80+32 > 100
	w.Write([]byte(third))
	w.Write([]byte("quarta\n"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	old, err := os.ReadFile(p + ".1")
	if err != nil {
		t.Fatalf("backup assente: %v", err)
	}
	if len(old) != 80 {
		t.Errorf("backup = %d byte, voglio 80", len(old))
	}
	cur, _ := os.ReadFile(p)
	if string(cur) != third+"quarta\n" {
		t.Errorf("file attuale = %q", cur)
	}

	// riapertura: riparte dalla dimensione esistente e tiene un solo backup
	w, err = OpenLog(p, 100)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(bytes.Repeat([]byte("b"), 70))
	w.Close()
	old, _ = os.ReadFile(p + ".1")
	if string(old) != third+"quarta\n" {
		t.Errorf("secondo backup = %q", old)
	}
	if _, err := os.Stat(p + ".2"); !os.IsNotExist(err) {
		t.Errorf("c'e' un secondo backup")
	}
}

func TestLogFileBigWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "diario.log")
	w, err := OpenLog(p, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if n, err := w.Write(bytes.Repeat([]byte("x"), 25)); err != nil || n != 25 {
		t.Fatalf("Write = %d, %v", n, err)
	}
}
