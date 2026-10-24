package desktop

import "github.com/oLdpZ/diario-gilda/app/internal/config"

// Testi della scelta del personaggio principale.
const (
	MenuMain     = "Personaggio principale…"
	PromptMain   = "Qual è il tuo personaggio principale?\nNel diario verrà raccontato solo lui: le sessioni degli altri personaggi non vengono inviate."
	MsgNoChars   = "Non trovo ancora personaggi: entra in gioco una volta con il tuo personaggio principale e riprova."
	MsgMainLater = "Scegli il tuo personaggio principale dal menu dell'icona (" + MenuMain + "): finché non lo scegli il diario non invia le sessioni nuove."
	MsgMainSet   = "Personaggio principale: %s. Nel diario verrà raccontato solo lui."
)

// AskMain chiede il personaggio principale fra chars (nell'ordine di
// run.Characters), proponendo current se c'e', altrimenti il primo. Senza
// personaggi avvisa e restituisce ErrCanceled, come quando il giocatore annulla.
func AskMain(ui UI, chars []config.Character, current config.Character) (config.Character, error) {
	if len(chars) == 0 {
		ui.Info(MsgNoChars)
		return config.Character{}, ErrCanceled
	}
	labels := make([]string, len(chars))
	def := ""
	for i, c := range chars {
		labels[i] = c.Label()
		if current.Is(c.Char, c.Realm) {
			def = labels[i]
		}
	}
	if def == "" {
		def = labels[0]
	}
	got, err := ui.List(PromptMain, labels, def)
	if err != nil {
		return config.Character{}, err
	}
	for i, l := range labels {
		if l == got {
			return chars[i], nil
		}
	}
	return config.Character{}, ErrCanceled
}
