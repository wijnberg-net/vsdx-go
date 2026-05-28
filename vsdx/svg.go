package vsdx

import (
	"bytes"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strings"
)

// SVGOptions controls the output of ShapeToSVG.
type SVGOptions struct {
	Width      float64 // output viewBox width in px (default: 100)
	Height     float64 // output viewBox height in px (default: 100)
	Precision  int     // decimal places for coordinates (default: 2)
	BrandColor string  // override brand color detection (default: auto-detect)
}

// SVGOption is a functional option for ShapeToSVG.
type SVGOption func(*SVGOptions)

// WithSize sets the output viewBox dimensions.
func WithSize(w, h float64) SVGOption {
	return func(o *SVGOptions) { o.Width = w; o.Height = h }
}

// WithPrecision sets the coordinate decimal precision.
func WithPrecision(p int) SVGOption {
	return func(o *SVGOptions) { o.Precision = p }
}

// WithBrandColor overrides brand color auto-detection.
func WithBrandColor(c string) SVGOption {
	return func(o *SVGOptions) { o.BrandColor = c }
}

// SVGResult contains the SVG output and detected metadata.
type SVGResult struct {
	SVG        []byte  // clean SVG markup
	BrandColor string  // auto-detected or overridden brand color (#RRGGBB)
	Width      float64 // actual viewBox width (may differ from requested if aspect-ratio preserved)
	Height     float64 // actual viewBox height
}

// ArrowDef defines an SVG marker for Visio arrow types.
//
// W and H are the natural Visio lend dimensions (lend1 is W=1, H=2; lend4 is
// W=2, H=2; lend22 is W=4.5, H=2.25, etc.). The marker is rendered at
// markerWidth = W × scale and markerHeight = H × scale where scale depends on
// arrow size and stroke width — this preserves Visio's per-type proportions
// (a circle stays square, a long diamond stays long, etc.).
//
// Setback is the lend-unit distance from the line attachment point to the
// far back-edge of the marker. Open arrows (chevrons, bars) have Setback=0
// because the line passes through them; closed shapes have Setback=W (or W/2
// for centered symbols like circle and square) so the line stops at the back
// edge of the shape. Values reverse-engineered from Visio's v:setback per
// arrow type (lend4 W=2 setback=2.0, lend11 W=1.5 setback=0.75, lend22
// W=4.5 setback=4.5).
//
// Paths are auto-generated from Visio's lend1..lend30 symbols by
// arrow-inspect/generate3.py; see that script for the lend → viewBox 0..10
// coordinate mapping.
type ArrowDef struct {
	Path    string  // SVG path data in viewBox 0..10
	W       float64 // natural Visio width (along line direction)
	H       float64 // natural Visio full height (perpendicular to line)
	RefX    float64 // attachment point X in viewBox 0..10
	RefY    float64 // attachment point Y in viewBox 0..10
	Filled  bool    // true = fill, false = stroke outline
	Setback float64 // lend-unit distance for line setback (0 = open, W = closed, W/2 = centered)
}

// LengthMult returns the arrow's W/H aspect ratio (used by callers that need
// to differentiate long vs square vs tall arrows).
func (a ArrowDef) LengthMult() float64 {
	if a.H == 0 {
		return 1.0
	}
	return a.W / a.H
}

