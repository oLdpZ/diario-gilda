// Package config legge e scrive la configurazione utente (config.json).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/oLdpZ/diario-gilda/app/internal/build"
)

const fileName = "config.json"

type Config struct {
	Player    string `json:"player"`
	WowDir    string `json:"wowDir"`
	ClientDir string `json:"clientDir"`
	Webhook   string `json:"webhook,omitempty"` // vuoto = quello di build (vedi EffectiveWebhook)
}

// EffectiveWebhook restituisce il webhook da usare: quello della configurazione
// se impostato, altrimenti quello incorporato al build. Non va mai scritto nel log.
func EffectiveWebhook(c Config) string {
	if c.Webhook != "" {
		return c.Webhook
	}
	return build.WebhookURL
}

// Dir restituisce la cartella dati dell'app: UserConfigDir/DiarioGilda.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cartella di configurazione: %w", err)
	}
	return filepath.Join(base, "DiarioGilda"), nil
}

// Load legge config.json; file assente -> Config vuota.
// Il webhook di build non viene sostituito qui: si legge con EffectiveWebhook.
func Load(dir string) (Config, error) {
	var c Config
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Config{}, fmt.Errorf("lettura %s: %w", fileName, err)
	default:
		if err := json.Unmarshal(data, &c); err != nil {
			return Config{}, fmt.Errorf("%s non valido: %w", fileName, err)
		}
	}
	return c, nil
}

// Save scrive config.json in modo atomico (file temporaneo + rename).
func Save(dir string, c Config) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creazione %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("codifica %s: %w", fileName, err)
	}
	tmp, err := os.CreateTemp(dir, fileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("file temporaneo %s: %w", fileName, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("scrittura %s: %w", fileName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync %s: %w", fileName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("chiusura %s: %w", fileName, err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, fileName)); err != nil {
		return fmt.Errorf("rename %s: %w", fileName, err)
	}
	return nil
}
