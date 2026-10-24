package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oLdpZ/diario-gilda/app/internal/config"
)

func mkClient(t *testing.T, root, name string) string {
	t.Helper()
	p := filepath.Join(root, name)
	for _, d := range []string{"Interface", "WTF"} {
		if err := os.MkdirAll(filepath.Join(p, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

const cancel = "\x00"

// fakeUI risponde con le liste preparate; cancel (o lista finita) = annulla.
type fakeUI struct {
	dirs, names           []string
	errors, infos         []string
	prompts, entryDefault []string
	picks, listDefault    []string
	lists                 [][]string
}

func (f *fakeUI) SelectDir(title string) (string, error) {
	f.prompts = append(f.prompts, title)
	if len(f.dirs) == 0 || f.dirs[0] == cancel {
		return "", ErrCanceled
	}
	d := f.dirs[0]
	f.dirs = f.dirs[1:]
	return d, nil
}

func (f *fakeUI) Entry(prompt, def string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	f.entryDefault = append(f.entryDefault, def)
	if len(f.names) == 0 || f.names[0] == cancel {
		return "", ErrCanceled
	}
	n := f.names[0]
	f.names = f.names[1:]
	return n, nil
}

func (f *fakeUI) List(prompt string, items []string, def string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	f.lists = append(f.lists, items)
	f.listDefault = append(f.listDefault, def)
	if len(f.picks) == 0 || f.picks[0] == cancel {
		return "", ErrCanceled
	}
	p := f.picks[0]
	f.picks = f.picks[1:]
	return p, nil
}

func (f *fakeUI) Error(msg string) { f.errors = append(f.errors, msg) }
func (f *fakeUI) Info(msg string)  { f.infos = append(f.infos, msg) }

func okSetup(ui UI, roots ...string) Setup {
	return Setup{Roots: roots, UI: ui,
		Save: func(config.Config) error { return nil }, Enable: func() error { return nil }}
}

func TestNeedsSetup(t *testing.T) {
	cl := mkClient(t, t.TempDir(), "_classic_")
	gone := filepath.Join(t.TempDir(), "disinstallato")
	cases := []struct {
		c    config.Config
		want bool
	}{
		{config.Config{}, true},
		{config.Config{Player: "Gilda"}, true},
		{config.Config{ClientDir: cl}, true},
		{config.Config{Player: "Gilda", ClientDir: cl}, false},
		{config.Config{Player: "Gilda", ClientDir: gone}, true}, // cartella sparita
	}
	for _, c := range cases {
		if got := NeedsSetup(c.c); got != c.want {
			t.Errorf("NeedsSetup(%+v) = %v", c.c, got)
		}
	}
}

func TestFindClientInRoots(t *testing.T) {
	empty := t.TempDir()
	root := t.TempDir()
	mkClient(t, root, "_retail_")
	want := mkClient(t, root, "_forever_")
	w, c, ok := FindClient([]string{filepath.Join(empty, "manca"), empty, root})
	if !ok || c != want || w != root {
		t.Fatalf("FindClient = %q %q %v", w, c, ok)
	}
	if _, _, ok := FindClient([]string{empty}); ok {
		t.Fatal("trovato un client in una cartella vuota")
	}
}

func TestClientIn(t *testing.T) {
	root := t.TempDir()
	cl := mkClient(t, root, "_classic_era_")
	// installazione senza client Forever: non si sceglie da soli
	if _, _, ok := ClientIn(root); ok {
		t.Errorf("ClientIn(installazione senza Forever) ha scelto un client")
	}
	// il client scelto direttamente si accetta anche se non e' Forever
	if w, c, ok := ClientIn(cl); !ok || c != cl || w != root {
		t.Errorf("ClientIn(client) = %q %q %v", w, c, ok)
	}
	fv := mkClient(t, root, "_forever_")
	if w, c, ok := ClientIn(root); !ok || c != fv || w != root {
		t.Errorf("ClientIn(installazione) = %q %q %v", w, c, ok)
	}
	if _, _, ok := ClientIn(t.TempDir()); ok {
		t.Error("ClientIn su cartella vuota")
	}
}

func TestFirstRunFoundAutomatically(t *testing.T) {
	root := t.TempDir()
	mkClient(t, root, "_classic_")
	cl := mkClient(t, root, "_forever_")
	ui := &fakeUI{names: []string{"  Aldo  "}}
	var saved config.Config
	var enabled bool
	got, err := FirstRun(config.Config{}, Setup{
		Roots: []string{root}, UI: ui,
		Save:   func(c config.Config) error { saved = c; return nil },
		Enable: func() error { enabled = true; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{Player: "Aldo", WowDir: root, ClientDir: cl}
	if got != want || saved != want {
		t.Errorf("config = %+v, salvata %+v", got, saved)
	}
	if !enabled {
		t.Error("avvio automatico non attivato")
	}
	if len(ui.prompts) != 1 || ui.prompts[0] != PromptName {
		t.Errorf("domande = %q", ui.prompts)
	}
	if len(ui.infos) != 1 || ui.infos[0] != MsgDone {
		t.Errorf("messaggi = %q", ui.infos)
	}
}

func TestFirstRunAsksFolder(t *testing.T) {
	wrong := t.TempDir()
	root := t.TempDir()
	cl := mkClient(t, root, "_forever_")
	ui := &fakeUI{dirs: []string{wrong, root}, names: []string{"", "Bea"}}
	got, err := FirstRun(config.Config{}, okSetup(ui, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientDir != cl || got.WowDir != root || got.Player != "Bea" {
		t.Errorf("config = %+v", got)
	}
	if len(ui.errors) != 2 { // cartella sbagliata + nome vuoto
		t.Errorf("errori = %q", ui.errors)
	}
	if ui.prompts[0] != PromptWowDir || ui.prompts[1] != PromptWowDir || ui.prompts[2] != PromptName {
		t.Errorf("domande = %q", ui.prompts)
	}
}

func TestFirstRunKeepsKnownValues(t *testing.T) {
	ui := &fakeUI{}
	root := t.TempDir()
	cfg := config.Config{Player: "Ciro", ClientDir: mkClient(t, root, "_classic_"), WowDir: root}
	got, err := FirstRun(cfg, okSetup(ui))
	if err != nil || got != cfg || len(ui.prompts) != 0 {
		t.Errorf("FirstRun = %+v, %v, domande %q", got, err, ui.prompts)
	}
}

func TestFirstRunOnlyMissingName(t *testing.T) {
	ui := &fakeUI{names: []string{"Ugo"}}
	cl := mkClient(t, t.TempDir(), "_classic_")
	got, err := FirstRun(config.Config{ClientDir: cl, WowDir: "w"}, okSetup(ui))
	if err != nil || got.Player != "Ugo" || got.ClientDir != cl || len(ui.prompts) != 1 {
		t.Errorf("FirstRun = %+v, %v, domande %q", got, err, ui.prompts)
	}
}

// TestFirstRunClientGone: WoW spostato o disinstallato: si richiede la
// cartella, il nome resta.
func TestFirstRunClientGone(t *testing.T) {
	root := t.TempDir()
	cl := mkClient(t, root, "_forever_")
	ui := &fakeUI{dirs: []string{root}}
	cfg := config.Config{Player: "Vito", ClientDir: filepath.Join(t.TempDir(), "sparito"), WowDir: "vecchio"}
	got, err := FirstRun(cfg, okSetup(ui, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Player != "Vito" || got.ClientDir != cl || got.WowDir != root {
		t.Errorf("config = %+v", got)
	}
	if len(ui.prompts) != 1 || ui.prompts[0] != PromptWowDir {
		t.Errorf("domande = %q", ui.prompts)
	}
}

func TestFirstRunCanceled(t *testing.T) {
	t.Run("cartella", func(t *testing.T) {
		saved := false
		s := okSetup(&fakeUI{}, t.TempDir())
		s.Save = func(config.Config) error { saved = true; return nil }
		if _, err := FirstRun(config.Config{}, s); !errors.Is(err, ErrCanceled) || saved {
			t.Errorf("err = %v, salvata %v", err, saved)
		}
	})
	t.Run("nome", func(t *testing.T) {
		root := t.TempDir()
		mkClient(t, root, "_forever_")
		saved := false
		s := okSetup(&fakeUI{names: []string{cancel}}, root)
		s.Save = func(config.Config) error { saved = true; return nil }
		if _, err := FirstRun(config.Config{}, s); !errors.Is(err, ErrCanceled) || saved {
			t.Errorf("err = %v, salvata %v", err, saved)
		}
	})
}

func TestFirstRunAutostartFailureNotFatal(t *testing.T) {
	root := t.TempDir()
	mkClient(t, root, "_forever_")
	ui := &fakeUI{names: []string{"Dino"}}
	s := okSetup(ui, root)
	s.Enable = func() error { return errors.New("registro") }
	if _, err := FirstRun(config.Config{}, s); err != nil {
		t.Fatalf("errore dell'avvio automatico fatale: %v", err)
	}
	if len(ui.infos) != 1 {
		t.Errorf("messaggio finale assente")
	}
}

func TestFirstRunSaveError(t *testing.T) {
	root := t.TempDir()
	mkClient(t, root, "_forever_")
	ui := &fakeUI{names: []string{"Ezio"}}
	s := okSetup(ui, root)
	s.Save = func(config.Config) error { return errors.New("disco") }
	if _, err := FirstRun(config.Config{}, s); !errors.Is(err, ErrSave) {
		t.Fatalf("err = %v, voglio ErrSave", err)
	}
	if len(ui.infos) != 0 {
		t.Errorf("messaggio di successo dopo un errore: %q", ui.infos)
	}
}

func TestAskName(t *testing.T) {
	ui := &fakeUI{names: []string{"   ", "Fede\n"}}
	n, err := AskName(ui, "Vecchio")
	if err != nil || n != "Fede" {
		t.Fatalf("AskName = %q, %v", n, err)
	}
	if ui.entryDefault[0] != "Vecchio" {
		t.Errorf("default = %q", ui.entryDefault[0])
	}
	ui = &fakeUI{names: []string{strings.Repeat("x", MaxNameLen+1), "Gino"}}
	if n, _ := AskName(ui, ""); n != "Gino" || len(ui.errors) != 1 {
		t.Errorf("nome troppo lungo accettato: %q, errori %q", n, ui.errors)
	}
	ui = &fakeUI{names: []string{strings.Repeat("è", MaxNameLen)}}
	if n, _ := AskName(ui, ""); n == "" {
		t.Errorf("limite in byte invece che in caratteri")
	}
}

// TestFirstRunDialogError: un errore delle finestre (non annullamento) non e'
// un errore di salvataggio.
func TestFirstRunDialogError(t *testing.T) {
	boom := errors.New("zenity assente")
	s := okSetup(errUI{boom}, t.TempDir())
	_, err := FirstRun(config.Config{}, s)
	if !errors.Is(err, boom) || errors.Is(err, ErrSave) || errors.Is(err, ErrCanceled) {
		t.Errorf("err = %v", err)
	}
}

type errUI struct{ err error }

func (e errUI) SelectDir(string) (string, error)              { return "", e.err }
func (e errUI) Entry(string, string) (string, error)          { return "", e.err }
func (e errUI) List(string, []string, string) (string, error) { return "", e.err }
func (errUI) Error(string)                                    {}
func (errUI) Info(string)                                     {}

// Ruling F2: senza un client Forever non si sceglie da soli, si chiede.
func TestFirstRunNoForeverAsks(t *testing.T) {
	root := t.TempDir()
	mkClient(t, root, "_classic_")
	mkClient(t, root, "_retail_")
	ui := &fakeUI{dirs: []string{root, filepath.Join(root, "_classic_")}, names: []string{"Lia"}}
	got, err := FirstRun(config.Config{}, okSetup(ui, root))
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientDir != filepath.Join(root, "_classic_") || got.WowDir != root {
		t.Errorf("config = %+v", got)
	}
	if len(ui.prompts) < 1 || ui.prompts[0] != PromptWowDir || !strings.Contains(PromptWowDir, "WoW Forever") {
		t.Errorf("domande = %q", ui.prompts)
	}
	// la cartella di installazione senza Forever non basta: errore, poi il client scelto a mano
	if len(ui.errors) != 1 || ui.errors[0] != MsgNoClient {
		t.Errorf("errori = %q", ui.errors)
	}
}

func TestFindClientOnlyForever(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	mkClient(t, a, "_classic_")
	want := mkClient(t, b, "_classic_beta_")
	if w, c, ok := FindClient([]string{a, b}); !ok || c != want || w != b {
		t.Fatalf("FindClient = %q %q %v", w, c, ok)
	}
	if _, _, ok := FindClient([]string{a}); ok {
		t.Fatal("scelto da solo un client non Forever")
	}
}

func TestPreferForever(t *testing.T) {
	root := t.TempDir()
	old := mkClient(t, root, "_classic_")
	cfg := config.Config{Player: "Ivo", WowDir: root, ClientDir: old}
	if got, changed := PreferForever(cfg); changed || got != cfg {
		t.Fatalf("senza Forever: %+v %v", got, changed)
	}
	fv := mkClient(t, root, "_forever_")
	got, changed := PreferForever(cfg)
	if !changed || got.ClientDir != fv || got.WowDir != root || got.Player != "Ivo" {
		t.Fatalf("con Forever: %+v %v", got, changed)
	}
	if again, changed := PreferForever(got); changed || again != got {
		t.Fatalf("gia' Forever: %+v %v", again, changed)
	}
	// WowDir vuota: si usa la cartella che contiene il client
	cfg.WowDir = ""
	if got, changed := PreferForever(cfg); !changed || got.ClientDir != fv || got.WowDir != root {
		t.Fatalf("senza WowDir: %+v %v", got, changed)
	}
	// cartella non impostata: nulla da fare
	if _, changed := PreferForever(config.Config{}); changed {
		t.Fatal("config vuota cambiata")
	}
}

func TestChooseClient(t *testing.T) {
	root := t.TempDir()
	fv := mkClient(t, root, "_forever_")
	ui := &fakeUI{dirs: []string{t.TempDir(), root}}
	w, c, err := ChooseClient(ui)
	if err != nil || w != root || c != fv {
		t.Fatalf("ChooseClient = %q %q %v", w, c, err)
	}
	if len(ui.errors) != 1 {
		t.Errorf("errori = %q", ui.errors)
	}
	if _, _, err := ChooseClient(&fakeUI{}); !errors.Is(err, ErrCanceled) {
		t.Errorf("annullato: %v", err)
	}
}

func TestAskMain(t *testing.T) {
	gino := config.Character{Char: "Gino", Realm: "Forever"}
	grizzo := config.Character{Char: "Grizzo", Realm: "Forever"}
	chars := []config.Character{gino, grizzo}

	ui := &fakeUI{picks: []string{"Grizzo (Forever)"}}
	got, err := AskMain(ui, chars, config.Character{})
	if err != nil || got != grizzo {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(ui.lists) != 1 || len(ui.lists[0]) != 2 || ui.listDefault[0] != "Gino (Forever)" || ui.prompts[0] != PromptMain {
		t.Fatalf("finestra: %v default %v prompt %v", ui.lists, ui.listDefault, ui.prompts)
	}

	// il main attuale e' proposto
	ui = &fakeUI{picks: []string{"Gino (Forever)"}}
	if _, err := AskMain(ui, chars, config.Character{Char: "grizzo", Realm: "forever"}); err != nil || ui.listDefault[0] != "Grizzo (Forever)" {
		t.Fatalf("default: %v, %v", ui.listDefault, err)
	}

	// annullato
	ui = &fakeUI{picks: []string{cancel}}
	if _, err := AskMain(ui, chars, config.Character{}); err != ErrCanceled {
		t.Fatalf("annullato: %v", err)
	}

	// nessun personaggio: avviso, niente finestra di scelta
	ui = &fakeUI{}
	if _, err := AskMain(ui, nil, config.Character{}); err != ErrCanceled || len(ui.lists) != 0 || len(ui.infos) != 1 || ui.infos[0] != MsgNoChars {
		t.Fatalf("senza personaggi: %v %v %v", err, ui.lists, ui.infos)
	}
}