// visioArrowTypes maps Visio arrow type indices to marker definitions.
// Auto-generated from Visio's lend1..lend30 symbols by arrow-inspect/generate3.py.
// W, H are Visio's natural lend dimensions; the marker is rendered at
// markerWidth = W × scaleFactor and markerHeight = H × scaleFactor so each
// arrow keeps its proper aspect ratio. Bezier paths may extend slightly
// outside viewBox 0..10; the emitter sets overflow="visible".
var visioArrowTypes = map[int]ArrowDef{
	0: {}, // None
	1:  {Path: "M 0 0 L 10 5 L 0 10",                                                                       W: 1.000, H: 2.000, RefX: 0, RefY: 5, Filled: false, Setback: 0},     // lend1  open chevron
	2:  {Path: "M 0 10 L 10 5 L 0 0 L 0 10 z",                                                              W: 1.000, H: 2.000, RefX: 0, RefY: 5, Filled: true,  Setback: 1.000}, // lend2  filled triangle (small)
	3:  {Path: "M 0 10 L 10 5 L 0 0",                                                                       W: 2.000, H: 2.000, RefX: 0, RefY: 5, Filled: false, Setback: 0},     // lend3  open chevron (med)
	4:  {Path: "M 0 10 L 10 5 L 0 0 L 0 10 z",                                                              W: 2.000, H: 2.000, RefX: 0, RefY: 5, Filled: true,  Setback: 2.000}, // lend4  filled triangle (med)
	5:  {Path: "M 0 9.998 L 10 5 L 0.094 0.035 C 1.641 3.178 1.635 6.862 0.077 10 z",                       W: 2.000, H: 2.000, RefX: 0, RefY: 5, Filled: true,  Setback: 1.700}, // lend5  filled swept-back
	6:  {Path: "M 1.413 9.968 L 10 5 L 1.36 0 C 0 3.147 0 6.852 1.36 10 z",                                 W: 2.329, H: 2.013, RefX: 0, RefY: 5, Filled: true,  Setback: 2.200}, // lend6  filled swept-back (variant)
	7:  {Path: "M 10 0 C 7.802 3.016 4.256 4.864 0.426 4.993 L 0.426 5.007 C 4.256 5.136 7.802 6.984 10 10", W: 1.843, H: 1.919, RefX: 0, RefY: 5, Filled: false, Setback: 0},     // lend7  S-curve fishtail
	8:  {Path: "M 0 1 L 10 5 L 0 9",                                                                        W: 2.000, H: 2.000, RefX: 0, RefY: 5, Filled: false, Setback: 0},     // (gap in spec) thin open chevron
	9:  {Path: "M 10 0 L 0 10",                                                                             W: 2.000, H: 2.000, RefX: 5, RefY: 5, Filled: false, Setback: 0},     // lend9  diagonal slash
	10: {Path: "M 5 10 C 2.239 10 0 7.761 0 5 0 2.239 2.239 0 5 0 7.761 0 10 2.239 10 5 10 7.761 7.761 10 5 10 Z", W: 1.500, H: 1.500, RefX: 5, RefY: 5, Filled: true,  Setback: 0.750}, // lend10 filled circle
	11: {Path: "M 10 0 L 10 10 L 0 10 L 0 0 L 10 0 z",                                                      W: 1.500, H: 1.500, RefX: 5, RefY: 5, Filled: true,  Setback: 0.750}, // lend11 filled square
	12: {Path: "M 0 10 L 10 5 L 0 0",                                                                       W: 3.000, H: 2.000, RefX: 0, RefY: 5, Filled: false, Setback: 0},     // lend12 open chevron (long)
	13: {Path: "M 0 10 L 10 5 L 0 0 L 0 10 z",                                                              W: 3.000, H: 2.000, RefX: 0, RefY: 5, Filled: true,  Setback: 3.000}, // lend13 filled triangle (long)
	14: {Path: "M 0 0 L 10 5 L 0 10 L 0 0",                                                                 W: 3.000, H: 2.000, RefX: 0, RefY: 5, Filled: false, Setback: 3.000}, // lend14 outlined triangle (long)
	15: {Path: "M 0 0 L 10 5 L 0 10 L 0 0",                                                                 W: 1.000, H: 2.000, RefX: 0, RefY: 5, Filled: false, Setback: 1.000}, // lend15 outlined triangle (small)
	16: {Path: "M 10 5 L 0 0 L 0 10 L 10 5",                                                                W: 2.000, H: 2.000, RefX: 0, RefY: 5, Filled: false, Setback: 2.000}, // lend16 outlined triangle (med, reversed winding)
	17: {Path: "M 0 9.963 L 10 5 L 0.094 0.07 C 1.647 3.203 1.635 6.877 0.059 10",                          W: 2.000, H: 2.014, RefX: 0, RefY: 5, Filled: false, Setback: 1.700}, // lend17 open swept-back
	18: {Path: "M 1.413 9.968 L 10 5 L 1.36 0 C 0 3.147 0 6.852 1.36 10",                                   W: 2.329, H: 2.013, RefX: 0, RefY: 5, Filled: false, Setback: 2.200}, // lend18 open swept-back (variant)
	19: {Path: "M 0 10 C 2.024 6.96 2.061 3.222 0.099 0.152 L 0.117 0.15 C 2.165 3.106 5.824 4.938 9.799 4.998 L 9.799 5.002 C 5.824 5.062 2.165 6.894 0.117 9.85", W: 1.912, H: 2.216, RefX: 0, RefY: 5, Filled: false, Setback: 1.600}, // lend19 open double swept-back
	20: {Path: "M 5 10 C 2.239 10 0 7.761 0 5 0 2.239 2.239 0 5 0 7.761 0 10 2.239 10 5 10 7.761 7.761 10 5 10 Z", W: 1.500, H: 1.500, RefX: 5, RefY: 5, Filled: false, Setback: 0.750}, // lend20 open circle
	21: {Path: "M 10 0 L 10 10 L 0 10 L 0 0 L 10 0",                                                        W: 1.500, H: 1.500, RefX: 5, RefY: 5, Filled: false, Setback: 0.750}, // lend21 open square
	22: {Path: "M 0 5 L 5 0 L 10 5 L 5 10 L 0 5",                                                           W: 4.500, H: 2.250, RefX: 0, RefY: 5, Filled: false, Setback: 4.500}, // lend22 long diamond (open)
	// Bar/Y-arrow markers (back-anchored): visual elements sit BEHIND the line
	// attachment in Visio (overlapping last few sw of line). RefX=10 puts the
	// line attachment at viewBox right edge; path elements at lower viewBox X
	// then sit behind the attachment in line direction.
	23: {Path: "M 10 10 L 6 0",                                                                             W: 2.500, H: 2.000, RefX: 10, RefY: 5, Filled: false, Setback: 0},     // lend23 offset diagonal
	24: {Path: "M 10 0 L 10 10",                                                                            W: 1.500, H: 1.500, RefX: 10, RefY: 5, Filled: false, Setback: 0},     // lend24 single bar
	25: {Path: "M 6.667 0 L 6.667 10 M 10 0 L 10 10",                                                       W: 2.250, H: 1.500, RefX: 10, RefY: 5, Filled: false, Setback: 0},     // lend25 double bar
	26: {Path: "M 5 0 L 5 10 M 7.5 0 L 7.5 10 M 10 0 L 10 10",                                              W: 3.000, H: 1.500, RefX: 10, RefY: 5, Filled: false, Setback: 0},     // lend26 triple bar
	27: {Path: "M 0 0 L 10 5 L 0 10",                                                                       W: 1.500, H: 1.500, RefX: 10, RefY: 5, Filled: false, Setback: 0},     // lend27 small open chevron (back-anchored)
	28: {Path: "M 0 0 L 7.5 5 L 0 10 M 10 0 L 10 10",                                                       W: 2.000, H: 1.500, RefX: 10, RefY: 5, Filled: false, Setback: 0},     // lend28 chevron + bar
	29: {Path: "M 7.5 10 C 6.119 10 5 7.761 5 5 5 2.239 6.119 0 7.5 0 8.881 0 10 2.239 10 5 10 7.761 8.881 10 7.5 10 Z M 0 0 L 5 5 L 0 10 M 5 5 L 0 5", W: 3.000, H: 1.500, RefX: 10, RefY: 5, Filled: false, Setback: 3.000}, // lend29 chevron + tail + circle
	30: {Path: "M 7.5 10 C 6.119 10 5 7.761 5 5 5 2.239 6.119 0 7.5 0 8.881 0 10 2.239 10 5 10 7.761 8.881 10 7.5 10 Z M 2.5 0 L 2.5 10 M 5 5 L 0 5",   W: 3.000, H: 1.500, RefX: 10, RefY: 5, Filled: false, Setback: 3.000}, // lend30 bar + tail + circle
	// Types 31-45: variants documented in MS-VSDX §2.2.7. Not present in
	// vsdx-test or other current fixtures; reasonable approximations kept here
	// so existing renders that reference these IDs still emit a marker.
	31: {Path: "M0 5 A3 3 0 1 1 6 5 A3 3 0 1 1 0 5 M6 5 L10 5", W: 2.0, H: 2.0, RefX: 0, RefY: 5, Filled: false},
	32: {Path: "M0 5 A3 3 0 1 1 6 5 A3 3 0 1 1 0 5 M6 5 L10 5 M8 2 L8 8", W: 2.0, H: 2.0, RefX: 0, RefY: 5, Filled: false},
	33: {Path: "M0 5 A3 3 0 1 1 6 5 A3 3 0 1 1 0 5 M6 5 L10 5 M7 2 L7 8 M9 2 L9 8", W: 2.0, H: 2.0, RefX: 0, RefY: 5, Filled: false},
	34: {Path: "M0 5 L3 2 L6 5 L3 8 z M4 5 L7 2 L10 5 L7 8 z", W: 2.0, H: 2.0, RefX: 0, RefY: 5, Filled: false},
	35: {Path: "M0 5 A5 5 0 1 1 10 5 A5 5 0 1 1 0 5 M5 5 L10 5", W: 2.0, H: 2.0, RefX: 0, RefY: 5, Filled: true},
	36: {Path: "M0 5 A5 5 0 1 1 10 5 A5 5 0 1 1 0 5 M10 5 L14 5 M12 2 L12 8", W: 2.8, H: 2.0, RefX: 0, RefY: 5, Filled: true},
	37: {Path: "M0 5 A5 5 0 1 1 10 5 A5 5 0 1 1 0 5 M10 5 L16 5 M12 2 L12 8 M14 2 L14 8", W: 3.2, H: 2.0, RefX: 0, RefY: 5, Filled: true},
	38: {Path: "M0 5 L3 2 L6 5 L3 8 z M4 5 L7 2 L10 5 L7 8 z", W: 2.0, H: 2.0, RefX: 0, RefY: 5, Filled: true},
	39: {Path: "M0 0 L6 5 L0 10 z M4 0 L10 5 L4 10 z", W: 2.0, H: 2.0, RefX: 0, RefY: 5, Filled: true},
	40: {Path: "M0 0 L6 5 L0 10 M4 0 L10 5 L4 10", W: 2.0, H: 2.0, RefX: 0, RefY: 5, Filled: false},
	41: {Path: "M0 2 L10 5 L0 8", W: 2.0, H: 2.0, RefX: 0, RefY: 5, Filled: false},
	42: {Path: "M0 5 A5 5 0 1 1 10 5 A5 5 0 1 1 0 5 z", W: 2.4, H: 2.0, RefX: 0, RefY: 5, Filled: true},
	43: {Path: "M0 0 L5 5 L0 10 M5 0 L10 5 L5 10", W: 2.4, H: 2.0, RefX: 0, RefY: 5, Filled: false},
	44: {Path: "M0 0 L8 5 L0 10 M8 2 L8 8", W: 2.0, H: 2.0, RefX: 0, RefY: 5, Filled: false},
	45: {Path: "M0 0 L5 5 L0 10 M5 0 L10 5 L5 10 M10 2 L10 8", W: 2.0, H: 2.0, RefX: 0, RefY: 5, Filled: false},
}

