package desktop

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// TestLogFileRotateFails: se il rename non riesce si continua sullo stesso
// file, si segnala una volta sola e si riprova solo dopo altri max byte.
func TestLogFileRotateFails(t *testing.T) {
	p := filepath.Join(t.TempDir(), "diario.log")
	w, err := OpenLog(p, 100)
	if err != nil {
		t.Fatal(err)
	}
	tries := 0
	w.rename = func(string, string) error { tries++; return errors.New("file in uso") }
	line := append(bytes.Repeat([]byte("c"), 49), '\n') // 50 byte
	for i := 0; i < 6; i++ {                            // 300 byte: 1-2 tentativi
		if _, err := w.Write(line); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	w.Close()
	if tries < 1 || tries > 2 {
		t.Errorf("tentativi di rotazione = %d, voglio 1 o 2", tries)
	}
	b, _ := os.ReadFile(p)
	if n := strings.Count(string(b), "rotazione del log non riuscita"); n != 1 {
		t.Errorf("avvisi nel log = %d, voglio 1\n%s", n, b)
	}
	if n := strings.Count(string(b), string(line)); n != 6 {
		t.Errorf("righe scritte = %d, voglio 6", n)
	}
}
