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

func TestWebhookDefault(t *testing.T) {
	old := build.WebhookURL
	build.WebhookURL = "https://default/hook"
	defer func() { build.WebhookURL = old }()
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c.Webhook != "https://default/hook" {
		t.Fatalf("webhook = %q", c.Webhook)
	}
}
