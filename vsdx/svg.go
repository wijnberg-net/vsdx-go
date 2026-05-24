package vsdx

import (
	"bytes"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strconv"
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
type ArrowDef struct {
	Path      string  // SVG path data
	Width     float64 // marker width relative to line weight
	Height    float64 // marker height relative to line weight
	RefX      float64 // attachment point X
	RefY      float64 // attachment point Y
	Filled    bool    // whether the arrow is filled
	LengthMult float64 // length multiplier (1.0 = standard, 1.5 = longer arrow like type 13)
}

// visioArrowTypes maps Visio arrow type indices to SVG path definitions.
// RefX=0 places arrow back at line end, so tip extends forward by marker width.
// Path is shortened by setback to compensate, placing tip at original endpoint.
// LengthMult adjusts arrow length relative to standard (type 4 = 1.0, type 13 = 1.5).
var visioArrowTypes = map[int]ArrowDef{
	0:  {}, // None
	1:  {Path: "M0 0 L10 5 L0 10 z", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: true, LengthMult: 1.0},       // Triangle (filled)
	2:  {Path: "M0 0 L10 5 L0 10 L2 5 z", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: true, LengthMult: 1.0},  // Stealth
	3:  {Path: "M0 0 L10 5 L0 10", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: false, LengthMult: 1.0},        // Triangle (open)
	4:  {Path: "M0 0 L10 5 L0 10 z", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: true, LengthMult: 1.0},  // Line arrow (short triangle)
	5:  {Path: "M10 5 L2 0 L3.4 5 L2 10 z", Width: 10, Height: 10, RefX: 2, RefY: 5, Filled: true, LengthMult: 1.0}, // Stealth with concave back
	6:  {Path: "M0 5 L5 0 L10 5 L5 10 z", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: true, LengthMult: 1.0},  // Diamond (filled)
	7:  {Path: "M0 5 L5 0 L10 5 L5 10 z", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: false, LengthMult: 1.0}, // Diamond (open)
	8:  {Path: "M0 5 A5 5 0 1 1 10 5 A5 5 0 1 1 0 5 z", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: false, LengthMult: 1.0},  // Oval (open)
	9:  {Path: "M0 0 L10 5 L0 10 z", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: true, LengthMult: 1.0},       // Double triangle
	10: {Path: "M0 0 L10 5 L0 10 z", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: true, LengthMult: 1.0},       // Triangle 45°
	13: {Path: "M0 0 L10 5 L0 10 z", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: true, LengthMult: 1.5},       // Standard arrow (longer)
	14: {Path: "M0 0 L10 5 L0 10", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: false, LengthMult: 1.5},        // Open arrow (longer)
	22: {Path: "M0 5 L5 0 L10 5 M0 5 L5 10 L10 5", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: false, LengthMult: 1.0}, // Fletching
	45: {Path: "M0 0 L10 5 L0 10 z", Width: 10, Height: 10, RefX: 0, RefY: 5, Filled: true, LengthMult: 1.0},       // Filled arrow (fallback)
}

// arrowSizeMultipliers maps Visio arrow size indices (0-6) to scale multipliers.
var arrowSizeMultipliers = []float64{0.5, 0.7, 1.0, 1.3, 1.6, 2.0, 2.5}

// arrowMarkerKey generates a unique key for a marker definition.
func arrowMarkerKey(arrowType int, size int, color string, isEnd bool) string {
	pos := "start"
	if isEnd {
		pos = "end"
	}
	return fmt.Sprintf("arrow_%d_%d_%s_%s", arrowType, size, strings.TrimPrefix(color, "#"), pos)
}

// svgRenderResult holds the output from rendering a sub-shape.
type svgRenderResult struct {
	pathSVG     string
	strokeWidth float64
	markers     []markerRef
	gradientID  string    // gradient definition ID (if any)
	gradient    *Gradient // gradient definition (if any)
	shadowID    string    // shadow filter ID (if any)
	shadow      *Shadow   // shadow definition (if any)
}

// markerRef identifies a marker needed for a path.
type markerRef struct {
	key       string
	arrowType int
	size      int
	color     string
	isEnd     bool
}

// ShapeToSVG converts a Visio shape (typically a master shape) to SVG bytes.
// The shape's geometry sections are converted to SVG paths with styling.
// Group shapes recursively include all child shape geometry.
// This is the canonical entry point using the RenderTree architecture.
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

// ShapeToSVGLegacy converts a Visio shape using the legacy renderer.
// This is kept for comparison and validation purposes.
func ShapeToSVGLegacy(shape *Shape, opts ...SVGOption) (*SVGResult, error) {
	o := &SVGOptions{
		Width:     100,
		Height:    100,
		Precision: 2,
	}
	for _, opt := range opts {
		opt(o)
	}

	// Collect all sub-shapes to render.
	// For groups: use child shapes. For simple shapes: just this shape.
	subShapes := collectRenderableShapes(shape)
	if len(subShapes) == 0 {
		return nil, fmt.Errorf("shape has no geometry to convert")
	}

	// Determine the bounding box in shape-local coordinates (inches).
	shapeW := shape.Width()
	shapeH := shape.Height()
	// Track negative dimensions - connectors can have negative width/height
	// indicating direction (left/up instead of right/down).
	negativeWidth := shapeW < 0
	negativeHeight := shapeH < 0
	// Use absolute values for scaling
	if shapeW < 0 {
		shapeW = -shapeW
	}
	if shapeW == 0 {
		shapeW = 1
	}
	if shapeH < 0 {
		shapeH = -shapeH
	}
	if shapeH == 0 {
		shapeH = 1
	}

	// Uniform scale: fit the shape proportionally into the requested output box.
	// This preserves the original aspect ratio instead of stretching.
	scale := o.Width / shapeW
	if s := o.Height / shapeH; s < scale {
		scale = s
	}
	scaleX := scale
	scaleY := scale
	// Actual output dimensions after uniform scaling.
	outW := shapeW * scale
	outH := shapeH * scale

	// Color frequency map for brand color detection.
	colorFreq := make(map[string]int)

	var pathElements []string
	var maxStrokeWidth float64
	markerDefs := make(map[string]markerRef)   // unique markers needed
	gradientDefs := make(map[string]*Gradient) // unique gradients needed
	shadowDefs := make(map[string]*Shadow)     // unique shadows needed
	for _, ss := range subShapes {
		result := renderSubShapeWithMarkers(ss, shape, scaleX, scaleY, o, colorFreq)
		if result.pathSVG != "" {
			pathElements = append(pathElements, result.pathSVG)
			if result.strokeWidth > maxStrokeWidth {
				maxStrokeWidth = result.strokeWidth
			}
			for _, m := range result.markers {
				markerDefs[m.key] = m
			}
			if result.gradientID != "" && result.gradient != nil {
				gradientDefs[result.gradientID] = result.gradient
			}
			if result.shadowID != "" && result.shadow != nil {
				shadowDefs[result.shadowID] = result.shadow
			}
		}
	}

	if len(pathElements) == 0 {
		return nil, fmt.Errorf("no geometry paths produced from shape")
	}

	// Detect brand color.
	brandColor := o.BrandColor
	if brandColor == "" {
		brandColor = detectBrandColor(colorFreq)
	}

	// Pad viewBox by half the max stroke width so strokes aren't clipped.
	// Also account for arrow markers which extend beyond the stroke.
	arrowPad := 0.0
	if len(markerDefs) > 0 {
		arrowPad = maxStrokeWidth * 3 // arrows can extend ~3x stroke width
	}
	pad := maxStrokeWidth/2 + arrowPad
	// For negative width/height shapes, the path extends in the negative direction.
	// Adjust viewBox origin accordingly.
	vbX := -pad
	vbY := -pad
	if negativeWidth {
		vbX = -outW - pad
	}
	if negativeHeight {
		vbY = -outH - pad
	}
	vbW := outW + 2*pad
	vbH := outH + 2*pad

	// Build SVG document.
	var svg strings.Builder
	svg.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s %s %s %s">`,
		fmtPrec(vbX, o.Precision), fmtPrec(vbY, o.Precision),
		fmtPrec(vbW, o.Precision), fmtPrec(vbH, o.Precision)))
	svg.WriteByte('\n')

	// Generate <defs> section with marker, gradient, and shadow definitions.
	if len(markerDefs) > 0 || len(gradientDefs) > 0 || len(shadowDefs) > 0 {
		svg.WriteString("  <defs>\n")
		for _, m := range markerDefs {
			markerSVG := generateMarkerSVG(m, o.Precision)
			if markerSVG != "" {
				svg.WriteString(markerSVG)
				svg.WriteByte('\n')
			}
		}
		for id, grad := range gradientDefs {
			gradSVG := gradientToSVGDef(grad, id, o.Precision)
			if gradSVG != "" {
				svg.WriteString("    ")
				svg.WriteString(gradSVG)
				svg.WriteByte('\n')
			}
		}
		for id, shadow := range shadowDefs {
			shadowSVG := shadowToSVGFilter(shadow, id, scaleX, scaleY, o.Precision)
			if shadowSVG != "" {
				svg.WriteString("    ")
				svg.WriteString(shadowSVG)
				svg.WriteByte('\n')
			}
		}
		svg.WriteString("  </defs>\n")
	}

	// Check if the shape has rotation (Angle property in radians).
	shapeAngle := toFloat(shape.CellValue("Angle"))
	hasRotation := shapeAngle != 0

	// If rotated, wrap content in a group with rotation transform.
	// Rotation is around the center of the output SVG.
	// Note: Negate the angle because Visio uses counter-clockwise positive (Y-up)
	// while SVG uses clockwise positive, and we've already flipped the Y axis.
	if hasRotation {
		angleDeg := -shapeAngle * 180 / math.Pi
		cx := outW / 2
		cy := outH / 2
		svg.WriteString(fmt.Sprintf(`  <g transform="rotate(%s %s %s)">`,
			fmtPrec(angleDeg, o.Precision),
			fmtPrec(cx, o.Precision),
			fmtPrec(cy, o.Precision)))
		svg.WriteByte('\n')
	}

	for _, p := range pathElements {
		svg.WriteString(p)
		svg.WriteByte('\n')
	}

	// Render text from all sub-shapes.
	for _, ss := range subShapes {
		textSVG := renderShapeText(ss, shape, scaleX, scaleY, o)
		if textSVG != "" {
			svg.WriteString(textSVG)
			svg.WriteByte('\n')
		}
	}

	// Close rotation group if present.
	if hasRotation {
		svg.WriteString("  </g>\n")
	}

	svg.WriteString("</svg>\n")

	return &SVGResult{
		SVG:        []byte(svg.String()),
		BrandColor: brandColor,
		Width:      outW,
		Height:     outH,
	}, nil
}

