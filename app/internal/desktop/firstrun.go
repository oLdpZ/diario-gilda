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
	PromptWowDir = "Scegli la cartella di WoW Forever"
	PromptName   = "Con che nome vuoi comparire nel diario?"
	MsgDone      = "Fatto! Il Diario di Gilda ora lavora da solo. Lo trovi vicino all'orologio."
	MsgNoClient  = "In questa cartella non trovo WoW Forever. Scegli la cartella di World of Warcraft che lo contiene, oppure direttamente la cartella del client di Forever (quella con dentro il gioco, Wow….exe)."
	MsgNameEmpty = "Scrivi un nome, per favore."
	MsgNameLong  = "Il nome è troppo lungo: al massimo %d caratteri."
)

// MaxNameLen e' la lunghezza massima del nome, in caratteri.
const MaxNameLen = 40

// ErrCanceled: il giocatore ha chiuso una finestra senza rispondere.
var ErrCanceled = errors.New("annullato")

// ErrSave avvolge l'errore di Setup.Save.
var ErrSave = errors.New("salvataggio configurazione")

// UI sono le finestre di dialogo; SelectDir, Entry e List restituiscono
// ErrCanceled se il giocatore annulla.
type UI interface {
	SelectDir(title string) (string, error)
	Entry(prompt, def string) (string, error)
	List(prompt string, items []string, def string) (string, error)
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

// ClientIn cerca un client dentro la cartella scelta dal giocatore: dir stessa
// se e' un client (wowdir.IsClient: scelta esplicita, anche se non e' Forever), altrimenti
// il primo client Forever di wowdir.Clients(dir). Un'installazione con soli
// client non Forever non basta: si fa scegliere il client a mano. Restituisce
// anche la cartella di installazione (quella che contiene il client).
func ClientIn(dir string) (wowDir, clientDir string, ok bool) {
	dir = filepath.Clean(dir)
	if wowdir.IsClient(dir) {
		return filepath.Dir(dir), dir, true
	}
	if cl := foreverIn(dir); cl != "" {
		return dir, cl, true
	}
	return "", "", false
}

// foreverIn e' il primo client Forever di root, o "".
func foreverIn(root string) string {
	if cl := wowdir.Clients(root); len(cl) > 0 && wowdir.IsForever(cl[0]) {
		return cl[0]
	}
	return ""
}

// FindClient prova le cartelle abituali, nell'ordine, e accetta solo un client
// Forever: un altro client (retail, classic) non si sceglie mai da solo.
func FindClient(roots []string) (wowDir, clientDir string, ok bool) {
	for _, r := range roots {
		if cl := foreverIn(r); cl != "" {
			return r, cl, true
		}
	}
	return "", "", false
}

// ChooseClient chiede la cartella finche' non contiene un client (vedi
// ClientIn). ErrCanceled se il giocatore annulla.
func ChooseClient(ui UI) (wowDir, clientDir string, err error) {
	for {
		dir, err := ui.SelectDir(PromptWowDir)
		if err != nil {
			return "", "", err
		}
		if w, c, ok := ClientIn(dir); ok {
			return w, c, nil
		}
		ui.Error(MsgNoClient)
	}
}

// PreferForever passa al client Forever se quello salvato non lo e' e nella
// stessa installazione ora ce n'e' uno (es. Forever installato dopo il primo
// avvio). changed dice se cfg e' cambiata (va salvata).
func PreferForever(cfg config.Config) (out config.Config, changed bool) {
	if !clientOK(cfg.ClientDir) || wowdir.IsForever(cfg.ClientDir) {
		return cfg, false
	}
	root := cfg.WowDir
	if root == "" {
		root = filepath.Dir(filepath.Clean(cfg.ClientDir))
	}
	cl := foreverIn(root)
	if cl == "" {
		return cfg, false
	}
	cfg.WowDir, cfg.ClientDir = root, cl
	return cfg, true
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
		if !ok {
			var err error
			if w, c, err = ChooseClient(s.UI); err != nil {
				return cfg, err
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
