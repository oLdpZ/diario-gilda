package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oLdpZ/diario-gilda/app/internal/build"
)

func TestLoadMissingFile(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c != (Config{}) {
		t.Fatalf("atteso zero value, ottenuto %+v", c)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub")
	want := Config{Player: "Gigi", WowDir: `D:\WoW`, ClientDir: `D:\WoW\_retail_`, Webhook: "https://example/hook"}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("file temporanei rimasti: %v", entries)
	}
}

func TestLoadCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "config.json") {
		t.Fatalf("atteso errore con config.json, ottenuto %v", err)
	}
}

func TestLoadDoesNotSubstituteWebhook(t *testing.T) {
	old := build.WebhookURL
	build.WebhookURL = "https://default/hook"
	defer func() { build.WebhookURL = old }()
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Webhook != "" {
		t.Fatalf("Load non deve sostituire il webhook di build: %q", c.Webhook)
	}
	c.Player = "Gigi"
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "default/hook") || strings.Contains(string(data), "webhook") {
		t.Fatalf("webhook di build salvato in config.json: %s", data)
	}
}

func TestEffectiveWebhook(t *testing.T) {
	old := build.WebhookURL
	build.WebhookURL = "https://default/hook"
	defer func() { build.WebhookURL = old }()
	if got := EffectiveWebhook(Config{}); got != "https://default/hook" {
		t.Errorf("vuoto: %q", got)
	}
	if got := EffectiveWebhook(Config{Webhook: "https://mio/hook"}); got != "https://mio/hook" {
		t.Errorf("impostato: %q", got)
	}
}

func TestJSONFieldNames(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, Config{Player: "Gigi", WowDir: "W", ClientDir: "C"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{`"player"`, `"wowDir"`, `"clientDir"`} {
		if !strings.Contains(string(data), k) {
			t.Errorf("manca %s in %s", k, data)
		}
	}
}

// Un config.json salvato prima dei tag JSON (chiavi "Player", ...) si legge ancora.
func TestLoadOldFieldNames(t *testing.T) {
	dir := t.TempDir()
	old := `{"Player":"Gigi","WowDir":"D:\\WoW","ClientDir":"D:\\WoW\\_retail_","Webhook":"https://x/h"}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Player: "Gigi", WowDir: `D:\WoW`, ClientDir: `D:\WoW\_retail_`, Webhook: "https://x/h"}
	if c != want {
		t.Fatalf("got %+v want %+v", c, want)
	}
}
