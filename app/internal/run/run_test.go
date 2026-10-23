package run

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oLdpZ/diario-gilda/app/internal/config"
	"github.com/oLdpZ/diario-gilda/app/internal/state"
)

// Valori della fixture del Piano 1 (tests/fixtures/DiarioGilda.lua).
const (
	id1       = "Realm-Gino-1793000000"
	id2       = "Realm-Gino-1793086400"
	stop1     = int64(1793001395)
	lastEvt2  = int64(1793086745)
	session1H = "\n\t\t}, -- [1]\n" // fine della sessione 1 (gli eventi sono piu' indentati)
	session2H = "\n\t\t}, -- [2]\n"
)

// testLoc fa cadere le righe di murloc.txt (24/10/2026 21:02:03 ...) nel
// combattimento della prima sessione della fixture (from = 1793000060).
var testLoc = time.FixedZone("T", int(time.Date(2026, 10, 24, 21, 2, 3, 0, time.UTC).Unix()-1793000060))

func fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "fixtures", "DiarioGilda.lua"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// only1 toglie la seconda sessione, only2 la prima.
func only1(t *testing.T) string {
	src := fixture(t)
	i := strings.Index(src, session1H) + len(session1H)
	j := strings.Index(src, session2H) + len(session2H)
	if i < len(session1H) || j < len(session2H) {
		t.Fatal("fixture inattesa")
	}
	return src[:i] + src[j:]
}

func only2(t *testing.T) string {
	src := fixture(t)
	start := strings.Index(src, "\t[\"sessions\"] = {\n") + len("\t[\"sessions\"] = {\n")
	i := strings.Index(src, session1H) + len(session1H)
	return src[:start] + src[i:]
}

type tree struct {
	client string
	data   string
}

func (tr tree) svPath(account string) string {
	return filepath.Join(tr.client, "WTF", "Account", account, "SavedVariables", "DiarioGilda.lua")
}

