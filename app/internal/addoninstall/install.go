// Package addoninstall installa in Interface/AddOns l'addon incorporato nell'app.
// L'installazione e' a scambio: copia in DiarioGilda.new, sposta l'attuale in
// DiarioGilda.old, mette la nuova al suo posto e solo alla fine cancella .old;
// a qualsiasi errore rimette .old, cosi' resta sempre un DiarioGilda funzionante.
package addoninstall

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/oLdpZ/diario-gilda/addon"
)

const (
	dirName = "DiarioGilda"
	tocName = "DiarioGilda.toc"
)

// Embedded e' l'addon incorporato; la radice contiene la cartella DiarioGilda/.
var Embedded fs.FS = addon.FS

// Version restituisce la riga "## Version:" del TOC in fsys (radice DiarioGilda/), o "".
func Version(fsys fs.FS) string {
	b, err := fs.ReadFile(fsys, path.Join(dirName, tocName))
	if err != nil {
		return ""
	}
	return tocVersion(b)
}

// Installed restituisce la versione installata in addonsDir/DiarioGilda, o "".
func Installed(addonsDir string) string {
	b, err := os.ReadFile(filepath.Join(addonsDir, dirName, tocName))
	if err != nil {
		return ""
	}
	return tocVersion(b)
}

// NeedsInstall e' vero se l'addon manca o ha una versione diversa da quella in fsys.
// Falso se fsys non ha una versione (non c'e' niente di valido da installare).
func NeedsInstall(fsys fs.FS, addonsDir string) bool {
	want := Version(fsys)
	return want != "" && Installed(addonsDir) != want
}

// Install copia l'addon di fsys in addonsDir/DiarioGilda sostituendo quello presente.
func Install(fsys fs.FS, addonsDir string) error {
	return install(fsys, addonsDir, os.Rename)
}

func install(fsys fs.FS, addonsDir string, rename func(string, string) error) error {
	if Version(fsys) == "" {
		return errors.New("addon incorporato senza versione nel TOC")
	}
	cur := filepath.Join(addonsDir, dirName)
	newDir := cur + ".new"
	oldDir := cur + ".old"

	// Un'installazione interrotta fra i due rename lascia solo .old: rimettila.
	if !exists(cur) && exists(oldDir) {
		if err := rename(oldDir, cur); err != nil {
			return fmt.Errorf("ripristino di %s: %w", oldDir, err)
		}
	}
	if err := os.RemoveAll(newDir); err != nil {
		return fmt.Errorf("pulizia di %s: %w", newDir, err)
	}
	if err := copyTree(fsys, dirName, newDir); err != nil {
		os.RemoveAll(newDir)
		return fmt.Errorf("copia dell'addon: %w", err)
	}
	if err := os.RemoveAll(oldDir); err != nil {
		os.RemoveAll(newDir)
		return fmt.Errorf("pulizia di %s: %w", oldDir, err)
	}
	hadCur := exists(cur)
	if hadCur {
		if err := rename(cur, oldDir); err != nil {
			os.RemoveAll(newDir)
			return fmt.Errorf("spostamento dell'addon attuale: %w", err)
		}
	}
	if err := rename(newDir, cur); err != nil {
		if hadCur {
			if rerr := rename(oldDir, cur); rerr != nil {
				return fmt.Errorf("installazione dell'addon: %w (ripristino fallito: %v)", err, rerr)
			}
		}
		os.RemoveAll(newDir)
		return fmt.Errorf("installazione dell'addon: %w", err)
	}
	// Se .old non si cancella resta li': WoW non la carica (nome diverso dal TOC)
	// e la prossima installazione la toglie.
	os.RemoveAll(oldDir)
	return nil
}

func copyTree(fsys fs.FS, root, dst string) error {
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: non e' un file normale", p)
		}
		return copyFile(fsys, p, target)
	})
}

func copyFile(fsys fs.FS, src, dst string) error {
	in, err := fsys.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("%s: %w", src, err)
	}
	return out.Close()
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func tocVersion(b []byte) string {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text()) // toglie anche il \r delle righe CRLF
		rest, ok := strings.CutPrefix(line, "##")
		if !ok {
			continue
		}
		if v, ok := strings.CutPrefix(strings.TrimSpace(rest), "Version:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
