// Package payload costruisce il file (JSON compresso gzip) inviato per ogni sessione.
package payload

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/oLdpZ/diario-gilda/app/internal/addondata"
	"github.com/oLdpZ/diario-gilda/app/internal/episodes"
)

// MaxUpload e' la dimensione massima del file compresso (limite Discord senza boost: 10 MB).
const MaxUpload = 8 << 20

// session e' addondata.Session con gli eventi gia' serializzati (una sola volta,
// cosi' i tentativi di troncamento non rifanno il lavoro costoso).
type session struct {
	addondata.Session
	Events []json.RawMessage `json:"events"`
}

type document struct {
	Formato       int                `json:"formato"`
	Giocatore     string             `json:"giocatore"`
	App           string             `json:"app"`
	Interrotta    bool               `json:"interrotta"`
	Troncata      int                `json:"troncata,omitempty"`
	Sessione      session            `json:"sessione"`
	Combattimenti []episodes.Episode `json:"combattimenti"`
}

// Build produce nome e contenuto del file da inviare. Non modifica s ne' eps.
// Se il file supera MaxUpload toglie prima le liste dettagliate degli episodi,
// poi gli eventi piu' vecchi (campo "troncata" = quanti ne ha tolti).
func Build(player, appVersion string, s addondata.Session, eps []episodes.Episode, interrupted bool) (string, []byte, error) {
	name := "sessione_" + sanitize(player) + "_" + sanitize(s.ID) + ".json.gz"
	raw := make([]json.RawMessage, len(s.Events))
	for i, e := range s.Events {
		b, err := json.Marshal(e)
		if err != nil {
			return "", nil, fmt.Errorf("evento %d: %w", i, err)
		}
		raw[i] = b
	}
	doc := document{
		Formato: 1, Giocatore: player, App: appVersion, Interrotta: interrupted,
		Sessione: session{Session: s, Events: raw}, Combattimenti: append([]episodes.Episode{}, eps...),
	}
	data, err := encode(doc)
	if err != nil {
		return "", nil, err
	}
	if len(data) <= MaxUpload {
		return name, data, nil
	}

	// 1) niente liste dettagliate negli episodi (copie: il chiamante non cambia)
	for i := range doc.Combattimenti {
		doc.Combattimenti[i].Enemies = []episodes.NameCount{}
		doc.Combattimenti[i].Healers = []episodes.NameAmount{}
		doc.Combattimenti[i].Kills = []episodes.NameCount{}
	}
	if data, err = encode(doc); err != nil {
		return "", nil, err
	}
	if len(data) <= MaxUpload {
		return name, data, nil
	}

	// 2) elimina gli eventi piu' vecchi, stimando quanti tenerne dal rapporto di compressione
	total := len(raw)
	keep := total
	for keep > 0 {
		next := int(float64(keep) * float64(MaxUpload) / float64(len(data)) * 0.95)
		if next >= keep {
			next = keep - 1
		}
		keep = next
		doc.Sessione.Events = raw[total-keep:]
		doc.Troncata = total - keep
		if data, err = encode(doc); err != nil {
			return "", nil, err
		}
		if len(data) <= MaxUpload {
			return name, data, nil
		}
	}
	return "", nil, errors.New("sessione troppo grande anche senza eventi")
}

func encode(doc document) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(doc); err != nil {
		return nil, fmt.Errorf("codifica json: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("compressione: %w", err)
	}
	return buf.Bytes(), nil
}

func sanitize(s string) string {
	out := []rune(s)
	for i, r := range out {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			out[i] = '_'
		}
	}
	return string(out)
}
