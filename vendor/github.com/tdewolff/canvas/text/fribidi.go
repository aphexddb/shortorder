package text

import (
	"golang.org/x/text/unicode/bidi"
)

// EmbeddingLevels returns the embedding levels for each rune of a mixed LTR/RTL string. A change in level means a change in direction.
func EmbeddingLevels(str []rune) []int {
	levels := make([]int, len(str))
	if len(str) == 0 {
		return levels
	}

	var p bidi.Paragraph
	if _, err := p.SetString(string(str)); err != nil {
		return levels
	}
	order, err := p.Order()
	if err != nil {
		return levels
	}

	for i := 0; i < order.NumRuns(); i++ {
		run := order.Run(i)
		start, end := run.Pos()
		level := 0
		if run.Direction() == bidi.RightToLeft {
			level = 1
		}
		for j := start; j <= end && j < len(levels); j++ {
			if 0 <= j {
				levels[j] = level
			}
		}
	}
	return levels
}
