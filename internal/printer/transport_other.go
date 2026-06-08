//go:build !windows && !linux

package printer

// sendRaw is not implemented off Windows yet.
func sendRaw(target Info, data []byte) error {
	return errUnsupportedOS
}