// generateMarkerSVG generates an SVG <marker> element for an arrow.
func generateMarkerSVG(m markerRef, precision int) string {
	def, ok := visioArrowTypes[m.arrowType]
	if !ok || def.Path == "" {
		// Use triangle as fallback for unknown arrow types.
		def = visioArrowTypes[1]
		if def.Path == "" {
			return ""
		}
	}

	// Calculate size multiplier.
	sizeMult := 1.0
	if m.size >= 0 && m.size < len(arrowSizeMultipliers) {
		sizeMult = arrowSizeMultipliers[m.size]
	}

	// Apply arrow length multiplier (e.g., type 13 is 1.5x longer than type 4)
	lengthMult := def.LengthMult
	if lengthMult == 0 {
		lengthMult = 1.0
	}

	// Scale marker size relative to line width.
	// markerWidth/Height are in stroke-width units (markerUnits="strokeWidth").
	scaleFactor := sizeMult * 0.36
	w := def.Width * scaleFactor * lengthMult
	h := def.Height * scaleFactor

	// refX=0 for all arrows - arrow back at attachment, tip extends forward
	// Path shortening ensures the line ends before the arrow
	refX := def.RefX
	refY := def.RefY

	fillAttr := "none"
	strokeAttr := m.color
	if def.Filled {
		fillAttr = m.color
		strokeAttr = "none"
	}

	// For arrows to point INTO the connected shapes:
	// - Start marker: should point backward (into the shape where connector begins)
	// - End marker: should point forward (into the shape where connector ends)
	// With arrow tip at positive X and orient="auto", arrow points along path direction.
	// Use auto-start-reverse at START to flip it backward into the start shape.
	orient := "auto"
	if !m.isEnd {
		orient = "auto-start-reverse"
	}

	return fmt.Sprintf(`    <marker id="%s" viewBox="0 0 %s %s" refX="%s" refY="%s" markerWidth="%s" markerHeight="%s" markerUnits="strokeWidth" orient="%s"><path d="%s" fill="%s" stroke="%s"/></marker>`,
		m.key,
		fmtPrec(def.Width, precision), fmtPrec(def.Height, precision),
		fmtPrec(refX, precision), fmtPrec(refY, precision),
		fmtPrec(w, precision), fmtPrec(h, precision),
		orient,
		def.Path, fillAttr, strokeAttr)
}

// renderShapeText renders text content from a shape as an SVG <text> element.
func renderShapeText(ss renderableShape, parent *Shape, scaleX, scaleY float64, o *SVGOptions) string {
	s := ss.shape
	text := s.Text()
	if text == "" {
		return ""
	}

	// Compute effective style for text properties.
	es := s.ComputeEffectiveStyle()

	// Escape text for XML.
	text = escapeXML(text)

	parentH := parent.Height()
	// Use absolute value for negative heights (common in connectors)
	if parentH < 0 {
		parentH = -parentH
	}
	if parentH == 0 {
		parentH = 1
	}

	// Get text block position - defaults to center of shape.
	txtPinX := toFloat(s.CellValue("TxtPinX"))
	txtPinY := toFloat(s.CellValue("TxtPinY"))
	if txtPinX == 0 {
		txtPinX = ss.localW / 2
	}
	if txtPinY == 0 {
		txtPinY = ss.localH / 2
	}

	// Get text block dimensions (used for layout calculations).
	_ = toFloat(s.CellValue("TxtWidth"))  // reserved for future multi-line layout
	_ = toFloat(s.CellValue("TxtHeight")) // reserved for future multi-line layout

	// Get text angle (rotation in radians).
	txtAngle := toFloat(s.CellValue("TxtAngle"))

	// Get vertical alignment: 0=top, 1=middle, 2=bottom.
	// Default is 1 (middle) when not specified.
	vertAlignStr := s.CellValue("VerticalAlign")
	vertAlign := 1 // default to middle
	if vertAlignStr != "" {
		vertAlign = int(toFloat(vertAlignStr))
	}

	// Horizontal alignment from EffectiveStyle (already resolved).
	// 0=left, 1=center, 2=right, 3=justify.
	horzAlign := es.TextAlign
	if horzAlign == 0 && s.CellValue("Para.HorzAlign") == "" {
		horzAlign = 1 // default to center when not specified
	}

	// Get margins.
	leftMargin := toFloat(s.CellValue("LeftMargin"))
	rightMargin := toFloat(s.CellValue("RightMargin"))
	topMargin := toFloat(s.CellValue("TopMargin"))
	bottomMargin := toFloat(s.CellValue("BottomMargin"))

	// Calculate text position in shape coordinates.
	// Text block position is relative to shape's pin point.
	textX := txtPinX + ss.offsetX + leftMargin
	_ = txtPinY // pin position used as base, then overridden by alignment

	// Determine vertical position based on alignment.
	var textY float64
	switch vertAlign {
	case 0: // Top
		textY = ss.offsetY + ss.localH - topMargin
	case 2: // Bottom
		textY = ss.offsetY + bottomMargin
	default: // Middle (1)
		textY = ss.offsetY + ss.localH/2
	}

	// Convert to SVG coordinates.
	svgX, svgY := toSVGCoords(textX, textY, parentH, scaleX, scaleY)

	// SVG text-anchor based on horizontal alignment.
	textAnchor := "middle"
	switch horzAlign {
	case 0: // Left
		textAnchor = "start"
		svgX, _ = toSVGCoords(ss.offsetX+leftMargin, textY, parentH, scaleX, scaleY)
	case 2: // Right
		textAnchor = "end"
		svgX, _ = toSVGCoords(ss.offsetX+ss.localW-rightMargin, textY, parentH, scaleX, scaleY)
	}

	// SVG dominant-baseline based on vertical alignment.
	baseline := "middle"
	switch vertAlign {
	case 0:
		baseline = "hanging"
	case 2:
		baseline = "alphabetic"
	}

	// Font size from EffectiveStyle (in points, convert to SVG units).
	// Scale down slightly to better match Visio rendering.
	fontSizePx := (es.FontSize / 72.0) * ((scaleX + scaleY) / 2) * 0.9

	// Text color from EffectiveStyle.
	textColor := es.TextColor
	if textColor == "" || textColor == "#000000" {
		// Check if we need white text for contrast against a dark fill.
		if es.FillForegnd != "" && isDarkColor(es.FillForegnd) {
			textColor = "#FFFFFF"
		} else if textColor == "" {
			textColor = "#000000"
		}
	}

	// Font weight and style from EffectiveStyle.
	fontWeight := "normal"
	if es.Bold {
		fontWeight = "bold"
	}
	fontStyle := "normal"
	if es.Italic {
		fontStyle = "italic"
	}

	// Build transform for rotation.
	transform := ""
	if txtAngle != 0 {
		angleDeg := txtAngle * 180 / math.Pi
		transform = fmt.Sprintf(` transform="rotate(%s %s %s)"`,
			fmtPrec(-angleDeg, o.Precision),
			fmtPrec(svgX, o.Precision),
			fmtPrec(svgY, o.Precision))
	}

	// Calculate available text width for wrapping.
	availableWidth := (ss.localW - leftMargin - rightMargin) * ((scaleX + scaleY) / 2)
	// Approximate character width: ~0.48 of font size for average proportional font.
	// Slightly higher than typical to ensure text wraps where Visio wraps it.
	charWidth := fontSizePx * 0.48

	// Wrap text to fit within shape width.
	lines := wrapText(text, availableWidth, charWidth)
	// Remove empty trailing lines.
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) == 1 {
		return fmt.Sprintf(`  <text x="%s" y="%s" text-anchor="%s" dominant-baseline="%s" fill="%s" font-size="%s" font-weight="%s" font-style="%s"%s>%s</text>`,
			fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision),
			textAnchor, baseline, textColor,
			fmtPrec(fontSizePx, o.Precision), fontWeight, fontStyle,
			transform, text)
	}

	// Multi-line text: use tspans.
	lineHeight := fontSizePx * 1.2
	totalTextHeight := lineHeight * float64(len(lines))

	// Adjust starting Y position to center the text block vertically.
	startY := svgY
	if vertAlign == 1 { // Middle aligned
		startY = svgY - (totalTextHeight-lineHeight)/2
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf(`  <text x="%s" y="%s" text-anchor="%s" dominant-baseline="%s" fill="%s" font-size="%s" font-weight="%s" font-style="%s"%s>`,
		fmtPrec(svgX, o.Precision), fmtPrec(startY, o.Precision),
		textAnchor, baseline, textColor,
		fmtPrec(fontSizePx, o.Precision), fontWeight, fontStyle,
		transform))

	for i, line := range lines {
		dy := "0"
		if i > 0 {
			dy = fmtPrec(lineHeight, o.Precision)
		}
		result.WriteString(fmt.Sprintf(`<tspan x="%s" dy="%s">%s</tspan>`,
			fmtPrec(svgX, o.Precision), dy, escapeXML(line)))
	}
	result.WriteString("</text>")
	return result.String()
}

