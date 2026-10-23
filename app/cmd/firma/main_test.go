package main

import (
	"bytes"
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
	os.WriteFile(file, []byte("x"), 0o644)
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
		{"sign missing file", []string{"sign", file + ".nope"}, func(string) string { return strings.Repeat("A", 88) }},
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
