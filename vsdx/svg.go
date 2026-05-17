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
type ArrowDef struct {
	Path   string  // SVG path data
	Width  float64 // marker width relative to line weight
	Height float64 // marker height relative to line weight
	RefX   float64 // attachment point X
	RefY   float64 // attachment point Y
	Filled bool    // whether the arrow is filled
}

// visioArrowTypes maps Visio arrow type indices to SVG path definitions.
// Defined by MS-VSDX spec §2.4.4.
var visioArrowTypes = map[int]ArrowDef{
	0:  {}, // None
	1:  {Path: "M0 0 L10 5 L0 10 z", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: true},      // Triangle (filled)
	2:  {Path: "M0 0 L10 5 L0 10 L2 5 z", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: true}, // Stealth
	3:  {Path: "M0 0 L10 5 L0 10", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: false},       // Triangle (open)
	4:  {Path: "M0 0 L10 5 M0 10 L10 5", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: false}, // Line (open)
	5:  {Path: "M10 5 A5 5 0 1 0 10 5.01 z", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: true},  // Oval (filled)
	6:  {Path: "M5 0 L10 5 L5 10 L0 5 z", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: true},     // Diamond (filled)
	7:  {Path: "M5 0 L10 5 L5 10 L0 5 z", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: false},    // Diamond (open)
	8:  {Path: "M10 5 A5 5 0 1 0 10 5.01 z", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: false}, // Oval (open)
	9:  {Path: "M0 0 L10 5 L0 10 z M0 2 L8 5 L0 8 z", Width: 12, Height: 10, RefX: 12, RefY: 5, Filled: true}, // Double triangle
	10: {Path: "M0 0 L10 4 L10 6 L0 10 z", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: true},    // Triangle 45°
	13: {Path: "M0 0 L10 5 L0 10 z", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: true},          // Standard arrow
	14: {Path: "M0 0 L10 5 L0 10", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: false},           // Open arrow
	22: {Path: "M0 5 L5 0 L10 5 M0 5 L5 10 L10 5", Width: 10, Height: 10, RefX: 5, RefY: 5, Filled: false}, // Fletching
	45: {Path: "M0 0 L10 5 L0 10 z", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: true},          // Filled arrow (fallback)
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
func ShapeToSVG(shape *Shape, opts ...SVGOption) (*SVGResult, error) {
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
	if shapeW <= 0 {
		shapeW = 1
	}
	if shapeH <= 0 {
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
	vbX := -pad
	vbY := -pad
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

	w := def.Width * sizeMult
	h := def.Height * sizeMult
	refX := def.RefX * sizeMult
	refY := def.RefY * sizeMult

	fillAttr := "none"
	strokeAttr := m.color
	if def.Filled {
		fillAttr = m.color
		strokeAttr = "none"
	}

	orient := "auto-start-reverse"
	if m.isEnd {
		orient = "auto"
	}

	return fmt.Sprintf(`    <marker id="%s" viewBox="0 0 %s %s" refX="%s" refY="%s" markerWidth="%s" markerHeight="%s" orient="%s"><path d="%s" fill="%s" stroke="%s"/></marker>`,
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

	// Escape text for XML.
	text = escapeXML(text)

	parentH := parent.Height()
	if parentH <= 0 {
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

	// Get text block dimensions.
	txtWidth := toFloat(s.CellValue("TxtWidth"))
	txtHeight := toFloat(s.CellValue("TxtHeight"))
	if txtWidth == 0 {
		txtWidth = ss.localW
	}
	if txtHeight == 0 {
		txtHeight = ss.localH
	}

	// Get text angle (rotation in radians).
	txtAngle := toFloat(s.CellValue("TxtAngle"))

	// Get vertical alignment: 0=top, 1=middle, 2=bottom.
	vertAlign := int(toFloat(s.CellValue("VerticalAlign")))

	// Get horizontal alignment from paragraph: 0=left, 1=center, 2=right, 3=justify.
	horzAlign := int(toFloat(s.CellValue("Para.HorzAlign")))
	if horzAlign == 0 {
		horzAlign = 1 // default to center
	}

	// Get margins.
	leftMargin := toFloat(s.CellValue("LeftMargin"))
	rightMargin := toFloat(s.CellValue("RightMargin"))
	topMargin := toFloat(s.CellValue("TopMargin"))
	bottomMargin := toFloat(s.CellValue("BottomMargin"))

	// Calculate text position in shape coordinates.
	// Text block position is relative to shape's pin point.
	textX := txtPinX + ss.offsetX + leftMargin
	textY := txtPinY + ss.offsetY

	// Adjust for vertical alignment.
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

	// Get font size (Character section, Char.Size cell).
	fontSize := toFloat(s.CellValue("Char.Size"))
	if fontSize == 0 {
		fontSize = 12.0 / 72.0 // Default 12pt in inches
	}
	fontSizePx := fontSize * ((scaleX + scaleY) / 2)

	// Get text color.
	textColor := resolveColor(s.TextColor())
	if textColor == "" {
		textColor = "#000000"
	}

	// Get font weight and style.
	fontWeight := "normal"
	if s.CellValue("Char.Style") == "17" || s.CellValue("Char.Style") == "1" {
		fontWeight = "bold"
	}
	fontStyle := "normal"
	if s.CellValue("Char.Style") == "18" || s.CellValue("Char.Style") == "2" {
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

	// Handle multi-line text.
	lines := strings.Split(text, "\n")
	if len(lines) == 1 {
		return fmt.Sprintf(`  <text x="%s" y="%s" text-anchor="%s" dominant-baseline="%s" fill="%s" font-size="%s" font-weight="%s" font-style="%s"%s>%s</text>`,
			fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision),
			textAnchor, baseline, textColor,
			fmtPrec(fontSizePx, o.Precision), fontWeight, fontStyle,
			transform, text)
	}

	// Multi-line text: use tspans.
	var result strings.Builder
	result.WriteString(fmt.Sprintf(`  <text x="%s" y="%s" text-anchor="%s" dominant-baseline="%s" fill="%s" font-size="%s" font-weight="%s" font-style="%s"%s>`,
		fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision),
		textAnchor, baseline, textColor,
		fmtPrec(fontSizePx, o.Precision), fontWeight, fontStyle,
		transform))

	lineHeight := fontSizePx * 1.2
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
	offsetX        float64   // X offset in parent group's coordinate space (inches)
	offsetY        float64   // Y offset in parent group's coordinate space (inches)
	localW, localH float64   // shape's own width/height
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
				// Recursive: collect from nested group
				for _, nested := range collectRenderableShapes(child) {
					nested.offsetX += ox
					nested.offsetY += oy
					result = append(result, nested)
				}
			} else if shapeHasGeometry(child) {
				for _, g := range child.Geometries {
					if len(g.Rows) == 0 {
						continue
					}
					result = append(result, renderableShape{
						shape:   child,
						geom:    g,
						offsetX: ox,
						offsetY: oy,
						localW:  childW,
						localH:  childH,
					})
				}
			}
		}
		return result
	}

	// Simple shape — emit one renderable per geometry section.
	var result []renderableShape
	for _, g := range shape.Geometries {
		if len(g.Rows) > 0 {
			result = append(result, renderableShape{
				shape:   shape,
				geom:    g,
				offsetX: 0,
				offsetY: 0,
				localW:  shape.Width(),
				localH:  shape.Height(),
			})
		}
	}
	return result
}

