package desktop

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oLdpZ/diario-gilda/app/internal/update"
)

func TestUpdaterOnce(t *testing.T) {
	rel := &update.Release{Version: "0.2.0"}
	boom := errors.New("cartella di sola lettura")
	cases := []struct {
		name       string
		check      *update.Release
		checkErr   error
		installErr error
		restarted  bool
		failed     int
	}{
		{name: "nessuna novita"},
		{name: "rete assente", checkErr: errors.New("offline")},
		{name: "installata", check: rel, restarted: true},
		{name: "installazione fallita", check: rel, installErr: boom, failed: 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var installed, restarted bool
			var fails []string
			u := &Updater{
				Check:   func(context.Context) (*update.Release, error) { return c.check, c.checkErr },
				Install: func(context.Context, *update.Release) error { installed = true; return c.installErr },
				Restart: func() error { restarted = true; return nil },
				Failed:  func(msg string) { fails = append(fails, msg) },
			}
			done := u.Once(context.Background())
			if done != c.restarted || restarted != c.restarted || len(fails) != c.failed {
				t.Errorf("done=%v restarted=%v fails=%q", done, restarted, fails)
			}
			if installed != (c.check != nil) {
				t.Errorf("Install chiamata = %v", installed)
			}
			if c.failed > 0 && fails[0] != boom.Error() {
				t.Errorf("messaggio = %q", fails[0])
			}
		})
	}
}

// TestUpdaterFailedOnce: lo stesso errore si mostra una volta sola.
func TestUpdaterFailedOnce(t *testing.T) {
	n := 0
	msg := "primo"
	u := &Updater{
		Check:   func(context.Context) (*update.Release, error) { return &update.Release{}, nil },
		Install: func(context.Context, *update.Release) error { return errors.New(msg) },
		Restart: func() error { t.Fatal("riavvio dopo un errore"); return nil },
		Failed:  func(string) { n++ },
	}
	u.Once(context.Background())
	u.Once(context.Background())
	if n != 1 {
		t.Errorf("Failed chiamata %d volte", n)
	}
	msg = "secondo"
	u.Once(context.Background())
	if n != 2 {
		t.Errorf("errore nuovo non mostrato (%d)", n)
	}
}

func TestUpdaterRestartError(t *testing.T) {
	var fails []string
	u := &Updater{
		Check:   func(context.Context) (*update.Release, error) { return &update.Release{}, nil },
		Install: func(context.Context, *update.Release) error { return nil },
		Restart: func() error { return errors.New("exec") },
		Failed:  func(m string) { fails = append(fails, m) },
	}
	if !u.Once(context.Background()) {
		t.Error("dopo un'installazione riuscita il controllo deve fermarsi")
	}
	if len(fails) != 1 {
		t.Errorf("errore di riavvio non mostrato")
	}
}

// TestUpdaterLoop: controlla subito, poi a ogni intervallo; si ferma dopo
// un'installazione o alla cancellazione del contesto.
func TestUpdaterLoop(t *testing.T) {
	calls := 0
	u := &Updater{
		Check: func(context.Context) (*update.Release, error) {
			calls++
			if calls == 3 {
				return &update.Release{}, nil
			}
			return nil, nil
		},
		Install: func(context.Context, *update.Release) error { return nil },
		Restart: func() error { return nil },
	}
	done := make(chan struct{})
	go func() { u.Loop(context.Background(), time.Millisecond); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Loop non si e' fermato dopo l'installazione")
	}
	if calls != 3 {
		t.Errorf("controlli = %d", calls)
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	first := make(chan struct{}, 1)
	u2 := &Updater{Check: func(context.Context) (*update.Release, error) {
		select {
		case first <- struct{}{}:
		default:
		}
		return nil, nil
	}}
	done = make(chan struct{})
	go func() { u2.Loop(ctx, time.Hour); close(done) }()
	select {
	case <-first:
	case <-time.After(5 * time.Second):
		t.Fatal("nessun controllo all'avvio")
	}
	cancelCtx()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Loop non si ferma alla cancellazione")
	}
}
