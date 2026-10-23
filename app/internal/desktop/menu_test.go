package desktop

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTitleLabel(t *testing.T) {
	if got := TitleLabel("0.2.0"); got != "Diario di Gilda 0.2.0" {
		t.Errorf("TitleLabel = %q", got)
	}
}

func TestLastSentLabel(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	now := time.Date(2026, 10, 24, 21, 30, 0, 0, loc)
	cases := []struct {
		t    time.Time
		want string
	}{
		{time.Time{}, "Nessun invio ancora"},
		{time.Date(2026, 10, 24, 20, 5, 0, 0, loc), "Ultimo invio: 20:05"},
		{time.Date(2026, 10, 24, 19, 5, 0, 0, time.UTC), "Ultimo invio: 20:05"}, // nel fuso di now
		{time.Date(2026, 10, 23, 23, 59, 0, 0, loc), "Ultimo invio: 23/10 23:59"},
		{time.Date(2025, 10, 24, 8, 0, 0, 0, loc), "Ultimo invio: 24/10/2025 08:00"},
	}
	for _, c := range cases {
		if got := LastSentLabel(c.t, now); got != c.want {
			t.Errorf("LastSentLabel(%v) = %q, voglio %q", c.t, got, c.want)
		}
	}
}

func TestLastSentFile(t *testing.T) {
	dir := t.TempDir()
	if got := LoadLastSent(dir); !got.IsZero() {
		t.Fatalf("senza file = %v", got)
	}
	ts := time.Unix(1_790_000_000, 0)
	if err := SaveLastSent(dir, ts); err != nil {
		t.Fatal(err)
	}
	if got := LoadLastSent(dir); !got.Equal(ts) {
		t.Errorf("LoadLastSent = %v, voglio %v", got, ts)
	}
}

func TestUpdateFailedLabel(t *testing.T) {
	if got := UpdateFailedLabel("cartella di sola lettura"); got != "Aggiornamento non riuscito: cartella di sola lettura" {
		t.Errorf("UpdateFailedLabel = %q", got)
	}
	long := UpdateFailedLabel(strings.Repeat("è", 200) + "\nseconda riga")
	if n := utf8.RuneCountInString(long); n > MaxMenuLabel || !strings.HasSuffix(long, "…") || strings.Contains(long, "\n") {
		t.Errorf("etichetta lunga = %q (%d caratteri)", long, n)
	}
}
