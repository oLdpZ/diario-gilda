package episodes

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oLdpZ/diario-gilda/app/internal/addondata"
	"github.com/oLdpZ/diario-gilda/app/internal/combatlog"
)

var loc = time.FixedZone("T", 2*3600)

func load(t *testing.T, rel string) []combatlog.Event {
	t.Helper()
	evs, err := combatlog.Read([]string{rel}, time.Time{}, time.Time{}, loc)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func at(h, m, s int) int64 { return time.Date(2026, 10, 24, h, m, s, 0, loc).Unix() }

func sess(evs ...addondata.Event) addondata.Session {
	return addondata.Session{ID: "x", Char: "Gino", Realm: "Realm", Events: evs}
}

func TestCombatMurloc(t *testing.T) {
	evs := load(t, filepath.Join("..", "combatlog", "testdata", "murloc.txt"))
	s := sess(addondata.Event{T: at(21, 2, 22), K: "combat", Fields: map[string]any{
		"from": at(21, 2, 3), "to": at(21, 2, 21), "minHp": int64(12)}})
	eps := Build(s, evs, loc)
	if len(eps) != 1 {
		t.Fatalf("episodi: %d", len(eps))
	}
	e := eps[0]
	if e.Kind != "combat" || e.From != at(21, 2, 3) || e.To != at(21, 2, 21) {
		t.Errorf("kind/from/to: %+v", e)
	}
	if e.EnemyCount != 6 {
		t.Errorf("EnemyCount=%d", e.EnemyCount)
	}
	want := []NameCount{{"Murloc Forager", 3}, {"Murloc Netter", 3}}
	if len(e.Enemies) != 2 || e.Enemies[0] != want[0] || e.Enemies[1] != want[1] {
		t.Errorf("Enemies=%v", e.Enemies)
	}
	if e.MinHP == nil || *e.MinHP != 12 {
		t.Errorf("MinHP=%v", e.MinHP)
	}
	if len(e.Healers) != 1 || e.Healers[0].Name != "Mara-Realm" || e.Healers[0].Amount != 640+90+655+90 {
		t.Errorf("Healers=%v", e.Healers)
	}
	if len(e.Kills) != 1 || e.Kills[0] != (NameCount{"Murloc Forager", 1}) {
		t.Errorf("Kills=%v", e.Kills)
	}
	if e.BiggestHitTaken == nil || e.BiggestHitTaken.Amount != 126 || !e.BiggestHitTaken.Crit {
		t.Errorf("BiggestHitTaken=%+v", e.BiggestHitTaken)
	}
	if e.BiggestHitDone == nil || e.BiggestHitDone.Amount != 210 || e.BiggestHitDone.Spell != "Heroic Strike" {
		t.Errorf("BiggestHitDone=%+v", e.BiggestHitDone)
	}
	if e.KilledBy != nil || len(e.Environmental) != 0 {
		t.Errorf("unexpected KilledBy/Environmental: %+v %v", e.KilledBy, e.Environmental)
	}
}

func TestDeathFalling(t *testing.T) {
	evs := load(t, filepath.Join("testdata", "death.txt"))
	s := sess(addondata.Event{T: at(21, 6, 42), K: "death", Fields: map[string]any{}})
	eps := Build(s, evs, loc)
	if len(eps) != 1 || eps[0].Kind != "death" {
		t.Fatalf("eps=%+v", eps)
	}
	e := eps[0]
	if len(e.Environmental) == 0 || e.Environmental[0].Name != "Falling" || e.Environmental[0].Amount != 340 {
		t.Errorf("Environmental=%v", e.Environmental)
	}
	if e.KilledBy == nil || e.KilledBy.Who != "Falling" {
		t.Errorf("KilledBy=%+v", e.KilledBy)
	}
	// il colpo subito da un altro giocatore e' ignorato
	if e.EnemyCount != 1 || e.DamageTaken != 40+340 {
		t.Errorf("EnemyCount=%d DamageTaken=%d", e.EnemyCount, e.DamageTaken)
	}
	if e.MinHP != nil {
		t.Errorf("MinHP=%v", e.MinHP)
	}
}

func TestNoLogEvents(t *testing.T) {
	s := sess(
		addondata.Event{T: 1000, K: "combat", Fields: map[string]any{"from": int64(990), "to": int64(1000), "minHp": float64(40)}},
		addondata.Event{T: 2000, K: "combat", Fields: map[string]any{"from": int64(1990), "to": int64(2000)}},
		addondata.Event{T: 3000, K: "loot", Fields: map[string]any{}},
	)
	eps := Build(s, nil, loc)
	if len(eps) != 2 {
		t.Fatalf("eps=%d", len(eps))
	}
	if eps[0].MinHP == nil || *eps[0].MinHP != 40 || eps[1].MinHP != nil {
		t.Errorf("MinHP: %v %v", eps[0].MinHP, eps[1].MinHP)
	}
	b, _ := json.Marshal(eps[1])
	js := string(b)
	for _, w := range []string{`"enemies":[]`, `"healers":[]`, `"environmental":[]`, `"kills":[]`} {
		if !strings.Contains(js, w) {
			t.Errorf("manca %s in %s", w, js)
		}
	}
	if strings.Contains(js, "minHp") || strings.Contains(js, "killedBy") || strings.Contains(js, "biggestHit") {
		t.Errorf("puntatori nil non omessi: %s", js)
	}
}

func TestForeignPlayerIgnored(t *testing.T) {
	evs := load(t, filepath.Join("testdata", "death.txt"))
	f := map[string]any{"from": at(21, 6, 40), "to": at(21, 6, 42)}
	stranger := addondata.Session{Char: "Stranger", Realm: "Realm", Events: []addondata.Event{{T: at(21, 6, 42), K: "combat", Fields: f}}}
	if e := Build(stranger, evs, loc)[0]; e.EnemyCount != 1 || e.DamageTaken != 999 {
		t.Errorf("%+v", e)
	}
	nobody := addondata.Session{Char: "Nobody", Realm: "Realm", Events: stranger.Events}
	if e := Build(nobody, evs, loc)[0]; e.EnemyCount != 0 || e.DamageTaken != 0 {
		t.Errorf("%+v", e)
	}
}
