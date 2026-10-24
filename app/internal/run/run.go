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
	"sort"
	"strconv"
	"strings"
	"sync"
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
	logMargin   = 60         // s attorno alla sessione letti dal registro combattimenti
	maxAge      = 50 * 86400 // s: oltre non si invia (lo stato dimentica gli id dopo 60 giorni)
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
	// GameRunning dice se WoW e' aperto (nil = chiuso). Con WoW aperto lo stop
	// su disco puo' essere quello di un /reload: vedi final.
	GameRunning func() bool
	// NeedMain viene chiamata quando ci sono sessioni pronte ma Cfg.Main non e'
	// scelto: quelle sessioni restano in attesa. chars sono i personaggi delle
	// SavedVariables, il piu' recente prima.
	NeedMain func(chars []config.Character)

	saveState func(dir string, st *state.State) error // solo per i test; nil = state.Save
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

func (e Env) gameRunning() bool { return e.GameRunning != nil && e.GameRunning() }

func (e Env) save(st *state.State) error {
	if e.saveState != nil {
		return e.saveState(e.DataDir, st)
	}
	return state.Save(e.DataDir, st)
}

func (e Env) outboxDir() string { return filepath.Join(e.DataDir, "outbox") }

// cycleMu serializza i Cycle (Watch e il comando "Invia ora" dell'interfaccia):
// due giri insieme accoderebbero e invierebbero la stessa sessione due volte.
var cycleMu sync.Mutex

// Cycle accoda le sessioni definitive non ancora accodate, salva lo stato e
// svuota la coda. Un file SavedVariables illeggibile o a meta' scrittura viene
// saltato (si riprova al giro dopo). Se lo stato non si salva non si invia nulla
// (altrimenti la stessa sessione partirebbe a ogni giro): i file restano in coda
// e al giro dopo vengono riscritti con lo stesso nome. L'errore restituito e'
// quello dello stato o dell'invio; il prossimo Cycle riprova. Sicuro da chiamare
// da piu' goroutine: i giri sono serializzati.
func Cycle(ctx context.Context, env Env) error {
	cycleMu.Lock()
	defer cycleMu.Unlock()
	now := env.now().Unix()
	st, err := loadState(env, now)
	if err != nil {
		return err
	}

	if env.Cfg.ClientDir == "" {
		env.logf("cartella del client non impostata: solo invio della coda")
	} else if queued := enqueue(env, st, now); queued > 0 {
		if err := env.save(st); err != nil {
			return fmt.Errorf("salvataggio stato (invio rimandato): %w", err)
		}
	}

	if env.Send == nil {
		return nil
	}
	n, err := outbox.Flush(ctx, env.outboxDir(), env.Log, env.Send)
	if n > 0 {
		env.logf("inviati %d file", n)
	}
	return err
}

// loadState carica lo stato; se state.json e' corrotto lo mette da parte come
// state.json.corrotto e riparte vuoto (al massimo un duplicato, che il bot scarta).
func loadState(env Env, now int64) (*state.State, error) {
	st, err := state.LoadAt(env.DataDir, now)
	if err == nil {
		return st, nil
	}
	if !errors.Is(err, state.ErrCorrupt) {
		return nil, fmt.Errorf("lettura stato: %w", err) // errore di disco, non di formato
	}
	p := filepath.Join(env.DataDir, stateFile)
	if rerr := os.Rename(p, filepath.Join(env.DataDir, corruptFile)); rerr != nil {
		return nil, fmt.Errorf("stato corrotto non spostato: %w", rerr)
	}
	env.logf("stato corrotto (%v): spostato in %s, si riparte da vuoto", err, corruptFile)
	return &state.State{Sent: map[string]int64{}}, nil
}

