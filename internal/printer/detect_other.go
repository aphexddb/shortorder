//go:build !windows && !linux

package printer

import "fmt"

// errUnsupportedOS is returned by the printer transport on platforms with no
// implemented backend (currently macOS). The binary still builds and runs on
// these platforms (so goreleaser can produce the full matrix); only the print
// path is inert until a CUPS backend is added. Windows and Linux are supported.
var errUnsupportedOS = fmt.Errorf("printing is not implemented on this OS (supported: windows, linux)")

// detect returns no printers on non-Windows platforms.
func detect() ([]Info, error) {
	return nil, nil
}
