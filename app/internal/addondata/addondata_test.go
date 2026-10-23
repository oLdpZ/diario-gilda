package addondata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oLdpZ/diario-gilda/app/internal/svparse"
)

func loadFixture(t *testing.T) DB {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "fixtures", "DiarioGilda.lua"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := svparse.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := g["DiarioGildaDB"].(map[string]any)
	if !ok {
		t.Fatal("DiarioGildaDB missing")
	}
	db, err := Decode(g)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestDecodeFixture(t *testing.T) {
	db := loadFixture(t)
	if db.V != 1 || len(db.Sessions) != 2 {
		t.Fatalf("V=%d sessions=%d", db.V, len(db.Sessions))
	}
	s := db.Sessions[0]
	if s.Stop == nil || *s.Stop != 1793001395 {
		t.Fatalf("Stop = %v", s.Stop)
	}
	if s.ID != "Realm-Gino-1793000000" || s.Guild != "I Murloc Rampanti" || s.Level != 12 || s.Start != 1793000000 {
		t.Fatalf("session = %+v", s)
	}
	var found bool
	for _, e := range s.Events {
		if e.K == "combat" {
			found = true
			if e.Fields["minHp"] != int64(12) {
				t.Fatalf("minHp = %#v", e.Fields["minHp"])
			}
			if _, has := e.Fields["t"]; has {
				t.Fatal("t must not be in Fields")
			}
		}
	}
	if !found {
		t.Fatal("no combat event")
	}
	if db.Sessions[1].Stop != nil {
		t.Fatal("second session must have nil Stop")
	}
}

func TestSessionJSON(t *testing.T) {
	db := loadFixture(t)
	b, err := json.Marshal(db.Sessions[0])
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	ev := back["events"].([]any)[0].(map[string]any)
	if ev["t"] != float64(1793000000) || ev["k"] != "zone" || ev["z"] != "Westfall" {
		t.Fatalf("event = %v", ev)
	}
	if back["stop"] != float64(1793001395) || back["id"] != "Realm-Gino-1793000000" {
		t.Fatalf("session = %v", back)
	}
	b2, err := json.Marshal(db.Sessions[1])
	if err != nil {
		t.Fatal(err)
	}
	var back2 map[string]any
	if err := json.Unmarshal(b2, &back2); err != nil {
		t.Fatal(err)
	}
	if _, has := back2["stop"]; has {
		t.Fatal("stop must be omitted when absent")
	}
}

func TestDecodeMissingFieldsAndErrors(t *testing.T) {
	db, err := Decode(map[string]any{
		"v": int64(1),
		"sessions": []any{map[string]any{
			"id": "x", "level": 5.0,
			"events": []any{map[string]any{"t": int64(1), "k": "group", "members": []any{}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := db.Sessions[0]
	if s.Level != 5 || s.Stop != nil || s.Guild != "" {
		t.Fatalf("session = %+v", s)
	}
	if m, ok := s.Events[0].Fields["members"].([]any); !ok || len(m) != 0 {
		t.Fatalf("members = %#v", s.Events[0].Fields["members"])
	}
	if _, err := Decode(map[string]any{"v": int64(2)}); err == nil {
		t.Fatal("expected unsupported format error")
	}
	// no sessions at all is fine (empty table decodes as []any{}, absent as nil)
	if db, err := Decode(map[string]any{"v": int64(1), "sessions": []any{}}); err != nil || len(db.Sessions) != 0 {
		t.Fatalf("db=%+v err=%v", db, err)
	}
}

func TestDecodeSkipsMalformed(t *testing.T) {
	good := func(id string) map[string]any { return map[string]any{"id": id} }
	db, err := Decode(map[string]any{
		"v": int64(1),
		"sessions": []any{
			good("a"),
			"junk",
			[]any{},                           // empty table: a table, but no id -> dropped
			map[string]any{"events": []any{}}, // no id -> dropped
			map[string]any{"id": "b", "events": []any{"junk", map[string]any{"t": int64(4)}, []any{}, map[string]any{"t": int64(5), "k": "level"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(db.Sessions) != 2 || db.Sessions[0].ID != "a" || db.Sessions[1].ID != "b" {
		t.Fatalf("sessions = %+v", db.Sessions)
	}
	if len(db.Sessions[1].Events) != 1 {
		t.Fatalf("events = %+v", db.Sessions[1].Events)
	}
	b, err := json.Marshal(db.Sessions[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"events":[]`) {
		t.Fatalf("json = %s", b)
	}
}