// wrapText splits text into lines that fit within the given width.
// charWidth is the approximate width per character in SVG units.
func wrapText(text string, maxWidth, charWidth float64) []string {
	if maxWidth <= 0 || charWidth <= 0 {
		return []string{text}
	}

	// Handle existing newlines first.
	inputLines := strings.Split(text, "\n")
	var result []string

	for _, inputLine := range inputLines {
		words := strings.Fields(inputLine)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}

		var currentLine strings.Builder
		currentWidth := 0.0
		spaceWidth := charWidth // approximate space width

		for i, word := range words {
			wordWidth := float64(len(word)) * charWidth

			if i == 0 {
				// First word on line
				currentLine.WriteString(word)
				currentWidth = wordWidth
			} else if currentWidth+spaceWidth+wordWidth <= maxWidth {
				// Word fits on current line
				currentLine.WriteString(" ")
				currentLine.WriteString(word)
				currentWidth += spaceWidth + wordWidth
			} else {
				// Word doesn't fit, start new line
				result = append(result, currentLine.String())
				currentLine.Reset()
				currentLine.WriteString(word)
				currentWidth = wordWidth
			}
		}

		if currentLine.Len() > 0 {
			result = append(result, currentLine.String())
		}
	}

	return result
}

// escapeXML escapes special XML characters in a string.
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
type renderableShape struct {
	shape          *Shape
	geom           *Geometry // specific geometry section to render
	geomIndex      int       // index of this geometry within the shape (0=first, 1=second, etc.)
	totalGeoms     int       // total number of geometries in the shape
	offsetX        float64   // X offset in parent group's coordinate space (inches)
	offsetY        float64   // Y offset in parent group's coordinate space (inches)
	localW, localH float64   // shape's own width/height
	angle          float64   // rotation angle in radians (from Angle cell)
}

// shapeHasGeometry returns true if the shape has at least one non-empty geometry section.
func shapeHasGeometry(s *Shape) bool {
	for _, g := range s.Geometries {
		if len(g.Rows) > 0 {
			return true
		}
	}
	return false
}

// collectRenderableShapes returns all leaf shapes with geometry.
// Each geometry section within a shape is returned as a separate renderable.
func collectRenderableShapes(shape *Shape) []renderableShape {
	if shape.ShapeType == "Group" {
		var result []renderableShape

		// Collect the group's own geometry (if any) - stored separately to add LAST
		// This ensures proper Z-order: children render first (back), group geometry last (front)
		// Important for shapes like Can where the top ellipse should overlay the body
		var groupGeoms []renderableShape
		geoms := shape.Geometries
		if len(geoms) == 0 {
			// Page group shape may not have direct geometry - check master
			if ms := shape.MasterShape(); ms != nil {
				geoms = ms.Geometries
			}
		}
		totalGeoms := len(geoms)
		for i, g := range geoms {
			if len(g.Rows) > 0 {
				groupGeoms = append(groupGeoms, renderableShape{
					shape:      shape, // Use page shape for styling
					geom:       g,
					geomIndex:  i,
					totalGeoms: totalGeoms,
					offsetX:    0,
					offsetY:    0,
					localW:     shape.Width(),
					localH:     shape.Height(),
				})
			}
		}

		// Collect child shapes with z-order adjustment:
		// Multi-geometry shapes (like icon backgrounds with circles) should render first,
		// then single-geometry shapes (like meridian lines) render on top.
		// This handles cases like Azure globe icons where the white circle background
		// should be behind all the meridian detail shapes.
		var multiGeomChildren []renderableShape
		var singleGeomChildren []renderableShape
		var nestedGroupChildren []renderableShape

		for _, child := range shape.ChildShapes() {
			childW := child.Width()
			childH := child.Height()
			if childW <= 0 {
				childW = shape.Width()
			}
			if childH <= 0 {
				childH = shape.Height()
			}

			// Child position relative to group's local space.
			// PinX/PinY is center position; LocPinX/LocPinY is center offset within shape.
			ox := child.X() - child.LocX()
			oy := child.Y() - child.LocY()

			if child.ShapeType == "Group" {
				// Recursive: collect from nested group (preserves their internal z-order)
				for _, nested := range collectRenderableShapes(child) {
					nested.offsetX += ox
					nested.offsetY += oy
					nestedGroupChildren = append(nestedGroupChildren, nested)
				}
			} else {
				// Determine geometry count to classify the shape
				var geoms []*Geometry
				var totalGeoms int
				usePageShape := true

				if shapeHasGeometry(child) {
					geoms = child.Geometries
					totalGeoms = len(geoms)
				} else if ms := child.MasterShape(); ms != nil && shapeHasGeometry(ms) {
					geoms = ms.Geometries
					totalGeoms = len(geoms)
					usePageShape = true // Still use page shape for styling
				}

				// Get child's rotation angle (radians)
				childAngle := toFloat(child.CellValue("Angle"))

				for i, g := range geoms {
					if len(g.Rows) == 0 {
						continue
					}
					rs := renderableShape{
						shape:      child,
						geom:       g,
						geomIndex:  i,
						totalGeoms: totalGeoms,
						offsetX:    ox,
						offsetY:    oy,
						localW:     childW,
						localH:     childH,
						angle:      childAngle,
					}
					if !usePageShape {
						rs.shape = child // Ensure page shape for styling
					}
					if totalGeoms > 1 {
						multiGeomChildren = append(multiGeomChildren, rs)
					} else {
						singleGeomChildren = append(singleGeomChildren, rs)
					}
				}
			}
		}

		// Add in z-order: multi-geom backgrounds first, then single-geom details, then nested groups
		result = append(result, multiGeomChildren...)
		result = append(result, singleGeomChildren...)
		result = append(result, nestedGroupChildren...)

		// Add group's own geometry LAST so it renders on top (front)
		result = append(result, groupGeoms...)
		return result
	}

	// Simple shape — emit one renderable per geometry section.
	var result []renderableShape

	// First try shape's own geometry
	shapeTotalGeoms := len(shape.Geometries)
	for i, g := range shape.Geometries {
		if len(g.Rows) > 0 {
			result = append(result, renderableShape{
				shape:      shape,
				geom:       g,
				geomIndex:  i,
				totalGeoms: shapeTotalGeoms,
				offsetX:    0,
				offsetY:    0,
				localW:     shape.Width(),
				localH:     shape.Height(),
			})
		}
	}

	// If no geometry, check for master shape with geometry
	if len(result) == 0 {
		if ms := shape.MasterShape(); ms != nil {
			msTotalGeoms := len(ms.Geometries)
			for i, g := range ms.Geometries {
				if len(g.Rows) > 0 {
					result = append(result, renderableShape{
						shape:      shape, // Keep page shape for styling
						geom:       g,     // Use master's geometry
						geomIndex:  i,
						totalGeoms: msTotalGeoms,
						offsetX:    0,
						offsetY:    0,
						localW:     shape.Width(),
						localH:     shape.Height(),
					})
				}
			}
		}
	}

	return result
}

// renderSubShapeWithMarkers converts a single geometry section to an SVG <path> with marker support.
func renderSubShapeWithMarkers(ss renderableShape, parent *Shape, scaleX, scaleY float64, o *SVGOptions, colorFreq map[string]int) svgRenderResult {
	return renderSubShapeInternal(ss, parent, scaleX, scaleY, o, colorFreq)
}

