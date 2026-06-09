package escpos

import (
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/font"
	"golang.org/x/image/font/gofont/gomono"
)

// Bundled fonts for SVG rendering.
//
// tdewolff/canvas resolves a <text> element's font by scanning the host's OS
// font directories, and it PANICS when a family can't be found. On a headless
// appliance with no fonts installed that would crash every text render, and
// where fonts do exist two hosts would rasterize text differently. Neither is
// acceptable for a deterministic print service.
//
// So we ship our own fonts inside the binary and install them as canvas's
// system-font index, replacing the host scan entirely. Two real typefaces are
// embedded as the fallbacks the operator asked for — Roboto (sans-serif) and
// Gelasio (a serif; the open, metrically Georgia-compatible face, since Georgia
// itself is proprietary and not redistributable) — plus Go Mono for monospace.
// The CSS generic families and the common named fonts an SVG is likely to ask
// for all map onto these. A name we don't recognize is logged and aliased to
// the sans fallback on the fly (see SVGImage), so text always renders and every
// miss is visible in the logs.
//
// Both fonts are licensed under the SIL Open Font License (see the .txt files in
// the fonts directory). canvas loads fonts from files, so the embedded bytes are
// materialized once into a per-user cache directory.

//go:embed fonts/Roboto.ttf
var robotoTTF []byte

//go:embed fonts/Gelasio.ttf
var gelasioTTF []byte

// Internal family names for the three bundled faces.
const (
	familySans  = "shortorder-sans"  // Roboto
	familySerif = "shortorder-serif" // Gelasio
	familyMono  = "shortorder-mono"  // Go Mono
)

var (
	fontsOnce sync.Once
	fontMu    sync.Mutex        // guards fontIndex mutation, the canvas index, and SVG rendering
	fontIndex *font.SystemFonts // our authoritative index; aliases are added here on a miss
	fontDir   string            // where the materialized .ttf and index live
)

// ensureBundledFonts installs the bundled fonts into canvas exactly once.
func ensureBundledFonts() {
	fontsOnce.Do(seedBundledFonts)
}

func seedBundledFonts() {
	dir, err := bundledFontDir()
	if err != nil {
		slog.Error("svg fonts: cannot create cache dir; falling back to host fonts (text may fail to render)", "err", err)
		return
	}
	fontDir = dir

	// The SVG parser only ever requests the Regular style and keys by family
	// name, so one Regular face per family is enough; the renderer synthesizes
	// any bold/italic.
	reg := font.ParseStyleCSS(400, false)
	sf := &font.SystemFonts{
		Generics: map[string][]string{},
		Fonts:    map[string]map[font.Style]font.FontMetadata{},
	}
	loaded := map[string]bool{}
	add := func(family string, ttf []byte) {
		path := filepath.Join(dir, family+".ttf")
		if err := writeIfAbsent(path, ttf); err != nil {
			slog.Error("svg fonts: cannot materialize bundled font", "family", family, "err", err)
			return
		}
		sf.Fonts[family] = map[font.Style]font.FontMetadata{reg: {Filename: path, Family: family, Style: reg}}
		loaded[family] = true
	}
	add(familySans, robotoTTF)
	add(familySerif, gelasioTTF)
	add(familyMono, gomono.TTF)

	// Map CSS generic families to the bundled faces.
	for _, g := range []string{"serif", "ui-serif"} {
		sf.Generics[g] = []string{familySerif}
	}
	for _, g := range []string{"sans-serif", "system-ui", "ui-sans-serif", "ui-rounded", "cursive", "fantasy", "emoji", "math", "fangsong"} {
		sf.Generics[g] = []string{familySans}
	}
	for _, g := range []string{"monospace", "ui-monospace"} {
		sf.Generics[g] = []string{familyMono}
	}
	// Map the common named fonts an SVG is likely to request onto the nearest
	// bundled fallback, so they resolve silently instead of logging a miss.
	for _, n := range []string{"Georgia", "Times New Roman", "Times", "Garamond", "Cambria", "Book Antiqua", "Palatino", "Palatino Linotype", "Noto Serif", "PT Serif", "Source Serif Pro", "Merriweather", "Gelasio"} {
		sf.Generics[n] = []string{familySerif}
	}
	for _, n := range []string{"Roboto", "Arial", "Helvetica", "Helvetica Neue", "Verdana", "Tahoma", "Segoe UI", "Calibri", "Open Sans", "Lato", "Noto Sans", "Inter"} {
		sf.Generics[n] = []string{familySans}
	}
	for _, n := range []string{"Courier New", "Courier", "Consolas", "Monaco", "Menlo", "Roboto Mono", "Source Code Pro", "Go Mono"} {
		sf.Generics[n] = []string{familyMono}
	}

	fontIndex = sf
	if installFontIndex() {
		slog.Info("svg fonts: bundled fonts ready",
			"sans", loaded[familySans], "serif", loaded[familySerif], "mono", loaded[familyMono], "dir", dir)
	}
}

// aliasFont points an unrecognized family at the sans fallback and reinstalls
// the index, so a later render resolves it instead of panicking. It returns
// false when nothing changed (already aliased, or the sans fallback is missing),
// which the caller uses to stop retrying. Callers must hold fontMu.
func aliasFont(name string) bool {
	if fontIndex == nil || name == "" {
		return false
	}
	if _, ok := fontIndex.Generics[name]; ok {
		return false // already aliased — avoid an infinite retry loop
	}
	if _, ok := fontIndex.Fonts[name]; ok {
		return false
	}
	if _, ok := fontIndex.Fonts[familySans]; !ok {
		return false // no fallback face to point at
	}
	fontIndex.Generics[name] = []string{familySans}
	return installFontIndex()
}

// installFontIndex persists the in-memory index and installs it as canvas's
// system-font index (CacheSystemFonts loads the gob and replaces the host scan).
func installFontIndex() bool {
	if fontIndex == nil || fontDir == "" {
		return false
	}
	gobPath := filepath.Join(fontDir, "index.gob")
	if err := fontIndex.Save(gobPath); err != nil {
		slog.Error("svg fonts: cannot save font index", "err", err)
		return false
	}
	if err := canvas.CacheSystemFonts(gobPath, nil); err != nil {
		slog.Error("svg fonts: cannot install font index", "err", err)
		return false
	}
	return true
}

// bundledFontDir returns a writable directory for the materialized fonts,
// preferring the user cache dir and falling back to the temp dir.
func bundledFontDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "shortorder", "fonts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// writeIfAbsent writes data to path unless a file of the same size is already
// there (the bundled bytes are fixed, so size equality means it is current).
func writeIfAbsent(path string, data []byte) error {
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(data)) {
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}
