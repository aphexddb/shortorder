//go:build linux

package printer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// detect finds supported USB printers on Linux by walking sysfs
// (/sys/bus/usb/devices), matching each device's USB VID/PID (or product name)
// against the allowlist, and mapping it to its usblp character device
// (/dev/usb/lpN). The kernel's in-box usblp driver must be bound to the printer
// for the lpN node to exist.
func detect() ([]Info, error) {
	const usbRoot = "/sys/bus/usb/devices"
	entries, err := os.ReadDir(usbRoot)
	if err != nil {
		// No USB sysfs (non-Linux container, etc.) — nothing to report.
		return nil, nil
	}

	var out []Info
	for _, e := range entries {
		dir := filepath.Join(usbRoot, e.Name())

		vidS := readSysAttr(dir, "idVendor")
		pidS := readSysAttr(dir, "idProduct")
		if vidS == "" || pidS == "" {
			continue // an interface or hub, not a device with USB ids
		}
		vid, pid := hex4(vidS), hex4(pidS)
		product := readSysAttr(dir, "product")

		m := matchByUSB(vid, pid)
		if m == nil {
			m = matchByName(product, "")
		}
		if m == nil {
			continue
		}

		node := findLPNode(dir)
		if node == "" {
			continue // printer present but usblp isn't bound (no lpN node)
		}

		friendly := product
		if friendly == "" {
			friendly = m.Name
		}
		out = append(out, Info{
			Name:  friendly,
			Model: m.Name,
			USB:   fmt.Sprintf("VID_%04X PID_%04X", vid, pid),
			Path:  node,
		})
	}
	return out, nil
}

// findLPNode locates the /dev/usb/lpN node for a USB device by looking under its
// interface directories for the usblp-created class entry.
func findLPNode(deviceDir string) string {
	base := filepath.Base(deviceDir)
	ifaces, _ := filepath.Glob(filepath.Join(deviceDir, base+":*"))
	for _, iface := range ifaces {
		for _, cls := range []string{"usbmisc", "usb"} {
			lps, _ := filepath.Glob(filepath.Join(iface, cls, "lp*"))
			for _, lp := range lps {
				name := filepath.Base(lp) // e.g. "lp0"
				for _, cand := range []string{"/dev/usb/" + name, "/dev/" + name} {
					if _, err := os.Stat(cand); err == nil {
						return cand
					}
				}
			}
		}
	}
	return ""
}

// readSysAttr reads a single sysfs attribute file, trimmed, or "" on error.
func readSysAttr(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
