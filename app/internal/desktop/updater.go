package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"time"

	"github.com/oLdpZ/diario-gilda/app/internal/update"
)

// UpdateEvery e' l'intervallo fra due controlli degli aggiornamenti.
const UpdateEvery = 6 * time.Hour

// Updater controlla e installa gli aggiornamenti. Check e Install sono di
// solito update.Check e update.Install (che verifica la firma prima di
// applicare); Restart avvia il nuovo eseguibile e chiude questo; Failed mostra
// un errore al giocatore (una volta per ogni messaggio diverso). Failed e
// Restart possono essere nil.
type Updater struct {
	Check   func(ctx context.Context) (*update.Release, error)
	Install func(ctx context.Context, rel *update.Release) error
	Restart func() error
	Failed  func(msg string)

	lastFail string
}

func (u *Updater) fail(msg string) {
	log.Printf("aggiornamento: %s", msg)
	if msg == u.lastFail || u.Failed == nil {
		return
	}
	u.lastFail = msg
	u.Failed(msg)
}

// Once fa un controllo. Restituisce true se un aggiornamento e' stato
// installato (anche se il riavvio non e' riuscito): non serve controllare ancora.
// Un errore di rete va solo nel log (il PC puo' essere offline).
func (u *Updater) Once(ctx context.Context) bool {
	rel, err := u.Check(ctx)
	if err != nil {
		log.Printf("controllo aggiornamenti: %v", err)
		return false
	}
	if rel == nil {
		return false
	}
	log.Printf("aggiornamento disponibile: %s", rel.Version)
	if err := u.Install(ctx, rel); err != nil {
		if IsNetworkError(err) {
			log.Printf("aggiornamento non scaricato (rete): %v", err)
			return false
		}
		u.fail(err.Error())
		return false
	}
	log.Printf("aggiornamento %s installato, riavvio", rel.Version)
	if u.Restart != nil {
		if err := u.Restart(); err != nil {
			u.fail(fmt.Sprintf("riavvio non riuscito (%v): chiudi e riapri il Diario di Gilda", err))
		}
	}
	return true
}

// Loop controlla subito e poi ogni every, finche' ctx non e' cancellato o un
// aggiornamento non e' stato installato.
func (u *Updater) Loop(ctx context.Context, every time.Duration) {
	if u.Once(ctx) {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if u.Once(ctx) {
				return
			}
		}
	}
}

// IsNetworkError riconosce gli errori di rete e di tempo scaduto (download
// interrotto, PC offline): non vale la pena mostrarli al giocatore. Gli errori
// di verifica e di applicazione invece si mostrano.
func IsNetworkError(err error) bool {
	var ue *url.Error
	var ne net.Error
	return errors.As(err, &ue) || errors.As(err, &ne) ||
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}
