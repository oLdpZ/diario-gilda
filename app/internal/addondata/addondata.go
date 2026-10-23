// Package addondata decodifica i dati di DiarioGildaDB (vedi docs/formato-dati.md)
// gia' letti da svparse in strutture Go.
package addondata

import (
	"encoding/json"
	"errors"
	"fmt"
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
	if globals["sessions"] == nil {
		return db, nil
	}
	list, ok := globals["sessions"].([]any)
	if !ok {
		return db, errors.New("sessions: atteso un elenco")
	}
	for i, raw := range list {
		m, ok := raw.(map[string]any)
		if !ok {
			return db, fmt.Errorf("sessions[%d]: attesa una tabella", i+1)
		}
		s, err := decodeSession(m)
		if err != nil {
			return db, fmt.Errorf("sessions[%d]: %w", i+1, err)
		}
		db.Sessions = append(db.Sessions, s)
	}
	return db, nil
}

func decodeSession(m map[string]any) (Session, error) {
	s := Session{
		ID: str(m["id"]), Char: str(m["char"]), Realm: str(m["realm"]), Guild: str(m["guild"]),
		Class: str(m["class"]), Race: str(m["race"]), Build: str(m["build"]), Addon: str(m["addon"]),
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
	if m["events"] != nil {
		evs, ok := m["events"].([]any)
		if !ok {
			return s, errors.New("events: atteso un elenco")
		}
		for i, raw := range evs {
			em, ok := raw.(map[string]any)
			if !ok {
				return s, fmt.Errorf("events[%d]: attesa una tabella", i+1)
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
	}
	return s, nil
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