// arrowSizeMultipliers maps Visio arrow size indices (0-6) to scale multipliers.
// Reverse-engineered from Visio SVG exports: size 2 → scale 1.84, size 4 → scale 2.32.
// Linear interpolation: scale = 1.84 + 0.24 * (size - 2), normalized to size 2 = 1.0.
var arrowSizeMultipliers = []float64{0.74, 0.87, 1.0, 1.13, 1.26, 1.39, 1.52}

// ArrowLengthMult returns the length-to-height aspect ratio (W/H) for a given
// arrow type. Most arrows are 1.0, longer arrows (12-14) are 1.5, long diamond
// (22) is 2.0, etc.
func ArrowLengthMult(arrowType int) float64 {
	if def, ok := visioArrowTypes[arrowType]; ok {
		return def.LengthMult()
	}
	return 1.0
}

// arrowMarkerKey generates a unique key for a marker definition.
func arrowMarkerKey(arrowType int, size int, color string, isEnd bool) string {
	pos := "start"
	if isEnd {
		pos = "end"
	}
	return fmt.Sprintf("arrow_%d_%d_%s_%s", arrowType, size, strings.TrimPrefix(color, "#"), pos)
}

// svgRenderResult holds the output from rendering a sub-shape.
func ShapeToSVG(shape *Shape, opts ...SVGOption) (*SVGResult, error) {
	o := &SVGOptions{
		Width:     100,
		Height:    100,
		Precision: 2,
	}
	for _, opt := range opts {
		opt(o)
	}

	return EmitRenderTreeWithResult(shape, o.Width, o.Height, o.Precision, o.BrandColor)
}