// renderSubShapeInternal is the core rendering function that supports markers, gradients, and shadows.
func renderSubShapeInternal(ss renderableShape, parent *Shape, scaleX, scaleY float64, o *SVGOptions, colorFreq map[string]int) svgRenderResult {
	s := ss.shape
	geom := ss.geom
	if geom == nil || len(geom.Rows) == 0 {
		return svgRenderResult{}
	}

	// Compute effective style ONCE - all rendering decisions derive from this.
	es := s.ComputeEffectiveStyle()

	// Check if this is an ellipse-only geometry (e.g., cylinder top lid).
	// For such shapes, we suppress the stroke because it would create an ugly
	// line cutting through the body when the ellipse overlays another shape.
	isEllipseOnly := false
	if len(geom.Rows) == 1 {
		for _, row := range geom.Rows {
			if strings.ToLower(row.RowType()) == "ellipse" {
				isEllipseOnly = true
			}
		}
	}

	parentW := parent.Width()
	parentH := parent.Height()
	// Track if height was originally negative (common in connectors going downward).
	// This affects Y coordinate transformation.
	negativeHeight := parentH < 0
	// Use absolute values for negative dimensions (common in connectors)
	if parentW < 0 {
		parentW = -parentW
	}
	if parentW == 0 {
		parentW = 1
	}
	if parentH < 0 {
		parentH = -parentH
	}
	if parentH == 0 {
		parentH = 1
	}

	// Sort geometry rows by IX.
	sortedRows := sortGeometryRows(geom.Rows)
	numRows := len(sortedRows)

	// Check if shape has begin/end arrows - needed for direction-establishing LineTo stubs.
	hasBeginArrow := es.BeginArrow > 0
	hasEndArrow := es.EndArrow > 0

	// Local coordinate transform that handles negative-height shapes correctly.
	// For normal shapes: Y flips from bottom-up (Visio) to top-down (SVG).
	// For negative-height shapes (connectors going down): Y coords are already negative,
	// so we just negate them instead of subtracting from parentH.
	localToSVG := func(visioX, visioY float64) (float64, float64) {
		svgX := visioX * scaleX
		var svgY float64
		if negativeHeight {
			svgY = -visioY * scaleY
		} else {
			svgY = (parentH - visioY) * scaleY
		}
		return svgX, svgY
	}

	// Build SVG path data.
	var d strings.Builder
	var prevX, prevY float64   // track current point in SVG space
	var startX, startY float64 // track path origin for direction calculation
	needStartStub := false     // flag to add direction stub after MoveTo for begin arrows

	for rowIdx, row := range sortedRows {
		isLastRow := rowIdx == numRows-1
		rt := strings.ToLower(row.RowType())
		switch rt {
		case "moveto":
			// Absolute coordinates in shape's local space (inches)
			sx, sy := row.X(), row.Y()
			svgX, svgY := localToSVG(sx+ss.offsetX, sy+ss.offsetY)
			d.WriteString(fmt.Sprintf("M%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY
			startX, startY = svgX, svgY
			needStartStub = hasBeginArrow

		case "lineto":
			sx, sy := row.X(), row.Y()
			svgX, svgY := localToSVG(sx+ss.offsetX, sy+ss.offsetY)
			// For LineTo, the start stub (if any) IS the first line segment, so we just continue
			needStartStub = false
			d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "relmoveto":
			// RelMoveTo: coordinates are 0..1 fractions of shape's own width/height
			rx, ry := row.X(), row.Y()
			absX := rx*ss.localW + ss.offsetX
			absY := ry*ss.localH + ss.offsetY
			svgX, svgY := localToSVG(absX, absY)
			d.WriteString(fmt.Sprintf("M%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			startX, startY = svgX, svgY
			prevX, prevY = svgX, svgY
			needStartStub = hasBeginArrow

		case "rellineto":
			rx, ry := row.X(), row.Y()
			absX := rx*ss.localW + ss.offsetX
			absY := ry*ss.localH + ss.offsetY
			svgX, svgY := localToSVG(absX, absY)
			needStartStub = false
			d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "arcto":
			sx, sy := row.X(), row.Y()
			svgX, svgY := localToSVG(sx+ss.offsetX, sy+ss.offsetY)
			bow := cellFloat(row, "A")

			arcSVG := arcToSVG(prevX, prevY, svgX, svgY, bow*((scaleX+scaleY)/2), o.Precision)
			d.WriteString(arcSVG)
			prevX, prevY = svgX, svgY

		case "ellipticalarcto":
			ex, ey := row.X(), row.Y()
			svgEndX, svgEndY := localToSVG(ex+ss.offsetX, ey+ss.offsetY)
			// A = control point X, B = control point Y, C = rotation angle (degrees), D = aspect ratio
			cpX := cellFloat(row, "A") + ss.offsetX
			cpY := cellFloat(row, "B") + ss.offsetY
			rotAngleDeg := cellFloat(row, "C") // rotation angle in degrees
			aspectRatio := cellFloat(row, "D") // major axis / minor axis ratio

			// Convert control point to SVG coords
			_, svgCpY := localToSVG(cpX, cpY)

			// Calculate arc parameters from start, control, and end points
			// Start point is prevX, prevY (in Visio coords before SVG transform)
			startX := (prevX/scaleX - ss.offsetX)
			startY := parentH - (prevY/scaleY) - ss.offsetY

			arcSVG := ellipticalArcToSVG(startX, startY, cpX-ss.offsetX, cpY-ss.offsetY, ex, ey,
				aspectRatio, rotAngleDeg, scaleX, scaleY, parentH, ss.offsetX, ss.offsetY, o.Precision)
			if arcSVG != "" {
				d.WriteString(arcSVG)
			} else {
				// Fallback to line if arc calculation fails
				d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgEndX, o.Precision), fmtPrec(svgEndY, o.Precision)))
			}
			_ = svgCpY // used for sweep direction
			prevX, prevY = svgEndX, svgEndY

		case "relellipticalarcto":
			rx, ry := row.X(), row.Y()
			absX := rx*ss.localW + ss.offsetX
			absY := ry*ss.localH + ss.offsetY
			svgX, svgY := localToSVG(absX, absY)
			// A = control point (relative), B = control point Y, C = rotation angle (degrees), D = aspect ratio
			cpRX := cellFloat(row, "A")
			cpRY := cellFloat(row, "B")
			rotAngleDeg := cellFloat(row, "C")
			aspectRatio := cellFloat(row, "D")
			cpX := cpRX*ss.localW + ss.offsetX
			cpY := cpRY*ss.localH + ss.offsetY

			startX := (prevX/scaleX - ss.offsetX)
			startY := parentH - (prevY/scaleY) - ss.offsetY

			arcSVG := ellipticalArcToSVG(startX, startY, cpX-ss.offsetX, cpY-ss.offsetY, absX-ss.offsetX, absY-ss.offsetY,
				aspectRatio, rotAngleDeg, scaleX, scaleY, parentH, ss.offsetX, ss.offsetY, o.Precision)
			if arcSVG != "" {
				d.WriteString(arcSVG)
			} else {
				d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			}
			prevX, prevY = svgX, svgY

		case "relcubbezto":
			// RelCubBezTo: cubic bezier with relative coords (0-1 fractions)
			rx, ry := row.X(), row.Y()
			absX := rx*ss.localW + ss.offsetX
			absY := ry*ss.localH + ss.offsetY
			svgX, svgY := localToSVG(absX, absY)
			// A, B = first control point; C, D = second control point
			cp1X := cellFloat(row, "A")*ss.localW + ss.offsetX
			cp1Y := cellFloat(row, "B")*ss.localH + ss.offsetY
			cp2X := cellFloat(row, "C")*ss.localW + ss.offsetX
			cp2Y := cellFloat(row, "D")*ss.localH + ss.offsetY
			cp1SvgX, cp1SvgY := localToSVG(cp1X, cp1Y)
			cp2SvgX, cp2SvgY := localToSVG(cp2X, cp2Y)
			// Add direction-establishing LineTo stub at start if needed for begin arrow.
			if needStartStub {
				dx := cp1SvgX - prevX
				dy := cp1SvgY - prevY
				stubLen := math.Sqrt(dx*dx + dy*dy)
				if stubLen > 0.01 {
					ext := stubLen * 0.01
					if ext < 0.1 {
						ext = 0.1
					}
					stubX := prevX + (dx/stubLen)*ext
					stubY := prevY + (dy/stubLen)*ext
					d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(stubX, o.Precision), fmtPrec(stubY, o.Precision)))
					prevX, prevY = stubX, stubY
				}
				needStartStub = false
			}
			d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
				fmtPrec(cp1SvgX, o.Precision), fmtPrec(cp1SvgY, o.Precision),
				fmtPrec(cp2SvgX, o.Precision), fmtPrec(cp2SvgY, o.Precision),
				fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "relquadbezto":
			// RelQuadBezTo: quadratic bezier with relative coords (0-1 fractions)
			rx, ry := row.X(), row.Y()
			absX := rx*ss.localW + ss.offsetX
			absY := ry*ss.localH + ss.offsetY
			svgX, svgY := localToSVG(absX, absY)
			// A, B = control point
			cpX := cellFloat(row, "A")*ss.localW + ss.offsetX
			cpY := cellFloat(row, "B")*ss.localH + ss.offsetY
			cpSvgX, cpSvgY := localToSVG(cpX, cpY)
			d.WriteString(fmt.Sprintf("Q%s %s %s %s",
				fmtPrec(cpSvgX, o.Precision), fmtPrec(cpSvgY, o.Precision),
				fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "polylineto":
			// PolylineTo: A cell contains POLYLINE formula with vertex list
			sx, sy := row.X(), row.Y()
			svgX, svgY := localToSVG(sx+ss.offsetX, sy+ss.offsetY)
			// Parse A cell for intermediate points and draw lines
			aFormula := cellString(row, "A")
			pts := parsePolylinePoints(aFormula, ss.localW, ss.localH, ss.offsetX, ss.offsetY)
			for _, pt := range pts {
				ptSvgX, ptSvgY := localToSVG(pt.x, pt.y)
				d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(ptSvgX, o.Precision), fmtPrec(ptSvgY, o.Precision)))
				prevX, prevY = ptSvgX, ptSvgY
			}
			// Final point
			d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "infiniteline":
			// InfiniteLine: two points defining an infinite line; render as line segment
			sx, sy := row.X(), row.Y()
			ax := cellFloat(row, "A")
			ay := cellFloat(row, "B")
			svgX1, svgY1 := localToSVG(sx+ss.offsetX, sy+ss.offsetY)
			svgX2, svgY2 := localToSVG(ax+ss.offsetX, ay+ss.offsetY)
			d.WriteString(fmt.Sprintf("M%s %s L%s %s",
				fmtPrec(svgX1, o.Precision), fmtPrec(svgY1, o.Precision),
				fmtPrec(svgX2, o.Precision), fmtPrec(svgY2, o.Precision)))
			prevX, prevY = svgX2, svgY2

		case "ellipse":
			// Ellipse: center (X,Y), control points (A,B) on major axis, (C,D) on minor axis.
			// SVG ellipse uses center (cx,cy) and radii (rx, ry).
			cx, cy := row.X(), row.Y()
			svgCx, svgCy := localToSVG(cx+ss.offsetX, cy+ss.offsetY)
			// (A,B) is a point on the ellipse defining the end of the first semi-axis.
			ax := cellFloat(row, "A")
			ay := cellFloat(row, "B")
			// (C,D) is a point on the ellipse defining the end of the second semi-axis.
			cx2 := cellFloat(row, "C")
			cy2 := cellFloat(row, "D")
			// Calculate radii from control points.
			rx := math.Sqrt(math.Pow(ax-cx, 2)+math.Pow(ay-cy, 2)) * ((scaleX + scaleY) / 2)
			ry := math.Sqrt(math.Pow(cx2-cx, 2)+math.Pow(cy2-cy, 2)) * ((scaleX + scaleY) / 2)
			if rx < 0.001 {
				rx = 0.001
			}
			if ry < 0.001 {
				ry = 0.001
			}
			// Draw ellipse as two arcs (SVG has no ellipse path command in d attribute).
			// M cx-rx,cy A rx,ry 0 1,0 cx+rx,cy A rx,ry 0 1,0 cx-rx,cy Z
			d.WriteString(fmt.Sprintf("M%s %s A%s %s 0 1 0 %s %s A%s %s 0 1 0 %s %s Z",
				fmtPrec(svgCx-rx, o.Precision), fmtPrec(svgCy, o.Precision),
				fmtPrec(rx, o.Precision), fmtPrec(ry, o.Precision),
				fmtPrec(svgCx+rx, o.Precision), fmtPrec(svgCy, o.Precision),
				fmtPrec(rx, o.Precision), fmtPrec(ry, o.Precision),
				fmtPrec(svgCx-rx, o.Precision), fmtPrec(svgCy, o.Precision)))
			prevX, prevY = svgCx-rx, svgCy

		case "nurbsto":
			sx, sy := row.X(), row.Y()
			svgX, svgY := localToSVG(sx+ss.offsetX, sy+ss.offsetY)

			// Parse NURBS formula from E cell for control points.
			eFormula := cellString(row, "E")
			nurbsInfo := parseNURBSData(eFormula)

			// Add direction-establishing LineTo stub at start if needed for begin arrow.
			if needStartStub && nurbsInfo != nil && len(nurbsInfo.cps) > 0 {
				// Stub goes from MoveTo point toward first control point.
				var cp1AbsX, cp1AbsY float64
				if nurbsInfo.xType == 0 {
					cp1AbsX = nurbsInfo.cps[0].x*ss.localW + ss.offsetX
				} else {
					cp1AbsX = nurbsInfo.cps[0].x + ss.offsetX
				}
				if nurbsInfo.yType == 0 {
					cp1AbsY = nurbsInfo.cps[0].y*ss.localH + ss.offsetY
				} else {
					cp1AbsY = nurbsInfo.cps[0].y + ss.offsetY
				}
				cp1SvgX, cp1SvgY := localToSVG(cp1AbsX, cp1AbsY)
				dx := cp1SvgX - prevX
				dy := cp1SvgY - prevY
				stubLen := math.Sqrt(dx*dx + dy*dy)
				if stubLen > 0.01 {
					// Add a small stub (1% of distance to first CP, min 0.1 SVG units)
					ext := stubLen * 0.01
					if ext < 0.1 {
						ext = 0.1
					}
					stubX := prevX + (dx/stubLen)*ext
					stubY := prevY + (dy/stubLen)*ext
					d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(stubX, o.Precision), fmtPrec(stubY, o.Precision)))
					prevX, prevY = stubX, stubY
				}
				needStartStub = false
			}

			if nurbsInfo != nil && len(nurbsInfo.cps) == 2 {
				// Degree 3, 2 interior control points → cubic Bezier.
				// xType/yType: 0 = proportional (0-1), 1 = absolute (inches)
				var cp1AbsX, cp1AbsY, cp2AbsX, cp2AbsY float64
				if nurbsInfo.xType == 0 {
					cp1AbsX = nurbsInfo.cps[0].x*ss.localW + ss.offsetX
					cp2AbsX = nurbsInfo.cps[1].x*ss.localW + ss.offsetX
				} else {
					cp1AbsX = nurbsInfo.cps[0].x + ss.offsetX
					cp2AbsX = nurbsInfo.cps[1].x + ss.offsetX
				}
				if nurbsInfo.yType == 0 {
					cp1AbsY = nurbsInfo.cps[0].y*ss.localH + ss.offsetY
					cp2AbsY = nurbsInfo.cps[1].y*ss.localH + ss.offsetY
				} else {
					cp1AbsY = nurbsInfo.cps[0].y + ss.offsetY
					cp2AbsY = nurbsInfo.cps[1].y + ss.offsetY
				}
				cp1SvgX, cp1SvgY := localToSVG(cp1AbsX, cp1AbsY)
				cp2SvgX, cp2SvgY := localToSVG(cp2AbsX, cp2AbsY)
				d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
					fmtPrec(cp1SvgX, o.Precision), fmtPrec(cp1SvgY, o.Precision),
					fmtPrec(cp2SvgX, o.Precision), fmtPrec(cp2SvgY, o.Precision),
					fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			} else if nurbsInfo != nil && len(nurbsInfo.cps) >= 3 {
				// 3+ control points: create multiple cubic Bezier segments for S-curves.
				// Convert all control points to absolute coordinates first.
				absPoints := make([][2]float64, len(nurbsInfo.cps))
				for i, cp := range nurbsInfo.cps {
					if nurbsInfo.xType == 0 {
						absPoints[i][0] = cp.x*ss.localW + ss.offsetX
					} else {
						absPoints[i][0] = cp.x + ss.offsetX
					}
					if nurbsInfo.yType == 0 {
						absPoints[i][1] = cp.y*ss.localH + ss.offsetY
					} else {
						absPoints[i][1] = cp.y + ss.offsetY
					}
				}

				// Check if control points form an L-shape (degenerate case).
				// This happens when dynamic connectors store simplified geometry.
				// Detect by checking if all X coords are equal OR all Y coords are equal
				// for contiguous subsets of points.
				isLShaped := false
				if len(absPoints) == 3 {
					// Check for L-shape: first two points same X, last two points same Y (or vice versa)
					eps := 0.001
					sameX01 := math.Abs(absPoints[0][0]-absPoints[1][0]) < eps
					sameY12 := math.Abs(absPoints[1][1]-absPoints[2][1]) < eps
					sameY01 := math.Abs(absPoints[0][1]-absPoints[1][1]) < eps
					sameX12 := math.Abs(absPoints[1][0]-absPoints[2][0]) < eps
					isLShaped = (sameX01 && sameY12) || (sameY01 && sameX12)
				}

				if isLShaped {
					// Generate smooth S-curve for L-shaped control points.
					// L-shaped connectors have control points forming a right angle.
					// Like Visio, we use TWO cubic bezier segments for a smooth S-curve.
					startX, startY := ss.offsetX, ss.offsetY // MoveTo origin
					endX, endY := (sx + ss.offsetX), (sy + ss.offsetY)

					// Determine the L-shape type based on control points:
					// Type 1: Down then right (sameX01 && sameY12)
					// Type 2: Right then down (sameY01 && sameX12)
					eps := 0.001
					sameX01 := math.Abs(absPoints[0][0]-absPoints[1][0]) < eps

					// Calculate the corner point (where the L bends)
					cornerX, cornerY := absPoints[1][0], absPoints[1][1]

					// Midpoint for the S-curve transition
					midX := (startX + endX) / 2
					midY := (startY + endY) / 2

					// For S-curve, we use the corner position to determine curve direction
					var cp1X, cp1Y, cp2X, cp2Y, cp3X, cp3Y, cp4X, cp4Y float64
					if sameX01 {
						// L goes vertically first (down), then horizontally (right)
						// First bezier: start → mid, curving toward corner
						cp1X = startX
						cp1Y = startY + (cornerY-startY)*0.5
						cp2X = startX + (midX-startX)*0.2
						cp2Y = midY
						// Second bezier: mid → end, curving away from corner
						cp3X = endX - (endX-midX)*0.2
						cp3Y = midY
						cp4X = endX
						cp4Y = endY - (endY-cornerY)*0.5
					} else {
						// L goes horizontally first (right), then vertically (down)
						// First bezier: start → mid, curving toward corner
						cp1X = startX + (cornerX-startX)*0.5
						cp1Y = startY
						cp2X = midX
						cp2Y = startY + (midY-startY)*0.2
						// Second bezier: mid → end, curving away from corner
						cp3X = midX
						cp3Y = endY - (endY-midY)*0.2
						cp4X = endX - (endX-cornerX)*0.5
						cp4Y = endY
					}

					// First bezier: start to midpoint
					cp1SvgX, cp1SvgY := localToSVG(cp1X, cp1Y)
					cp2SvgX, cp2SvgY := localToSVG(cp2X, cp2Y)
					midSvgX, midSvgY := localToSVG(midX, midY)
					d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
						fmtPrec(cp1SvgX, o.Precision), fmtPrec(cp1SvgY, o.Precision),
						fmtPrec(cp2SvgX, o.Precision), fmtPrec(cp2SvgY, o.Precision),
						fmtPrec(midSvgX, o.Precision), fmtPrec(midSvgY, o.Precision)))

					// Second bezier: midpoint to end
					cp3SvgX, cp3SvgY := localToSVG(cp3X, cp3Y)
					cp4SvgX, cp4SvgY := localToSVG(cp4X, cp4Y)
					d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
						fmtPrec(cp3SvgX, o.Precision), fmtPrec(cp3SvgY, o.Precision),
						fmtPrec(cp4SvgX, o.Precision), fmtPrec(cp4SvgY, o.Precision),
						fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
				} else if len(absPoints) == 4 {
					// For 4 control points (common S-curve): use two cubic Beziers
					// First half: start to midpoint using cp0, cp1
					// Second half: midpoint to end using cp2, cp3
					// Calculate midpoint between cp1 and cp2
					midX := (absPoints[1][0] + absPoints[2][0]) / 2
					midY := (absPoints[1][1] + absPoints[2][1]) / 2

					// First cubic: prev → cp0 → cp1 → mid
					cp0SvgX, cp0SvgY := localToSVG(absPoints[0][0], absPoints[0][1])
					cp1SvgX, cp1SvgY := localToSVG(absPoints[1][0], absPoints[1][1])
					midSvgX, midSvgY := localToSVG(midX, midY)
					d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
						fmtPrec(cp0SvgX, o.Precision), fmtPrec(cp0SvgY, o.Precision),
						fmtPrec(cp1SvgX, o.Precision), fmtPrec(cp1SvgY, o.Precision),
						fmtPrec(midSvgX, o.Precision), fmtPrec(midSvgY, o.Precision)))

					// Second cubic: mid → cp2 → cp3 → end
					cp2SvgX, cp2SvgY := localToSVG(absPoints[2][0], absPoints[2][1])
					cp3SvgX, cp3SvgY := localToSVG(absPoints[3][0], absPoints[3][1])
					d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
						fmtPrec(cp2SvgX, o.Precision), fmtPrec(cp2SvgY, o.Precision),
						fmtPrec(cp3SvgX, o.Precision), fmtPrec(cp3SvgY, o.Precision),
						fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
				} else if len(absPoints) == 3 {
					// 3 control points (non-L-shaped): split into two cubic Beziers at cp1
					// First cubic: prev → cp0 → mid(cp0,cp1) → cp1
					cp0SvgX, cp0SvgY := localToSVG(absPoints[0][0], absPoints[0][1])
					mid01X := (absPoints[0][0] + absPoints[1][0]) / 2
					mid01Y := (absPoints[0][1] + absPoints[1][1]) / 2
					mid01SvgX, mid01SvgY := localToSVG(mid01X, mid01Y)
					cp1SvgX, cp1SvgY := localToSVG(absPoints[1][0], absPoints[1][1])
					d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
						fmtPrec(cp0SvgX, o.Precision), fmtPrec(cp0SvgY, o.Precision),
						fmtPrec(mid01SvgX, o.Precision), fmtPrec(mid01SvgY, o.Precision),
						fmtPrec(cp1SvgX, o.Precision), fmtPrec(cp1SvgY, o.Precision)))

					// Second cubic: cp1 → mid(cp1,cp2) → cp2 → end
					mid12X := (absPoints[1][0] + absPoints[2][0]) / 2
					mid12Y := (absPoints[1][1] + absPoints[2][1]) / 2
					mid12SvgX, mid12SvgY := localToSVG(mid12X, mid12Y)
					cp2SvgX, cp2SvgY := localToSVG(absPoints[2][0], absPoints[2][1])
					d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
						fmtPrec(mid12SvgX, o.Precision), fmtPrec(mid12SvgY, o.Precision),
						fmtPrec(cp2SvgX, o.Precision), fmtPrec(cp2SvgY, o.Precision),
						fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
				} else {
					// For 5+ points: use first and last as control points (simplified)
					cp1SvgX, cp1SvgY := localToSVG(absPoints[0][0], absPoints[0][1])
					lastIdx := len(absPoints) - 1
					cp2SvgX, cp2SvgY := localToSVG(absPoints[lastIdx][0], absPoints[lastIdx][1])
					d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
						fmtPrec(cp1SvgX, o.Precision), fmtPrec(cp1SvgY, o.Precision),
						fmtPrec(cp2SvgX, o.Precision), fmtPrec(cp2SvgY, o.Precision),
						fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
				}
			} else if nurbsInfo != nil && len(nurbsInfo.cps) == 1 {
				// Degree 2, 1 control point → quadratic Bezier.
				var cpAbsX, cpAbsY float64
				if nurbsInfo.xType == 0 {
					cpAbsX = nurbsInfo.cps[0].x*ss.localW + ss.offsetX
				} else {
					cpAbsX = nurbsInfo.cps[0].x + ss.offsetX
				}
				if nurbsInfo.yType == 0 {
					cpAbsY = nurbsInfo.cps[0].y*ss.localH + ss.offsetY
				} else {
					cpAbsY = nurbsInfo.cps[0].y + ss.offsetY
				}
				cpSvgX, cpSvgY := localToSVG(cpAbsX, cpAbsY)
				d.WriteString(fmt.Sprintf("Q%s %s %s %s",
					fmtPrec(cpSvgX, o.Precision), fmtPrec(cpSvgY, o.Precision),
					fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			} else {
				// Fallback: straight line to endpoint.
				d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			}

			// For connectors with end arrows, add a small direction-establishing LineTo
			// at the end of the path. This ensures SVG markers point in the overall
			// path direction, not the curve tangent direction (which can be
			// perpendicular when the last control point is at the same X or Y as
			// the endpoint).
			if hasEndArrow && isLastRow {
				dx := svgX - startX
				dy := svgY - startY
				pathLen := math.Sqrt(dx*dx + dy*dy)
				if pathLen > 0.01 {
					// Add a small extension (1% of path length, min 0.1 SVG units)
					ext := pathLen * 0.01
					if ext < 0.1 {
						ext = 0.1
					}
					extX := svgX + (dx/pathLen)*ext
					extY := svgY + (dy/pathLen)*ext
					d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(extX, o.Precision), fmtPrec(extY, o.Precision)))
					prevX, prevY = extX, extY
				} else {
					prevX, prevY = svgX, svgY
				}
			} else {
				prevX, prevY = svgX, svgY
			}

		default:
			// Unknown row type: skip
		}
	}

	pathData := d.String()
	if pathData == "" {
		return svgRenderResult{}
	}

	// Check if path should be closed.
	noFill := geomCellBool(geom, "NoFill")
	noLine := geomCellBool(geom, "NoLine")
	noShow := geomCellBool(geom, "NoShow")
	if noShow {
		return svgRenderResult{}
	}

	// Build style attributes using EffectiveStyle - all resolution is done.
	fill := es.EffectiveFillColor()
	stroke := es.EffectiveLineColor()
	linePatternInt := es.LinePattern

	var styleAttrs []string
	var gradientID string

	// Check for gradient fill (requires generating SVG defs).
	gradient := s.FillGradient()
	if gradient != nil && !noFill && es.FillPattern != 0 {
		gradientID = fmt.Sprintf("grad_%s", s.ID)
		fill = fmt.Sprintf("url(#%s)", gradientID)
	} else {
		// Override fill for geometry visibility flags.
		if noFill {
			fill = "none"
		}

		// For multi-geometry shapes, secondary geometries may need color inversion
		// to contrast with the background (e.g., Azure Web Role globe circle).
		if ss.totalGeoms > 1 && ss.geomIndex > 0 && fill != "none" && fill != "" && isDarkColor(fill) {
			fill = "#FFFFFF"
			if stroke == "" || stroke == "none" || !isDarkColor(stroke) {
				stroke = es.FillForegnd
			}
		}
	}

	// Override stroke for geometry visibility flags.
	if noLine {
		stroke = "none"
	} else if isEllipseOnly && fill != "none" && fill != "" {
		// For ellipse-only geometries (e.g., cylinder top lid), use a white stroke
		// to create the 3D highlight effect that Visio renders around the ellipse.
		stroke = "#FFFFFF"
	}

	// Track colors for brand detection.
	if fill != "none" && fill != "" {
		colorFreq[strings.ToUpper(fill)]++
	}
	if stroke != "none" && stroke != "" {
		colorFreq[strings.ToUpper(stroke)]++
	}

	// Stroke width from EffectiveStyle (already in points).
	strokeWidth := es.LineWeight
	if strokeWidth <= 0 && stroke != "none" {
		strokeWidth = 2.25 // Default Visio connector stroke weight
	}
	// For ellipse-only shapes (cylinder caps), ensure the white highlight stroke is visible.
	if isEllipseOnly && stroke == "#FFFFFF" {
		minHighlightWidth := 2.0
		if strokeWidth < minHighlightWidth {
			strokeWidth = minHighlightWidth
		}
	}

	// Fill opacity from EffectiveStyle.
	if es.FillForegndTrans > 0 && es.FillForegndTrans < 1.0 {
		styleAttrs = append(styleAttrs, fmt.Sprintf(`fill-opacity="%s"`, fmtPrec(es.EffectiveFillOpacity(), o.Precision)))
	}

	// Line pattern (dash array).
	if stroke != "none" && linePatternInt > 1 {
		dashArray := linePatternToSVG(linePatternInt, strokeWidth)
		if dashArray != "" {
			styleAttrs = append(styleAttrs, fmt.Sprintf(`stroke-dasharray="%s"`, dashArray))
		}
	}

	// Arrow markers from EffectiveStyle (already resolved through inheritance chain).
	var markers []markerRef
	beginArrow := es.BeginArrow
	endArrow := es.EndArrow
	beginArrowSize := es.BeginArrowSize
	endArrowSize := es.EndArrowSize

	if stroke != "none" && beginArrow > 0 {
		key := arrowMarkerKey(beginArrow, beginArrowSize, stroke, false)
		markers = append(markers, markerRef{
			key:       key,
			arrowType: beginArrow,
			size:      beginArrowSize,
			color:     stroke,
			isEnd:     false,
		})
		styleAttrs = append(styleAttrs, fmt.Sprintf(`marker-start="url(#%s)"`, key))
	}
	if stroke != "none" && endArrow > 0 {
		key := arrowMarkerKey(endArrow, endArrowSize, stroke, true)
		markers = append(markers, markerRef{
			key:       key,
			arrowType: endArrow,
			size:      endArrowSize,
			color:     stroke,
			isEnd:     true,
		})
		styleAttrs = append(styleAttrs, fmt.Sprintf(`marker-end="url(#%s)"`, key))
	}

	// Shadow support using EffectiveStyle.
	var shadowID string
	var shadow *Shadow
	if es.HasShadow() {
		shadow = s.ShapeShadow()
		if shadow != nil {
			shadowID = fmt.Sprintf("shadow_%s", s.ID)
			styleAttrs = append(styleAttrs, fmt.Sprintf(`filter="url(#%s)"`, shadowID))
		}
	}

	// Build the extra attributes string.
	extraAttrs := ""
	if len(styleAttrs) > 0 {
		extraAttrs = " " + strings.Join(styleAttrs, " ")
	}

	// Shorten path for arrow markers using pre-computed setbacks from EffectiveStyle.
	if beginArrow > 0 && pathData != "" && es.BeginArrowSetback > 0 {
		pathData = shortenPathStart(pathData, es.BeginArrowSetback, o.Precision)
	}
	if endArrow > 0 && pathData != "" && es.EndArrowSetback > 0 {
		pathData = shortenPathEnd(pathData, es.EndArrowSetback, o.Precision)
	}

	// Build vector-effect attribute for non-scaling stroke.
	vectorEffect := ""
	if stroke != "none" && stroke != "" {
		vectorEffect = ` vector-effect="non-scaling-stroke"`
	}

	pathElem := fmt.Sprintf(`  <path d="%s" fill="%s" stroke="%s" stroke-width="%s"%s%s/>`, pathData, fill, stroke, fmtPrec(strokeWidth, o.Precision), vectorEffect, extraAttrs)

	// Apply rotation if the shape has a non-zero angle.
	// Rotation is around the shape's center. Negate the angle because Visio uses
	// counter-clockwise positive (Y-up) while SVG uses clockwise positive (Y-down).
	if ss.angle != 0 {
		angleDeg := -ss.angle * 180 / math.Pi
		// Calculate rotation center in SVG coordinates
		centerX := (ss.offsetX + ss.localW/2) * scaleX
		centerY := (parentH - ss.offsetY - ss.localH/2) * scaleY
		pathElem = fmt.Sprintf("  <g transform=\"rotate(%s %s %s)\">\n%s\n  </g>",
			fmtPrec(angleDeg, o.Precision),
			fmtPrec(centerX, o.Precision),
			fmtPrec(centerY, o.Precision),
			pathElem)
	}

	return svgRenderResult{
		pathSVG:     pathElem,
		strokeWidth: strokeWidth,
		markers:     markers,
		gradientID:  gradientID,
		gradient:    gradient,
		shadowID:    shadowID,
		shadow:      shadow,
	}
}

