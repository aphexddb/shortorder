//go:build !latex
// +build !latex

package canvas

import "fmt"

// ParseLaTeX parses a LaTeX formula and returns a path.
func ParseLaTeX(formula string) (*Path, error) {
	return nil, fmt.Errorf("canvas: embedded LaTeX fonts are disabled in the shortorder fork; build with -tags=latex to use external LaTeX tools")
}
