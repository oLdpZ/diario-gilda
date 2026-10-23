package desktop

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processNames elenca i nomi degli eseguibili in esecuzione (Toolhelp32,
// senza lanciare tasklist e senza finestre di console).
func processNames() ([]string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("elenco processi: %w", err)
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []string
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		out = append(out, windows.UTF16ToString(pe.ExeFile[:]))
	}
	return out, nil
}
