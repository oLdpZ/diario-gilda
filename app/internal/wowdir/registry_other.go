//go:build !windows

package wowdir

// RegistryRoots: il registro c'e' solo su Windows.
func RegistryRoots() []string { return nil }
