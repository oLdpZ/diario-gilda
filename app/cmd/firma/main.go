// Comando firma: genera la coppia di chiavi ed25519 e firma gli zip delle release.
//
//	firma keygen        stampa PUBLIC_KEY=<base64> e FIRMA_KEY=<base64>
//	firma sign <file>   firma <file> con la chiave privata in FIRMA_KEY e scrive <file>.sig
//	firma verify <file> controlla <file>.sig con la chiave pubblica in PUBLIC_KEY
//
// PUBLIC_KEY va in build.PublicKey (-ldflags -X); FIRMA_KEY e' un segreto del
// workflow di release e non va mai nel repository.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oLdpZ/diario-gilda/app/internal/update"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	var err error
	switch {
	case len(args) == 1 && args[0] == "keygen":
		err = keygen(stdout)
	case len(args) == 2 && args[0] == "sign":
		err = sign(args[1], getenv("FIRMA_KEY"))
	case len(args) == 2 && args[0] == "verify":
		err = verify(args[1], getenv("PUBLIC_KEY"))
	default:
		fmt.Fprintln(stderr, "uso: firma keygen | firma sign <file> (chiave in FIRMA_KEY) | firma verify <file> (chiave in PUBLIC_KEY)")
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "firma:", err)
		return 1
	}
	return 0
}

func keygen(w io.Writer) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generazione chiavi: %w", err)
	}
	_, err = fmt.Fprintf(w, "PUBLIC_KEY=%s\nFIRMA_KEY=%s\n",
		base64.StdEncoding.EncodeToString(pub), base64.StdEncoding.EncodeToString(priv))
	return err
}

func sign(file, keyB64 string) error {
	if keyB64 == "" {
		return errors.New("FIRMA_KEY non impostata")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keyB64))
	if err != nil {
		return errors.New("FIRMA_KEY non e' base64 valido")
	}
	var priv ed25519.PrivateKey
	switch len(raw) {
	case ed25519.PrivateKeySize:
		priv = ed25519.NewKeyFromSeed(raw[:ed25519.SeedSize])
		if !priv.Equal(ed25519.PrivateKey(raw)) {
			return errors.New("FIRMA_KEY non coerente (chiave pubblica interna diversa)")
		}
	case ed25519.SeedSize:
		priv = ed25519.NewKeyFromSeed(raw)
	default:
		return fmt.Errorf("FIRMA_KEY di %d byte: attesi %d o %d", len(raw), ed25519.PrivateKeySize, ed25519.SeedSize)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("lettura: %w", err)
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n"
	if err := os.WriteFile(file+".sig", []byte(sig), 0o644); err != nil {
		return fmt.Errorf("scrittura firma: %w", err)
	}
	return nil
}

func verify(file, pubB64 string) error {
	if pubB64 == "" {
		return errors.New("PUBLIC_KEY non impostata")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("lettura: %w", err)
	}
	sig, err := os.ReadFile(file + ".sig")
	if err != nil {
		return fmt.Errorf("lettura firma: %w", err)
	}
	return update.Verify(data, sig, pubB64)
}
