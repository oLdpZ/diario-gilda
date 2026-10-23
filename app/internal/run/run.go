// Package run collega i pezzi: legge le SavedVariables dell'addon, accoda le
// sessioni definitive (vedi docs/formato-dati.md, "Ciclo di vita delle sessioni")
// e svuota la coda verso Discord; Watch ripete il ciclo quando i file cambiano.
package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/oLdpZ/diario-gilda/app/internal/addondata"
	"github.com/oLdpZ/diario-gilda/app/internal/build"
	"github.com/oLdpZ/diario-gilda/app/internal/combatlog"
	"github.com/oLdpZ/diario-gilda/app/internal/config"
	"github.com/oLdpZ/diario-gilda/app/internal/episodes"
	"github.com/oLdpZ/diario-gilda/app/internal/outbox"
	"github.com/oLdpZ/diario-gilda/app/internal/payload"
	"github.com/oLdpZ/diario-gilda/app/internal/state"
	"github.com/oLdpZ/diario-gilda/app/internal/svparse"
	"github.com/oLdpZ/diario-gilda/app/internal/wowdir"
)

const (
	reloadGap   = 600 // s: un /reload entro questo tempo riapre la sessione
	crashAge    = 12 * 3600
	logMargin   = 60 // s attorno alla sessione letti dal registro combattimenti
	maxSVSize   = 64 << 20
	fullEvery   = 5 * time.Minute
	stateFile   = "state.json"
	corruptFile = "state.json.corrotto"
)

// Env e' tutto cio' che serve al ciclo. Send riceve il nome e il contenuto di un
// file della coda; non deve mai riportare il webhook nei suoi errori.
type Env struct {
	Cfg     config.Config
	DataDir string
	Now     func() time.Time
	Loc     *time.Location
	Send    func(name string, data []byte) error
	Log     func(format string, args ...any)
}

func (e Env) logf(format string, args ...any) {
	if e.Log != nil {
		e.Log(format, args...)
	}
}

func (e Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e Env) loc() *time.Location {
	if e.Loc != nil {
		return e.Loc
	}
	return time.Local
}

func (e Env) outboxDir() string { return filepath.Join(e.DataDir, "outbox") }