func (tr tree) writeSV(t *testing.T, account, content string) {
	t.Helper()
	p := tr.svPath(account)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newTree crea un albero WoW finto; withLog copia murloc.txt in Logs/.
func newTree(t *testing.T, withLog bool) tree {
	t.Helper()
	root := t.TempDir()
	tr := tree{client: filepath.Join(root, "WoW", "_forever_"), data: filepath.Join(root, "data")}
	for _, d := range []string{"Interface", "WTF", "Logs"} {
		if err := os.MkdirAll(filepath.Join(tr.client, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if withLog {
		src, err := os.ReadFile(filepath.Join("..", "combatlog", "testdata", "murloc.txt"))
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(tr.client, "Logs", "WoWCombatLog-102426_210158.txt")
		if err := os.WriteFile(p, src, 0o644); err != nil {
			t.Fatal(err)
		}
		mt := time.Unix(stop1, 0)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	return tr
}

type sent struct {
	name string
	doc  map[string]any
}

type recorder struct {
	mu   sync.Mutex
	got  []sent
	fail bool
	logs []string
}

func (r *recorder) send(name string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("rete assente")
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	r.got = append(r.got, sent{name, doc})
	return nil
}

func (r *recorder) setFail(v bool) { r.mu.Lock(); r.fail = v; r.mu.Unlock() }

func (r *recorder) count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.got) }

// take restituisce e azzera gli invii registrati.
func (r *recorder) take() []sent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.got
	r.got = nil
	return out
}

func (r *recorder) logf(format string, args ...any) {
	r.mu.Lock()
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
	r.mu.Unlock()
}

func (r *recorder) logged(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, l := range r.logs {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func (tr tree) env(r *recorder, now int64) Env {
	return Env{
		Cfg:     config.Config{Player: "Gigi", ClientDir: tr.client, Webhook: "https://segreto.example/hook"},
		DataDir: tr.data,
		Now:     func() time.Time { return time.Unix(now, 0) },
		Loc:     testLoc,
		Send:    r.send,
		Log:     r.logf,
	}
}

func cycle(t *testing.T, env Env) {
	t.Helper()
	if err := Cycle(context.Background(), env); err != nil {
		t.Fatalf("Cycle: %v", err)
	}
}

func sessionID(s sent) string {
	sess, _ := s.doc["sessione"].(map[string]any)
	id, _ := sess["id"].(string)
	return id
}

func ids(ss []sent) []string {
	out := []string{}
	for _, s := range ss {
		out = append(out, sessionID(s))
	}
	return out
}

func outboxCount(t *testing.T, tr tree) int {
	t.Helper()
	es, err := os.ReadDir(filepath.Join(tr.data, "outbox"))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(es)
}

func readState(t *testing.T, tr tree) *state.State {
	t.Helper()
	st, err := state.LoadAt(tr.data, stop1)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// firstCombat restituisce il primo episodio "combat" del documento.
func firstCombat(t *testing.T, s sent) map[string]any {
	t.Helper()
	eps, _ := s.doc["combattimenti"].([]any)
	for _, e := range eps {
		if m, _ := e.(map[string]any); m["kind"] == "combat" {
			return m
		}
	}
	t.Fatalf("nessun episodio combat in %v", s.doc["combattimenti"])
	return nil
}

func TestReloadWindow(t *testing.T) {
	tr := newTree(t, true)
	tr.writeSV(t, "ACC", only1(t))
	r := &recorder{}

	cycle(t, tr.env(r, stop1+60))
	if got := r.take(); len(got) != 0 {
		t.Fatalf("Stop+60: nessun invio atteso (un /reload puo' riaprirla), ottenuti %v", ids(got))
	}
	if readState(t, tr).Has(id1) {
		t.Fatal("Stop+60: la sessione non deve risultare in coda")
	}

	cycle(t, tr.env(r, stop1+601))
	got := r.take()
	if len(got) != 1 || sessionID(got[0]) != id1 {
		t.Fatalf("Stop+601: atteso 1 invio di %s, ottenuti %v", id1, ids(got))
	}
	if !strings.HasPrefix(got[0].name, "sessione_Gigi_") {
		t.Errorf("nome file: %s", got[0].name)
	}

	cycle(t, tr.env(r, stop1+700))
	if got := r.take(); len(got) != 0 {
		t.Fatalf("giro successivo: nessun invio atteso, ottenuti %v", ids(got))
	}
}

func TestFixtureClosedSentOpenKept(t *testing.T) {
	tr := newTree(t, true)
	tr.writeSV(t, "ACC", fixture(t))
	r := &recorder{}
	env := tr.env(r, stop1+601)
	env.Cfg.Webhook = ""
	cycle(t, env)
	got := r.take()
	if len(got) != 1 || sessionID(got[0]) != id1 {
		t.Fatalf("atteso solo %s, ottenuti %v", id1, ids(got))
	}
	d := got[0].doc
	if d["giocatore"] != "Gigi" || d["interrotta"] != false {
		t.Errorf("giocatore/interrotta: %v %v", d["giocatore"], d["interrotta"])
	}
	if n, _ := firstCombat(t, got[0])["enemyCount"].(float64); n == 0 {
		t.Errorf("episodio senza dati del registro: %v", firstCombat(t, got[0]))
	}
	cycle(t, tr.env(r, stop1+700))
	if got := r.take(); len(got) != 0 {
		t.Fatalf("giro successivo: %v", ids(got))
	}
}

func TestNewerSessionSendsImmediately(t *testing.T) {
	tr := newTree(t, true)
	tr.writeSV(t, "ACC", fixture(t))
	r := &recorder{}
	cycle(t, tr.env(r, stop1+60))
	if got := r.take(); len(got) != 1 || sessionID(got[0]) != id1 {
		t.Fatalf("atteso %s subito (c'e' una sessione piu' nuova), ottenuti %v", id1, ids(got))
	}
}

func TestNewerSessionInAnotherFile(t *testing.T) {
	tr := newTree(t, true)
	tr.writeSV(t, "A", only1(t))
	tr.writeSV(t, "B", only2(t))
	r := &recorder{}
	cycle(t, tr.env(r, stop1+60))
	if got := r.take(); len(got) != 1 || sessionID(got[0]) != id1 {
		t.Fatalf("la sessione piu' nuova in un altro file rende definitiva %s, ottenuti %v", id1, ids(got))
	}
}

func TestInterrupted(t *testing.T) {
	tr := newTree(t, true)
	tr.writeSV(t, "ACC", fixture(t))
	r := &recorder{}

	cycle(t, tr.env(r, lastEvt2+11*3600))
	if got := r.take(); len(got) != 1 || sessionID(got[0]) != id1 {
		t.Fatalf("+11 ore: atteso solo %s, ottenuti %v", id1, ids(got))
	}

	cycle(t, tr.env(r, lastEvt2+13*3600))
	got := r.take()
	if len(got) != 1 || sessionID(got[0]) != id2 {
		t.Fatalf("+13 ore: atteso %s, ottenuti %v", id2, ids(got))
	}
	if got[0].doc["interrotta"] != true {
		t.Errorf("interrotta = %v", got[0].doc["interrotta"])
	}
}

func TestSendFailureKeepsQueue(t *testing.T) {
	tr := newTree(t, true)
	tr.writeSV(t, "ACC", only1(t))
	r := &recorder{fail: true}

	if err := Cycle(context.Background(), tr.env(r, stop1+601)); err == nil {
		t.Fatal("atteso errore di invio")
	}
	if n := outboxCount(t, tr); n != 1 {
		t.Fatalf("outbox: %d file", n)
	}
	if !readState(t, tr).Has(id1) {
		t.Fatal("lo stato deve segnare la sessione in coda")
	}
	if strings.Contains(strings.Join(r.logs, "\n"), "segreto") {
		t.Fatal("webhook nel log")
	}

	// ancora offline: nessun duplicato in coda
	_ = Cycle(context.Background(), tr.env(r, stop1+700))
	if n := outboxCount(t, tr); n != 1 {
		t.Fatalf("outbox dopo il secondo giro: %d file", n)
	}

	r.setFail(false)
	cycle(t, tr.env(r, stop1+800))
	if got := r.take(); len(got) != 1 || sessionID(got[0]) != id1 {
		t.Fatalf("atteso 1 invio, ottenuti %v", ids(got))
	}
	if n := outboxCount(t, tr); n != 0 {
		t.Fatalf("outbox non svuotata: %d", n)
	}
	cycle(t, tr.env(r, stop1+900))
	if got := r.take(); len(got) != 0 {
		t.Fatalf("duplicato: %v", ids(got))
	}
}

func TestTruncatedSavedVariables(t *testing.T) {
	tr := newTree(t, true)
	full := fixture(t)
	tr.writeSV(t, "ACC", full[:len(full)/2])
	r := &recorder{}

	cycle(t, tr.env(r, stop1+601))
	if got := r.take(); len(got) != 0 {
		t.Fatalf("file troncato: nessun invio atteso, ottenuti %v", ids(got))
	}
	if st := readState(t, tr); len(st.Sent) != 0 {
		t.Fatalf("file troncato: nessuna sessione marcata, stato %v", st.Sent)
	}
	if !r.logged("DiarioGilda.lua") {
		t.Errorf("errore di parse non registrato nel log: %v", r.logs)
	}

	tr.writeSV(t, "ACC", full)
	cycle(t, tr.env(r, stop1+700))
	if got := r.take(); len(got) != 1 || sessionID(got[0]) != id1 {
		t.Fatalf("file intero: atteso %s, ottenuti %v", id1, ids(got))
	}
}

func TestNoCombatLog(t *testing.T) {
	for _, tc := range []struct {
		name   string
		noLogs bool
	}{{"Logs vuota", false}, {"Logs assente", true}} {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTree(t, false)
			if tc.noLogs {
				if err := os.Remove(filepath.Join(tr.client, "Logs")); err != nil {
					t.Fatal(err)
				}
			}
			tr.writeSV(t, "ACC", only1(t))
			r := &recorder{}
			cycle(t, tr.env(r, stop1+601))
			got := r.take()
			if len(got) != 1 {
				t.Fatalf("atteso 1 invio, ottenuti %v", ids(got))
			}
			ep := firstCombat(t, got[0])
			if n, _ := ep["enemyCount"].(float64); n != 0 {
				t.Errorf("senza registro enemyCount = %v", n)
			}
			if ep["minHp"] != float64(12) {
				t.Errorf("minHp dalla sessione: %v", ep["minHp"])
			}
		})
	}
}

func TestCorruptState(t *testing.T) {
	tr := newTree(t, true)
	tr.writeSV(t, "ACC", only1(t))
	if err := os.MkdirAll(tr.data, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"state.json", "state.json.corrotto"} {
		if err := os.WriteFile(filepath.Join(tr.data, f), []byte("{rotto"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := &recorder{}
	cycle(t, tr.env(r, stop1+601))
	if got := r.take(); len(got) != 1 {
		t.Fatalf("atteso 1 invio con stato corrotto, ottenuti %v", ids(got))
	}
	if _, err := os.Stat(filepath.Join(tr.data, "state.json.corrotto")); err != nil {
		t.Fatalf("state.json.corrotto: %v", err)
	}
	if !readState(t, tr).Has(id1) {
		t.Fatal("nuovo stato non salvato")
	}
	if !r.logged("corrotto") {
		t.Errorf("stato corrotto non registrato nel log: %v", r.logs)
	}
}

func TestOversizedSavedVariablesSkipped(t *testing.T) {
	tr := newTree(t, true)
	tr.writeSV(t, "BIG", only1(t))
	if err := os.Truncate(tr.svPath("BIG"), maxSVSize+1); err != nil {
		t.Fatal(err)
	}
	r := &recorder{}
	cycle(t, tr.env(r, stop1+601))
	if got := r.take(); len(got) != 0 {
		t.Fatalf("file troppo grande: %v", ids(got))
	}
	if !r.logged("troppo grande") {
		t.Errorf("log: %v", r.logs)
	}
}

func TestEmptyClientDirOnlyFlushes(t *testing.T) {
	tr := newTree(t, true)
	r := &recorder{}
	env := tr.env(r, stop1)
	env.Cfg.ClientDir = ""
	cycle(t, env)
}

// --- Watch ---

func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func TestWatchStopsOnCancel(t *testing.T) {
	tr := newTree(t, true)
	r := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { Watch(ctx, tr.env(r, stop1+601), 10*time.Millisecond, 20*time.Millisecond); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch non si ferma alla cancellazione del contesto")
	}
}

func TestWatchCyclesAtStartAndOnStableChange(t *testing.T) {
	tr := newTree(t, true)
	full := fixture(t)
	tr.writeSV(t, "ACC", full[:len(full)/2]) // WoW sta scrivendo
	r := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		watch(ctx, tr.env(r, stop1+601), 10*time.Millisecond, 30*time.Millisecond, time.Hour)
		close(done)
	}()
	if !waitFor(t, 2*time.Second, func() bool { return r.logged("DiarioGilda.lua") }) {
		t.Fatal("nessun Cycle all'avvio")
	}
	tr.writeSV(t, "ACC", full)
	mt := time.Now().Add(time.Second) // ModTime sicuramente diverso
	_ = os.Chtimes(tr.svPath("ACC"), mt, mt)
	if !waitFor(t, 3*time.Second, func() bool { return r.count() == 1 }) {
		t.Fatalf("file cambiato e stabile: atteso 1 invio, ottenuti %d", r.count())
	}
	cancel()
	<-done
}

func TestWatchSurvivesCycleError(t *testing.T) {
	tr := newTree(t, true)
	tr.writeSV(t, "ACC", only1(t))
	r := &recorder{fail: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		watch(ctx, tr.env(r, stop1+601), 10*time.Millisecond, 20*time.Millisecond, 50*time.Millisecond)
		close(done)
	}()
	if !waitFor(t, 2*time.Second, func() bool { return outboxCount(t, tr) == 1 }) {
		t.Fatal("sessione non accodata")
	}
	r.setFail(false)
	// il giro periodico riprova l'invio dopo l'errore
	if !waitFor(t, 3*time.Second, func() bool { return r.count() == 1 }) {
		t.Fatalf("dopo l'errore Watch non ha riprovato: %d invii", r.count())
	}
	cancel()
	<-done
	if strings.Contains(strings.Join(r.logs, "\n"), "segreto") {
		t.Fatal("webhook nel log")
	}
}
