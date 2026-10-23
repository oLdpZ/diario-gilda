package desktop

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"image"
	_ "image/png"
)

// IconPNG e' l'icona dell'app (64x64).
//
//go:embed icon.png
var IconPNG []byte

// ICO incapsula un PNG in un file .ico a una sola immagine (formato accettato
// da Windows da Vista in poi): l'icona della barra di Windows vuole un .ico.
func ICO(pngData []byte) []byte {
	w, h := 0, 0
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(pngData)); err == nil {
		w, h = cfg.Width, cfg.Height
	}
	var b bytes.Buffer
	le := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	le([3]uint16{0, 1, 1})     // riservato, tipo icona, una immagine
	b.WriteByte(byte(w % 256)) // 0 = 256
	b.WriteByte(byte(h % 256))
	b.WriteByte(0)           // colori della palette
	b.WriteByte(0)           // riservato
	le([2]uint16{1, 32})     // piani, bit per pixel
	le(uint32(len(pngData))) // dimensione dei dati
	le(uint32(22))           // offset dei dati: 6 + 16
	b.Write(pngData)
	return b.Bytes()
}