// enqueue legge tutte le SavedVariables del client e accoda le sessioni
// definitive del personaggio principale; quelle degli altri personaggi si
// segnano senza inviarle. Senza personaggio principale non accoda nulla e lo
// chiede (NeedMain). Restituisce quante sessioni ha segnato nello stato.
func enqueue(env Env, st *state.State, now int64) int {
	all := readAll(env)
	running := env.gameRunning()
	main := env.Cfg.Main
	marked, waiting := 0, false
	seen := map[string]bool{}
	for _, s := range all {
		if seen[s.ID] || st.Has(s.ID) {
			continue
		}
		seen[s.ID] = true
		ok, interrupted := final(s, all, now, running)
		if !ok {
			continue
		}
		if main.IsZero() {
			waiting = true
			continue
		}
		if !main.Is(s.Char, s.Realm) {
			env.logf("sessione %s di %s: personaggio secondario, non inviata", s.ID, s.Char)
			st.MarkQueued(s.ID, now)
			marked++
			continue
		}
		if now-s.Start > maxAge {
			// segnata senza invio: dopo una lunga pausa lo stato potrebbe averla dimenticata
			env.logf("sessione %s troppo vecchia, non inviata", s.ID)
			st.MarkQueued(s.ID, now)
			marked++
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
	if waiting {
		env.logf("personaggio principale non scelto: sessioni in attesa")
		if env.NeedMain != nil {
			env.NeedMain(characters(all))
		}
	}
	return marked
}

// readAll legge le sessioni di tutte le SavedVariables del client; un file
// illeggibile si salta (si riprova al giro dopo).
func readAll(env Env) []addondata.Session {
	var all []addondata.Session
	for _, f := range wowdir.SavedVariables(env.Cfg.ClientDir) {
		db, err := readSV(f)
		if err != nil {
			env.logf("%v (si riprova al prossimo giro)", err)
			continue
		}
		all = append(all, db.Sessions...)
	}
	return all
}

// Characters restituisce i personaggi delle SavedVariables di clientDir, il
// piu' recente (ultima sessione iniziata) prima.
func Characters(clientDir string) []config.Character {
	return characters(readAll(Env{Cfg: config.Config{ClientDir: clientDir}}))
}

func characters(all []addondata.Session) []config.Character {
	type seen struct {
		c    config.Character
		last int64
	}
	var out []seen
outer:
	for _, s := range all {
		if s.Char == "" {
			continue
		}
		for i := range out {
			if out[i].c.Is(s.Char, s.Realm) {
				out[i].last = max(out[i].last, s.Start)
				continue outer
			}
		}
		out = append(out, seen{config.Character{Char: s.Char, Realm: s.Realm}, s.Start})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].last > out[j].last })
	chars := make([]config.Character, len(out))
	for i, x := range out {
		chars[i] = x.c
	}
	return chars
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
	// solo le righe che riguardano il giocatore: una serata in raid ne ha milioni
	evs, err := combatlog.ReadFiltered(combatlog.Files(wowdir.LogsDir(env.Cfg.ClientDir)), from, to, env.loc(), episodes.Relevant(s))
	if err != nil {
		env.logf("registro combattimenti (sessione %s): %v", s.ID, err)
	}
	eps := episodes.Build(s, evs, env.loc())
	return payload.Build(env.Cfg.Player, build.Version, s, eps, interrupted)
}

// final dice se la sessione e' definitiva e se e' interrotta (crash). Con WoW
// aperto (running) uno stop vecchio non basta: dopo un /reload il file ha lo
// stop del reload ma l'addon ha riaperto la sessione in memoria, quindi conta
// solo una sessione piu' nuova dello stesso personaggio. La regola dei 600 s
// vale a gioco chiuso.
func final(s addondata.Session, all []addondata.Session, now int64, running bool) (ok, interrupted bool) {
	if s.Stop == nil {
		return now-lastTime(s) > crashAge, true
	}
	if !running && now-*s.Stop > reloadGap {
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
		changed := cur != last
		if changed {
			last, pending, changedAt = cur, true, time.Now()
		}
		// il giro periodico parte anche se i file cambiano di continuo
		if (!changed && pending && time.Since(changedAt) >= stable) || time.Since(lastRun) >= every {
			if !changed {
				pending = false
			}
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
