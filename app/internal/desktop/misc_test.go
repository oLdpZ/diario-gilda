package desktop

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestIsWoWProcess(t *testing.T) {
	yes := []string{"Wow.exe", "WowClassic.exe", "wowclassicb.exe", "WowT.exe", "Wow-64.exe",
		"World of Warcraft", "World of Warcraft Classic"}
	no := []string{"DiarioGilda.exe", "Battle.net.exe", "explorer.exe", "WowVoiceProxy.exe",
		"WowError.exe", "Agent.exe", "Wow", ""}
	for _, n := range yes {
		if !IsWoWProcess(n) {
			t.Errorf("%q non riconosciuto", n)
		}
	}
	for _, n := range no {
		if IsWoWProcess(n) {
			t.Errorf("%q preso per WoW", n)
		}
	}
}

func TestIcon(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(IconPNG))
	if err != nil {
		t.Fatalf("icon.png: %v", err)
	}
	ico := ICO(IconPNG)
	if !bytes.Equal(ico[:6], []byte{0, 0, 1, 0, 1, 0}) {
		t.Fatalf("intestazione ICO = % x", ico[:6])
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if int(ico[6]) != w%256 || int(ico[7]) != h%256 {
		t.Errorf("dimensioni %dx%d, voce %d %d", w, h, ico[6], ico[7])
	}
	size := binary.LittleEndian.Uint32(ico[14:])
	off := binary.LittleEndian.Uint32(ico[18:])
	if off != 22 || int(size) != len(IconPNG) || !bytes.Equal(ico[off:], IconPNG) {
		t.Errorf("voce ICO non valida: size %d off %d", size, off)
	}
}
