//go:build linux

package printer

import (
	"fmt"
	"os"
)

// sendRaw writes the ESC/POS stream straight to the printer's usblp character
// device (e.g. /dev/usb/lp0). The process must have write access to that node —
// the bundled systemd unit runs as root, which does.
func sendRaw(target Info, data []byte) error {
	if target.Path == "" {
		return fmt.Errorf("printer %q has no device path", target.Name)
	}
	if len(data) == 0 {
		return fmt.Errorf("nothing to print")
	}

	f, err := os.OpenFile(target.Path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", target.Path, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write to %s: %w", target.Path, err)
	}
	return nil
}
