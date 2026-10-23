package desktop

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"

	"github.com/ncruces/zenity"
)

// AppName e' il titolo delle finestre.
const AppName = "Diario di Gilda"

// Dialogs e' la UI vera, con le finestre di sistema (zenity). Non testata.
type Dialogs struct{}

func canceled(err error) error {
	if errors.Is(err, zenity.ErrCanceled) {
		return ErrCanceled
	}
	return err
}

// SelectDir chiede una cartella.
func (Dialogs) SelectDir(title string) (string, error) {
	p, err := zenity.SelectFile(zenity.Directory(), zenity.Title(title))
	return p, canceled(err)
}

// Entry chiede un testo, proponendo def.
func (Dialogs) Entry(prompt, def string) (string, error) {
	s, err := zenity.Entry(prompt, zenity.Title(AppName), zenity.EntryText(def))
	return s, canceled(err)
}

// Error mostra un errore e aspetta che il giocatore lo chiuda.
func (Dialogs) Error(msg string) {
	if err := zenity.Error(msg, zenity.Title(AppName)); err != nil && !errors.Is(err, zenity.ErrCanceled) {
		log.Printf("finestra di errore non mostrata (%v): %s", err, msg)
	}
}

// Info mostra un messaggio e aspetta che il giocatore lo chiuda.
func (Dialogs) Info(msg string) {
	if err := zenity.Info(msg, zenity.Title(AppName)); err != nil && !errors.Is(err, zenity.ErrCanceled) {
		log.Printf("messaggio non mostrato (%v): %s", err, msg)
	}
}

// Notify mostra una notifica di sistema, senza aspettare.
func Notify(msg string) {
	log.Printf("notifica: %s", msg)
	if err := zenity.Notify(msg, zenity.Title(AppName)); err != nil {
		log.Printf("notifica non mostrata: %v", err)
	}
}

// OpenFolder apre dir nel gestore dei file del sistema.
func OpenFolder(dir string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", dir) // esce con 1 anche se riesce: non si aspetta
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("apertura di %s: %w", dir, err)
	}
	go cmd.Wait()
	return nil
}
