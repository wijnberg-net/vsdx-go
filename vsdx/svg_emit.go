package vsdx

import (
	"fmt"
	"math"
	"strings"
)

// SVGEmitter emits SVG from a resolved RenderNode tree.
// This is a PURE serializer - it performs NO computation, NO style resolution,
// NO geometry mutation, and NO semantic analysis.
// All decisions must be made before calling Emit().
type SVGEmitter struct {
	precision      int
	node           *RenderNode
	outW           float64
	outH           float64
	scale          float64 // scale factor (pixels per inch)
	negativeWidth  bool    // original shape had negative width (connector going left)
	negativeHeight bool    // original shape had negative height (connector going down)
	shadowFilterID string  // unique filter ID for shadows in this shape
}

// NewSVGEmitter creates a new emitter for the given render tree.
func NewSVGEmitter(node *RenderNode, outW, outH float64, precision int) *SVGEmitter {
	return &SVGEmitter{
		precision: precision,
		node:      node,
		outW:      outW,
		outH:      outH,
		scale:     72.0, // default: 72 pixels per inch
	}
}

// NewSVGEmitterWithNegative creates an emitter that handles negative dimensions.
// Use this for shapes like connectors that may have negative width/height.
func NewSVGEmitterWithNegative(node *RenderNode, outW, outH float64, precision int, negW, negH bool) *SVGEmitter {
	return &SVGEmitter{
		precision:      precision,
		node:           node,
		outW:           outW,
		outH:           outH,
		scale:          72.0, // default: 72 pixels per inch
		negativeWidth:  negW,
		negativeHeight: negH,
	}
}

// NewSVGEmitterWithScale creates an emitter with a custom scale factor.
func NewSVGEmitterWithScale(node *RenderNode, outW, outH float64, precision int, scale float64, negW, negH bool) *SVGEmitter {
	return &SVGEmitter{
		precision:      precision,
		node:           node,
		outW:           outW,
		outH:           outH,
		scale:          scale,
		negativeWidth:  negW,
		negativeHeight: negH,
	}
}

// Emit generates SVG from the render tree.
// Returns the complete SVG document as bytes.
func (e *SVGEmitter) Emit() []byte {
	var svg strings.Builder

	// Collect all definitions needed
	markers := e.collectMarkers(e.node)
	gradients := e.collectGradients(e.node)
	shadows := e.collectShadows(e.node)
	fillPatterns := e.collectFillPatterns(e.node)

	// Calculate viewBox padding based on stroke widths and markers
	maxStrokeWidth := e.findMaxStrokeWidth(e.node)
	arrowPad := 0.0
	if len(markers) > 0 {
		arrowPad = maxStrokeWidth * 3
	}
	pad := maxStrokeWidth/2 + arrowPad

	// ViewBox - handle negative dimensions for connectors
	vbX := -pad
	vbY := -pad
	vbW := e.outW + 2*pad
	vbH := e.outH + 2*pad

	// Connectors with negative width/height need shifted viewBox
	if e.negativeWidth {
		vbX = -e.outW - pad
	}
	if e.negativeHeight {
		vbY = -e.outH - pad
	}

	// Start SVG document
	svg.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s %s %s %s">`,
		e.fmtNum(vbX), e.fmtNum(vbY), e.fmtNum(vbW), e.fmtNum(vbH)))
	svg.WriteByte('\n')

	// Emit <defs> section
	hasShadows := len(shadows) > 0
	// Use shape ID to make filter ID unique when multiple shapes are combined
	shadowFilterID := "shadow_blur"
	if e.node.Shape != nil && e.node.Shape.ID != "" {
		shadowFilterID = fmt.Sprintf("shadow_blur_%s", e.node.Shape.ID)
	}
	if len(markers) > 0 || len(gradients) > 0 || hasShadows || len(fillPatterns) > 0 {
		svg.WriteString("  <defs>\n")
		for _, m := range markers {
			svg.WriteString(e.emitMarker(m))
			svg.WriteByte('\n')
		}
		for id, g := range gradients {
			svg.WriteString("    ")
			svg.WriteString(gradientToSVGDef(g, id, e.precision))
			svg.WriteByte('\n')
		}
		// Fill patterns for hatching (patterns 2-24)
		for _, fp := range fillPatterns {
			svg.WriteString("    ")
			svg.WriteString(fillPatternToSVG(fp.PatternType, fp.ID, fp.ForeColor, fp.BackColor, fp.Scale))
			svg.WriteByte('\n')
		}
		// Single blur filter for shadows in this shape (unique ID per shape)
		if hasShadows {
			svg.WriteString(fmt.Sprintf(`    <filter id="%s"><feGaussianBlur stdDeviation="2"/></filter>`, shadowFilterID))
			svg.WriteByte('\n')
		}
		svg.WriteString("  </defs>\n")
	}
	e.shadowFilterID = shadowFilterID // store for use in emitShadowPath

	// Check for rotation on root shape
	shapeAngle := toFloat(e.node.Shape.CellValue("Angle"))
	hasRotation := shapeAngle != 0

	if hasRotation {
		angleDeg := -shapeAngle * 180 / math.Pi
		cx := e.outW / 2
		cy := e.outH / 2
		svg.WriteString(fmt.Sprintf(`  <g transform="rotate(%s %s %s)">`,
			e.fmtNum(angleDeg), e.fmtNum(cx), e.fmtNum(cy)))
		svg.WriteByte('\n')
	}

	// Emit all geometry
	e.emitGeometry(&svg, e.node)

	// Emit all text
	e.emitText(&svg, e.node)

	// Close rotation group
	if hasRotation {
		svg.WriteString("  </g>\n")
	}

	svg.WriteString("</svg>\n")

	return []byte(svg.String())
}

