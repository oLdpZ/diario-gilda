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

func (f *fakeUI) Error(msg string) { f.errors = append(f.errors, msg) }
func (f *fakeUI) Info(msg string)  { f.infos = append(f.infos, msg) }

func okSetup(ui UI, roots ...string) Setup {
	return Setup{Roots: roots, UI: ui,
		Save: func(config.Config) error { return nil }, Enable: func() error { return nil }}
}

func TestNeedsSetup(t *testing.T) {
	cases := []struct {
		c    config.Config
		want bool
	}{
		{config.Config{}, true},
		{config.Config{Player: "Gilda"}, true},
		{config.Config{ClientDir: "x"}, true},
		{config.Config{Player: "Gilda", ClientDir: "x"}, false},
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
	if w, c, ok := ClientIn(root); !ok || c != cl || w != root {
		t.Errorf("ClientIn(installazione) = %q %q %v", w, c, ok)
	}
	if w, c, ok := ClientIn(cl); !ok || c != cl || w != root {
		t.Errorf("ClientIn(client) = %q %q %v", w, c, ok)
	}
	if _, _, ok := ClientIn(t.TempDir()); ok {
		t.Error("ClientIn su cartella vuota")
	}
}

func TestFirstRunFoundAutomatically(t *testing.T) {
	root := t.TempDir()
	cl := mkClient(t, root, "_classic_")
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
	cl := mkClient(t, root, "_classic_")
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
	cfg := config.Config{Player: "Ciro", ClientDir: "c", WowDir: "w"}
	got, err := FirstRun(cfg, okSetup(ui))
	if err != nil || got != cfg || len(ui.prompts) != 0 {
		t.Errorf("FirstRun = %+v, %v, domande %q", got, err, ui.prompts)
	}
}

func TestFirstRunOnlyMissingName(t *testing.T) {
	ui := &fakeUI{names: []string{"Ugo"}}
	got, err := FirstRun(config.Config{ClientDir: "c", WowDir: "w"}, okSetup(ui))
	if err != nil || got.Player != "Ugo" || got.ClientDir != "c" || len(ui.prompts) != 1 {
		t.Errorf("FirstRun = %+v, %v, domande %q", got, err, ui.prompts)
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
		mkClient(t, root, "_classic_")
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
	mkClient(t, root, "_classic_")
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
	mkClient(t, root, "_classic_")
	ui := &fakeUI{names: []string{"Ezio"}}
	s := okSetup(ui, root)
	s.Save = func(config.Config) error { return errors.New("disco") }
	if _, err := FirstRun(config.Config{}, s); err == nil {
		t.Fatal("errore di salvataggio ignorato")
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
