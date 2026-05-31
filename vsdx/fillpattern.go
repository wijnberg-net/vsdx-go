package vsdx

import (
	"fmt"
	"strings"
)

// patternBitmap maps each fillPattern type to its canonical 8×8 bitmap as
// decoded from Visio's SVG export PNGs. Each string is 8 rows of 8 chars,
// '.' = background, 'X' = foreground. Patterns absent from this map fall
// back to the legacy line/circle emitters below.
var patternBitmap = map[int][8]string{
	2: { // Thin diagonal upward
		".......X",
		"......X.",
		".....X..",
		"....X...",
		"...X....",
		"..X.....",
		".X......",
		"X.......",
	},
	3: { // L-shape grid (top edge + left edge)
		"XXXXXXXX",
		"X.......",
		"X.......",
		"X.......",
		"X.......",
		"X.......",
		"X.......",
		"X.......",
	},
	4: { // X pattern (crosshatch)
		"X......X",
		".X....X.",
		"..X..X..",
		"...XX...",
		"...XX...",
		"..X..X..",
		".X....X.",
		"X......X",
	},
	5: { // Thin diagonal downward
		"X.......",
		".X......",
		"..X.....",
		"...X....",
		"....X...",
		".....X..",
		"......X.",
		".......X",
	},
	6: { // Horizontal line at top
		"XXXXXXXX",
		"........",
		"........",
		"........",
		"........",
		"........",
		"........",
		"........",
	},
	7: { // Vertical line at left
		"X.......",
		"X.......",
		"X.......",
		"X.......",
		"X.......",
		"X.......",
		"X.......",
		"X.......",
	},
	8: { // Dense pattern: inverse-X (Visio's "shading")
		".XXX.XXX",
		"XX.XXX.X",
		".XXX.XXX",
		"XX.XXX.X",
		".XXX.XXX",
		"XX.XXX.X",
		".XXX.XXX",
		"XX.XXX.X",
	},
	9: { // Checkerboard
		".X.X.X.X",
		"X.X.X.X.",
		".X.X.X.X",
		"X.X.X.X.",
		".X.X.X.X",
		"X.X.X.X.",
		".X.X.X.X",
		"X.X.X.X.",
	},
}

// fillPatternToSVG generates an SVG <pattern> element for a Visio fill pattern.
// Patterns 2-24 are hatching patterns (lines, grids, dots).
// Patterns 25-40 are gradient-like patterns (handled separately via FillGradient).
//
// Pixel-accurate mode: when patternType has an entry in patternBitmap, we
// emit the pattern as an 8×8 grid of <rect> elements inside a viewBox so
// the rasteriser sees the same pixel structure Visio's embedded 8×8 PNG
// produces. Other patterns fall back to the legacy line/circle emitters.
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

	// Tile size in user-space units. Visio uses 6 for hatch patterns; we
	// expose the bitmap as `viewBox="0 0 8 8"` so each "pixel" is 1 unit
	// in the bitmap's coordinate frame, then SVG scales the viewBox down
	// to the 6-unit tile.
	size := 6.0 * scale

	// Pixel-accurate path: emit one rect per filled cell of the 8×8 bitmap
	// inside a viewBox so the renderer sees exactly Visio's pixel
	// structure (the same effect Visio gets by embedding an 8×8 PNG with
	// image-rendering="optimizeSpeed").
	if bm, ok := patternBitmap[patternType]; ok {
		var svg strings.Builder
		svg.WriteString(fmt.Sprintf(
			`<pattern id="%s" patternUnits="userSpaceOnUse" width="%g" height="%g" viewBox="0 0 8 8">`,
			id, size, size))
		svg.WriteString(fmt.Sprintf(`<rect width="8" height="8" fill="%s" shape-rendering="crispEdges"/>`, backColor))
		for y, row := range bm {
			for x, ch := range row {
				if ch == 'X' {
					svg.WriteString(fmt.Sprintf(
						`<rect x="%d" y="%d" width="1" height="1" fill="%s" shape-rendering="crispEdges"/>`,
						x, y, foreColor))
				}
			}
		}
		svg.WriteString("</pattern>")
		return svg.String()
	}

	var svg strings.Builder
	svg.WriteString(fmt.Sprintf(`<pattern id="%s" patternUnits="userSpaceOnUse" width="%g" height="%g">`, id, size, size))

	// Background rectangle
	svg.WriteString(fmt.Sprintf(`<rect width="%g" height="%g" fill="%s"/>`, size, size, backColor))

	// crispEdges suppresses anti-aliasing on the hatch strokes so they
	// rasterise into clean 1-pixel-wide diagonals matching Visio's PNG
	// bitmaps. Without this hint, rsvg-convert and Chrome both bloom each
	// 1pt stroke into a ~1.5px-wide soft band that looks "fuzzier" than
	// Visio's crisp pixel-aligned hatch.
	se := ` shape-rendering="crispEdges"`

	// Pattern-specific content. Visio's canonical pattern bitmaps (decoded
	// from the base64 PNGs in Visio's SVG export) put the hatch on the
	// tile EDGES, not centered — that's how adjacent tiles join into a
	// continuous diagonal / grid / horizontal-line pattern. Earlier
	// vsdx-go centered the strokes and added "bleed" pieces that produced
	// a chaotic mess for patterns 2 and 5.
	switch patternType {
	case 2: // Thin diagonal upward (bottom-left → top-right). Single line.
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="%g" x2="%g" y2="0" stroke="%s" stroke-width="1"%s/>`, size, size, foreColor, se))

	case 3: // L-shaped grid: top edge + left edge. Tiles into a square grid.
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="0" x2="%g" y2="0" stroke="%s" stroke-width="1"%s/>`, size, foreColor, se))
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="0" x2="0" y2="%g" stroke="%s" stroke-width="1"%s/>`, size, foreColor, se))

	case 4: // Crosshatch (diagonal both ways).
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="1"%s/>`, size, size, foreColor, se))
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="%g" x2="%g" y2="0" stroke="%s" stroke-width="1"%s/>`, size, size, foreColor, se))

	case 5: // Thin diagonal downward (top-left → bottom-right). Single line.
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="0" x2="%g" y2="%g" stroke="%s" stroke-width="1"%s/>`, size, size, foreColor, se))

	case 6: // Horizontal line at top edge — tiles into evenly-spaced rows.
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="0" x2="%g" y2="0" stroke="%s" stroke-width="1"%s/>`, size, foreColor, se))

	case 7: // Vertical line at left edge — tiles into evenly-spaced columns.
		svg.WriteString(fmt.Sprintf(`<line x1="0" y1="0" x2="0" y2="%g" stroke="%s" stroke-width="1"%s/>`, size, foreColor, se))

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
