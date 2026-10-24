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
	"github.com/oLdpZ/diario-gilda/app/internal/outbox"
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
	msgMoveApp  = "Sposta il Diario di Gilda nella cartella Applicazioni (se non sei amministratore del Mac, in quella della tua cartella Inizio) e riaprilo da lì: così potrà partire da solo e aggiornarsi."
)

func main() {
	if err := start(); err != nil {
		log.Printf("errore: %v", err)
		msg := "Il Diario di Gilda non può partire."
		var se *startError
		if errors.As(err, &se) {
			msg = se.msg
		}
		desktop.Dialogs{}.Error(msg + "\n\n" + err.Error())
		os.Exit(1)
	}
}

// startError e' un errore di avvio con il messaggio da mostrare al giocatore.
type startError struct {
	msg string
	err error
}

func (e *startError) Error() string { return e.err.Error() }
func (e *startError) Unwrap() error { return e.err }

func fatal(msg string, err error) error { return &startError{msg: msg, err: err} }

func start() error {
	if err := desktop.CheckBuild(); err != nil {
		return fatal(desktop.AppName, err)
	}
	const noDataDir = "Non riesco a usare la cartella dei dati del Diario di Gilda."
	dir, err := config.Dir()
	if err != nil {
		return fatal(noDataDir, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fatal(noDataDir, err)
	}
	// prima il lock, poi il log: un secondo avvio avvisa ed esce senza toccare diario.log
	lock, err := desktop.Acquire(dir)
	if errors.Is(err, desktop.ErrLocked) {
		desktop.Dialogs{}.Info(desktop.MsgRunning)
		return nil
	}
	if err != nil {
		return fatal(noDataDir, err)
	}
	defer lock.Release()
	lf, err := desktop.OpenLog(filepath.Join(dir, "diario.log"), desktop.LogMax)
	if err != nil {
		return fatal("Non riesco a scrivere il registro diario.log.", err)
	}
	log.SetOutput(lf)
	defer lf.Close()
	log.Printf("avvio Diario di Gilda %s (%s/%s)", build.Version, runtime.GOOS, runtime.GOARCH)

	update.CleanupOld()
	exe, err := os.Executable()
	if err != nil {
		return fatal("Non trovo il percorso del programma.", err)
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
	if errors.Is(err, desktop.ErrSave) {
		return fatal("Non riesco a salvare la configurazione.", err)
	}
	if err != nil {
		return fatal("Le finestre del primo avvio non funzionano.", err)
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
	// Forever installato dopo il primo avvio: si passa a quello (Ruling F2)
	if c, changed := desktop.PreferForever(cfg); changed {
		log.Printf("trovato il client Forever %s: lo uso al posto di %s", c.ClientDir, cfg.ClientDir)
		if err := config.Save(dir, c); err != nil {
			log.Printf("cartella del client non salvata: %v", err)
		}
		cfg = c
	}
	log.Printf("client: %s", cfg.ClientDir)

	installAddon(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	a := &app{dir: dir, exe: exe, ui: ui, ctx: ctx, stop: cancel, cfg: cfg, lock: lock}
	a.lastSent = desktop.LoadLastSent(dir)
	desktop.RunTray(desktop.TitleLabel(build.Version), desktop.LastSentLabel(a.lastSent, time.Now()),
		desktop.Handlers{SendNow: a.sendNow, Rename: a.rename, ChooseWowDir: a.chooseWowDir, OpenLogs: a.openLogs, Quit: desktop.QuitTray},
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

	cfgMu     sync.Mutex // serializza leggi-cambia-salva della configurazione
	watchMu   sync.Mutex // un solo restartWatch alla volta (nome e cartella dal menu)
	mu        sync.Mutex
	cfg       config.Config
	lock      *desktop.Lock
	tray      *desktop.Tray
	lastSent  time.Time
	watchStop context.CancelFunc
	watchDone chan struct{}

	clickStop context.CancelFunc // ferma l'"Invia ora" in corso

	sending, renaming, choosing, restarting atomic.Bool
}

func (a *app) config() config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

// env prepara il ciclo; il webhook resta dentro Send e non va mai nel log.
// Gli invii usano ctx: cancellarlo interrompe anche un upload in corso.
func (a *app) env(ctx context.Context, cfg config.Config) run.Env {
	e := run.Env{Cfg: cfg, DataDir: a.dir, Loc: time.Local, Log: log.Printf, GameRunning: desktop.WoWRunning}
	wh := config.EffectiveWebhook(cfg)
	if wh == "" {
		log.Printf("nessun webhook in questa build: le sessioni restano in coda")
		return e
	}
	e.Send = func(name string, data []byte) error {
		sctx, cancel := context.WithTimeout(ctx, sendTimeout)
		defer cancel()
		if err := discord.Upload(sctx, wh, name, data, ""); err != nil {
			if discord.Permanent(err) {
				return fmt.Errorf("%w: %w", outbox.ErrRejected, err) // va in outbox/scartati
			}
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
	if a.restarting.Load() {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	done := make(chan struct{})
	env := a.env(ctx, a.config())
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
	if a.restarting.Load() || !a.sending.CompareAndSwap(false, true) {
		return
	}
	defer a.sending.Store(false)
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	a.mu.Lock()
	tray, before := a.tray, a.lastSent
	a.clickStop = cancel
	a.mu.Unlock()
	tray.SetSending(true)
	defer tray.SetSending(false)

	cfg := a.config()
	if config.EffectiveWebhook(cfg) == "" {
		desktop.Notify("Questa versione non ha un canale Discord: non posso inviare.")
		return
	}
	err := run.Cycle(ctx, a.env(ctx, cfg))
	a.mu.Lock()
	after := a.lastSent
	a.mu.Unlock()
	switch {
	case err != nil && ctx.Err() != nil:
		log.Printf("invio manuale interrotto: %v", err)
	case err != nil:
		log.Printf("invio manuale: %v", err)
		desktop.Notify("Invio non riuscito: " + err.Error())
	case after.After(before):
		desktop.Notify("Diario inviato.")
	default:
		desktop.Notify("Niente di nuovo da inviare (una sessione parte una decina di minuti dopo che hai chiuso World of Warcraft).")
	}
}

func (a *app) rename() {
	if a.restarting.Load() || !a.renaming.CompareAndSwap(false, true) {
		return
	}
	defer a.renaming.Store(false)
	cfg := a.config()
	name, err := desktop.AskName(a.ui, cfg.Player)
	if err != nil || name == cfg.Player || a.restarting.Load() {
		return
	}
	if _, err := a.updateConfig(func(c *config.Config) { c.Player = name }); err != nil {
		log.Printf("cambio nome: %v", err)
		a.ui.Error("Non riesco a salvare il nome: " + err.Error())
		return
	}
	log.Printf("nome cambiato")
	a.restartWatch()
}

// chooseWowDir e' la voce "Scegli cartella di WoW…": chiede la cartella come al
// primo avvio, la salva, installa l'addon li' e riparte a guardare quella.
func (a *app) chooseWowDir() {
	if a.restarting.Load() || !a.choosing.CompareAndSwap(false, true) {
		return
	}
	defer a.choosing.Store(false)
	w, c, err := desktop.ChooseClient(a.ui)
	if err != nil {
		if !errors.Is(err, desktop.ErrCanceled) {
			log.Printf("scelta cartella di WoW: %v", err)
		}
		return
	}
	if a.restarting.Load() {
		return
	}
	if old := a.config(); old.ClientDir == c && old.WowDir == w {
		return
	}
	cfg, err := a.updateConfig(func(cf *config.Config) { cf.WowDir, cf.ClientDir = w, c })
	if err != nil {
		log.Printf("scelta cartella di WoW: %v", err)
		a.ui.Error("Non riesco a salvare la cartella: " + err.Error())
		return
	}
	log.Printf("cartella del client cambiata: %s", cfg.ClientDir)
	installAddon(cfg)
	a.restartWatch()
}

// updateConfig applica f alla configurazione attuale e la salva; in memoria
// cambia solo se il salvataggio riesce.
func (a *app) updateConfig(f func(*config.Config)) (config.Config, error) {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	cfg := a.config()
	f(&cfg)
	if err := config.Save(a.dir, cfg); err != nil {
		return cfg, err
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	return cfg, nil
}

// restartWatch fa ripartire Watch, che ha la configurazione vecchia.
func (a *app) restartWatch() {
	a.watchMu.Lock()
	defer a.watchMu.Unlock()
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
	a.restarting.Store(true)
	a.mu.Lock()
	clickStop := a.clickStop
	a.mu.Unlock()
	if clickStop != nil {
		clickStop() // interrompe un "Invia ora" in corso (al piu' un duplicato)
	}
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
		a.restarting.Store(false)
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