// toSVGCoords transforms Visio coordinates (inches, Y-up, bottom-left origin)
// to SVG coordinates (pixels, Y-down, top-left origin).
func toSVGCoords(visioX, visioY, parentHeight, scaleX, scaleY float64) (float64, float64) {
	// Flip Y: SVG Y = (parentHeight - visioY) * scale
	svgX := visioX * scaleX
	svgY := (parentHeight - visioY) * scaleY
	return svgX, svgY
}

// ellipticalArcToSVG converts a Visio EllipticalArcTo to an SVG arc command.
// Parameters are in Visio coordinates (Y-up). The function handles coordinate
// transformation to SVG (Y-down).
// aspectRatio is major axis / minor axis (from Visio D cell).
// rotAngleDeg is the rotation of the major axis in degrees (from Visio C cell).
func ellipticalArcToSVG(startX, startY, cpX, cpY, endX, endY, aspectRatio, rotAngleDeg, scaleX, scaleY, parentH, offsetX, offsetY float64, precision int) string {
	// Convert all points to SVG coordinates
	svgStartX := (startX + offsetX) * scaleX
	svgStartY := (parentH - startY - offsetY) * scaleY
	svgEndX := (endX + offsetX) * scaleX
	svgEndY := (parentH - endY - offsetY) * scaleY
	svgCpX := (cpX + offsetX) * scaleX
	svgCpY := (parentH - cpY - offsetY) * scaleY

	// Calculate chord from start to end
	dx := svgEndX - svgStartX
	dy := svgEndY - svgStartY
	chordLen := math.Sqrt(dx*dx + dy*dy)
	if chordLen < 1e-10 {
		return ""
	}

	// Calculate distance from control point to chord midpoint
	midX := (svgStartX + svgEndX) / 2
	midY := (svgStartY + svgEndY) / 2
	cpDx := svgCpX - midX
	cpDy := svgCpY - midY
	bow := math.Sqrt(cpDx*cpDx + cpDy*cpDy)

	// Determine sweep direction based on which side of the chord the control point is
	// Cross product of chord vector and (start -> control point) vector
	// In SVG, sweep=0 is counter-clockwise, sweep=1 is clockwise
	// For an arc from left to right:
	//   - sweep=1 (clockwise) curves UP (toward smaller Y)
	//   - sweep=0 (counter-clockwise) curves DOWN (toward larger Y)
	// We want the arc to curve TOWARD the control point
	cross := dx*(svgCpY-svgStartY) - dy*(svgCpX-svgStartX)
	sweep := 1
	if cross > 0 {
		// Control point is below the chord (larger Y in SVG), curve down toward it
		sweep = 0
	}

	// If control point is on the chord (bow ~= 0), draw a line
	if bow < 1e-6 {
		return fmt.Sprintf("L%s %s", fmtPrec(svgEndX, precision), fmtPrec(svgEndY, precision))
	}

	// aspectRatio = major axis / minor axis
	// For a horizontal ellipse (rotation = 0), the major axis is horizontal (rx)
	if aspectRatio <= 0 {
		aspectRatio = 1.0
	}

	// Calculate radii based on bow height and aspect ratio.
	// The bow is how far the arc curves away from the chord.
	// For an elliptical arc, ry (minor radius) relates to the bow height.
	// We use the bow as a basis for ry, scaled to produce a visible curve.
	//
	// For the arc to pass through the control point while spanning the chord:
	// - ry should be approximately equal to bow (for small arcs)
	// - rx = ry * aspectRatio
	//
	// However, rx must be at least half the chord length to span the endpoints.
	var rx, ry float64
	minRx := chordLen / 2

	// Start with ry based on bow height
	ry = bow
	rx = ry * aspectRatio

	// If rx is too small to span the chord, scale up both radii proportionally
	if rx < minRx {
		scale := minRx / rx * 1.01 // slightly larger to ensure valid arc
		rx = rx * scale
		ry = ry * scale
	}

	// Large arc flag: if bow is greater than the minor radius, we're drawing more than half the ellipse
	largeArc := 0
	if bow > ry {
		largeArc = 1
	}

	return fmt.Sprintf("A%s %s %s %d %d %s %s",
		fmtPrec(rx, precision), fmtPrec(ry, precision),
		fmtPrec(rotAngleDeg, precision), largeArc, sweep,
		fmtPrec(svgEndX, precision), fmtPrec(svgEndY, precision))
}

