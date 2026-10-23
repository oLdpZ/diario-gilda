// Comando diario: il programmino del Diario di Gilda. Vive vicino all'orologio,
// installa l'addon, raccoglie le sessioni finite e le invia a Discord, si
// aggiorna da solo. La logica testabile sta in internal/desktop e negli altri
// pacchetti; qui c'e' solo la colla.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oLdpZ/diario-gilda/app/internal/addoninstall"
	"github.com/oLdpZ/diario-gilda/app/internal/autostart"
	"github.com/oLdpZ/diario-gilda/app/internal/build"
	"github.com/oLdpZ/diario-gilda/app/internal/config"
	"github.com/oLdpZ/diario-gilda/app/internal/desktop"
	"github.com/oLdpZ/diario-gilda/app/internal/discord"
	"github.com/oLdpZ/diario-gilda/app/internal/run"
	"github.com/oLdpZ/diario-gilda/app/internal/update"
	"github.com/oLdpZ/diario-gilda/app/internal/wowdir"
)

const (
	apiBase     = "https://api.github.com"
	repo        = "oLdpZ/diario-gilda"
	sendTimeout = 2 * time.Minute
	pollEvery   = 10 * time.Second
	stableFor   = 5 * time.Second
	stopWait    = 10 * time.Second
	msgMoveApp  = "Sposta il Diario di Gilda nella cartella Applicazioni e riaprilo da lì: così potrà partire da solo e aggiornarsi."
)

func main() {
	if err := start(); err != nil {
		log.Printf("errore: %v", err)
		os.Exit(1)
	}
}

