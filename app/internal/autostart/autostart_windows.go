package autostart

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// Enable scrive (o aggiorna) il valore DiarioGilda nella chiave Run dell'utente.
func Enable(exePath string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("avvio automatico: apertura chiave Run: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue(valueName, RunValue(exePath)); err != nil {
		return fmt.Errorf("avvio automatico: scrittura valore: %w", err)
	}
	return nil
}

// Enabled dice se il valore DiarioGilda e' presente nella chiave Run.
func Enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(valueName)
	return err == nil
}
