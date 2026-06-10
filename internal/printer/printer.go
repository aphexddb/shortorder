// Package printer detects supported USB thermal receipt printers and sends raw
// ESC/POS byte streams to them.
//
// Only an explicit allowlist of models is supported. The target hardware is any
// Epson-compatible ESC/POS USB receipt printer — the common identity for most
// budget 80mm/58mm receipt printers. (It's tested with a Volcora v-WRP2-A1W,
// which enumerates under an Epson-compatible identity: USB VID 0x04B8 /
// PID 0x0E20.) The allowlist matches on both the Epson-compatible USB
// identifiers a device reports and a set of known name substrings.
package printer

import (
	"strings"
	"sync"
)

// Info describes a detected, supported printer.
type Info struct {
	Name  string `json:"name"`  // friendly device name, e.g. "EPSON TM-T20II"
	Model string `json:"model"` // the supported model it matched
	USB   string `json:"usb"`   // USB identity, e.g. "VID_04B8 PID_0E20"
	Path  string `json:"path"`  // OS device path used to open the device
}

// Model is an entry in the supported-printer allowlist.
type Model struct {
	// Name is the human-facing model name.
	Name string
	// NameMatch holds lowercase substrings; a detected printer matches if its
	// queue name or driver name contains any of them.
	NameMatch []string
	// USBVendor / USBProduct are the USB IDs the device reports. Zero means
	// "don't match on USB id".
	USBVendor  uint16
	USBProduct uint16
}

// supportedModels is the allowlist. Add a Model here to support more hardware.
var supportedModels = []Model{
	{
		Name:       "Epson-compatible ESC/POS",
		NameMatch:  []string{"volcora", "wrp2", "wrp-2", "v-wrp", "tm-t20", "tm-m30", "tm-t88"},
		USBVendor:  0x04B8, // Seiko Epson Corp. (Epson-compatible ESC/POS)
		USBProduct: 0x0E20,
	},
}

// matchByName returns the supported model whose NameMatch substrings appear in
// the given queue/driver name, or nil if none match.
func matchByName(name, driver string) *Model {
	n := strings.ToLower(name + " " + driver)
	for i := range supportedModels {
		for _, sub := range supportedModels[i].NameMatch {
			if sub != "" && strings.Contains(n, sub) {
				return &supportedModels[i]
			}
		}
	}
	return nil
}

// matchByUSB returns the supported model with the given USB ids, or nil.
func matchByUSB(vendor, product uint16) *Model {
	for i := range supportedModels {
		m := &supportedModels[i]
		if m.USBVendor != 0 && m.USBVendor == vendor && m.USBProduct == product {
			return m
		}
	}
	return nil
}

// hex4 parses a 4-hex-digit string (e.g. "04b8") into a uint16. Non-hex
// characters contribute zero. Used to read USB VID/PID values from OS-specific
// device identifiers.
func hex4(s string) uint16 {
	var v uint16
	for _, c := range s {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= uint16(c - '0')
		case c >= 'a' && c <= 'f':
			v |= uint16(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= uint16(c-'A') + 10
		}
	}
	return v
}

// SupportedModels returns the names of all allowlisted models.
func SupportedModels() []string {
	names := make([]string, len(supportedModels))
	for i, m := range supportedModels {
		names[i] = m.Name
	}
	return names
}

// Detect returns the supported printers currently connected to this machine.
func Detect() ([]Info, error) {
	return detect()
}

// printMu serializes jobs: concurrent writes to the same device interleave
// their bytes mid-command and desync the printer.
var printMu sync.Mutex

// Print sends raw data (an ESC/POS stream) to the given detected printer.
// Jobs are serialized; a call blocks while another print is in flight.
func Print(target Info, data []byte) error {
	printMu.Lock()
	defer printMu.Unlock()
	return sendRaw(target, data)
}
