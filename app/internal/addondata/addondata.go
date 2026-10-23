// Package addondata decodifica i dati di DiarioGildaDB (vedi docs/formato-dati.md)
// gia' letti da svparse in strutture Go.
package addondata

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
)

// DB e' il contenuto di DiarioGildaDB che interessa al programmino.
type DB struct {
	V        int
	Sessions []Session
}

// Session e' una sessione di gioco. Ogni campo puo' mancare (vedi "Campi assenti"):
// i campi mancanti restano a zero, Stop resta nil.
type Session struct {
	ID      string  `json:"id"`
	Char    string  `json:"char"`
	Realm   string  `json:"realm"`
	Guild   string  `json:"guild,omitempty"`
	Class   string  `json:"class"`
	Race    string  `json:"race"`
	Level   int     `json:"level"`
	Build   string  `json:"build"`
	Addon   string  `json:"addon"`
	Start   int64   `json:"start"`
	Stop    *int64  `json:"stop,omitempty"`
	Dropped int     `json:"dropped"`
	Events  []Event `json:"events"`
}

// Event ha t e k comuni; Fields contiene tutti gli altri campi decodificati
// (int64, float64, string, bool, []any, map[string]any).
type Event struct {
	T      int64
	K      string
	Fields map[string]any
}

// MarshalJSON appiattisce l'evento: {"t":..., "k":..., <campi>...}.
func (e Event) MarshalJSON() ([]byte, error) {
	m := make(map[string]any, len(e.Fields)+2)
	for k, v := range e.Fields {
		m[k] = v
	}
	m["t"] = e.T
	m["k"] = e.K
	return json.Marshal(m)
}

// Decode converte i globali di svparse.Parse (o la sola tabella DiarioGildaDB) in DB.
func Decode(globals map[string]any) (DB, error) {
	// accetta sia i globali di svparse.Parse sia la tabella DiarioGildaDB stessa
	if inner, ok := globals["DiarioGildaDB"].(map[string]any); ok {
		globals = inner
	}
	var db DB
	v, ok := num(globals["v"])
	if !ok || v != 1 {
		return db, errors.New("formato non supportato")
	}
	db.V = int(v)
	for _, raw := range elems(globals["sessions"]) {
		m, ok := asTable(raw)
		if !ok {
			continue // sessione malformata: saltata, le altre restano
		}
		db.Sessions = append(db.Sessions, decodeSession(m))
	}
	return db, nil
}

// asTable accetta una mappa o una tabella vuota (che svparse decodifica come []any vuoto).
func asTable(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	case []any:
		if len(t) == 0 {
			return map[string]any{}, true
		}
	}
	return nil, false
}

// elems restituisce gli elementi di un elenco; per una tabella con buchi
// (decodificata come mappa) usa le chiavi intere in ordine.
func elems(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case map[string]any:
		var keys []int
		for k := range t {
			if n, err := strconv.Atoi(k); err == nil {
				keys = append(keys, n)
			}
		}
		sort.Ints(keys)
		out := make([]any, 0, len(keys))
		for _, n := range keys {
			out = append(out, t[strconv.Itoa(n)])
		}
		return out
	}
	return nil
}

func decodeSession(m map[string]any) Session {
	s := Session{
		ID: str(m["id"]), Char: str(m["char"]), Realm: str(m["realm"]), Guild: str(m["guild"]),
		Class: str(m["class"]), Race: str(m["race"]), Build: str(m["build"]), Addon: str(m["addon"]),
		Events: []Event{},
	}
	if n, ok := num(m["level"]); ok {
		s.Level = int(n)
	}
	if n, ok := num(m["start"]); ok {
		s.Start = n
	}
	if n, ok := num(m["stop"]); ok {
		s.Stop = &n
	}
	if n, ok := num(m["dropped"]); ok {
		s.Dropped = int(n)
	}
	for _, raw := range elems(m["events"]) {
		em, ok := asTable(raw)
		if !ok {
			continue
		}
		e := Event{K: str(em["k"]), Fields: make(map[string]any, len(em))}
		if n, ok := num(em["t"]); ok {
			e.T = n
		}
		for k, v := range em {
			if k != "t" && k != "k" {
				e.Fields[k] = v
			}
		}
		s.Events = append(s.Events, e)
	}
	return s
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// num legge un numero tollerando int64 e float64.
func num(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	}
	return 0, false
}