// Cycle accoda le sessioni definitive non ancora accodate, salva lo stato e
// svuota la coda. Un file SavedVariables illeggibile o a meta' scrittura viene
// saltato (si riprova al giro dopo). L'errore restituito e' quello dello stato
// o dell'invio; la coda resta su disco e il prossimo Cycle riprova.
func Cycle(ctx context.Context, env Env) error {
	now := env.now().Unix()
	st, err := loadState(env, now)
	if err != nil {
		return err
	}

	var errs []error
	if env.Cfg.ClientDir == "" {
		env.logf("cartella del client non impostata: solo invio della coda")
	} else if queued := enqueue(env, st, now); queued > 0 {
		if err := state.Save(env.DataDir, st); err != nil {
			errs = append(errs, fmt.Errorf("salvataggio stato: %w", err))
		}
	}

	if env.Send != nil {
		n, err := outbox.Flush(ctx, env.outboxDir(), env.Send)
		if n > 0 {
			env.logf("inviati %d file", n)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// loadState carica lo stato; se state.json e' corrotto lo mette da parte come
// state.json.corrotto e riparte vuoto (al massimo un duplicato, che il bot scarta).
func loadState(env Env, now int64) (*state.State, error) {
	st, err := state.LoadAt(env.DataDir, now)
	if err == nil {
		return st, nil
	}
	p := filepath.Join(env.DataDir, stateFile)
	if _, rerr := os.ReadFile(p); rerr != nil {
		return nil, fmt.Errorf("lettura stato: %w", err) // errore di disco, non di formato
	}
	if rerr := os.Rename(p, filepath.Join(env.DataDir, corruptFile)); rerr != nil {
		return nil, fmt.Errorf("stato corrotto non spostato: %w", rerr)
	}
	env.logf("stato corrotto (%v): spostato in %s, si riparte da vuoto", err, corruptFile)
	return &state.State{Sent: map[string]int64{}}, nil
}

// enqueue legge tutte le SavedVariables del client e accoda le sessioni
// definitive. Restituisce quante sessioni ha segnato nello stato.
func enqueue(env Env, st *state.State, now int64) int {
	var all []addondata.Session
	for _, f := range wowdir.SavedVariables(env.Cfg.ClientDir) {
		db, err := readSV(f)
		if err != nil {
			env.logf("%v (si riprova al prossimo giro)", err)
			continue
		}
		all = append(all, db.Sessions...)
	}

	marked := 0
	seen := map[string]bool{}
	for _, s := range all {
		if seen[s.ID] || st.Has(s.ID) {
			continue
		}
		seen[s.ID] = true
		ok, interrupted := final(s, all, now)
		if !ok {
			continue
		}
		name, data, err := build1(env, s, interrupted)
		if err != nil {
			// errore deterministico: riprovare non serve, si segna per non ripeterlo ogni giro
			env.logf("sessione %s scartata: %v", s.ID, err)
			st.MarkQueued(s.ID, now)
			marked++
			continue
		}
		if err := outbox.Put(env.outboxDir(), name, data); err != nil {
			env.logf("sessione %s non accodata: %v", s.ID, err)
			continue
		}
		st.MarkQueued(s.ID, now)
		marked++
		env.logf("sessione %s in coda (%s)", s.ID, name)
	}
	return marked
}

func readSV(path string) (addondata.DB, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return addondata.DB{}, fmt.Errorf("%s: %w", path, err)
	}
	if fi.Size() > maxSVSize {
		return addondata.DB{}, fmt.Errorf("%s troppo grande (%d byte), saltato", path, fi.Size())
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return addondata.DB{}, fmt.Errorf("%s: %w", path, err)
	}
	g, err := svparse.Parse(src)
	if err != nil {
		return addondata.DB{}, fmt.Errorf("%s: %w", path, err)
	}
	db, err := addondata.Decode(g)
	if err != nil {
		return addondata.DB{}, fmt.Errorf("%s: %w", path, err)
	}
	return db, nil
}

// build1 legge il registro attorno alla sessione e produce il file da accodare.
func build1(env Env, s addondata.Session, interrupted bool) (string, []byte, error) {
	end := lastTime(s)
	if s.Stop != nil {
		end = *s.Stop
	}
	from, to := time.Unix(s.Start-logMargin, 0), time.Unix(end+logMargin, 0)
	evs, err := combatlog.Read(combatlog.Files(wowdir.LogsDir(env.Cfg.ClientDir)), from, to, env.loc())
	if err != nil {
		env.logf("registro combattimenti (sessione %s): %v", s.ID, err)
	}
	eps := episodes.Build(s, evs, env.loc())
	return payload.Build(env.Cfg.Player, build.Version, s, eps, interrupted)
}

// final dice se la sessione e' definitiva e se e' interrotta (crash).
func final(s addondata.Session, all []addondata.Session, now int64) (ok, interrupted bool) {
	if s.Stop == nil {
		return now-lastTime(s) > crashAge, true
	}
	if now-*s.Stop > reloadGap {
		return true, false
	}
	k := charKey(s)
	if k == "" {
		return false, false
	}
	for _, o := range all {
		if o.ID != s.ID && o.Start > s.Start && charKey(o) == k {
			return true, false
		}
	}
	return false, false
}

// lastTime e' l'ora dell'ultimo evento, o start se non ce ne sono.
func lastTime(s addondata.Session) int64 {
	t := s.Start
	for _, e := range s.Events {
		if e.T > t {
			t = e.T
		}
	}
	return t
}

// charKey e' "realm-char"; se mancano si ricava dall'id ("Realm-Char-<start>").
func charKey(s addondata.Session) string {
	if s.Realm != "" && s.Char != "" {
		return s.Realm + "-" + s.Char
	}
	i := strings.LastIndexByte(s.ID, '-')
	if i <= 0 {
		return ""
	}
	if _, err := strconv.ParseInt(s.ID[i+1:], 10, 64); err != nil {
		return ""
	}
	return s.ID[:i]
}

// Watch esegue Cycle all'avvio, quando i file SavedVariables cambiano e poi
// restano uguali per stable, e comunque ogni 5 minuti. Ritorna quando ctx e'
// cancellato. Gli errori di Cycle vanno nel log e il controllo continua.
func Watch(ctx context.Context, env Env, poll, stable time.Duration) {
	watch(ctx, env, poll, stable, fullEvery)
}

func watch(ctx context.Context, env Env, poll, stable, every time.Duration) {
	if poll <= 0 {
		poll = 10 * time.Second
	}
	run := func() {
		if err := Cycle(ctx, env); err != nil && ctx.Err() == nil {
			env.logf("giro non completato: %v", err)
		}
	}
	last := snapshot(env.Cfg.ClientDir)
	run()
	lastRun := time.Now()
	pending := false
	var changedAt time.Time

	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		cur := snapshot(env.Cfg.ClientDir)
		if cur != last {
			last, pending, changedAt = cur, true, time.Now()
			continue
		}
		if (pending && time.Since(changedAt) >= stable) || time.Since(lastRun) >= every {
			pending = false
			run()
			lastRun = time.Now()
		}
	}
}

// snapshot riassume percorso, ModTime e dimensione dei file SavedVariables.
func snapshot(clientDir string) string {
	if clientDir == "" {
		return ""
	}
	var b strings.Builder
	for _, f := range wowdir.SavedVariables(clientDir) {
		fi, err := os.Stat(f)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "%s|%d|%d\n", f, fi.ModTime().UnixNano(), fi.Size())
	}
	return b.String()
}