// generateMarkerSVG generates an SVG <marker> element for an arrow.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// renderableShape bundles a shape geometry section with its position relative to the parent group.
// A single shape with multiple geometry sections produces multiple renderableShapes.
func arcToSVG(x1, y1, x2, y2, bow float64, precision int) string {
	if math.Abs(bow) < 1e-6 {
		// Zero bow = straight line
		return fmt.Sprintf("L%s %s", fmtPrec(x2, precision), fmtPrec(y2, precision))
	}

	dx := x2 - x1
	dy := y2 - y1
	chord := math.Sqrt(dx*dx + dy*dy)
	if chord < 1e-10 {
		return ""
	}

	// radius = (chord²/4 + bow²) / (2 * |bow|)
	absBow := math.Abs(bow)
	radius := (chord*chord/4 + bow*bow) / (2 * absBow)

	// Large arc: if the arc subtends more than 180°
	largeArc := 0
	if absBow > radius {
		largeArc = 1
	}

	// Sweep direction: bow > 0 means left of direction = counterclockwise in Visio (Y-up),
	// which maps to clockwise in SVG (Y-down), so sweep=1.
	// After Y-flip, the bow sign is effectively inverted.
	sweep := 0
	if bow < 0 {
		sweep = 1
	}

	return fmt.Sprintf("A%s %s 0 %d %d %s %s",
		fmtPrec(radius, precision), fmtPrec(radius, precision),
		largeArc, sweep,
		fmtPrec(x2, precision), fmtPrec(y2, precision))
}

// sortGeometryRows returns geometry rows sorted by their IX attribute.
func sortGeometryRows(rows map[string]*GeometryRow) []*GeometryRow {
	sorted := make([]*GeometryRow, 0, len(rows))
	for _, r := range rows {
		sorted = append(sorted, r)
	}
	sort.Slice(sorted, func(i, j int) bool {
		a, _ := parseIX(sorted[i].Index())
		b, _ := parseIX(sorted[j].Index())
		return a < b
	})
	return sorted
}

func parseIX(s string) (int, bool) {
	v := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			v = v*10 + int(c-'0')
		} else {
			return v, false
		}
	}
	return v, len(s) > 0
}

