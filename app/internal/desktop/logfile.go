package desktop

import (
	"fmt"
	"os"
	"sync"
)

// LogMax e' la dimensione massima di diario.log prima della rotazione.
const LogMax = 1 << 20

// LogFile e' un file di log che, superato max byte, diventa <path>.1 (il
// backup precedente si perde) e riparte vuoto. Sicuro fra goroutine.
type LogFile struct {
	mu   sync.Mutex
	path string
	max  int64
	f    *os.File
	size int64
}

// OpenLog apre (in aggiunta) il log in path.
func OpenLog(path string, max int64) (*LogFile, error) {
	l := &LogFile{path: path, max: max}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *LogFile) open() error {
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("log: %w", err)
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("log: %w", err)
	}
	l.f, l.size = f, fi.Size()
	return nil
}

// Write scrive p, ruotando prima se il file supererebbe il limite. Una scrittura
// piu' grande del limite va comunque intera nel file nuovo.
func (l *LogFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		if err := l.open(); err != nil {
			return 0, err
		}
	}
	if l.size > 0 && l.size+int64(len(p)) > l.max {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	return n, err
}

// rotate sposta il log in .1 e ne apre uno nuovo; se lo spostamento non
// riesce si continua sullo stesso file. Se la riapertura fallisce l.f resta nil
// e la prossima Write riprova.
func (l *LogFile) rotate() error {
	l.f.Close()
	l.f = nil
	if err := os.Rename(l.path, l.path+".1"); err != nil {
		os.Remove(l.path + ".1")
		os.Rename(l.path, l.path+".1")
	}
	return l.open()
}

// Close chiude il file.
func (l *LogFile) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}
