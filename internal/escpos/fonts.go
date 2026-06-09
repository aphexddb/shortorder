package escpos

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

// Bundled-font seeding for SVG rendering.
//
// tdewolff/canvas resolves a <text> element's font by scanning the host's OS
// font directories, and it PANICS when a family can't be found. On a headless
// appliance with no fonts installed that would crash every text render — and
// even where fonts exist, two hosts with different fonts would rasterize text
// differently. Neither is acceptable for a deterministic print service.
//
// So before rendering any SVG we replace canvas's system-font index with a
// fixed one of our own: the CSS generic families (serif, sans-serif, monospace,
// and the rest) plus the parser's default all resolve to fonts bundled in this
// binary (the BSD-licensed Go fonts). Host fonts are never consulted, so the
// same markup rasterizes identically everywhere. A specific named font that
// isn't one of ours won't match; SVGImage recovers that into a clean error
// instead of a crash (and a comma list ending in a generic, e.g.
// "Helvetica, Arial, sans-serif", still resolves to the bundled fallback).
//
// canvas loads fonts from files, so the bundled bytes are materialized once
// into a per-user cache directory and an index gob is handed to canvas.
var fontsOnce sync.Once

func ensureBundledFonts() {
	fontsOnce.Do(seedBundledFonts)
}

func seedBundledFonts() {
	dir, err := bundledFontDir()
	if err != nil {
		return // fall back to canvas's host-font lookup; SVGImage still recovers panics
	}
	sansPath := filepath.Join(dir, "shortorder-sans.ttf")
	monoPath := filepath.Join(dir, "shortorder-mono.ttf")
	if err := writeIfAbsent(sansPath, goregular.TTF); err != nil {
		return
	}
	if err := writeIfAbsent(monoPath, gomono.TTF); err != nil {
		return
	}

	// The SVG parser only ever requests the Regular style and keys by family
	// name, so one Regular face per family is enough (bold/italic are synthesized
	// by the renderer).
	reg := font.ParseStyleCSS(400, false)
	sf := &font.SystemFonts{
		Generics: map[string][]string{},
		Fonts: map[string]map[font.Style]font.FontMetadata{
			"shortorder-sans": {reg: {Filename: sansPath, Family: "shortorder-sans", Style: reg}},
			"shortorder-mono": {reg: {Filename: monoPath, Family: "shortorder-mono", Style: reg}},
		},
	}
	for _, g := range []string{"serif", "sans-serif", "system-ui", "ui-serif", "ui-sans-serif", "ui-rounded", "cursive", "fantasy", "emoji", "math", "fangsong"} {
		sf.Generics[g] = []string{"shortorder-sans"}
	}
	for _, g := range []string{"monospace", "ui-monospace"} {
		sf.Generics[g] = []string{"shortorder-mono"}
	}

	gobPath := filepath.Join(dir, "shortorder-fonts.gob")
	if err := sf.Save(gobPath); err != nil {
		return
	}
	// CacheSystemFonts loads the gob (it exists) and installs it as canvas's
	// system-font index, replacing any host scan.
	_ = canvas.CacheSystemFonts(gobPath, nil)
}

// bundledFontDir returns a writable directory for the materialized font files,
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