// cellFloat returns the float64 value of a named cell in a geometry row.
func resolveColor(c string) string {
	if c == "" {
		return ""
	}
	c = strings.TrimSpace(c)

	// Already hex
	if strings.HasPrefix(c, "#") {
		// Normalize to uppercase
		return strings.ToUpper(c)
	}

	// RGB(r,g,b) format
	if strings.HasPrefix(strings.ToUpper(c), "RGB(") && strings.HasSuffix(c, ")") {
		inner := c[4 : len(c)-1]
		parts := strings.Split(inner, ",")
		if len(parts) == 3 {
			r := int(toFloat(strings.TrimSpace(parts[0])))
			g := int(toFloat(strings.TrimSpace(parts[1])))
			b := int(toFloat(strings.TrimSpace(parts[2])))
			return fmt.Sprintf("#%02X%02X%02X", clamp(r), clamp(g), clamp(b))
		}
	}

	// Visio indexed color (integer) — map to standard Visio color palette.
	if len(c) > 0 && c[0] >= '0' && c[0] <= '9' {
		idx := int(toFloat(c))
		if idx >= 0 && idx < len(visioColorPalette) {
			return visioColorPalette[idx]
		}
		return ""
	}

	// Visio theme reference (e.g., "Themed") — cannot resolve without theme data
	return ""
}

// lightenColor returns a lighter version of a hex color by mixing with white.
// factor: 0.0 = original color, 1.0 = white. Typical use: 0.2-0.4 for subtle lightening.
func isDarkColor(c string) bool {
	if !strings.HasPrefix(c, "#") || len(c) < 7 {
		return false
	}
	// Parse hex color
	r, g, b := 0, 0, 0
	fmt.Sscanf(c, "#%02X%02X%02X", &r, &g, &b)
	// Calculate relative luminance (simplified)
	// Using sRGB luminance formula: 0.2126*R + 0.7152*G + 0.0722*B
	luminance := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	// If luminance is below 128, consider it dark
	return luminance < 128
}

// visioColorPalette maps Visio indexed colors (0-23) to hex values.
// Defined by the MS-VSDX spec §2.2.5.3.3.1 default color table.
var visioColorPalette = []string{
	"#000000", // 0  Black
	"#FFFFFF", // 1  White
	"#FF0000", // 2  Red
	"#00FF00", // 3  Green
	"#0000FF", // 4  Blue
	"#FFFF00", // 5  Yellow
	"#FF00FF", // 6  Magenta
	"#00FFFF", // 7  Cyan
	"#800000", // 8  Dark Red
	"#008000", // 9  Dark Green
	"#000080", // 10 Dark Blue
	"#808000", // 11 Dark Yellow (Olive)
	"#800080", // 12 Purple
	"#008080", // 13 Teal
	"#C0C0C0", // 14 Silver (Light Gray)
	"#E0E0E0", // 15 Ice Blue (Lighter Gray)
	"#8080FF", // 16 Light Blue
	"#FF80FF", // 17 Light Magenta
	"#80FFFF", // 18 Light Cyan
	"#FFFF80", // 19 Light Yellow
	"#0000A0", // 20 Dark Blue 2
	"#FF8000", // 21 Orange
	"#80FF00", // 22 Chartreuse
	"#00FF80", // 23 Spring Green
}

