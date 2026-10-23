//go:build !windows

package desktop

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lockFile usa flock esclusivo senza attesa; il lock e' legato al file aperto,
// quindi anche un secondo Acquire nello stesso processo fallisce.
func lockFile(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrLocked
	}
	return fmt.Errorf("lock: %w", err)
}

func unlockFile(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
