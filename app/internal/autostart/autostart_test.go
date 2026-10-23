package autostart

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// TestPlist controlla che il plist sia XML valido, con l'etichetta, RunAtLoad
// vero e il percorso passato (anche con caratteri da rendere sicuri).
func TestPlist(t *testing.T) {
	exe := "/Applications/Diario & Gilda.app/Contents/MacOS/DiarioGilda"
	b := Plist(exe)

	dec := xml.NewDecoder(strings.NewReader(string(b)))
	dec.Strict = true
	var texts, elems []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("XML non valido: %v\n%s", err, b)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			elems = append(elems, v.Name.Local)
			if v.Name.Local == "true" && len(texts) > 0 && texts[len(texts)-1] == "RunAtLoad" {
				texts = append(texts, "<true>")
			}
		case xml.CharData:
			if s := strings.TrimSpace(string(v)); s != "" {
				texts = append(texts, s)
			}
		}
	}

	if Label != "it.oldpz.diariogilda" {
		t.Errorf("Label = %q", Label)
	}
	for _, kv := range [][2]string{{"Label", Label}, {"ProgramArguments", exe}, {"RunAtLoad", "<true>"}} {
		if !follows(texts, kv[0], kv[1]) {
			t.Errorf("chiave %q non seguita da %q in %q", kv[0], kv[1], texts)
		}
	}
	if elems[0] != "plist" {
		t.Errorf("radice = %q", elems[0])
	}
	if !strings.Contains(string(b), "Diario &amp; Gilda") {
		t.Errorf("percorso senza escape: %s", b)
	}
}

func follows(list []string, k, v string) bool {
	for i := 0; i+1 < len(list); i++ {
		if list[i] == k && list[i+1] == v {
			return true
		}
	}
	return false
}

func TestRunValue(t *testing.T) {
	if got := RunValue(`C:\Users\a b\DiarioGilda.exe`); got != `"C:\Users\a b\DiarioGilda.exe"` {
		t.Errorf("RunValue = %s", got)
	}
}