func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// linePatternToSVG converts a Visio LinePattern value (0-23) to SVG stroke-dasharray.
// Values match Visio's SVG export exactly: dashes are 7×weight, gaps 5×weight,
// dots are 0-length (rendered as round caps when stroke-linecap=round), with
// 3×weight gaps for sparse dots. Pattern values are defined in MS-VSDX §2.4.4.
func linePatternToSVG(pattern int, weight float64) string {
	if weight <= 0 {
		weight = 1
	}
	// Visio's standard segment sizes (extracted from real Visio SVG exports):
	dash := weight * 7    // long visible dash
	gap := weight * 5     // standard gap between segments
	sparseGap := weight * 3 // tighter gap for dot-only patterns
	switch pattern {
	case 0, 1: // None, Solid
		return ""
	case 2: // Dash
		return fmt.Sprintf("%.2f %.2f", dash, gap)
	case 3: // Dot
		return fmt.Sprintf("0 %.2f", gap)
	case 4: // Dash-Dot
		return fmt.Sprintf("%.2f %.2f 0 %.2f", dash, gap, gap)
	case 5: // Dash-Dot-Dot
		return fmt.Sprintf("%.2f %.2f 0 %.2f 0 %.2f", dash, gap, gap, gap)
	case 6: // Dash-Dash-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f 0 %.2f", dash, gap, dash, gap, gap)
	case 7: // Long Dash
		return fmt.Sprintf("%.2f %.2f", weight*10, gap)
	case 8: // Long Dash-Short Dash
		return fmt.Sprintf("%.2f %.2f %.2f %.2f", weight*10, gap, dash, gap)
	case 9: // Long Dash-Short Dash-Short Dash
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f",
			weight*10, gap, dash, gap, dash, gap)
	case 10: // Sparse Dot
		return fmt.Sprintf("0 %.2f", sparseGap)
	case 11: // Dense Dot
		return fmt.Sprintf("0 %.2f", weight*2)
	case 12: // Dash-Sparse Dot
		return fmt.Sprintf("%.2f %.2f 0 %.2f", dash, sparseGap, sparseGap)
	case 13: // Dash-Dash-Sparse Dot-Sparse Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f 0 %.2f 0 %.2f",
			dash, gap, dash, gap, sparseGap, sparseGap)
	case 14: // Long Dash-Dash
		return fmt.Sprintf("%.2f %.2f %.2f %.2f", weight*10, gap, dash, gap)
	case 15: // Long Dash-Dash-Dash
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f",
			weight*10, gap, dash, gap, dash, gap)
	case 16: // Dash-Dash-Dot-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f 0 %.2f 0 %.2f",
			dash, gap, dash, gap, gap, gap)
	case 17: // Dash-Dot-Dot-Dot
		return fmt.Sprintf("%.2f %.2f 0 %.2f 0 %.2f 0 %.2f", dash, gap, gap, gap, gap)
	case 18: // Dash-Dash-Dash-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f 0 %.2f",
			dash, gap, dash, gap, dash, gap, gap)
	case 19: // Dash-Dash-Dash-Dot-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f 0 %.2f 0 %.2f",
			dash, gap, dash, gap, dash, gap, gap, gap)
	case 20: // Long Dot
		return fmt.Sprintf("%.2f %.2f", weight*2, gap)
	case 21: // Long Dash-Dot
		return fmt.Sprintf("%.2f %.2f 0 %.2f", weight*10, gap, gap)
	case 22: // Long Dash-Dot-Dot
		return fmt.Sprintf("%.2f %.2f 0 %.2f 0 %.2f", weight*10, gap, gap, gap)
	case 23: // Long Dash-Dot-Dot-Dot
		return fmt.Sprintf("%.2f %.2f 0 %.2f 0 %.2f 0 %.2f", weight*10, gap, gap, gap, gap)
	default:
		return ""
	}
}

// detectBrandColor finds the most frequent non-white, non-black color.
func detectBrandColor(freq map[string]int) string {
	var best string
	var bestCount int
	for color, count := range freq {
		upper := strings.ToUpper(color)
		// Skip white/near-white and black/near-black
		if upper == "#FFFFFF" || upper == "#000000" || upper == "#FFF" || upper == "#000" {
			continue
		}
		if count > bestCount {
			bestCount = count
			best = color
		}
	}
	return best
}

// nurbsControlPoint is an interior control point parsed from a NURBS formula.
type nurbsControlPoint struct {
	x, y   float64
	knot   float64
	weight float64
}

// nurbsData holds parsed NURBS formula data including coordinate type flags.
type nurbsData struct {
	knotLast   float64             // parameter domain end (typically != 1.0)
	degree     int                 // B-spline degree (typically 3 for cubic)
	xType      int                 // 0 = proportional (0-1), 1 = absolute (inches)
	yType      int                 // 0 = proportional (0-1), 1 = absolute (inches)
	cps        []nurbsControlPoint // interior control points
	lastCPKnot float64             // knot value of the LAST control point (from row's "A" cell)
}

