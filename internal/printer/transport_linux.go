//go:build linux

package printer

import (
	"fmt"
	"os"
	"time"
)

// The tested Epson-compatible clone (VID 04B8 / PID 0E20) has two firmware
// flaws on its USB input path; both corrupt jobs silently (the device ACKs
// every byte and the kernel reports clean writes):
//
//  1. Bytes arriving faster than the engine drains them are dropped once the
//     internal buffer fills. A raster command then comes up short on data,
//     the printer sits waiting for image bytes, and following jobs print as
//     glyph garbage. Pacing writes (~25KB/s, still faster than the physical
//     print rate) avoids it.
//  2. The tail of a job — up to a few KB — is intermittently discarded
//     instead of being flushed to the engine, truncating rasters and eating
//     trailing text/cut commands. Appending a NUL flush pad pushes the real
//     payload through; the printer ignores the NULs themselves. Verified
//     A/B: identical small-raster jobs lose their tail without the pad and
//     print intact with it.
const (
	usbWriteChunk = 256
	usbChunkDelay = 10 * time.Millisecond
	usbFlushPad   = 4096
	usbCloseDelay = 50 * time.Millisecond
)

// sendRaw writes the ESC/POS stream to the printer's usblp character device
// (e.g. /dev/usb/lp0), paced in small chunks and followed by a NUL flush pad —
// see the constants above. The process must have write access to that node —
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

	padded := make([]byte, len(data)+usbFlushPad)
	copy(padded, data)

	for off := 0; off < len(padded); off += usbWriteChunk {
		end := off + usbWriteChunk
		if end > len(padded) {
			end = len(padded)
		}
		if _, err := f.Write(padded[off:end]); err != nil {
			return fmt.Errorf("write to %s at %d/%d bytes: %w", target.Path, off, len(data), err)
		}
		if end < len(padded) {
			time.Sleep(usbChunkDelay)
		}
	}

	// Let the final URB complete before close; releasing the device can
	// cancel writes still in flight.
	time.Sleep(usbCloseDelay)
	return nil
}