func start() error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cartella dati: %w", err)
	}
	// prima il lock, poi il log: un secondo avvio esce senza toccare diario.log
	lock, err := desktop.Acquire(dir)
	if errors.Is(err, desktop.ErrLocked) {
		return nil
	}
	if err != nil {
		return err
	}
	defer lock.Release()
	if lf, err := desktop.OpenLog(filepath.Join(dir, "diario.log"), desktop.LogMax); err == nil {
		log.SetOutput(lf)
		defer lf.Close()
	}
	log.Printf("avvio Diario di Gilda %s (%s/%s)", build.Version, runtime.GOOS, runtime.GOARCH)

	update.CleanupOld()
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("percorso dell'eseguibile: %w", err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	// macOS: un'app aperta dalla cartella Download gira da una copia temporanea
	// di sola lettura; non si puo' ne' avviare da sola ne' aggiornare.
	translocated := runtime.GOOS == "darwin" && strings.Contains(exe, "/AppTranslocation/")

	ui := desktop.Dialogs{}
	cfg, err := config.Load(dir)
	if err != nil {
		log.Printf("configurazione illeggibile, la si rifa': %v", err)
		cfg = config.Config{}
	}
	firstRun := desktop.NeedsSetup(cfg)
	cfg, err = desktop.FirstRun(cfg, desktop.Setup{
		Roots: wowdir.DefaultRoots(runtime.GOOS),
		UI:    ui,
		Save:  func(c config.Config) error { return config.Save(dir, c) },
		Enable: func() error {
			if translocated {
				return errors.New("app in una posizione temporanea (App Translocation)")
			}
			return autostart.Enable(exe)
		},
	})
	if errors.Is(err, desktop.ErrCanceled) {
		log.Printf("primo avvio annullato")
		return nil
	}
	if err != nil {
		ui.Error("Non riesco a salvare la configurazione: " + err.Error())
		return err
	}
	if translocated {
		if firstRun {
			ui.Info(msgMoveApp)
		}
	} else if !firstRun && autostart.Enabled() {
		// l'app potrebbe essere stata spostata: aggiorna il percorso
		if err := autostart.Enable(exe); err != nil {
			log.Printf("avvio automatico: %v", err)
		}
	}
	log.Printf("client: %s", cfg.ClientDir)

	installAddon(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	a := &app{dir: dir, exe: exe, ui: ui, ctx: ctx, stop: cancel, cfg: cfg, lock: lock}
	a.lastSent = desktop.LoadLastSent(dir)
	desktop.RunTray(desktop.TitleLabel(build.Version), desktop.LastSentLabel(a.lastSent, time.Now()),
		desktop.Handlers{SendNow: a.sendNow, Rename: a.rename, OpenLogs: a.openLogs, Quit: desktop.QuitTray},
		a.ready, a.exit)
	log.Printf("uscita")
	return nil
}

// installAddon installa l'addon incorporato se manca o e' di un'altra versione.
func installAddon(cfg config.Config) {
	if fi, err := os.Stat(cfg.ClientDir); err != nil || !fi.IsDir() {
		log.Printf("cartella del client assente (%s): addon non installato", cfg.ClientDir)
		return
	}
	addons := wowdir.AddOnsDir(cfg.ClientDir)
	if !addoninstall.NeedsInstall(addoninstall.Embedded, addons) {
		return
	}
	if err := addoninstall.Install(addoninstall.Embedded, addons); err != nil {
		log.Printf("installazione addon: %v", err)
		desktop.Notify("Non riesco a installare l'addon: " + err.Error())
		return
	}
	log.Printf("addon %s installato in %s", addoninstall.Version(addoninstall.Embedded), addons)
	if desktop.WoWRunning() {
		desktop.Notify(desktop.MsgAddonNew)
	}
}

type app struct {
	dir, exe string
	ui       desktop.Dialogs
	ctx      context.Context
	stop     context.CancelFunc

	mu        sync.Mutex
	cfg       config.Config
	lock      *desktop.Lock
	tray      *desktop.Tray
	lastSent  time.Time
	watchStop context.CancelFunc
	watchDone chan struct{}

	sending, renaming atomic.Bool
}

func (a *app) config() config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

// env prepara il ciclo; il webhook resta dentro Send e non va mai nel log.
func (a *app) env(cfg config.Config) run.Env {
	e := run.Env{Cfg: cfg, DataDir: a.dir, Loc: time.Local, Log: log.Printf}
	wh := config.EffectiveWebhook(cfg)
	if wh == "" {
		log.Printf("nessun webhook in questa build: le sessioni restano in coda")
		return e
	}
	e.Send = func(name string, data []byte) error {
		ctx, cancel := context.WithTimeout(a.ctx, sendTimeout)
		defer cancel()
		if err := discord.Upload(ctx, wh, name, data, ""); err != nil {
			return err
		}
		a.markSent(time.Now())
		return nil
	}
	return e
}

func (a *app) markSent(t time.Time) {
	a.mu.Lock()
	a.lastSent = t
	tray := a.tray
	a.mu.Unlock()
	if err := desktop.SaveLastSent(a.dir, t); err != nil {
		log.Printf("%v", err)
	}
	tray.SetStatus(desktop.LastSentLabel(t, time.Now()))
}

func (a *app) ready(t *desktop.Tray) {
	a.mu.Lock()
	a.tray = t
	a.mu.Unlock()
	a.startWatch()

	u := &desktop.Updater{
		Check: func(ctx context.Context) (*update.Release, error) {
			return update.Check(ctx, nil, apiBase, repo, build.Version)
		},
		Install: func(ctx context.Context, rel *update.Release) error {
			return update.Install(ctx, nil, rel, build.Version)
		},
		Restart: a.restart,
		Failed: func(msg string) {
			t.SetUpdateError(desktop.UpdateFailedLabel(msg))
			desktop.Notify(desktop.UpdateFailed + msg)
		},
	}
	go u.Loop(a.ctx, desktop.UpdateEvery)

	// l'etichetta "Ultimo invio" passa dall'ora alla data quando cambia il giorno
	go func() {
		tk := time.NewTicker(10 * time.Minute)
		defer tk.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-tk.C:
				a.mu.Lock()
				ls := a.lastSent
				a.mu.Unlock()
				t.SetStatus(desktop.LastSentLabel(ls, time.Now()))
			}
		}
	}()
}