// parseNURBSData extracts data from a NURBS() formula string.
// Format: NURBS(knotLast, degree, xType, yType, x1,y1,knot1,weight1, x2,y2,knot2,weight2, ...)
// Returns the coordinate types and (x,y) pairs of interior control points.
func parseNURBSData(formula string) *nurbsData {
	// Strip "NURBS(" prefix and ")" suffix.
	formula = strings.TrimSpace(formula)
	upper := strings.ToUpper(formula)
	if !strings.HasPrefix(upper, "NURBS(") || !strings.HasSuffix(formula, ")") {
		return nil
	}
	inner := formula[6 : len(formula)-1]

	// Split by comma and trim spaces.
	parts := strings.Split(inner, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// Need at least 4 header values (knotLast, degree, xType, yType) + 4 per control point.
	if len(parts) < 8 {
		return nil
	}

	// Parse header: knotLast (index 0), degree (index 1), xType (index 2), yType (index 3)
	knotLast := toFloat(parts[0])
	degree := int(toFloat(parts[1]))
	xType := int(toFloat(parts[2]))
	yType := int(toFloat(parts[3]))

	// Control points start at index 4, in groups of 4: x, y, knot, weight.
	var cps []nurbsControlPoint
	for i := 4; i+3 < len(parts); i += 4 {
		x := toFloat(parts[i])
		y := toFloat(parts[i+1])
		k := toFloat(parts[i+2])
		w := toFloat(parts[i+3])
		if w == 0 {
			w = 1
		}
		cps = append(cps, nurbsControlPoint{x: x, y: y, knot: k, weight: w})
	}
	return &nurbsData{knotLast: knotLast, degree: degree, xType: xType, yType: yType, cps: cps}
}

// parseNURBSControlPoints extracts interior control points from a NURBS() formula string.
// Format: NURBS(knotLast, degree, xType, yType, x1,y1,knot1,weight1, x2,y2,knot2,weight2, ...)
// Returns the (x,y) pairs of interior control points.
// Note: This ignores xType/yType. Use parseNURBSData for full coordinate handling.
type polylinePoint struct {
	x, y float64
}

// parsePolylinePoints extracts vertices from a POLYLINE() formula string.
// Format: POLYLINE(xType, yType, x1,y1, x2,y2, ...)
// Returns the (x,y) points, converted to absolute coordinates.
func parsePolylinePoints(formula string, localW, localH, offsetX, offsetY float64) []polylinePoint {
	formula = strings.TrimSpace(formula)
	upper := strings.ToUpper(formula)
	if !strings.HasPrefix(upper, "POLYLINE(") || !strings.HasSuffix(formula, ")") {
		return nil
	}
	inner := formula[9 : len(formula)-1]

	parts := strings.Split(inner, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// Need at least xType, yType + at least one point (2 coords)
	if len(parts) < 4 {
		return nil
	}

	// xType and yType: 0 = proportional (0-1), 1 = absolute inches
	xType := toFloat(parts[0])
	yType := toFloat(parts[1])

	var pts []polylinePoint
	for i := 2; i+1 < len(parts); i += 2 {
		x := toFloat(parts[i])
		y := toFloat(parts[i+1])
		// Convert to absolute coordinates
		if xType == 0 {
			x = x*localW + offsetX
		} else {
			x = x + offsetX
		}
		if yType == 0 {
			y = y*localH + offsetY
		} else {
			y = y + offsetY
		}
		pts = append(pts, polylinePoint{x: x, y: y})
	}
	return pts
}

// cellString returns the string value of a named cell in a geometry row.
func fmtPrec(v float64, prec int) string {
	s := fmt.Sprintf("%.*f", prec, v)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// shortenPathEnd shortens an SVG path at the end by the given length.
// This is used to prevent connector lines from extending through arrowheads.
type ForeignImage struct {
	Data        []byte  // raw image bytes (EMF, PNG, JPEG, etc.)
	ForeignType string  // "EnhMetaFile", "Bitmap", "MetaFile"
	Compression string  // "PNG", "JPEG", "" (uncompressed)
	Extension   string  // file extension: "emf", "png", "jpeg", "wmf"
	Width       float64 // image width in inches (from ImgWidth cell)
	Height      float64 // image height in inches (from ImgHeight cell)
}

// ForeignImageData extracts the embedded image data from a Foreign-type shape.
// Returns nil if the shape has no ForeignData element or if the image cannot be resolved.
//
// Cisco Visio stencils store device icons as embedded EMF (Enhanced MetaFile) images.
// The resolution chain is: Shape ForeignData → Rel rId → page/master rels → media path → zip bytes.
func (s *Shape) ForeignImageData() *ForeignImage {
	fd := s.xml.FindElement("ForeignData")
	if fd == nil {
		return nil
	}

	foreignType := fd.SelectAttrValue("ForeignType", "")
	compression := fd.SelectAttrValue("CompressionType", "")

	// Resolve the relationship ID to a media path.
	relElem := fd.FindElement("Rel")
	if relElem == nil {
		return nil
	}
	relID := relElem.SelectAttrValue("id", "")
	if relID == "" {
		return nil
	}

	// Look up the target path in the page/master rels.
	page := s.Page
	if page == nil || page.RelsXML == nil {
		return nil
	}

	var targetPath string
	for _, rel := range page.RelsXML.Root().SelectElements("Relationship") {
		if rel.SelectAttrValue("Id", "") == relID {
			targetPath = rel.SelectAttrValue("Target", "")
			break
		}
	}
	if targetPath == "" {
		return nil
	}

	// Resolve relative path (e.g., "../media/image1.emf") to ZIP path.
	mediaPath := resolveMediaPath(page.filename, targetPath)

	// Look up in zip contents.
	vis := page.vis
	if vis == nil {
		return nil
	}
	data, ok := vis.ZipFileContents[mediaPath]
	if !ok {
		return nil
	}

	// Determine file extension.
	ext := extensionFromPath(mediaPath)
	if ext == "" {
		ext = extensionFromForeignType(foreignType, compression)
	}

	return &ForeignImage{
		Data:        data,
		ForeignType: foreignType,
		Compression: compression,
		Extension:   ext,
		Width:       toFloat(s.CellValue("ImgWidth")),
		Height:      toFloat(s.CellValue("ImgHeight")),
	}
}

// resolveMediaPath resolves a relative target path against the page's directory.
// e.g., page="visio/masters/master1.xml", target="../media/image1.emf" → "visio/media/image1.emf"
func resolveMediaPath(pagePath, target string) string {
	if !strings.Contains(target, "..") {
		// Absolute or same-directory path.
		dir := pagePath[:strings.LastIndex(pagePath, "/")+1]
		return dir + target
	}

	// Split and resolve ".." segments.
	base := strings.Split(pagePath[:strings.LastIndex(pagePath, "/")], "/")
	parts := strings.Split(target, "/")

	for _, p := range parts {
		if p == ".." {
			if len(base) > 0 {
				base = base[:len(base)-1]
			}
		} else if p != "." && p != "" {
			base = append(base, p)
		}
	}
	return strings.Join(base, "/")
}

// extensionFromPath extracts the file extension from a path.
func extensionFromPath(path string) string {
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		return strings.ToLower(path[idx+1:])
	}
	return ""
}

// extensionFromForeignType maps ForeignType + CompressionType to a file extension.
func extensionFromForeignType(foreignType, compression string) string {
	switch strings.ToLower(foreignType) {
	case "enhmetafile":
		return "emf"
	case "metafile":
		return "wmf"
	case "bitmap":
		switch strings.ToLower(compression) {
		case "png":
			return "png"
		case "jpeg":
			return "jpeg"
		default:
			return "bmp"
		}
	}
	return ""
}

// ConvertEMFToSVG converts EMF (Enhanced MetaFile) bytes to SVG using emf2svg-conv.
// Requires the emf2svg package to be installed (Debian: apt install emf2svg).
// Returns clean SVG bytes suitable for web display.
func ConvertEMFToSVG(emfData []byte) ([]byte, error) {
	cmd := exec.Command("emf2svg-conv", "-i", "/dev/stdin", "-o", "/dev/stdout")
	cmd.Stdin = bytes.NewReader(emfData)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("emf2svg-conv: %w (stderr: %s)", err, stderr.String())
	}

	svgBytes := stdout.Bytes()
	if len(svgBytes) == 0 {
		return nil, fmt.Errorf("emf2svg-conv produced empty output")
	}

	return svgBytes, nil
}

// MasterToSVG converts a master page's primary shape to SVG.
// It handles both geometry-based shapes and Foreign (EMF) shapes.
//
// For geometry shapes: uses ShapeToSVG to convert Visio geometry to SVG paths.
// For EMF shapes: extracts the embedded EMF and converts via emf2svg-conv.
//
// Returns the SVG result with brand color metadata.
func MasterToSVG(master *Page, opts ...SVGOption) (*SVGResult, error) {
	shapes := master.ChildShapes()
	if len(shapes) == 0 {
		return nil, fmt.Errorf("master %q has no shapes", master.Name())
	}

	shape := shapes[0]

	// Try geometry-based conversion first (works for shapes with real path data).
	if shape.ShapeType != "Foreign" {
		return ShapeToSVG(shape, opts...)
	}

	// Foreign shape — extract embedded image.
	img := shape.ForeignImageData()
	if img == nil {
		// Fall back to geometry (some Foreign shapes also have geometry).
		return ShapeToSVG(shape, opts...)
	}

	switch img.Extension {
	case "emf":
		svgBytes, err := ConvertEMFToSVG(img.Data)
		if err != nil {
			return nil, fmt.Errorf("converting EMF for %q: %w", master.Name(), err)
		}
		// Detect brand color from the generated SVG.
		brandColor := detectSVGBrandColor(string(svgBytes))
		return &SVGResult{SVG: svgBytes, BrandColor: brandColor}, nil

	case "svg":
		// Already SVG — return as-is.
		return &SVGResult{SVG: img.Data, BrandColor: detectSVGBrandColor(string(img.Data))}, nil

	default:
		return nil, fmt.Errorf("unsupported foreign image type %q for %q (need EMF or SVG)", img.Extension, master.Name())
	}
}

// detectSVGBrandColor scans SVG markup for the most frequent non-B&W fill/stroke color.
func detectSVGBrandColor(svg string) string {
	freq := make(map[string]int)

	// Simple scan for fill="..." and stroke="..." hex colors.
	for _, attr := range []string{"fill=", "stroke="} {
		idx := 0
		for {
			pos := strings.Index(svg[idx:], attr)
			if pos < 0 {
				break
			}
			pos += idx + len(attr)
			if pos < len(svg) && svg[pos] == '"' {
				end := strings.Index(svg[pos+1:], "\"")
				if end > 0 {
					color := svg[pos+1 : pos+1+end]
					if strings.HasPrefix(color, "#") && (len(color) == 7 || len(color) == 4) {
						freq[strings.ToUpper(color)]++
					}
				}
			}
			idx = pos + 1
		}
	}

	return detectBrandColor(freq)
}