func (e *SVGEmitter) collectMarkers(node *RenderNode) map[string]*MarkerDef {
	result := make(map[string]*MarkerDef)
	e.collectMarkersRecursive(node, result)
	return result
}

func (e *SVGEmitter) collectMarkersRecursive(node *RenderNode, result map[string]*MarkerDef) {
	for id, m := range node.Markers {
		result[id] = m
	}
	for _, child := range node.Children {
		e.collectMarkersRecursive(child, result)
	}
}

func (e *SVGEmitter) collectGradients(node *RenderNode) map[string]*Gradient {
	result := make(map[string]*Gradient)
	e.collectGradientsRecursive(node, result)
	return result
}

func (e *SVGEmitter) collectGradientsRecursive(node *RenderNode, result map[string]*Gradient) {
	for id, g := range node.Gradients {
		result[id] = g
	}
	for _, child := range node.Children {
		e.collectGradientsRecursive(child, result)
	}
}

func (e *SVGEmitter) collectShadows(node *RenderNode) map[string]*Shadow {
	result := make(map[string]*Shadow)
	e.collectShadowsRecursive(node, result)
	return result
}

func (e *SVGEmitter) collectShadowsRecursive(node *RenderNode, result map[string]*Shadow) {
	for id, s := range node.Shadows {
		result[id] = s
	}
	for _, child := range node.Children {
		e.collectShadowsRecursive(child, result)
	}
}

func (e *SVGEmitter) collectFillPatterns(node *RenderNode) map[string]*FillPatternDef {
	result := make(map[string]*FillPatternDef)
	e.collectFillPatternsRecursive(node, result)
	return result
}

func (e *SVGEmitter) collectFillPatternsRecursive(node *RenderNode, result map[string]*FillPatternDef) {
	for id, fp := range node.FillPatterns {
		result[id] = fp
	}
	for _, child := range node.Children {
		e.collectFillPatternsRecursive(child, result)
	}
}

func (e *SVGEmitter) findMaxStrokeWidth(node *RenderNode) float64 {
	max := 0.0
	e.findMaxStrokeWidthRecursive(node, &max)
	return max
}

func (e *SVGEmitter) findMaxStrokeWidthRecursive(node *RenderNode, max *float64) {
	for _, path := range node.Geometry {
		if path.StrokeWidth > *max {
			*max = path.StrokeWidth
		}
	}
	for _, child := range node.Children {
		e.findMaxStrokeWidthRecursive(child, max)
	}
}

