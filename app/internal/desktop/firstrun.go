package desktop

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/oLdpZ/diario-gilda/app/internal/config"
	"github.com/oLdpZ/diario-gilda/app/internal/wowdir"
)

// Testi del primo avvio.
const (
	PromptWowDir = "Dov'è installato World of Warcraft?"
	PromptName   = "Con che nome vuoi comparire nel diario?"
	MsgDone      = "Fatto! Il Diario di Gilda ora lavora da solo. Lo trovi vicino all'orologio."
	MsgNoClient  = "In questa cartella non trovo World of Warcraft. Scegli la cartella dove è installato (quella che contiene _classic_ o simili)."
	MsgNameEmpty = "Scrivi un nome, per favore."
	MsgNameLong  = "Il nome è troppo lungo: al massimo %d caratteri."
)

// MaxNameLen e' la lunghezza massima del nome, in caratteri.
const MaxNameLen = 40

// ErrCanceled: il giocatore ha chiuso una finestra senza rispondere.
var ErrCanceled = errors.New("annullato")

// ErrSave avvolge l'errore di Setup.Save.
var ErrSave = errors.New("salvataggio configurazione")

// UI sono le finestre di dialogo; SelectDir ed Entry restituiscono
// ErrCanceled se il giocatore annulla.
type UI interface {
	SelectDir(title string) (string, error)
	Entry(prompt, def string) (string, error)
	Error(msg string)
	Info(msg string)
}

// Setup e' cio' che serve a FirstRun: le cartelle dove cercare WoW, le finestre,
// il salvataggio della configurazione e l'attivazione dell'avvio automatico.
type Setup struct {
	Roots  []string
	UI     UI
	Save   func(config.Config) error
	Enable func() error
}

// NeedsSetup e' vero se manca il nome o la cartella del client, anche quando
// quella salvata non esiste piu' (WoW spostato o disinstallato).
func NeedsSetup(c config.Config) bool { return c.Player == "" || !clientOK(c.ClientDir) }

func clientOK(dir string) bool { return dir != "" && isDir(dir) }

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// ClientIn cerca un client dentro dir: dir stessa se ha Interface e WTF,
// altrimenti il primo di wowdir.Clients(dir). Restituisce anche la cartella di
// installazione (quella che contiene il client).
func ClientIn(dir string) (wowDir, clientDir string, ok bool) {
	dir = filepath.Clean(dir)
	if isDir(filepath.Join(dir, "Interface")) && isDir(filepath.Join(dir, "WTF")) {
		return filepath.Dir(dir), dir, true
	}
	if cl := wowdir.Clients(dir); len(cl) > 0 {
		return dir, cl[0], true
	}
	return "", "", false
}

// FindClient prova le cartelle abituali, nell'ordine.
func FindClient(roots []string) (wowDir, clientDir string, ok bool) {
	for _, r := range roots {
		if cl := wowdir.Clients(r); len(cl) > 0 {
			return r, cl[0], true
		}
	}
	return "", "", false
}

// FirstRun chiede solo cio' che manca in cfg (cartella di WoW se assente o
// sparita, poi nome), salva,
// attiva l'avvio automatico e saluta. Se non manca nulla restituisce cfg cosi'
// com'e'. ErrCanceled se il giocatore annulla: in quel caso non salva nulla.
// Un errore dell'avvio automatico finisce nel log ma non ferma il primo avvio.
func FirstRun(cfg config.Config, s Setup) (config.Config, error) {
	if !NeedsSetup(cfg) {
		return cfg, nil
	}
	if !clientOK(cfg.ClientDir) {
		if cfg.ClientDir != "" {
			log.Printf("cartella del client %s sparita: la si richiede", cfg.ClientDir)
		}
		w, c, ok := FindClient(s.Roots)
		for !ok {
			dir, err := s.UI.SelectDir(PromptWowDir)
			if err != nil {
				return cfg, err
			}
			if w, c, ok = ClientIn(dir); !ok {
				s.UI.Error(MsgNoClient)
			}
		}
		cfg.WowDir, cfg.ClientDir = w, c
	}
	if cfg.Player == "" {
		name, err := AskName(s.UI, "")
		if err != nil {
			return cfg, err
		}
		cfg.Player = name
	}
	if err := s.Save(cfg); err != nil {
		return cfg, fmt.Errorf("%w: %w", ErrSave, err)
	}
	if err := s.Enable(); err != nil {
		log.Printf("avvio automatico non attivato: %v", err)
	}
	s.UI.Info(MsgDone)
	return cfg, nil
}

// AskName chiede il nome finche' non e' valido (non vuoto, al massimo
// MaxNameLen caratteri, spazi ai lati tolti). ErrCanceled se si annulla.
func AskName(ui UI, current string) (string, error) {
	for {
		name, err := ui.Entry(PromptName, current)
		if err != nil {
			return "", err
		}
		name = strings.Join(strings.Fields(name), " ")
		switch n := utf8.RuneCountInString(name); {
		case n == 0:
			ui.Error(MsgNameEmpty)
		case n > MaxNameLen:
			ui.Error(fmt.Sprintf(MsgNameLong, MaxNameLen))
		default:
			return name, nil
		}
	}
}