func (a *app) startWatch() {
	ctx, cancel := context.WithCancel(a.ctx)
	done := make(chan struct{})
	env := a.env(a.config())
	a.mu.Lock()
	a.watchStop, a.watchDone = cancel, done
	a.mu.Unlock()
	go func() {
		defer close(done)
		run.Watch(ctx, env, pollEvery, stableFor)
	}()
}

// stopWatch ferma Watch e aspetta (al massimo stopWait) che finisca il giro in corso.
func (a *app) stopWatch() {
	a.mu.Lock()
	cancel, done := a.watchStop, a.watchDone
	a.watchStop, a.watchDone = nil, nil
	a.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	select {
	case <-done:
	case <-time.After(stopWait):
		log.Printf("il giro in corso non e' finito in tempo")
	}
}

func (a *app) sendNow() {
	if !a.sending.CompareAndSwap(false, true) {
		return
	}
	defer a.sending.Store(false)
	a.mu.Lock()
	tray, before := a.tray, a.lastSent
	a.mu.Unlock()
	tray.SetSending(true)
	defer tray.SetSending(false)

	cfg := a.config()
	if config.EffectiveWebhook(cfg) == "" {
		desktop.Notify("Questa versione non ha un canale Discord: non posso inviare.")
		return
	}
	err := run.Cycle(a.ctx, a.env(cfg))
	a.mu.Lock()
	after := a.lastSent
	a.mu.Unlock()
	switch {
	case err != nil:
		log.Printf("invio manuale: %v", err)
		desktop.Notify("Invio non riuscito: " + err.Error())
	case after.After(before):
		desktop.Notify("Diario inviato.")
	default:
		desktop.Notify("Niente di nuovo da inviare (una sessione parte una decina di minuti dopo il logout).")
	}
}

func (a *app) rename() {
	if !a.renaming.CompareAndSwap(false, true) {
		return
	}
	defer a.renaming.Store(false)
	cfg := a.config()
	name, err := desktop.AskName(a.ui, cfg.Player)
	if err != nil || name == cfg.Player {
		return
	}
	cfg.Player = name
	if err := config.Save(a.dir, cfg); err != nil {
		log.Printf("cambio nome: %v", err)
		a.ui.Error("Non riesco a salvare il nome: " + err.Error())
		return
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	log.Printf("nome cambiato")
	// Watch ha la configurazione vecchia: si riparte con quella nuova
	a.stopWatch()
	if a.ctx.Err() == nil {
		a.startWatch()
	}
}

func (a *app) openLogs() {
	if err := desktop.OpenFolder(a.dir); err != nil {
		log.Printf("%v", err)
	}
}

// restart avvia l'eseguibile aggiornato (stesso percorso, stessi argomenti) e
// chiude questo. Il lock si libera prima, se no il nuovo processo uscirebbe subito;
// se l'avvio fallisce lo si riprende e si continua con la versione in memoria.
func (a *app) restart() error {
	a.stopWatch()
	a.mu.Lock()
	lock := a.lock
	a.mu.Unlock()
	lock.Release()
	cmd := exec.Command(a.exe, os.Args[1:]...)
	if err := cmd.Start(); err != nil {
		if l, lerr := desktop.Acquire(a.dir); lerr == nil {
			a.mu.Lock()
			a.lock = l
			a.mu.Unlock()
		} else {
			log.Printf("lock non ripreso: %v", lerr)
		}
		a.startWatch()
		return err
	}
	cmd.Process.Release()
	desktop.QuitTray()
	return nil
}

func (a *app) exit() {
	a.stop() // interrompe anche un invio in corso: al massimo un duplicato, che il bot scarta
	a.stopWatch()
	a.mu.Lock()
	lock := a.lock
	a.mu.Unlock()
	lock.Release()
}