func (e *SVGEmitter) emitMarker(m *MarkerDef) string {
	if m == nil || m.Path == "" {
		return ""
	}

	fillAttr := "none"
	strokeAttr := m.Color
	if m.Filled {
		fillAttr = m.Color
		strokeAttr = "none"
	}

	orient := "auto"
	if !m.IsEnd {
		orient = "auto-start-reverse"
	}

	// preserveAspectRatio="none" lets the 10x10 path stretch to match
	// markerWidth/markerHeight independently. Without it, the default
	// "xMidYMid meet" forces uniform scaling, which squashes elongated
	// arrows (e.g. type 13 with markerWidth=5.4, markerHeight=3.6) back
	// into a square-aspect triangle - making "long" arrows look stubby.
	return fmt.Sprintf(`    <marker id="%s" viewBox="0 0 %s %s" refX="%s" refY="%s" markerWidth="%s" markerHeight="%s" markerUnits="strokeWidth" orient="%s" preserveAspectRatio="none"><path d="%s" fill="%s" stroke="%s"/></marker>`,
		m.ID,
		e.fmtNum(10), e.fmtNum(10), // viewBox dimensions (standard arrow size)
		e.fmtNum(m.RefX), e.fmtNum(m.RefY),
		e.fmtNum(m.Width), e.fmtNum(m.Height),
		orient,
		m.Path, fillAttr, strokeAttr)
}

func (e *SVGEmitter) emitGeometry(svg *strings.Builder, node *RenderNode) {
	if !node.Visible {
		return
	}

	// Collect valid paths for this node
	var validPaths []*ResolvedPath
	for _, path := range node.Geometry {
		if !path.NoShow && path.D != "" {
			validPaths = append(validPaths, path)
		}
	}

	// Helper to emit children
	emitChildren := func() {
		for _, child := range node.Children {
			if child.Angle != 0 {
				angleDeg := -child.Angle * 180 / math.Pi
				svg.WriteString(fmt.Sprintf("  <g transform=\"rotate(%s %s %s)\">\n",
					e.fmtNum(angleDeg), e.fmtNum(child.RotationCenterX), e.fmtNum(child.RotationCenterY)))
				e.emitGeometry(svg, child)
				svg.WriteString("  </g>\n")
			} else {
				e.emitGeometry(svg, child)
			}
		}
	}

	// Helper to emit this node's paths (with shadows right before each path)
	emitPaths := func() {
		if len(validPaths) >= 2 && e.canCombinePaths(validPaths) {
			// For compound paths, emit shadow for first path only
			first := validPaths[0]
			if first.FilterID != "" {
				if shadow, ok := node.Shadows[first.FilterID]; ok {
					e.emitShadowPath(svg, first, shadow)
				}
			}
			e.emitCompoundPath(svg, validPaths)
		} else {
			for _, path := range validPaths {
				// Emit shadow right before its path (like Visio)
				if path.FilterID != "" {
					if shadow, ok := node.Shadows[path.FilterID]; ok {
						e.emitShadowPath(svg, path, shadow)
					}
				}
				e.emitSinglePath(svg, path)
			}
		}
	}

	// Visio renders children BEFORE parent geometry in groups.
	// This ensures caps/overlays (parent geometry) appear on top of body shapes (children).
	// Example: cylinder has body as child and ellipse cap as parent geometry.
	// Shadows are rendered inline, right before each path.
	emitChildren()
	emitPaths()
}

// canCombinePaths returns true if all paths have matching fill/stroke styling
func (e *SVGEmitter) canCombinePaths(paths []*ResolvedPath) bool {
	if len(paths) < 2 {
		return false
	}
	first := paths[0]
	for _, p := range paths[1:] {
		if p.Fill != first.Fill || p.Stroke != first.Stroke ||
			p.StrokeWidth != first.StrokeWidth || p.FillOpacity != first.FillOpacity {
			return false
		}
	}
	return true
}