// arcToSVG converts a Visio ArcTo (with bow) to an SVG arc command.
// bow is the distance from the chord midpoint to the arc, scaled to SVG units.
// Positive bow = arc bows to the left of the direction of travel.
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
func cellFloat(row *GeometryRow, name string) float64 {
	if c, ok := row.Cells[name]; ok {
		return toFloat(c.Value())
	}
	return 0
}

// geomCellBool returns true if a geometry-level cell has value "1".
func geomCellBool(geom *Geometry, name string) bool {
	for _, c := range geom.Cells {
		if c.Name() == name && c.Value() == "1" {
			return true
		}
	}
	return false
}

// resolveColor normalizes a Visio color to #RRGGBB format.
// Visio stores colors as "#RRGGBB", "RGB(r,g,b)", theme index, or empty.
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
func lightenColor(c string, factor float64) string {
	if !strings.HasPrefix(c, "#") || len(c) < 7 {
		return c
	}
	r, g, b := 0, 0, 0
	fmt.Sscanf(c, "#%02X%02X%02X", &r, &g, &b)
	// Mix with white (255, 255, 255)
	r = int(float64(r) + (255-float64(r))*factor)
	g = int(float64(g) + (255-float64(g))*factor)
	b = int(float64(b) + (255-float64(b))*factor)
	return fmt.Sprintf("#%02X%02X%02X", clamp(r), clamp(g), clamp(b))
}

