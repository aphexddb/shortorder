//go:build windows

package printer

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// sendRaw opens the printer's USB device interface and writes the ESC/POS bytes
// directly to it (CreateFile + WriteFile). No spooler queue is involved.
func sendRaw(target Info, data []byte) error {
	if target.Path == "" {
		return fmt.Errorf("printer %q has no device path", target.Name)
	}
	if len(data) == 0 {
		return fmt.Errorf("nothing to print")
	}

	pathPtr, err := windows.UTF16PtrFromString(target.Path)
	if err != nil {
		return fmt.Errorf("device path: %w", err)
	}

	h, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return fmt.Errorf("open %s: %w", target.Name, err)
	}
	defer windows.CloseHandle(h)

	// Write in full; WriteFile may write fewer bytes per call on USB endpoints.
	for off := 0; off < len(data); {
		var written uint32
		chunk := data[off:]
		if err := windows.WriteFile(h, chunk, &written, nil); err != nil {
			return fmt.Errorf("write to %s: %w", target.Name, err)
		}
		if written == 0 {
			return fmt.Errorf("write to %s stalled at %d/%d bytes", target.Name, off, len(data))
		}
		off += int(written)
	}
	return nil
}
