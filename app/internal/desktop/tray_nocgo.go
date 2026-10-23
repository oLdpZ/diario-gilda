//go:build darwin && !cgo

// Su macOS l'icona richiede cgo. Questo file permette "GOOS=darwin
// CGO_ENABLED=0 go vet ./..." da Windows; la build vera per macOS si fa con cgo.

package desktop

import (
	"log"
	"sync"
)

// Handlers sono le azioni del menu.
type Handlers struct {
	SendNow, Rename, OpenLogs, Quit func()
}

// Tray senza icona: i metodi non fanno nulla.
type Tray struct{}

var (
	quitOnce sync.Once
	quitCh   = make(chan struct{})
)

// RunTray senza cgo non mostra l'icona: l'app lavora in silenzio finche'
// non si chiama QuitTray.
func RunTray(title, status string, h Handlers, ready func(*Tray), exit func()) {
	log.Printf("icona non disponibile: build per macOS senza cgo")
	if ready != nil {
		ready(&Tray{})
	}
	<-quitCh
	if exit != nil {
		exit()
	}
}

// QuitTray fa ritornare RunTray.
func QuitTray() { quitOnce.Do(func() { close(quitCh) }) }

func (t *Tray) SetStatus(string)      {}
func (t *Tray) SetUpdateError(string) {}
func (t *Tray) SetSending(bool)       {}