// emitCompoundPath emits multiple paths as a single compound path with evenodd fill
func (e *SVGEmitter) emitCompoundPath(svg *strings.Builder, paths []*ResolvedPath) {
	if len(paths) == 0 {
		return
	}

	// Combine all path data
	var combinedD strings.Builder
	for i, path := range paths {
		if i > 0 {
			combinedD.WriteByte(' ')
		}
		combinedD.WriteString(path.D)
	}

	// Use first path's styling (they should all match)
	first := paths[0]
	var attrs []string

	// Use fill pattern if available, otherwise use solid fill
	if first.FillPatternID != "" {
		attrs = append(attrs, fmt.Sprintf(`fill="url(#%s)"`, first.FillPatternID))
	} else {
		attrs = append(attrs, fmt.Sprintf(`fill="%s"`, first.Fill))
	}
	attrs = append(attrs, fmt.Sprintf(`stroke="%s"`, first.Stroke))
	attrs = append(attrs, fmt.Sprintf(`stroke-width="%s"`, e.fmtNum(first.StrokeWidth)))

	// Add fill-rule for compound paths to create holes
	attrs = append(attrs, `fill-rule="evenodd"`)

	if first.VectorEffect != "" {
		attrs = append(attrs, fmt.Sprintf(`vector-effect="%s"`, first.VectorEffect))
	}
	if first.FillOpacity > 0 && first.FillOpacity < 1.0 {
		attrs = append(attrs, fmt.Sprintf(`fill-opacity="%s"`, e.fmtNum(first.FillOpacity)))
	}
	if first.DashArray != "" {
		attrs = append(attrs, fmt.Sprintf(`stroke-dasharray="%s"`, first.DashArray))
	}
	if first.LineCap != "" {
		attrs = append(attrs, fmt.Sprintf(`stroke-linecap="%s"`, first.LineCap))
	} else {
		attrs = append(attrs, `stroke-linecap="round"`)
	}
	attrs = append(attrs, `stroke-linejoin="round"`)
	// Note: FilterID is used for shadow lookup only, not applied to main path

	svg.WriteString(fmt.Sprintf(`  <path d="%s" %s/>`, combinedD.String(), strings.Join(attrs, " ")))
	svg.WriteByte('\n')
}

// emitSinglePath emits a single path element
func (e *SVGEmitter) emitSinglePath(svg *strings.Builder, path *ResolvedPath) {
	var attrs []string

	// Use fill pattern if available, otherwise use solid fill
	if path.FillPatternID != "" {
		attrs = append(attrs, fmt.Sprintf(`fill="url(#%s)"`, path.FillPatternID))
	} else {
		attrs = append(attrs, fmt.Sprintf(`fill="%s"`, path.Fill))
	}
	attrs = append(attrs, fmt.Sprintf(`stroke="%s"`, path.Stroke))
	attrs = append(attrs, fmt.Sprintf(`stroke-width="%s"`, e.fmtNum(path.StrokeWidth)))

	if path.VectorEffect != "" {
		attrs = append(attrs, fmt.Sprintf(`vector-effect="%s"`, path.VectorEffect))
	}
	if path.FillOpacity > 0 && path.FillOpacity < 1.0 {
		attrs = append(attrs, fmt.Sprintf(`fill-opacity="%s"`, e.fmtNum(path.FillOpacity)))
	}
	if path.DashArray != "" {
		attrs = append(attrs, fmt.Sprintf(`stroke-dasharray="%s"`, path.DashArray))
	}
	if path.LineCap != "" {
		attrs = append(attrs, fmt.Sprintf(`stroke-linecap="%s"`, path.LineCap))
	} else {
		attrs = append(attrs, `stroke-linecap="round"`)
	}
	attrs = append(attrs, `stroke-linejoin="round"`)
	if path.MarkerStartID != "" {
		attrs = append(attrs, fmt.Sprintf(`marker-start="%s"`, path.MarkerStartID))
	}
	if path.MarkerEndID != "" {
		attrs = append(attrs, fmt.Sprintf(`marker-end="%s"`, path.MarkerEndID))
	}
	// Note: FilterID is used for shadow lookup, not applied directly to path

	svg.WriteString(fmt.Sprintf(`  <path d="%s" %s/>`, path.D, strings.Join(attrs, " ")))
	svg.WriteByte('\n')
}

// emitShadowPath emits a shadow copy of the path (like Visio does)
func (e *SVGEmitter) emitShadowPath(svg *strings.Builder, path *ResolvedPath, shadow *Shadow) {
	// Calculate offset in SVG units
	dx := shadow.OffsetX * e.scale
	dy := -shadow.OffsetY * e.scale // flip Y for SVG

	// Shadow opacity (from ShdwForegndTrans)
	opacity := shadow.Opacity
	if opacity == 0 {
		opacity = 0.22 // default like Visio
	}

	// Emit shadow group with offset transform
	svg.WriteString(fmt.Sprintf(`  <g transform="translate(%s,%s)" filter="url(#%s)">`,
		e.fmtNum(dx), e.fmtNum(dy), e.shadowFilterID))
	svg.WriteString(fmt.Sprintf(`<path d="%s" fill="%s" fill-opacity="%s" stroke="%s" stroke-opacity="%s" stroke-width="%s"/>`,
		path.D, shadow.Color, e.fmtNum(opacity), shadow.Color, e.fmtNum(opacity), e.fmtNum(path.StrokeWidth)))
	svg.WriteString("</g>\n")
}

