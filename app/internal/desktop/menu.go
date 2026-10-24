package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Testi del menu dell'icona.
const (
	MenuSendNow  = "Invia ora"
	MenuRename   = "Cambia nome…"
	MenuWowDir   = "Scegli cartella di WoW…"
	MenuOpenLogs = "Apri cartella dei registri"
	MenuQuit     = "Esci"
	NoSendYet    = "Nessun invio ancora"
	MsgAddonNew  = "Addon aggiornato: scrivi /reload in gioco."
	MsgRunning   = "Diario di Gilda è già in esecuzione: lo trovi vicino all'orologio."
	UpdateFailed = "Aggiornamento non riuscito: "
)

// TitleLabel e' la prima riga del menu (disabilitata).
func TitleLabel(version string) string { return "Diario di Gilda " + version }

// LastSentLabel descrive l'ultimo invio nel fuso di now: solo l'ora se e' di
// oggi, giorno e mese se di quest'anno, altrimenti la data intera.
func LastSentLabel(t, now time.Time) string {
	if t.IsZero() {
		return NoSendYet
	}
	t = t.In(now.Location())
	layout := "02/01/2006 15:04"
	switch {
	case t.Year() == now.Year() && t.YearDay() == now.YearDay():
		layout = "15:04"
	case t.Year() == now.Year():
		layout = "02/01 15:04"
	}
	return "Ultimo invio: " + t.Format(layout)
}

// MaxMenuLabel e' la lunghezza massima (in caratteri) di una riga del menu.
const MaxMenuLabel = 80

// UpdateFailedLabel e' la riga del menu che segnala un aggiornamento fallito:
// su una sola riga e accorciata con "…" se troppo lunga.
func UpdateFailedLabel(msg string) string {
	r := []rune(UpdateFailed + strings.Join(strings.Fields(msg), " "))
	if len(r) > MaxMenuLabel {
		r = append(r[:MaxMenuLabel-1], '…')
	}
	return string(r)
}

const lastSentFile = "ultimo-invio"

// LoadLastSent legge l'ora dell'ultimo invio riuscito; zero se non c'e'.
func LoadLastSent(dir string) time.Time {
	b, err := os.ReadFile(filepath.Join(dir, lastSentFile))
	if err != nil {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

// SaveLastSent salva l'ora dell'ultimo invio riuscito (secondi Unix).
func SaveLastSent(dir string, t time.Time) error {
	p := filepath.Join(dir, lastSentFile)
	if err := os.WriteFile(p+".tmp", []byte(strconv.FormatInt(t.Unix(), 10)+"\n"), 0o644); err != nil {
		return fmt.Errorf("ultimo invio: %w", err)
	}
	if err := os.Rename(p+".tmp", p); err != nil {
		return fmt.Errorf("ultimo invio: %w", err)
	}
	return nil
}