// isDarkColor returns true if the given color is dark (low luminance).
// Used to determine if white text should be used for contrast.
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
// Pattern values are defined in MS-VSDX spec §2.4.4.
func linePatternToSVG(pattern int, weight float64) string {
	if weight <= 0 {
		weight = 1
	}
	switch pattern {
	case 0, 1: // None, Solid
		return ""
	case 2: // Dash
		return fmt.Sprintf("%.2f %.2f", weight*4, weight*2)
	case 3: // Dot
		return fmt.Sprintf("%.2f %.2f", weight, weight*2)
	case 4: // Dash-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f", weight*4, weight*2, weight, weight*2)
	case 5: // Dash-Dot-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f",
			weight*4, weight*2, weight, weight*2, weight, weight*2)
	case 6: // Dash-Dash-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f",
			weight*4, weight*2, weight*4, weight*2, weight, weight*2)
	case 7: // Long Dash
		return fmt.Sprintf("%.2f %.2f", weight*8, weight*2)
	case 8: // Long Dash-Short Dash
		return fmt.Sprintf("%.2f %.2f %.2f %.2f", weight*8, weight*2, weight*2, weight*2)
	case 9: // Long Dash-Short Dash-Short Dash
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f",
			weight*8, weight*2, weight*2, weight*2, weight*2, weight*2)
	case 10: // Sparse Dot
		return fmt.Sprintf("%.2f %.2f", weight, weight*4)
	case 11: // Dense Dot
		return fmt.Sprintf("%.2f %.2f", weight, weight)
	case 12: // Dash-Sparse Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f", weight*4, weight*4, weight, weight*4)
	case 13: // Dash-Dash-Sparse Dot-Sparse Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f %.2f %.2f",
			weight*4, weight*2, weight*4, weight*4, weight, weight*4, weight, weight*4)
	case 14: // Long Dash-Dash
		return fmt.Sprintf("%.2f %.2f %.2f %.2f", weight*8, weight*2, weight*4, weight*2)
	case 15: // Long Dash-Dash-Dash
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f",
			weight*8, weight*2, weight*4, weight*2, weight*4, weight*2)
	case 16: // Dash-Dash-Dot-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f %.2f %.2f",
			weight*4, weight*2, weight*4, weight*2, weight, weight*2, weight, weight*2)
	case 17: // Dash-Dot-Dot-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f %.2f %.2f",
			weight*4, weight*2, weight, weight*2, weight, weight*2, weight, weight*2)
	case 18: // Dash-Dash-Dash-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f %.2f %.2f",
			weight*4, weight*2, weight*4, weight*2, weight*4, weight*2, weight, weight*2)
	case 19: // Dash-Dash-Dash-Dot-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f %.2f %.2f %.2f %.2f",
			weight*4, weight*2, weight*4, weight*2, weight*4, weight*2, weight, weight*2, weight, weight*2)
	case 20: // Long Dot
		return fmt.Sprintf("%.2f %.2f", weight*2, weight*2)
	case 21: // Long Dash-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f", weight*8, weight*2, weight, weight*2)
	case 22: // Long Dash-Dot-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f",
			weight*8, weight*2, weight, weight*2, weight, weight*2)
	case 23: // Long Dash-Dot-Dot-Dot
		return fmt.Sprintf("%.2f %.2f %.2f %.2f %.2f %.2f %.2f %.2f",
			weight*8, weight*2, weight, weight*2, weight, weight*2, weight, weight*2)
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
	x, y float64
}

