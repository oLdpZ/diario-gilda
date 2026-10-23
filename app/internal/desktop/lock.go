// Package desktop contiene la parte dell'app legata al desktop: un solo
// processo alla volta, primo avvio, icona e menu, log su file, controllo degli
// aggiornamenti. Le funzioni pure sono testate; le chiamate all'interfaccia
// grafica (tray.go, dialogs.go) sono colla sottile.
package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const lockName = "diario.lock"

// ErrLocked: un altro processo dell'app tiene gia' il lock.
var ErrLocked = errors.New("il Diario di Gilda e' gia' in esecuzione")

// Lock e' un lock del sistema operativo su un file in dir: si libera da solo
// quando il processo termina, anche dopo un crash (il file resta, ma non blocca).
type Lock struct {
	mu sync.Mutex
	f  *os.File
}

// Acquire prende il lock in dir (creata se manca); ErrLocked se e' gia' preso.
func Acquire(dir string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, lockName), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Release libera il lock; chiamarla piu' volte non e' un errore.
func (l *Lock) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	unlockFile(l.f)
	err := l.f.Close()
	l.f = nil
	return err
}
