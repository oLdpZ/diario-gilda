package payload

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/oLdpZ/diario-gilda/app/internal/addondata"
	"github.com/oLdpZ/diario-gilda/app/internal/episodes"
)

func unpack(t *testing.T, data []byte) map[string]json.RawMessage {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestBuildBasic(t *testing.T) {
	s := addondata.Session{ID: "abc 1", Events: []addondata.Event{{T: 1, K: "x"}, {T: 2, K: "y"}}}
	eps := []episodes.Episode{{From: 1, To: 2, Kind: "combat", Enemies: []episodes.NameCount{{Name: "a", N: 1}}}}
	name, data, err := Build("Élodie Rossi", "1.0.0", s, eps, true)
	if err != nil {
		t.Fatal(err)
	}
	if name != "sessione__lodie_Rossi_abc_1.json.gz" {
		t.Fatalf("name = %q", name)
	}
	m := unpack(t, data)
	var f int
	json.Unmarshal(m["formato"], &f)
	var g string
	json.Unmarshal(m["giocatore"], &g)
	var intr bool
	json.Unmarshal(m["interrotta"], &intr)
	if f != 1 || g != "Élodie Rossi" || !intr {
		t.Fatalf("header: %v %v %v", f, g, intr)
	}
	var sess addondata.Session
	var ce []episodes.Episode
	json.Unmarshal(m["sessione"], &sess)
	json.Unmarshal(m["combattimenti"], &ce)
	if len(ce) != 1 || len(ce[0].Enemies) != 1 {
		t.Fatalf("episodes: %+v", ce)
	}
	var sraw struct{ Events []json.RawMessage }
	json.Unmarshal(m["sessione"], &struct{}{})
	_ = sraw
	if _, ok := m["troncata"]; ok {
		t.Fatal("troncata should be absent")
	}
	if _, ok := m["app"]; !ok {
		t.Fatal("app missing")
	}
}

func TestBuildNilEpisodesIsEmptyArray(t *testing.T) {
	_, data, err := Build("p", "v", addondata.Session{ID: "1"}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	m := unpack(t, data)
	if string(m["combattimenti"]) != "[]" {
		t.Fatalf("combattimenti = %s", m["combattimenti"])
	}
}

func hexStr(r *rand.Rand, n int) string {
	const d = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = d[r.Intn(16)]
	}
	return string(b)
}

func TestBuildHugeTruncates(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	evs := make([]addondata.Event, 200000)
	for i := range evs {
		evs[i] = addondata.Event{T: int64(i), K: "dmg", Fields: map[string]any{"blob": hexStr(r, 150)}}
	}
	s := addondata.Session{ID: "big", Events: evs}
	eps := []episodes.Episode{{Kind: "combat", EnemyCount: 3,
		Enemies: []episodes.NameCount{{Name: "a", N: 1}}, Healers: []episodes.NameAmount{{Name: "h", Amount: 1}},
		Kills: []episodes.NameCount{{Name: "k", N: 1}}}}
	start := time.Now()
	_, data, err := Build("p", "v", s, eps, false)
	el := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > MaxUpload {
		t.Fatalf("size %d > MaxUpload", len(data))
	}
	if el > 20*time.Second {
		t.Fatalf("too slow: %v", el)
	}
	t.Logf("elapsed %v size %d", el, len(data))
	m := unpack(t, data)
	var tr int
	if err := json.Unmarshal(m["troncata"], &tr); err != nil || tr <= 0 {
		t.Fatalf("troncata = %s", m["troncata"])
	}
	var out struct {
		Events []addondata.Event `json:"events"`
	}
	var sraw struct {
		Events []struct{ T int64 } `json:"events"`
	}
	json.Unmarshal(m["sessione"], &sraw)
	_ = out
	if len(sraw.Events)+tr != 200000 {
		t.Fatalf("kept %d + dropped %d != 200000", len(sraw.Events), tr)
	}
	if sraw.Events[len(sraw.Events)-1].T != 199999 {
		t.Fatal("newest event must be kept")
	}
	var ce []episodes.Episode
	json.Unmarshal(m["combattimenti"], &ce)
	if len(ce[0].Enemies)+len(ce[0].Healers)+len(ce[0].Kills) != 0 || ce[0].EnemyCount != 3 {
		t.Fatalf("episode not trimmed: %+v", ce[0])
	}
	// caller data untouched
	if len(eps[0].Enemies) != 1 || len(s.Events) != 200000 {
		t.Fatal("caller data mutated")
	}
}

func TestBuildTrimEpisodesOnly(t *testing.T) {
	// many episodes with big lists, few events: dropping lists is enough, no troncata
	r := rand.New(rand.NewSource(2))
	var eps []episodes.Episode
	for i := 0; i < 1000; i++ {
		var en []episodes.NameCount
		for j := 0; j < 400; j++ {
			en = append(en, episodes.NameCount{Name: hexStr(r, 40), N: j})
		}
		eps = append(eps, episodes.Episode{Kind: "combat", Enemies: en})
	}
	_, data, err := Build("p", "v", addondata.Session{ID: "e"}, eps, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > MaxUpload {
		t.Fatal("too big")
	}
	m := unpack(t, data)
	if _, ok := m["troncata"]; ok {
		t.Fatal("troncata should be absent")
	}
	if strings.Contains(string(m["combattimenti"]), "\"name\"") {
		t.Fatal("enemy lists should be cleared")
	}
}
