package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oLdpZ/diario-gilda/app/internal/update"
)

func parseKeys(t *testing.T, out string) (pub, priv string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			t.Fatalf("bad line %q", line)
		}
		switch k {
		case "PUBLIC_KEY":
			pub = v
		case "FIRMA_KEY":
			priv = v
		}
	}
	if pub == "" || priv == "" {
		t.Fatalf("keys missing in %q", out)
	}
	return pub, priv
}

func TestKeygenSignVerify(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"keygen"}, func(string) string { return "" }, &out, &errOut); code != 0 {
		t.Fatalf("keygen exit %d: %s", code, errOut.String())
	}
	pub, priv := parseKeys(t, out.String())

	file := filepath.Join(t.TempDir(), "DiarioGilda-windows-amd64.zip")
	data := []byte("zip finto")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	env := func(k string) string {
		if k == "FIRMA_KEY" {
			return priv
		}
		return ""
	}
	out.Reset()
	if code := run([]string{"sign", file}, env, &out, &errOut); code != 0 {
		t.Fatalf("sign exit %d: %s", code, errOut.String())
	}
	sig, err := os.ReadFile(file + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String()+errOut.String(), priv) {
		t.Fatal("private key printed by sign")
	}
	if err := update.Verify(data, sig, pub); err != nil {
		t.Fatalf("verify: %v", err)
	}
	data[0] ^= 1
	if err := update.Verify(data, sig, pub); err == nil {
		t.Fatal("tampered data verified")
	}
}

func TestSignErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	seed := base64.StdEncoding.EncodeToString(make([]byte, ed25519.SeedSize)) // chiave valida
	none := func(string) string { return "" }
	bad := func(string) string { return "non-base64!" }
	cases := []struct {
		name string
		args []string
		env  func(string) string
	}{
		{"no args", nil, none},
		{"unknown", []string{"boh"}, none},
		{"sign no file", []string{"sign"}, none},
		{"sign no key", []string{"sign", file}, none},
		{"sign bad key", []string{"sign", file}, bad},
		{"sign missing file", []string{"sign", file + ".nope"}, func(string) string { return seed }},
	}
	for _, c := range cases {
		var out, errOut bytes.Buffer
		if code := run(c.args, c.env, &out, &errOut); code == 0 {
			t.Errorf("%s: exit 0", c.name)
		}
	}
	if _, err := os.Stat(file + ".sig"); err == nil {
		t.Fatal(".sig written on error")
	}
}

func TestSignWithSeed(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	seed := make([]byte, ed25519.SeedSize)
	env := func(string) string { return base64.StdEncoding.EncodeToString(seed) }
	var out, errOut bytes.Buffer
	if code := run([]string{"sign", file}, env, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	sig, err := os.ReadFile(file + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	if err := update.Verify([]byte("x"), sig, base64.StdEncoding.EncodeToString(pub)); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"keygen"}, func(string) string { return "" }, &out, &errOut); code != 0 {
		t.Fatalf("keygen exit %d", code)
	}
	pub, priv := parseKeys(t, out.String())
	otherOut := bytes.Buffer{}
	run([]string{"keygen"}, func(string) string { return "" }, &otherOut, &errOut)
	otherPub, _ := parseKeys(t, otherOut.String())

	file := filepath.Join(t.TempDir(), "DiarioGilda-macos.zip")
	if err := os.WriteFile(file, []byte("zip finto"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"sign", file}, func(k string) string { return map[string]string{"FIRMA_KEY": priv}[k] }, &out, &errOut); code != 0 {
		t.Fatalf("sign exit %d: %s", code, errOut.String())
	}
	env := func(pub string) func(string) string {
		return func(k string) string { return map[string]string{"PUBLIC_KEY": pub}[k] }
	}
	if code := run([]string{"verify", file}, env(pub), &out, &errOut); code != 0 {
		t.Fatalf("verify good: exit %d: %s", code, errOut.String())
	}
	cases := map[string]struct {
		file string
		pub  string
	}{
		"other key": {file, otherPub},
		"no key":    {file, ""},
	}
	for name, c := range cases {
		errOut.Reset()
		if code := run([]string{"verify", c.file}, env(c.pub), &out, &errOut); code == 0 {
			t.Errorf("%s: exit 0", name)
		}
	}
	sig, err := os.ReadFile(file + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file + ".sig"); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"verify", file}, env(pub), &out, &errOut); code == 0 {
		t.Error("missing sig: exit 0")
	}
	if err := os.WriteFile(file+".sig", sig, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("zip cambiato"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"verify", file}, env(pub), &out, &errOut); code == 0 {
		t.Error("tampered: exit 0")
	}
}

// verifySig e' una copia di update.Verify (il job di firma non deve caricare le
// dipendenze dell'aggiornamento): devono dare sempre lo stesso esito.
func TestVerifySigMatchesUpdate(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	other, _, _ := ed25519.GenerateKey(nil)
	data := []byte("zip finto")
	good := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data))
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	cases := []struct {
		name      string
		data      []byte
		sig, pubK string
		ok        bool
	}{
		{"buona", data, good, pubB64, true},
		{"buona con a capo e spazi", data, "  " + good + "\r\n", " " + pubB64 + "\n", true},
		{"dati cambiati", []byte("zip finta"), good, pubB64, false},
		{"altra chiave", data, good, base64.StdEncoding.EncodeToString(other), false},
		{"chiave vuota", data, good, "", false},
		{"chiave non base64", data, good, "!!", false},
		{"chiave corta", data, good, base64.StdEncoding.EncodeToString([]byte("corta")), false},
		{"firma non base64", data, "!!", pubB64, false},
		{"firma corta", data, base64.StdEncoding.EncodeToString([]byte("x")), pubB64, false},
	}
	for _, c := range cases {
		a := verifySig(c.data, []byte(c.sig), c.pubK)
		b := update.Verify(c.data, []byte(c.sig), c.pubK)
		if (a == nil) != c.ok || (b == nil) != c.ok {
			t.Errorf("%s: firma=%v update=%v, atteso ok=%v", c.name, a, b, c.ok)
		}
	}
}

// Il comando non deve importare app/internal/update (go-selfupdate e il resto
// non servono per firmare).
func TestNoUpdateImport(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range f.Imports {
		if strings.Contains(imp.Path.Value, "/internal/") {
			t.Errorf("main.go importa %s", imp.Path.Value)
		}
	}
}
