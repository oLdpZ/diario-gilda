//go:build !darwin || cgo

// L'icona usa fyne.io/systray: su Windows e Linux e' Go puro, su macOS richiede
// cgo (la build per macOS si fa sul runner macOS della CI con CGO_ENABLED=1).
// Senza cgo su macOS si compila tray_nocgo.go, che serve solo a "go vet".

package desktop

import (
	"runtime"

	"fyne.io/systray"
)

// Handlers sono le azioni del menu; ognuna parte in una goroutine sua.
type Handlers struct {
	SendNow, Rename, ChooseMain, ChooseWowDir, OpenLogs, Quit func()
}

// Tray e' il menu dell'icona gia' costruito. I metodi sono sicuri su un *Tray nil.
type Tray struct {
	status, updateLine, send *systray.MenuItem
}

// RunTray mostra l'icona e il menu e blocca finche' non si chiama QuitTray.
// Va chiamata dalla goroutine principale. ready riceve il menu pronto; exit
// viene chiamata prima che RunTray ritorni.
func RunTray(title, status string, h Handlers, ready func(*Tray), exit func()) {
	systray.Run(func() {
		if runtime.GOOS == "windows" {
			systray.SetIcon(ICO(IconPNG))
		} else {
			systray.SetIcon(IconPNG)
		}
		systray.SetTooltip(AppName)

		t := &Tray{}
		head := systray.AddMenuItem(title, "")
		head.Disable()
		t.status = systray.AddMenuItem(status, "")
		t.status.Disable()
		t.updateLine = systray.AddMenuItem("", "")
		t.updateLine.Disable()
		t.updateLine.Hide()
		systray.AddSeparator()
		t.send = systray.AddMenuItem(MenuSendNow, "")
		rename := systray.AddMenuItem(MenuRename, "")
		mainChar := systray.AddMenuItem(MenuMain, "")
		wowDir := systray.AddMenuItem(MenuWowDir, "")
		logs := systray.AddMenuItem(MenuOpenLogs, "")
		systray.AddSeparator()
		quit := systray.AddMenuItem(MenuQuit, "")

		on := func(mi *systray.MenuItem, f func()) {
			go func() {
				for range mi.ClickedCh {
					if f != nil {
						go f()
					}
				}
			}()
		}
		on(t.send, h.SendNow)
		on(rename, h.Rename)
		on(mainChar, h.ChooseMain)
		on(wowDir, h.ChooseWowDir)
		on(logs, h.OpenLogs)
		on(quit, h.Quit)
		if ready != nil {
			ready(t)
		}
	}, exit)
}

// QuitTray chiude l'icona: RunTray ritorna.
func QuitTray() { systray.Quit() }

// SetStatus cambia la riga dell'ultimo invio.
func (t *Tray) SetStatus(s string) {
	if t != nil {
		t.status.SetTitle(s)
	}
}

// SetUpdateError mostra la riga dell'aggiornamento fallito.
func (t *Tray) SetUpdateError(s string) {
	if t != nil {
		t.updateLine.SetTitle(s)
		t.updateLine.Show()
	}
}

// SetSending disabilita "Invia ora" durante un invio.
func (t *Tray) SetSending(on bool) {
	if t == nil {
		return
	}
	if on {
		t.send.Disable()
	} else {
		t.send.Enable()
	}
}

// CheckBuild dice se questa build puo' mostrare l'icona (qui sempre).
func CheckBuild() error { return nil }