func (e *SVGEmitter) emitText(svg *strings.Builder, node *RenderNode) {
	if !node.Visible {
		return
	}

	// Emit text for this node if it has text content
	if node.Text != nil && node.Text.Content != "" {
		t := node.Text
		content := escapeXML(t.Content)

		svg.WriteString(fmt.Sprintf(`  <text x="%s" y="%s" text-anchor="%s" dominant-baseline="%s" fill="%s" font-size="%s" font-weight="%s" font-style="%s"`,
			e.fmtNum(t.X), e.fmtNum(t.Y),
			t.TextAnchor, t.Baseline,
			t.Fill, e.fmtNum(t.FontSize),
			t.FontWeight, t.FontStyle))

		if t.FontFamily != "" {
			svg.WriteString(fmt.Sprintf(` font-family="%s"`, t.FontFamily))
		}
		if t.TextDecoration != "" {
			svg.WriteString(fmt.Sprintf(` text-decoration="%s"`, t.TextDecoration))
		}
		if t.SmallCaps {
			svg.WriteString(` font-variant="small-caps"`)
		}
		if t.TextPos == 1 { // Superscript
			svg.WriteString(` baseline-shift="super" font-size="70%"`)
		} else if t.TextPos == 2 { // Subscript
			svg.WriteString(` baseline-shift="sub" font-size="70%"`)
		}
		if t.Transform != "" {
			svg.WriteString(fmt.Sprintf(` transform="%s"`, t.Transform))
		}

		svg.WriteString(">")

		// Handle multi-line text
		if len(t.Lines) > 1 {
			for i, line := range t.Lines {
				if i == 0 {
					svg.WriteString(fmt.Sprintf(`<tspan x="%s" dy="0">%s</tspan>`,
						e.fmtNum(t.X), escapeXML(line)))
				} else {
					svg.WriteString(fmt.Sprintf(`<tspan x="%s" dy="1.2em">%s</tspan>`,
						e.fmtNum(t.X), escapeXML(line)))
				}
			}
		} else {
			svg.WriteString(content)
		}

		svg.WriteString("</text>")
		svg.WriteByte('\n')
	}

	// Always recurse to children (even if this node has no text)
	for _, child := range node.Children {
		e.emitText(svg, child)
	}
}

func (e *SVGEmitter) fmtNum(v float64) string {
	s := fmt.Sprintf("%.*f", e.precision, v)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// EmitRenderTree is a convenience function that builds and emits SVG from a shape.
// This is the new canonical entry point for SVG generation.
func EmitRenderTree(shape *Shape, width, height float64, precision int) ([]byte, error) {
	result, err := EmitRenderTreeWithResult(shape, width, height, precision, "")
	if err != nil {
		return nil, err
	}
	return result.SVG, nil
}

// EmitRenderTreeWithResult builds and emits SVG with full result metadata.
// brandColorOverride can be used to skip auto-detection; pass empty string for auto.
func EmitRenderTreeWithResult(shape *Shape, width, height float64, precision int, brandColorOverride string) (*SVGResult, error) {
	if shape == nil {
		return nil, fmt.Errorf("nil shape")
	}

	// Detect negative dimensions before taking absolute values
	// Connectors often have negative width/height indicating direction
	rawW := shape.Width()
	rawH := shape.Height()
	negW := rawW < 0
	negH := rawH < 0

	// Calculate scale using absolute dimensions
	shapeW := math.Abs(rawW)
	shapeH := math.Abs(rawH)
	if shapeW == 0 {
		shapeW = 1
	}
	if shapeH == 0 {
		shapeH = 1
	}

	scale := width / shapeW
	if s := height / shapeH; s < scale {
		scale = s
	}

	outW := shapeW * scale
	outH := shapeH * scale

	// Build render tree
	builder := NewRenderTreeBuilder(shape)
	tree := builder.BuildWithScale(scale, scale, precision)

	if tree == nil {
		return nil, fmt.Errorf("failed to build render tree")
	}

	// Emit SVG with scale and negative dimension flags for proper viewBox
	emitter := NewSVGEmitterWithScale(tree, outW, outH, precision, scale, negW, negH)
	svg := emitter.Emit()

	// Determine brand color
	brandColor := brandColorOverride
	if brandColor == "" {
		brandColor = builder.BrandColor()
	}

	return &SVGResult{
		SVG:        svg,
		BrandColor: brandColor,
		Width:      outW,
		Height:     outH,
	}, nil
}
