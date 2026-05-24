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
	negativeWidth  bool // original shape had negative width (connector going left)
	negativeHeight bool // original shape had negative height (connector going down)
}

// NewSVGEmitter creates a new emitter for the given render tree.
func NewSVGEmitter(node *RenderNode, outW, outH float64, precision int) *SVGEmitter {
	return &SVGEmitter{
		precision: precision,
		node:      node,
		outW:      outW,
		outH:      outH,
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
	if len(markers) > 0 || len(gradients) > 0 || len(shadows) > 0 {
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
		for id, s := range shadows {
			svg.WriteString("    ")
			svg.WriteString(shadowToSVGFilter(s, id, 1.0, 1.0, e.precision))
			svg.WriteByte('\n')
		}
		svg.WriteString("  </defs>\n")
	}

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

	return fmt.Sprintf(`    <marker id="%s" viewBox="0 0 %s %s" refX="%s" refY="%s" markerWidth="%s" markerHeight="%s" markerUnits="strokeWidth" orient="%s"><path d="%s" fill="%s" stroke="%s"/></marker>`,
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

	// Collect valid paths
	var validPaths []*ResolvedPath
	for _, path := range node.Geometry {
		if !path.NoShow && path.D != "" {
			validPaths = append(validPaths, path)
		}
	}

	// If multiple paths with same styling, combine into compound path
	// This creates holes using SVG's evenodd fill rule
	if len(validPaths) >= 2 && e.canCombinePaths(validPaths) {
		e.emitCompoundPath(svg, validPaths)
	} else {
		// Emit paths individually
		for _, path := range validPaths {
			e.emitSinglePath(svg, path)
		}
	}

	// Recurse to children
	for _, child := range node.Children {
		// Apply rotation transform if child has non-zero angle
		if child.Angle != 0 {
			// Negate angle because Visio uses counter-clockwise positive (Y-up)
			// while SVG uses clockwise positive (Y-down)
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

	attrs = append(attrs, fmt.Sprintf(`fill="%s"`, first.Fill))
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
	if first.FilterID != "" {
		attrs = append(attrs, fmt.Sprintf(`filter="url(#%s)"`, first.FilterID))
	}

	svg.WriteString(fmt.Sprintf(`  <path d="%s" %s/>`, combinedD.String(), strings.Join(attrs, " ")))
	svg.WriteByte('\n')
}

// emitSinglePath emits a single path element
func (e *SVGEmitter) emitSinglePath(svg *strings.Builder, path *ResolvedPath) {
	var attrs []string

	attrs = append(attrs, fmt.Sprintf(`fill="%s"`, path.Fill))
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
	if path.MarkerStartID != "" {
		attrs = append(attrs, fmt.Sprintf(`marker-start="%s"`, path.MarkerStartID))
	}
	if path.MarkerEndID != "" {
		attrs = append(attrs, fmt.Sprintf(`marker-end="%s"`, path.MarkerEndID))
	}
	if path.FilterID != "" {
		attrs = append(attrs, fmt.Sprintf(`filter="url(#%s)"`, path.FilterID))
	}

	svg.WriteString(fmt.Sprintf(`  <path d="%s" %s/>`, path.D, strings.Join(attrs, " ")))
	svg.WriteByte('\n')
}

func (e *SVGEmitter) emitText(svg *strings.Builder, node *RenderNode) {
	if !node.Visible {
		return
	}

	// Emit text for this node if it has text content
	if node.Text != nil && node.Text.Content != "" {
		t := node.Text
		content := escapeXML(t.Content)

		// Scale font size appropriately
		// Text position and font size should already be resolved in the render tree
		svg.WriteString(fmt.Sprintf(`  <text x="%s" y="%s" text-anchor="%s" dominant-baseline="%s" fill="%s" font-size="%s" font-weight="%s" font-style="%s"`,
			e.fmtNum(t.X), e.fmtNum(t.Y),
			t.TextAnchor, t.Baseline,
			t.Fill, e.fmtNum(t.FontSize),
			t.FontWeight, t.FontStyle))

		// Text decoration (underline, line-through)
		if t.TextDecoration != "" {
			svg.WriteString(fmt.Sprintf(` text-decoration="%s"`, t.TextDecoration))
		}

		if t.Transform != "" {
			svg.WriteString(fmt.Sprintf(` transform="%s"`, t.Transform))
		}

		svg.WriteString(">")

		// Handle multi-line text
		if len(t.Lines) > 1 {
			for i, line := range t.Lines {
				dy := "0"
				if i > 0 {
					dy = e.fmtNum(t.LineHeight)
				}
				svg.WriteString(fmt.Sprintf(`<tspan x="%s" dy="%s">%s</tspan>`,
					e.fmtNum(t.X), dy, escapeXML(line)))
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

	// Emit SVG with negative dimension flags for proper viewBox
	emitter := NewSVGEmitterWithNegative(tree, outW, outH, precision, negW, negH)
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
