package vsdx

import (
	"fmt"
	"strings"
)

// fillPatternToSVG generates an SVG <pattern> element for a Visio fill pattern.
// Patterns 2-24 are hatching patterns (lines, grids, dots).
// Patterns 25-40 are gradient-like patterns (handled separately via FillGradient).
func fillPatternToSVG(patternType int, id, foreColor, backColor string, scale float64) string {
	if patternType < 2 || patternType > 24 {
		return ""
	}

	if scale <= 0 {
		scale = 1.0
	}
	if foreColor == "" {
		foreColor = "#000000"
	}
	if backColor == "" {
		backColor = "#FFFFFF"
	}

	// Base pattern size (will be scaled)
	size := 8.0 * scale

	var svg strings.Builder
	svg.WriteString(fmt.Sprintf(`<pattern id="%s" patternUnits="userSpaceOnUse" width="%g" height="%g">`, id, size, size))

	// Background rectangle
	svg.WriteString(fmt.Sprintf(`<rect width="%g" height="%g" fill="%s"/>`, size, size, backColor))

	// Pattern-specific content
	switch patternType {
	case 2: // Diagonal lines (top-left to bottom-right)
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, size, size, foreColor))
		svg.WriteString(fmt.Sprintf(`<line x1="-%g" y1="0" x2="0" y2="%g" stroke="%s" stroke-width="1"/>`, size/2, size/2, foreColor))
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, size/2, size, size/2, foreColor))

	case 3: // Grid (horizontal and vertical lines)
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="%g" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, size/2, size, size/2, foreColor))
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, size/2, size/2, size, foreColor))

	case 4: // Crosshatch (diagonal both ways)
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, size, size, foreColor))
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="0" y2="%g" stroke="%s" stroke-width="1"/>`, size, size, foreColor))

	case 5: // Diagonal lines (bottom-left to top-right)
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="0" y2="%g" stroke="%s" stroke-width="1"/>`, size, size, foreColor))
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, size, size/2, size/2, foreColor))
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="%g" x2="0" y2="%g" stroke="%s" stroke-width="1"/>`, size/2, 0.0, size/2, foreColor))

	case 6: // Horizontal lines
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="%g" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, size/2, size, size/2, foreColor))

	case 7: // Vertical lines
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, size/2, size/2, size, foreColor))

	case 8: // Dense diagonal (narrow spacing)
		for i := 0.0; i < size*2; i += size / 4 {
			svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, i-size, i, size, foreColor))
		}

	case 9: // Dense diagonal (other direction)
		for i := 0.0; i < size*2; i += size / 4 {
			svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, size+size-i, 0.0-size+i, size, foreColor))
		}

	case 10: // Sparse dots
		svg.WriteString(fmt.Sprintf(`<circle cx="%g" cy="%g" r="1" fill="%s"/>`, size/2, size/2, foreColor))

	case 11: // Dense dots
		for x := size / 4; x < size; x += size / 2 {
			for y := size / 4; y < size; y += size / 2 {
				svg.WriteString(fmt.Sprintf(`<circle cx="%g" cy="%g" r="1" fill="%s"/>`, x, y, foreColor))
			}
		}

	case 12: // Very sparse dots
		svg.WriteString(fmt.Sprintf(`<circle cx="%g" cy="%g" r="0.5" fill="%s"/>`, size/2, size/2, foreColor))

	case 13: // Horizontal lines (narrow)
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="%g" x2="%g" y2="%g" stroke="%s" stroke-width="2"/>`, size/2, size, size/2, foreColor))

	case 14: // Vertical lines (narrow)
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="2"/>`, size/2, size/2, size, foreColor))

	case 15: // Diagonal (wider spacing)
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, size, size, foreColor))

	case 16: // Diagonal (other direction, wider)
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="0" y2="%g" stroke="%s" stroke-width="1"/>`, size, size, foreColor))

	case 17: // Very sparse small dots
		svg.WriteString(fmt.Sprintf(`<circle cx="%g" cy="%g" r="0.3" fill="%s"/>`, size/2, size/2, foreColor))

	case 18: // Horizontal dash
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="%g" x2="%g" y2="%g" stroke="%s" stroke-width="1" stroke-dasharray="2 2"/>`, size/2, size, size/2, foreColor))

	case 19: // Sparse dots (variant)
		svg.WriteString(fmt.Sprintf(`<circle cx="%g" cy="%g" r="0.8" fill="%s"/>`, size/2, size/2, foreColor))

	case 20: // Horizontal with dots
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="%g" x2="%g" y2="%g" stroke="%s" stroke-width="1"/>`, size/2, size, size/2, foreColor))
		svg.WriteString(fmt.Sprintf(`<circle cx="%g" cy="%g" r="0.5" fill="%s"/>`, size/4, size/4, foreColor))

	case 21: // Diagonal wide
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="2"/>`, size, size, foreColor))

	case 22: // Diagonal other direction wide
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="0" y2="%g" stroke="%s" stroke-width="2"/>`, size, size, foreColor))

	case 23: // Fine grid
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="%g" x2="%g" y2="%g" stroke="%s" stroke-width="0.5"/>`, size/4, size, size/4, foreColor))
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="%g" x2="%g" y2="%g" stroke="%s" stroke-width="0.5"/>`, size*3/4, size, size*3/4, foreColor))
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="0.5"/>`, size/4, size/4, size, foreColor))
		svg.WriteString(fmt.Sprintf(`<line x1="%g" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="0.5"/>`, size*3/4, size*3/4, size, foreColor))

	case 24: // Dense small dots
		for x := size / 8; x < size; x += size / 4 {
			for y := size / 8; y < size; y += size / 4 {
				svg.WriteString(fmt.Sprintf(`<circle cx="%g" cy="%g" r="0.5" fill="%s"/>`, x, y, foreColor))
			}
		}
	}

	svg.WriteString("</pattern>")
	return svg.String()
}

// createFillPatternDef creates a fill pattern definition.
func createFillPatternDef(patternType int, foreColor, backColor string) *FillPatternDef {
	if patternType < 2 || patternType > 24 {
		return nil
	}

	id := fmt.Sprintf("fillpat_%d_%s_%s",
		patternType,
		strings.TrimPrefix(foreColor, "#"),
		strings.TrimPrefix(backColor, "#"))

	return &FillPatternDef{
		ID:          id,
		PatternType: patternType,
		ForeColor:   foreColor,
		BackColor:   backColor,
		Scale:       1.0,
	}
}