// renderSubShapeWithMarkers converts a single geometry section to an SVG <path> with marker support.
func renderSubShapeWithMarkers(ss renderableShape, parent *Shape, scaleX, scaleY float64, o *SVGOptions, colorFreq map[string]int) svgRenderResult {
	return renderSubShapeInternal(ss, parent, scaleX, scaleY, o, colorFreq)
}

// renderSubShape converts a single sub-shape's geometry to an SVG <path> element.
// renderSubShape converts a single geometry section to an SVG <path>.
// Returns the SVG string and the computed stroke width (for viewBox padding).
func renderSubShape(ss renderableShape, parent *Shape, scaleX, scaleY float64, o *SVGOptions, colorFreq map[string]int) (string, float64) {
	result := renderSubShapeInternal(ss, parent, scaleX, scaleY, o, colorFreq)
	return result.pathSVG, result.strokeWidth
}

// renderSubShapeInternal is the core rendering function that supports markers, gradients, and shadows.
func renderSubShapeInternal(ss renderableShape, parent *Shape, scaleX, scaleY float64, o *SVGOptions, colorFreq map[string]int) svgRenderResult {
	s := ss.shape
	geom := ss.geom
	if geom == nil || len(geom.Rows) == 0 {
		return svgRenderResult{}
	}

	parentW := parent.Width()
	parentH := parent.Height()
	if parentW <= 0 {
		parentW = 1 //nolint:ineffassign // kept for symmetry with parentH guard
	}
	if parentH <= 0 {
		parentH = 1
	}

	// Sort geometry rows by IX.
	sortedRows := sortGeometryRows(geom.Rows)

	// Build SVG path data.
	var d strings.Builder
	var prevX, prevY float64 // track current point in SVG space

	for _, row := range sortedRows {
		rt := strings.ToLower(row.RowType())
		switch rt {
		case "moveto":
			// Absolute coordinates in shape's local space (inches)
			sx, sy := row.X(), row.Y()
			svgX, svgY := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)
			d.WriteString(fmt.Sprintf("M%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "lineto":
			sx, sy := row.X(), row.Y()
			svgX, svgY := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)
			d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "relmoveto":
			// RelMoveTo: coordinates are 0..1 fractions of shape's own width/height
			rx, ry := row.X(), row.Y()
			absX := rx*ss.localW + ss.offsetX
			absY := ry*ss.localH + ss.offsetY
			svgX, svgY := toSVGCoords(absX, absY, parentH, scaleX, scaleY)
			d.WriteString(fmt.Sprintf("M%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "rellineto":
			rx, ry := row.X(), row.Y()
			absX := rx*ss.localW + ss.offsetX
			absY := ry*ss.localH + ss.offsetY
			svgX, svgY := toSVGCoords(absX, absY, parentH, scaleX, scaleY)
			d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "arcto":
			sx, sy := row.X(), row.Y()
			svgX, svgY := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)
			bow := cellFloat(row, "A")

			arcSVG := arcToSVG(prevX, prevY, svgX, svgY, bow*((scaleX+scaleY)/2), o.Precision)
			d.WriteString(arcSVG)
			prevX, prevY = svgX, svgY

		case "ellipticalarcto":
			sx, sy := row.X(), row.Y()
			svgX, svgY := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)
			// A = control point X, B = control point Y, C = aspect ratio, D = angle
			cpX := cellFloat(row, "A")
			cpY := cellFloat(row, "B")
			_ = cpX
			_ = cpY
			// Approximate as line for now — elliptical arcs are complex and rare in stencils
			d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "relellipticalarcto":
			rx, ry := row.X(), row.Y()
			absX := rx*ss.localW + ss.offsetX
			absY := ry*ss.localH + ss.offsetY
			svgX, svgY := toSVGCoords(absX, absY, parentH, scaleX, scaleY)
			// Approximate as line (same as absolute version)
			d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "relcubbezto":
			// RelCubBezTo: cubic bezier with relative coords (0-1 fractions)
			rx, ry := row.X(), row.Y()
			absX := rx*ss.localW + ss.offsetX
			absY := ry*ss.localH + ss.offsetY
			svgX, svgY := toSVGCoords(absX, absY, parentH, scaleX, scaleY)
			// A, B = first control point; C, D = second control point
			cp1X := cellFloat(row, "A")*ss.localW + ss.offsetX
			cp1Y := cellFloat(row, "B")*ss.localH + ss.offsetY
			cp2X := cellFloat(row, "C")*ss.localW + ss.offsetX
			cp2Y := cellFloat(row, "D")*ss.localH + ss.offsetY
			cp1SvgX, cp1SvgY := toSVGCoords(cp1X, cp1Y, parentH, scaleX, scaleY)
			cp2SvgX, cp2SvgY := toSVGCoords(cp2X, cp2Y, parentH, scaleX, scaleY)
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
			svgX, svgY := toSVGCoords(absX, absY, parentH, scaleX, scaleY)
			// A, B = control point
			cpX := cellFloat(row, "A")*ss.localW + ss.offsetX
			cpY := cellFloat(row, "B")*ss.localH + ss.offsetY
			cpSvgX, cpSvgY := toSVGCoords(cpX, cpY, parentH, scaleX, scaleY)
			d.WriteString(fmt.Sprintf("Q%s %s %s %s",
				fmtPrec(cpSvgX, o.Precision), fmtPrec(cpSvgY, o.Precision),
				fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "polylineto":
			// PolylineTo: A cell contains POLYLINE formula with vertex list
			sx, sy := row.X(), row.Y()
			svgX, svgY := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)
			// Parse A cell for intermediate points and draw lines
			aFormula := cellString(row, "A")
			pts := parsePolylinePoints(aFormula, ss.localW, ss.localH, ss.offsetX, ss.offsetY)
			for _, pt := range pts {
				ptSvgX, ptSvgY := toSVGCoords(pt.x, pt.y, parentH, scaleX, scaleY)
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
			svgX1, svgY1 := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)
			svgX2, svgY2 := toSVGCoords(ax+ss.offsetX, ay+ss.offsetY, parentH, scaleX, scaleY)
			d.WriteString(fmt.Sprintf("M%s %s L%s %s",
				fmtPrec(svgX1, o.Precision), fmtPrec(svgY1, o.Precision),
				fmtPrec(svgX2, o.Precision), fmtPrec(svgY2, o.Precision)))
			prevX, prevY = svgX2, svgY2

		case "ellipse":
			// Ellipse: center (X,Y), control points (A,B) on major axis, (C,D) on minor axis.
			// SVG ellipse uses center (cx,cy) and radii (rx, ry).
			cx, cy := row.X(), row.Y()
			svgCx, svgCy := toSVGCoords(cx+ss.offsetX, cy+ss.offsetY, parentH, scaleX, scaleY)
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
			svgX, svgY := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)

			// Parse NURBS formula from E cell for control points.
			eFormula := cellString(row, "E")
			cps := parseNURBSControlPoints(eFormula)

			if len(cps) == 2 {
				// Degree 3, 2 interior control points → cubic Bezier.
				// Control point coordinates are proportional (0-1) of shape dimensions
				// when xType=0/yType=0, or absolute when xType=1/yType=1.
				// Visio uses the same proportional space as RelMoveTo.
				cp1AbsX := cps[0].x*ss.localW + ss.offsetX
				cp1AbsY := cps[0].y*ss.localH + ss.offsetY
				cp2AbsX := cps[1].x*ss.localW + ss.offsetX
				cp2AbsY := cps[1].y*ss.localH + ss.offsetY
				cp1SvgX, cp1SvgY := toSVGCoords(cp1AbsX, cp1AbsY, parentH, scaleX, scaleY)
				cp2SvgX, cp2SvgY := toSVGCoords(cp2AbsX, cp2AbsY, parentH, scaleX, scaleY)
				d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
					fmtPrec(cp1SvgX, o.Precision), fmtPrec(cp1SvgY, o.Precision),
					fmtPrec(cp2SvgX, o.Precision), fmtPrec(cp2SvgY, o.Precision),
					fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			} else if len(cps) == 1 {
				// Degree 2, 1 control point → quadratic Bezier.
				cpAbsX := cps[0].x*ss.localW + ss.offsetX
				cpAbsY := cps[0].y*ss.localH + ss.offsetY
				cpSvgX, cpSvgY := toSVGCoords(cpAbsX, cpAbsY, parentH, scaleX, scaleY)
				d.WriteString(fmt.Sprintf("Q%s %s %s %s",
					fmtPrec(cpSvgX, o.Precision), fmtPrec(cpSvgY, o.Precision),
					fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			} else {
				// Fallback: straight line to endpoint.
				d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			}
			prevX, prevY = svgX, svgY

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

	// Build style attributes.
	fill := resolveColor(s.FillColor())
	stroke := resolveColor(s.LineColor())
	lineWeight := s.LineWeight()
	var styleAttrs []string
	var gradientID string
	var gradient *Gradient

	// Check for gradient fill.
	gradient = s.FillGradient()
	if gradient != nil && !noFill {
		// Generate unique gradient ID based on shape ID.
		gradientID = fmt.Sprintf("grad_%s", s.ID)
		fill = fmt.Sprintf("url(#%s)", gradientID)
	} else {
		// Visio default: when FillForegnd is unresolvable but the shape has
		// an explicit FillPattern (solid) or FillForegndTrans, apply the Visio
		// default color index 0 (black) — the shape intends to be filled.
		fillPattern := s.CellValue("FillPattern")
		if fill == "" {
			if fillPattern == "1" || s.CellValue("FillForegndTrans") != "" {
				fill = "#000000"
			}
		}

		// FillPattern=0 means "no fill" regardless of FillForegnd value.
		if noFill || fill == "" || fillPattern == "0" {
			fill = "none"
		}
	}

	// LinePattern=0 means "no line" regardless of other line properties.
	// Empty LineColor means no explicit line was set — don't invent one.
	linePattern := s.CellValue("LinePattern")
	linePatternInt := int(toFloat(linePattern))
	if noLine || linePattern == "0" || stroke == "" {
		stroke = "none"
	}

	// Track colors for brand detection.
	if fill != "none" && fill != "" {
		colorFreq[strings.ToUpper(fill)]++
	}
	if stroke != "none" && stroke != "" {
		colorFreq[strings.ToUpper(stroke)]++
	}

	// Scale line weight.
	strokeWidth := lineWeight * ((scaleX + scaleY) / 2)
	if strokeWidth <= 0 && stroke != "none" {
		strokeWidth = 1
	}

	// Fill opacity.
	fillTrans := s.CellValue("FillForegndTrans")
	if fillTrans != "" {
		transVal := toFloat(fillTrans)
		if transVal > 0 {
			styleAttrs = append(styleAttrs, fmt.Sprintf(`fill-opacity="%s"`, fmtPrec(1-transVal, o.Precision)))
		}
	}

	// Line pattern (dash array).
	if stroke != "none" && linePatternInt > 1 {
		dashArray := linePatternToSVG(linePatternInt, strokeWidth)
		if dashArray != "" {
			styleAttrs = append(styleAttrs, fmt.Sprintf(`stroke-dasharray="%s"`, dashArray))
		}
	}

	// Arrow markers.
	var markers []markerRef
	beginArrow := int(toFloat(s.CellValue("BeginArrow")))
	endArrow := int(toFloat(s.CellValue("EndArrow")))
	beginArrowSize := int(toFloat(s.CellValue("BeginArrowSize")))
	endArrowSize := int(toFloat(s.CellValue("EndArrowSize")))

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

	// Shadow support.
	var shadowID string
	shadow := s.ShapeShadow()
	if shadow != nil {
		shadowID = fmt.Sprintf("shadow_%s", s.ID)
		styleAttrs = append(styleAttrs, fmt.Sprintf(`filter="url(#%s)"`, shadowID))
	}

	// Build the extra attributes string.
	extraAttrs := ""
	if len(styleAttrs) > 0 {
		extraAttrs = " " + strings.Join(styleAttrs, " ")
	}

	return svgRenderResult{
		pathSVG:     fmt.Sprintf(`  <path d="%s" fill="%s" stroke="%s" stroke-width="%s"%s/>`, pathData, fill, stroke, fmtPrec(strokeWidth, o.Precision), extraAttrs),
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

// parseNURBSControlPoints extracts interior control points from a NURBS() formula string.
// Format: NURBS(knotLast, degree, xType, yType, x1,y1,knot1,weight1, x2,y2,knot2,weight2, ...)
// Returns the (x,y) pairs of interior control points.
func parseNURBSControlPoints(formula string) []nurbsControlPoint {
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

	// Control points start at index 4, in groups of 4: x, y, knot, weight.
	var cps []nurbsControlPoint
	for i := 4; i+3 < len(parts); i += 4 {
		x := toFloat(parts[i])
		y := toFloat(parts[i+1])
		cps = append(cps, nurbsControlPoint{x: x, y: y})
	}
	return cps
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
