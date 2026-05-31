package vsdx

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// SVGEmitter emits SVG from a resolved RenderNode tree.
// This is a PURE serializer - it performs NO computation, NO style resolution,
// NO geometry mutation, and NO semantic analysis.
// All decisions must be made before calling Emit().
type SVGEmitter struct {
	precision        int
	node             *RenderNode
	outW             float64
	outH             float64
	scale            float64            // scale factor (pixels per inch)
	negativeWidth    bool               // original shape had negative width (connector going left)
	negativeHeight   bool               // original shape had negative height (connector going down)
	shadowFilterID   string             // unique filter ID for shadows in this shape
	softEdgesFilters map[string]float64 // collected soft-edge filter IDs → blur radius (pt)
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
	// Collect soft-edge filter IDs upfront so we can emit them in <defs>.
	softEdges := e.collectSoftEdges(e.node)
	if len(markers) > 0 || len(gradients) > 0 || hasShadows || len(fillPatterns) > 0 || len(softEdges) > 0 {
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
		// Soft-edges blur filters. Visio's SVG export drops these; we emit
		// them as feGaussianBlur in user-space units (points). stdDeviation
		// equals SoftEdgesSize so a "5pt" soft edge produces a 5-unit blur.
		for id, sizePt := range softEdges {
			svg.WriteString(fmt.Sprintf(`    <filter id="%s" x="-20%%" y="-20%%" width="140%%" height="140%%"><feGaussianBlur stdDeviation="%s"/></filter>`,
				id, e.fmtNum(sizePt)))
			svg.WriteByte('\n')
		}
		svg.WriteString("  </defs>\n")
	}
	e.shadowFilterID = shadowFilterID // store for use in emitShadowPath
	e.softEdgesFilters = softEdges    // store for emitSinglePath lookup

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

// collectSoftEdges walks the tree and returns a map from filter ID to blur
// radius (in points) for every path that requests a soft-edges effect.
func (e *SVGEmitter) collectSoftEdges(node *RenderNode) map[string]float64 {
	result := make(map[string]float64)
	e.collectSoftEdgesRecursive(node, result)
	return result
}

func (e *SVGEmitter) collectSoftEdgesRecursive(node *RenderNode, result map[string]float64) {
	if node == nil {
		return
	}
	for _, p := range node.Geometry {
		if p.SoftEdgesSize > 0 {
			id := fmt.Sprintf("softedges_%s", e.fmtNum(p.SoftEdgesSize))
			result[id] = p.SoftEdgesSize
		}
	}
	for _, child := range node.Children {
		e.collectSoftEdgesRecursive(child, result)
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

// pathHasClosedShape returns true if the path contains a closed sub-path
// that uses an arc command (A) — i.e. a circle/ellipse. Used to identify
// combo markers like lend29/30 (chevron + circle) which need to be drawn
// FORWARD of the line endpoint. Filled triangle markers (lend39 etc.)
// also have Z but draw BEHIND the endpoint, so we don't want to mirror
// them — hence the arc requirement.
func pathHasClosedShape(path string) bool {
	hasArc := false
	hasClose := false
	for _, c := range path {
		switch c {
		case 'A', 'a':
			hasArc = true
		case 'z', 'Z':
			hasClose = true
		}
	}
	return hasArc && hasClose
}

// isVerticalBarOnly returns true if every L command in the path moves only in Y
// from its preceding M (i.e. the path is one or more vertical line segments).
// Used to pick the right stroke-width compensation for bar arrows (24-26).
func isVerticalBarOnly(path string) bool {
	// Tokenize commands + numbers
	hasL := false
	tokens := []string{}
	cur := ""
	for _, c := range path {
		if c == ' ' || c == ',' {
			if cur != "" {
				tokens = append(tokens, cur)
				cur = ""
			}
		} else if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			if cur != "" {
				tokens = append(tokens, cur)
				cur = ""
			}
			tokens = append(tokens, string(c))
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		tokens = append(tokens, cur)
	}
	// Walk: track current point. For each L, check X matches previous point's X.
	var lastX string
	cmd := ""
	idx := 0
	for idx < len(tokens) {
		t := tokens[idx]
		if t == "M" || t == "L" || t == "C" || t == "Z" || t == "z" {
			cmd = t
			idx++
			continue
		}
		if cmd == "C" {
			// Bezier: not a bar
			return false
		}
		if cmd == "M" {
			if idx+1 >= len(tokens) {
				return false
			}
			lastX = tokens[idx]
			idx += 2
			continue
		}
		if cmd == "L" {
			if idx+1 >= len(tokens) {
				return false
			}
			if tokens[idx] != lastX {
				return false // not vertical
			}
			hasL = true
			idx += 2
			continue
		}
		idx++
	}
	return hasL
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

	// Pick orient and path-mirror flag based on arrow type and begin/end role.
	//
	// Path-layout convention (from generator script):
	//   - A-type (RefX=0): apex at viewBox right (10), body at left (0).
	//     These arrows have apex at lend origin = at line attachment.
	//   - B-type (RefX=10): body/back-elements at viewBox left (0),
	//     apex/far-elements at viewBox right (10). These arrows have lend
	//     origin = body of arrow, with apex/elements extending in lend +X.
	//
	// Visio places the arrow so that:
	//   - BEGIN: body at line start, apex/elements forward into line area
	//   - END: body at line end, apex/elements backward into line area
	//
	// For A-type:
	//   BEGIN: use auto-start-reverse so the (viewBox-right) apex lands AT
	//          line start and (viewBox-left) body opens forward into line.
	//   END: use auto. Apex extends forward past line end (the setback then
	//        pulls line back so apex visually sits at original endpoint).
	//
	// For B-type:
	//   BEGIN: use auto with refX=0 OR equivalent — body at line start, apex
	//          forward. With our path (body at viewBox 0, apex at viewBox 10)
	//          and refX=10, we need to mirror the path so body is at viewBox 10
	//          and use auto-start-reverse to flip the orient. OR keep path
	//          and use refX=0 + auto.
	//   END: body at line end, apex backward. Mirror path (body at viewBox 10,
	//        apex at viewBox 0) and use auto with refX=10.
	//
	// We choose to mirror the path for B-type END markers (keeping a single
	// canonical path per type), with refX always at the body position.
	orient := "auto"
	pathStr := m.Path
	refX := m.RefX
	isBackAnchored := m.RefX >= 9.5            // RefX=10 indicates back-anchored
	isCentered := m.RefX > 0.5 && m.RefX < 9.5 // RefX≈5 indicates centered (circle/square)

	if isCentered {
		// Centered shapes (lend10, 11, 20, 21) are symmetric around viewBox 5.
		// Visio offsets them by setback so the shape is mostly OFF the line area:
		//   BEGIN: shape mostly behind line start (line begins after shape)
		//   END:   shape mostly forward of line end (line ends before shape)
		// In our viewBox 0..10 with the shape at center, refX should shift to the
		// appropriate edge. For Setback=W/2 (always true for centered), the offset
		// equals 5 viewBox units exactly. So begin → refX=10, end → refX=0.
		if !m.IsEnd {
			refX = 10
		} else {
			refX = 0
		}
	} else if !m.IsEnd {
		if !isBackAnchored {
			// A-type begin: flip via auto-start-reverse so apex lands at line start
			orient = "auto-start-reverse"
		} else {
			// B-type begin: keep auto; path layout already puts body at viewBox left,
			// apex/elements at viewBox right. Need refX=0 for body at line start.
			refX = 0
		}
	} else if isBackAnchored {
		// B-type end has two sub-cases depending on whether the arrow has
		// a setback (line shortening):
		//   - Setback=0 (bars 23-28, single chevrons): the marker sits ON
		//     the line, BEHIND the visible endpoint (overlapping the last
		//     few sw of line). Keep path as-is with refX=10.
		//   - Setback>0 (combos with circle: 29, 30): line is shortened
		//     and the marker components (circle, bar, connecting line) sit
		//     FORWARD of the shortened endpoint so they sit at/past the
		//     original endpoint position. Mirror path and use refX=0.
		// Distinguish via the arrow's natural setback amount.
		// Note: m has no direct Setback field — we infer via the path layout.
		// Arrows with a closed shape (circle) at the +X end of their natural
		// lend coords need mirroring; bar-only arrows don't.
		if pathHasClosedShape(m.Path) {
			pathStr = mirrorPathX(m.Path)
			refX = 0
		}
		// else: keep path and refX=10 (bars BEHIND line endpoint)
	}

	// preserveAspectRatio="none" lets the 10x10 path stretch to match
	// markerWidth/markerHeight independently — without it, "xMidYMid meet"
	// forces uniform scaling and squashes elongated arrows back into a
	// square aspect.
	//
	// overflow="visible" lets bezier-based arrows (5,6,17,18,19) render
	// their slight overhang outside viewBox.
	//
	// Stroke-width compensates for viewBox-to-marker stretching so the rendered
	// outline thickness is 1 strokeWidth unit (= line thickness, like Visio).
	// Strategy depends on stroke direction in the path:
	//   - Vertical-only strokes (bar arrows 24/25/26): rendered width =
	//     stroke-width × horizontal_scale, so set stroke-width = 10/markerWidth
	//   - Otherwise (mixed/diagonal strokes): use average of horiz+vert scales,
	//     which works well for triangles, diamonds, circles
	horizScale := m.Width / 10.0
	vertScale := m.Height / 10.0
	avgScale := (horizScale + vertScale) / 2.0
	strokeW := 1.0
	if avgScale > 0 {
		strokeW = 1.0 / avgScale
	}
	// Detect vertical-bar-only paths: contains "M X _ L X _" patterns with
	// matching X values (no horizontal or diagonal strokes).
	if isVerticalBarOnly(pathStr) && horizScale > 0 {
		strokeW = 1.0 / horizScale
	}
	return fmt.Sprintf(`    <marker id="%s" viewBox="0 0 %s %s" refX="%s" refY="%s" markerWidth="%s" markerHeight="%s" markerUnits="strokeWidth" orient="%s" overflow="visible" preserveAspectRatio="none"><path d="%s" fill="%s" stroke="%s" stroke-width="%s" stroke-linecap="round" stroke-linejoin="round"/></marker>`,
		m.ID,
		e.fmtNum(10), e.fmtNum(10),
		e.fmtNum(refX), e.fmtNum(m.RefY),
		e.fmtNum(m.Width), e.fmtNum(m.Height),
		orient,
		pathStr, fillAttr, strokeAttr, e.fmtNum(strokeW))
}

// mirrorPathX flips a path's X coordinates around viewBox center 5 (10-x).
// Used to convert back-anchored arrow paths between BEGIN and END layouts.
func mirrorPathX(path string) string {
	var b strings.Builder
	tokens := []string{}
	cur := ""
	for _, c := range path {
		if c == ' ' || c == ',' {
			if cur != "" {
				tokens = append(tokens, cur)
				cur = ""
			}
		} else if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			if cur != "" {
				tokens = append(tokens, cur)
				cur = ""
			}
			tokens = append(tokens, string(c))
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		tokens = append(tokens, cur)
	}
	// Walk tokens, flip X on M/L/C coordinate pairs.
	cmd := ""
	idx := 0
	first := true
	for idx < len(tokens) {
		t := tokens[idx]
		if isAlpha(t) {
			cmd = t
			if !first {
				b.WriteByte(' ')
			}
			b.WriteString(t)
			first = false
			idx++
			continue
		}
		switch cmd {
		case "M", "L":
			if idx+1 >= len(tokens) {
				break
			}
			x, _ := strconv.ParseFloat(tokens[idx], 64)
			b.WriteByte(' ')
			b.WriteString(fmtMirrorCoord(10 - x))
			b.WriteByte(' ')
			b.WriteString(tokens[idx+1])
			idx += 2
		case "C":
			// Three X,Y pairs: control1, control2, end
			for k := 0; k < 3; k++ {
				if idx+1 >= len(tokens) {
					break
				}
				x, _ := strconv.ParseFloat(tokens[idx], 64)
				b.WriteByte(' ')
				b.WriteString(fmtMirrorCoord(10 - x))
				b.WriteByte(' ')
				b.WriteString(tokens[idx+1])
				idx += 2
			}
		default:
			if !first {
				b.WriteByte(' ')
			}
			b.WriteString(t)
			idx++
		}
	}
	return b.String()
}

func isAlpha(s string) bool {
	if len(s) != 1 {
		return false
	}
	c := s[0]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func fmtMirrorCoord(v float64) string {
	s := fmt.Sprintf("%.3f", v)
	// trim trailing zeros and trailing dot
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	if s == "-0" || s == "" {
		s = "0"
	}
	return s
}

func (e *SVGEmitter) emitGeometry(svg *strings.Builder, node *RenderNode) {
	if !node.Visible {
		return
	}

	// A Foreign embedded image (device icon, photo, …) forms the backdrop of
	// the shape. Emit it before children and geometry so any border or label
	// draws on top of it, matching Visio's z-order for Foreign shapes.
	if node.Image != nil {
		e.emitImage(svg, node.Image)
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
	if path.SoftEdgesSize > 0 {
		// Filter was registered in <defs> by the pre-walk; just reference it.
		filterID := fmt.Sprintf("softedges_%s", e.fmtNum(path.SoftEdgesSize))
		attrs = append(attrs, fmt.Sprintf(`filter="url(#%s)"`, filterID))
	}

	svg.WriteString(fmt.Sprintf(`  <path d="%s" %s/>`, path.D, strings.Join(attrs, " ")))
	svg.WriteByte('\n')
}

// emitImage emits an <image> element for a Foreign shape's embedded image.
// The href carries a base64 data URI so the SVG is self-contained (no external
// fetches). preserveAspectRatio="none" stretches the image to the shape box,
// matching how Visio sizes Foreign images to ImgWidth × ImgHeight.
func (e *SVGEmitter) emitImage(svg *strings.Builder, im *RenderImage) {
	if im == nil || im.DataURI == "" {
		return
	}
	svg.WriteString(fmt.Sprintf(`  <image x="%s" y="%s" width="%s" height="%s" preserveAspectRatio="none" href="%s"/>`,
		e.fmtNum(im.X), e.fmtNum(im.Y), e.fmtNum(im.Width), e.fmtNum(im.Height), im.DataURI))
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

		// Visio draws a white rect behind text on connectors so the line
		// doesn't bisect it. Estimate text bounds from font size and content.
		if t.BackgroundFill != "" {
			// Approximate text width: avg glyph ~0.5em (Calibri-like).
			lines := t.Lines
			if len(lines) == 0 {
				lines = []string{t.Content}
			}
			maxLen := 0
			for _, line := range lines {
				if n := len([]rune(line)); n > maxLen {
					maxLen = n
				}
			}
			widthEst := float64(maxLen) * t.FontSize * 0.5
			lineCount := len(lines)
			if lineCount == 0 {
				lineCount = 1
			}
			heightEst := t.FontSize * 0.8 * float64(lineCount)
			// position rect based on text-anchor
			var rectX float64
			switch t.TextAnchor {
			case "end":
				rectX = t.X - widthEst
			case "middle":
				rectX = t.X - widthEst/2
			default:
				rectX = t.X
			}
			// alphabetic baseline sits ~0.8em below top of glyph box
			rectY := t.Y - t.FontSize*0.75
			svg.WriteString(fmt.Sprintf(`  <rect x="%s" y="%s" width="%s" height="%s" fill="%s" stroke="none"/>`,
				e.fmtNum(rectX), e.fmtNum(rectY),
				e.fmtNum(widthEst), e.fmtNum(heightEst),
				t.BackgroundFill))
			svg.WriteByte('\n')
		}

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
		if t.WritingMode != "" {
			svg.WriteString(fmt.Sprintf(` style="writing-mode: %s; text-orientation: upright"`, t.WritingMode))
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