// nurbsData holds parsed NURBS formula data including coordinate type flags.
type nurbsData struct {
	knotLast float64             // parameter domain end (typically != 1.0)
	degree   int                 // B-spline degree (typically 3 for cubic)
	xType    int                 // 0 = proportional (0-1), 1 = absolute (inches)
	yType    int                 // 0 = proportional (0-1), 1 = absolute (inches)
	cps      []nurbsControlPoint // interior control points
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
		cps = append(cps, nurbsControlPoint{x: x, y: y})
	}
	return &nurbsData{knotLast: knotLast, degree: degree, xType: xType, yType: yType, cps: cps}
}

// parseNURBSControlPoints extracts interior control points from a NURBS() formula string.
// Format: NURBS(knotLast, degree, xType, yType, x1,y1,knot1,weight1, x2,y2,knot2,weight2, ...)
// Returns the (x,y) pairs of interior control points.
// Note: This ignores xType/yType. Use parseNURBSData for full coordinate handling.
func parseNURBSControlPoints(formula string) []nurbsControlPoint {
	data := parseNURBSData(formula)
	if data == nil {
		return nil
	}
	return data.cps
}

// polylinePoint represents a vertex in a polyline.
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
func cellString(row *GeometryRow, name string) string {
	if c, ok := row.Cells[name]; ok {
		return c.Value()
	}
	// Also check the F (formula) attribute — some cells store NURBS in F, not V.
	if c, ok := row.Cells[name]; ok {
		if f := c.Formula(); f != "" {
			return f
		}
	}
	return ""
}

// fmtPrec formats a float with the given number of decimal places,
// stripping trailing zeros for cleaner SVG output.
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
func shortenPathEnd(pathData string, shortenBy float64, precision int) string {
	if shortenBy <= 0 {
		return pathData
	}

	// Find the last command and its coordinates
	// We handle the common cases: L (lineto) and C (cubic bezier)
	pathData = strings.TrimSpace(pathData)
	if pathData == "" {
		return pathData
	}

	// Find the last command by scanning backwards
	lastCmdIdx := -1
	for i := len(pathData) - 1; i >= 0; i-- {
		c := pathData[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			lastCmdIdx = i
			break
		}
	}
	if lastCmdIdx < 0 {
		return pathData
	}

	cmd := pathData[lastCmdIdx]
	args := strings.TrimSpace(pathData[lastCmdIdx+1:])
	prefix := pathData[:lastCmdIdx]

	// Parse coordinates from the last command
	parts := strings.Fields(args)

	switch cmd {
	case 'L': // LineTo: L x y
		if len(parts) < 2 {
			return pathData
		}
		endX, _ := strconv.ParseFloat(parts[len(parts)-2], 64)
		endY, _ := strconv.ParseFloat(parts[len(parts)-1], 64)

		// Find previous point by looking for the second-to-last coordinate pair
		prevX, prevY := findPrevPoint(prefix)

		// Calculate direction and shorten
		dx := endX - prevX
		dy := endY - prevY
		length := math.Sqrt(dx*dx + dy*dy)
		if length > shortenBy {
			ratio := (length - shortenBy) / length
			newX := prevX + dx*ratio
			newY := prevY + dy*ratio
			return prefix + fmt.Sprintf("L%s %s", fmtPrec(newX, precision), fmtPrec(newY, precision))
		}

	case 'C': // Cubic Bezier: C cp1x cp1y cp2x cp2y x y
		if len(parts) < 6 {
			return pathData
		}
		cp2X, _ := strconv.ParseFloat(parts[len(parts)-4], 64)
		cp2Y, _ := strconv.ParseFloat(parts[len(parts)-3], 64)
		endX, _ := strconv.ParseFloat(parts[len(parts)-2], 64)
		endY, _ := strconv.ParseFloat(parts[len(parts)-1], 64)

		// Direction at end is from cp2 to end
		dx := endX - cp2X
		dy := endY - cp2Y
		length := math.Sqrt(dx*dx + dy*dy)
		// Only shorten if the tangent segment is longer than what we need to shorten.
		// Otherwise the shortening would be disproportionate or move the point backwards.
		if length > shortenBy {
			// Shorten along the tangent direction
			ratio := shortenBy / length
			newEndX := endX - dx*ratio
			newEndY := endY - dy*ratio
			// Also adjust cp2 slightly to maintain curve shape
			newCp2X := cp2X - dx*ratio*0.3
			newCp2Y := cp2Y - dy*ratio*0.3

			// Rebuild the C command with adjusted coordinates
			newArgs := fmt.Sprintf("%s %s %s %s %s %s",
				parts[0], parts[1], // cp1 unchanged
				fmtPrec(newCp2X, precision), fmtPrec(newCp2Y, precision),
				fmtPrec(newEndX, precision), fmtPrec(newEndY, precision))
			return prefix + "C" + newArgs
		}
	}

	return pathData
}

// shortenPathStart shortens an SVG path at the beginning by the given length.
// This is used to prevent connector lines from extending through start arrowheads.
func shortenPathStart(pathData string, shortenBy float64, precision int) string {
	if shortenBy <= 0 {
		return pathData
	}

	pathData = strings.TrimSpace(pathData)
	if pathData == "" || len(pathData) < 2 {
		return pathData
	}

	// Find the M (moveto) command which starts the path
	if pathData[0] != 'M' && pathData[0] != 'm' {
		return pathData
	}

	// Find where the M command's arguments end (at the next command)
	nextCmdIdx := -1
	for i := 1; i < len(pathData); i++ {
		c := pathData[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			nextCmdIdx = i
			break
		}
	}
	if nextCmdIdx < 0 {
		return pathData
	}

	// Parse M command coordinates
	mArgs := strings.TrimSpace(pathData[1:nextCmdIdx])
	mParts := strings.Fields(mArgs)
	if len(mParts) < 2 {
		return pathData
	}
	startX, _ := strconv.ParseFloat(mParts[0], 64)
	startY, _ := strconv.ParseFloat(mParts[1], 64)

	suffix := pathData[nextCmdIdx:]
	nextCmd := suffix[0]

	switch nextCmd {
	case 'L': // LineTo: find the endpoint and shorten towards it
		args := strings.TrimSpace(suffix[1:])
		// Find next command
		endIdx := len(args)
		for i := 0; i < len(args); i++ {
			c := args[i]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				endIdx = i
				break
			}
		}
		lParts := strings.Fields(args[:endIdx])
		if len(lParts) >= 2 {
			endX, _ := strconv.ParseFloat(lParts[0], 64)
			endY, _ := strconv.ParseFloat(lParts[1], 64)

			dx := endX - startX
			dy := endY - startY
			length := math.Sqrt(dx*dx + dy*dy)
			if length > shortenBy {
				ratio := shortenBy / length
				newStartX := startX + dx*ratio
				newStartY := startY + dy*ratio
				return fmt.Sprintf("M%s %s%s", fmtPrec(newStartX, precision), fmtPrec(newStartY, precision), suffix)
			}
		}

	case 'C': // Cubic Bezier: C cp1x cp1y cp2x cp2y x y
		args := strings.TrimSpace(suffix[1:])
		endIdx := len(args)
		for i := 0; i < len(args); i++ {
			c := args[i]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				endIdx = i
				break
			}
		}
		cParts := strings.Fields(args[:endIdx])
		if len(cParts) >= 6 {
			cp1X, _ := strconv.ParseFloat(cParts[0], 64)
			cp1Y, _ := strconv.ParseFloat(cParts[1], 64)

			// Direction at start is from start to cp1
			dx := cp1X - startX
			dy := cp1Y - startY
			length := math.Sqrt(dx*dx + dy*dy)
			if length > 0.001 {
				ratio := shortenBy / length
				newStartX := startX + dx*ratio
				newStartY := startY + dy*ratio
				// Adjust cp1 slightly to maintain curve shape
				newCp1X := cp1X + dx*ratio*0.3
				newCp1Y := cp1Y + dy*ratio*0.3

				// Rebuild path with new start point and adjusted cp1
				restArgs := strings.Join(cParts[2:], " ")
				return fmt.Sprintf("M%s %sC%s %s %s",
					fmtPrec(newStartX, precision), fmtPrec(newStartY, precision),
					fmtPrec(newCp1X, precision), fmtPrec(newCp1Y, precision),
					restArgs) + args[endIdx:]
			}
		}
	}

	return pathData
}

// findPrevPoint finds the last coordinate pair before the final command in a path.
func findPrevPoint(pathPrefix string) (float64, float64) {
	// Extract all numbers from the path
	parts := strings.Fields(pathPrefix)
	var nums []float64
	for _, p := range parts {
		// Skip command letters
		p = strings.TrimLeft(p, "MmLlCcSsQqTtAaHhVvZz")
		if p == "" {
			continue
		}
		if v, err := strconv.ParseFloat(p, 64); err == nil {
			nums = append(nums, v)
		}
	}

	// Last two numbers are the previous point
	if len(nums) >= 2 {
		return nums[len(nums)-2], nums[len(nums)-1]
	}
	return 0, 0
}

// ForeignImage contains embedded image data extracted from a Foreign-type shape.
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
