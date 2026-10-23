package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("avvio automatico: cartella utente: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

// Enable scrive il LaunchAgent; vale dal prossimo accesso.
func Enable(exePath string) error {
	p, err := plistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("avvio automatico: %w", err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, Plist(exePath), 0o644); err != nil {
		return fmt.Errorf("avvio automatico: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("avvio automatico: %w", err)
	}
	return nil
}

// Enabled dice se il LaunchAgent esiste.
func Enabled() bool {
	p, err := plistPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}
