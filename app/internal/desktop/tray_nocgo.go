//go:build darwin && !cgo

// Su macOS l'icona richiede cgo. Questo file esiste solo perche'
// "GOOS=darwin CGO_ENABLED=0 go vet ./..." funzioni da Windows: un eseguibile
// costruito cosi' NON e' valido e alla partenza mostra un errore ed esce.
//
// Nota per la release (Task 11): Go disattiva cgo da solo quando si
// cross-compila (per esempio GOARCH=amd64 su un runner arm64), quindi entrambe
// le build macOS (arm64 e amd64) devono impostare CGO_ENABLED=1, e la CI deve
// controllare che per darwin
//   go list -f '{{.GoFiles}}' ./app/internal/desktop
// contenga tray.go (e non tray_nocgo.go) prima di firmare.

package desktop

import (
	"errors"
	"log"
	"os"

	"github.com/ncruces/zenity"
)

// MsgBadBuild e' il messaggio di una build macOS senza cgo.
const MsgBadBuild = "Build non valida: icona non disponibile (cgo disattivato)"

// Handlers sono le azioni del menu.
type Handlers struct {
	SendNow, Rename, ChooseMain, ChooseWowDir, OpenLogs, Quit func()
}

// Tray senza icona.
type Tray struct{}

// RunTray in una build senza cgo segnala l'errore ed esce con codice 1.
func RunTray(title, status string, h Handlers, ready func(*Tray), exit func()) {
	log.Print(MsgBadBuild)
	zenity.Error(MsgBadBuild, zenity.Title(AppName))
	if exit != nil {
		exit()
	}
	os.Exit(1)
}

// QuitTray non fa nulla: RunTray non ritorna mai.
func QuitTray() {}

func (t *Tray) SetStatus(string)      {}
func (t *Tray) SetUpdateError(string) {}
func (t *Tray) SetSending(bool)       {}

// CheckBuild segnala che questa build macOS non ha l'icona.
func CheckBuild() error { return errors.New(MsgBadBuild) }
