package vsdx

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// RenderNode represents a node in the render tree.
// Each node contains all pre-computed information needed for SVG emission.
type RenderNode struct {
	Shape     *Shape
	Style     *EffectiveStyle
	Transform *ShapeTransform
	Geometry  []*ResolvedPath
	Text      *ResolvedText
	Children  []*RenderNode
	ZOrder    int
	Visible   bool
	// Rotation angle in radians (from shape's Angle cell)
	Angle float64
	// Rotation center in SVG coordinates (for applying rotation transform)
	RotationCenterX float64
	RotationCenterY float64
	// Marker definitions needed by this node's geometry
	Markers map[string]*MarkerDef
	// Gradient definitions needed by this node
	Gradients map[string]*Gradient
	// Shadow filter definitions needed by this node
	Shadows map[string]*Shadow
	// Fill pattern definitions needed by this node (hatching patterns 2-24)
	FillPatterns map[string]*FillPatternDef
}

// FillPatternDef contains a fill pattern definition for hatching.
type FillPatternDef struct {
	ID          string
	PatternType int     // Visio pattern type (2-24)
	ForeColor   string  // Pattern foreground color
	BackColor   string  // Pattern background color
	Scale       float64 // Pattern scale factor
}

// ResolvedPath contains pre-computed SVG path data ready for emission.
// All geometry resolution, transformations, and arrow setbacks have been applied.
type ResolvedPath struct {
	D             string  // SVG path data ready for emission (FINAL - no further mutation)
	Fill          string  // resolved fill color
	Stroke        string  // resolved stroke color
	StrokeWidth   float64 // stroke width in SVG units
	DashArray     string  // stroke-dasharray value
	LineCap       string  // stroke-linecap: "round", "square", "butt"
	FillOpacity   float64 // fill opacity (0-1)
	VectorEffect  string  // vector-effect attribute value
	MarkerStartID string  // marker-start URL (empty if none)
	MarkerEndID   string  // marker-end URL (empty if none)
	GradientID    string  // gradient definition ID
	FillPatternID string  // fill pattern ID for hatching (patterns 2-24)
	FilterID      string  // filter (shadow) ID
	NoShow        bool    // geometry-level hidden
}

// MarkerDef contains a resolved marker (arrow) definition.
type MarkerDef struct {
	ID        string
	ArrowType int
	Size      int
	Color     string
	IsEnd     bool
	Path      string  // SVG path for the marker
	Width     float64
	Height    float64
	RefX      float64
	RefY      float64
	Filled    bool
}

// ResolvedText contains pre-computed text rendering data.
type ResolvedText struct {
	Content        string
	X, Y           float64 // position in world coordinates
	FontSize       float64 // in SVG units
	FontFamily     string  // font family (e.g., "Calibri, sans-serif")
	FontWeight     string  // "normal" or "bold"
	FontStyle      string  // "normal" or "italic"
	TextDecoration string  // "none", "underline", "line-through", "overline", or combined
	Fill           string  // text color
	TextAnchor     string  // "start", "middle", or "end"
	Baseline       string  // "hanging", "middle", or "alphabetic"
	Transform      string  // rotation transform
	Lines          []string // for multi-line text
	LineHeight     float64
	SmallCaps      bool    // render as small capitals
	TextPos        int     // 0=normal, 1=superscript, 2=subscript
	// BackgroundFill is the fill colour for an opaque rectangle drawn behind
	// the text (matches Visio's textBkgnd rect for connector labels so the
	// line doesn't cut through the text). Empty = no background.
	BackgroundFill string
}

// RenderTreeBuilder builds a render tree from a shape hierarchy.
type RenderTreeBuilder struct {
	root       *Shape
	transforms map[string]*ShapeTransform
	scaleX     float64
	scaleY     float64
	precision  int
	colorFreq  map[string]int // tracks color usage for brand detection
	rootH      float64        // root shape height - used for ALL Y-flip calculations
}

// RenderTreeOptions configures the render tree building.
type RenderTreeOptions struct {
	Width     float64 // output width
	Height    float64 // output height
	Precision int     // decimal precision
}

// NewRenderTreeBuilder creates a builder for the given shape.
func NewRenderTreeBuilder(shape *Shape) *RenderTreeBuilder {
	return &RenderTreeBuilder{
		root:       shape,
		transforms: ComputeGroupTransforms(shape),
		scaleX:     1.0,
		scaleY:     1.0,
		precision:  2,
		colorFreq:  make(map[string]int),
	}
}

// WithOptions sets rendering options.
func (b *RenderTreeBuilder) WithOptions(opts RenderTreeOptions) *RenderTreeBuilder {
	shapeW := math.Abs(b.root.Width())
	shapeH := math.Abs(b.root.Height())
	if shapeW == 0 {
		shapeW = 1
	}
	if shapeH == 0 {
		shapeH = 1
	}

	// Uniform scale
	scale := opts.Width / shapeW
	if s := opts.Height / shapeH; s < scale {
		scale = s
	}
	b.scaleX = scale
	b.scaleY = scale
	b.precision = opts.Precision
	return b
}

// Build constructs the render tree with all geometry resolved.
func (b *RenderTreeBuilder) Build() *RenderNode {
	// Calculate scale factors
	shapeW := math.Abs(b.root.Width())
	shapeH := math.Abs(b.root.Height())
	if shapeW == 0 {
		shapeW = 1
	}
	if shapeH == 0 {
		shapeH = 1
	}

	// Store root height for Y-flip calculations (same as legacy renderer behavior)
	b.rootH = shapeH

	return b.buildNodeWithOffset(b.root, 0, shapeW, shapeH, 0, 0)
}

// BuildWithScale constructs the render tree with explicit scale factors.
func (b *RenderTreeBuilder) BuildWithScale(scaleX, scaleY float64, precision int) *RenderNode {
	b.scaleX = scaleX
	b.scaleY = scaleY
	b.precision = precision

	shapeW := math.Abs(b.root.Width())
	shapeH := math.Abs(b.root.Height())
	if shapeW == 0 {
		shapeW = 1
	}
	if shapeH == 0 {
		shapeH = 1
	}

	// Store root height for Y-flip calculations (same as legacy renderer behavior)
	b.rootH = shapeH

	return b.buildNodeWithOffset(b.root, 0, shapeW, shapeH, 0, 0)
}

func (b *RenderTreeBuilder) buildNodeWithOffset(shape *Shape, zOrder int, parentW, parentH, offsetX, offsetY float64) *RenderNode {
	node := &RenderNode{
		Shape:     shape,
		Style:     shape.ComputeEffectiveStyle(),
		Transform: b.transforms[shape.ID],
		ZOrder:    zOrder,
		Visible:   true,
		Markers:   make(map[string]*MarkerDef),
		Gradients: make(map[string]*Gradient),
		Shadows:   make(map[string]*Shadow),
	}

	// Check visibility
	if node.Style.NoShow {
		node.Visible = false
	}

	// Resolve geometry for visible shapes
	if node.Visible {
		node.Geometry = b.resolveAllGeometryWithOffset(shape, node.Style, parentW, parentH, offsetX, offsetY, node)
	}

	// Resolve text only for shapes with geometry.
	// Group shapes without geometry should not render their own text -
	// their text is typically for metadata/labeling, not display.
	// This matches legacy behavior where text is rendered per geometry.
	if node.Visible && shape.Text() != "" && len(node.Geometry) > 0 {
		negH := shape.Height() < 0
		// Use b.rootH for Y-flip (matches legacy behavior)
		node.Text = b.resolveText(shape, node.Style, node.Transform, offsetX, offsetY, b.rootH, negH)
	}

	// Build children recursively
	children := shape.ChildShapes()
	if len(children) > 0 {
		// Get child dimensions
		childW := math.Abs(shape.Width())
		childH := math.Abs(shape.Height())
		if childW == 0 {
			childW = parentW
		}
		if childH == 0 {
			childH = parentH
		}

		// Sort children by z-order if available
		sortedChildren := make([]*Shape, len(children))
		copy(sortedChildren, children)
		sort.SliceStable(sortedChildren, func(i, j int) bool {
			// Use OrderIndex if available
			oi := toFloat(sortedChildren[i].CellValue("OrderIndex"))
			oj := toFloat(sortedChildren[j].CellValue("OrderIndex"))
			return oi < oj
		})

		for i, child := range sortedChildren {
			// Calculate child offset within parent group's space
			// PinX/PinY is center position; LocPinX/LocPinY is center offset within shape
			//
			// X offset: Accumulate across hierarchy because X transform is linear scaling.
			// Y offset: Must also accumulate, but needs careful handling due to Y-flip.
			// The accumulated offsetY is in the shape's local coordinate space (Y-up),
			// and the Y-flip happens at the geometry resolver level using parentH.
			childOffsetX := offsetX + child.X() - child.LocX()
			childOffsetY := offsetY + child.Y() - child.LocY()
			// Use the GROUP's height as parentH for children, not the outer parentH
			childNode := b.buildNodeWithOffset(child, zOrder+i+1, childW, childH, childOffsetX, childOffsetY)

			// Capture child's rotation angle (radians)
			childAngle := toFloat(child.CellValue("Angle"))
			if childAngle != 0 {
				childNode.Angle = childAngle
				// Calculate rotation center in SVG coordinates
				// Rotation is around the LocPin point, which is at (LocPinX, LocPinY) in local coords.
				// After positioning, LocPin is at (childOffsetX + LocPinX, childOffsetY + LocPinY).
				locPinX := child.LocX()
				locPinY := child.LocY()
				childNode.RotationCenterX = (childOffsetX + locPinX) * b.scaleX
				childNode.RotationCenterY = (b.rootH - childOffsetY - locPinY) * b.scaleY
			}

			node.Children = append(node.Children, childNode)
		}
	}

	return node
}

func (b *RenderTreeBuilder) resolveAllGeometryWithOffset(shape *Shape, style *EffectiveStyle, parentW, parentH, offsetX, offsetY float64, node *RenderNode) []*ResolvedPath {
	var paths []*ResolvedPath

	geoms := shape.Geometries
	if len(geoms) == 0 {
		if ms := shape.MasterShape(); ms != nil {
			geoms = ms.Geometries
		}
	}

	shapeW := math.Abs(shape.Width())
	shapeH := shape.Height()
	negativeH := shapeH < 0
	if shapeH < 0 {
		shapeH = -shapeH
	}
	if shapeW == 0 {
		shapeW = parentW
	}
	if shapeH == 0 {
		shapeH = parentH
	}

	totalGeoms := len(geoms)
	geomIndex := 0
	for _, geom := range geoms {
		if len(geom.Rows) == 0 {
			continue
		}

		// Use the geometry resolver
		// IMPORTANT: Use b.rootH for Y-flip, not parentH.
		// The legacy renderer uses the top-level shape's height for ALL shapes,
		// regardless of nesting. This ensures correct Y-flip when offsetY
		// accumulates across multiple hierarchy levels.
		result := ResolveGeometry(
			shape,
			geom,
			style,
			shapeW, shapeH, // localW, localH
			offsetX, offsetY, // offset in parent space
			b.scaleX, b.scaleY,
			b.rootH, // rootH for Y-flip (matches legacy behavior)
			negativeH,
			b.precision,
			geomIndex, totalGeoms,
		)
		geomIndex++

		if result == nil || result.NoShow {
			continue
		}

		path := &ResolvedPath{
			D:             result.PathData,
			Fill:          result.Fill,
			Stroke:        result.Stroke,
			StrokeWidth:   result.StrokeWidth,
			DashArray:     result.DashArray,
			LineCap:       result.LineCap,
			FillOpacity:   result.FillOpacity,
			VectorEffect:  result.VectorEffect,
			MarkerStartID: result.MarkerStart,
			MarkerEndID:   result.MarkerEnd,
			GradientID:    result.GradientID,
			FillPatternID: result.FillPatternID,
			FilterID:      result.FilterID,
			NoShow:        result.NoShow,
		}

		// Collect fill pattern definitions
		if result.FillPatternID != "" {
			if node.FillPatterns == nil {
				node.FillPatterns = make(map[string]*FillPatternDef)
			}
			patDef := createFillPatternDef(style.FillPattern, style.FillForegnd, style.FillBkgnd)
			if patDef != nil {
				node.FillPatterns[patDef.ID] = patDef
			}
		}

		// Track colors for brand detection
		if result.Fill != "" && result.Fill != "none" {
			b.colorFreq[strings.ToUpper(result.Fill)]++
		}
		if result.Stroke != "" && result.Stroke != "none" {
			b.colorFreq[strings.ToUpper(result.Stroke)]++
		}

		// Collect marker definitions
		if result.MarkerStart != "" {
			markerID := extractMarkerID(result.MarkerStart)
			if markerID != "" {
				node.Markers[markerID] = b.createMarkerDef(style.BeginArrow, style.BeginArrowSize, result.Stroke, result.StrokeWidth, false)
			}
		}
		if result.MarkerEnd != "" {
			markerID := extractMarkerID(result.MarkerEnd)
			if markerID != "" {
				node.Markers[markerID] = b.createMarkerDef(style.EndArrow, style.EndArrowSize, result.Stroke, result.StrokeWidth, true)
			}
		}

		// Check for gradient fill
		if gradient := shape.FillGradient(); gradient != nil && style.FillPattern != 0 {
			gradID := fmt.Sprintf("grad_%s", shape.ID)
			path.GradientID = gradID
			path.Fill = fmt.Sprintf("url(#%s)", gradID)
			node.Gradients[gradID] = gradient
		}

		// Create shadow if enabled
		if style.HasShadow() {
			shadowID := fmt.Sprintf("shadow_%s_%d", shape.ID, geomIndex)
			shadow := &Shadow{
				Type:    style.ShapeShdwType,
				OffsetX: style.ShapeShdwOffsetX / 72.0, // points to inches
				OffsetY: style.ShapeShdwOffsetY / 72.0, // points to inches
				Color:   style.ShdwForegnd,
				Opacity: style.ShadowOpacity(),
				Blur:    style.ShapeShdwBlur / 72.0, // points to inches
				Scale:   style.ShapeShdwScaleFactor,
			}
			if shadow.Color == "" {
				shadow.Color = "#808080"
			}
			node.Shadows[shadowID] = shadow
			path.FilterID = shadowID
		}

		paths = append(paths, path)
	}

	return paths
}

func extractMarkerID(url string) string {
	// Extract ID from "url(#id)"
	if strings.HasPrefix(url, "url(#") && strings.HasSuffix(url, ")") {
		return url[5 : len(url)-1]
	}
	return ""
}

func (b *RenderTreeBuilder) createMarkerDef(arrowType, arrowSize int, color string, strokeWidth float64, isEnd bool) *MarkerDef {
	def, ok := visioArrowTypes[arrowType]
	if !ok || def.Path == "" {
		def = visioArrowTypes[1] // fallback to triangle
	}

	sizeMult := 1.0
	if arrowSize >= 0 && arrowSize < len(arrowSizeMultipliers) {
		sizeMult = arrowSizeMultipliers[arrowSize]
	}

	// Visio's setback fit (size 2): visualWidth = W × sizeMult × (5.17 + 1.85 sw) / 2
	// where W is Visio's natural lend width and "÷ 2" normalises against lend4
	// (W=2, the reference). The marker is rendered at:
	//   markerWidth  = W × naturalUnit (in stroke-width units)
	//   markerHeight = H × naturalUnit
	// so each arrow keeps Visio's natural per-type aspect (lend1 = 1:2 tall,
	// lend22 = 2:1 long diamond, lend10 = 1:1 circle, etc.).
	visualWidthAbs := def.W * sizeMult * (5.17 + 1.85*strokeWidth) / 2.0
	var naturalUnit float64 // size of "1 lend unit" in stroke-width units
	if strokeWidth > 0 {
		naturalUnit = sizeMult * (5.17 + 1.85*strokeWidth) / (2.0 * strokeWidth)
	} else {
		naturalUnit = 7.0 * sizeMult / 2.0
	}
	_ = visualWidthAbs // referenced for documentation; setback formula uses same math
	markerWidthSU := def.W * naturalUnit
	markerHeightSU := def.H * naturalUnit

	pos := "start"
	if isEnd {
		pos = "end"
	}

	// Include stroke width in marker ID since dimensions depend on it
	swKey := fmt.Sprintf("%.0f", strokeWidth*10) // e.g., "30" for 3pt, "10" for 1pt

	return &MarkerDef{
		ID:        fmt.Sprintf("arrow_%d_%d_%s_%s_%s", arrowType, arrowSize, strings.TrimPrefix(color, "#"), swKey, pos),
		ArrowType: arrowType,
		Size:      arrowSize,
		Color:     color,
		IsEnd:     isEnd,
		Path:      def.Path,
		Width:     markerWidthSU,
		Height:    markerHeightSU,
		RefX:      def.RefX,
		RefY:      def.RefY,
		Filled:    def.Filled,
	}
}

func (b *RenderTreeBuilder) resolveText(shape *Shape, style *EffectiveStyle, transform *ShapeTransform, offsetX, offsetY, parentH float64, negativeH bool) *ResolvedText {
	// FontSize is in points (72 pt/inch), convert to SVG units
	fontSizeSVG := (style.FontSize / 72.0) * ((b.scaleX + b.scaleY) / 2)

	// Determine text color - use white for dark fills (only if fill is visible)
	textColor := style.TextColor
	if textColor == "" || textColor == "#000000" {
		// Only use white text if fill is actually visible (FillPattern != 0)
		hasFill := style.FillPattern != 0 && style.FillForegnd != "" && style.FillForegnd != "none"
		if hasFill && isDarkColor(style.FillForegnd) {
			textColor = "#FFFFFF"
		} else if textColor == "" {
			textColor = "#000000"
		}
	}

	text := &ResolvedText{
		Content:    shape.Text(),
		FontSize:   fontSizeSVG,
		FontFamily: "", // Use default font for better cross-platform rendering
		FontWeight: "normal",
		FontStyle:  "normal",
		Fill:       textColor,
		TextAnchor: "middle",
		Baseline:   "alphabetic",
	}

	// For 1D connector shapes (have explicit Begin/End endpoints) Visio draws
	// a white rectangle behind the text so the connector line doesn't bisect
	// it. Detect a connector by the presence of BeginX/EndX cells.
	if shape.CellValue("BeginX") != "" || shape.CellValue("EndX") != "" {
		text.BackgroundFill = "#FFFFFF"
	}

	if style.Bold {
		text.FontWeight = "bold"
	}
	if style.Italic {
		text.FontStyle = "italic"
	}

	// Text decoration (underline, strikethrough, overline)
	var decorations []string
	if style.Underline {
		decorations = append(decorations, "underline")
	}
	if style.Overline {
		decorations = append(decorations, "overline")
	}
	if style.Strikethrough || style.DoubleStrikethrough {
		decorations = append(decorations, "line-through")
	}
	if len(decorations) > 0 {
		text.TextDecoration = strings.Join(decorations, " ")
	}

	// Small caps and text position (superscript/subscript)
	text.SmallCaps = style.SmallCaps
	text.TextPos = style.TextPos

	// Shape dimensions
	shapeW := math.Abs(shape.Width())
	shapeH := math.Abs(shape.Height())
	if shapeW == 0 {
		shapeW = 1
	}
	if shapeH == 0 {
		shapeH = 1
	}

	// Get text block position - defaults to center of shape.
	// TxtPinX/Y is in shape-local coords (relative to shape's lower-left bbox corner)
	// and may fall outside the bbox (e.g. for connector labels positioned along a curve).
	txtPinXCell := shape.CellValue("TxtPinX")
	txtPinYCell := shape.CellValue("TxtPinY")
	txtPinX := toFloat(txtPinXCell)
	txtPinY := toFloat(txtPinYCell)
	txtPinXSet := txtPinXCell != ""
	txtPinYSet := txtPinYCell != ""
	if !txtPinXSet {
		txtPinX = shapeW / 2
	}
	if !txtPinYSet {
		txtPinY = shapeH / 2
	}

	// Get text angle (rotation in radians)
	txtAngle := toFloat(shape.CellValue("TxtAngle"))

	// Vertical alignment: 0=top, 1=middle, 2=bottom
	vertAlign := 1
	if v := shape.CellValue("VerticalAlign"); v != "" {
		vertAlign = int(toFloat(v))
	}

	// Horizontal alignment from style
	horzAlign := style.TextAlign
	if horzAlign == 0 && shape.CellValue("Para.HorzAlign") == "" {
		horzAlign = 1 // default to center
	}

	// Margins - MS-VSDX default is 4pt = 0.0555556 inches
	const defaultMargin = 4.0 / 72.0 // 4pt in inches
	leftMargin := defaultMargin
	rightMargin := defaultMargin
	topMargin := defaultMargin
	bottomMargin := defaultMargin
	if v := shape.CellValue("LeftMargin"); v != "" {
		leftMargin = toFloat(v)
	}
	if v := shape.CellValue("RightMargin"); v != "" {
		rightMargin = toFloat(v)
	}
	if v := shape.CellValue("TopMargin"); v != "" {
		topMargin = toFloat(v)
	}
	if v := shape.CellValue("BottomMargin"); v != "" {
		bottomMargin = toFloat(v)
	}

	// Calculate text position in shape local coordinates
	textX := txtPinX + leftMargin
	var textY float64

	// Special case: shapes with zero height (connectors, InfiniteLine)
	// Text should be positioned at Y=0 (bottom in Visio coords, line position)
	zeroHeight := math.Abs(shape.Height()) < 0.001

	if txtPinYSet {
		textY = txtPinY
	} else {
		switch vertAlign {
		case 0: // Top
			textY = shapeH - topMargin
		case 2: // Bottom
			textY = bottomMargin
		default: // Middle
			if zeroHeight {
				// For zero-height shapes, position at geometry line (Y=0)
				textY = 0
			} else {
				textY = shapeH / 2
			}
		}
	}

	// SVG text-anchor based on horizontal alignment.
	// If TxtPinX is explicitly set, use it as the text block reference position
	// (text-anchor then aligns the text relative to that point). Otherwise fall
	// back to shape-bbox-based positioning.
	switch horzAlign {
	case 0: // Left
		text.TextAnchor = "start"
		if !txtPinXSet {
			textX = leftMargin
		}
	case 2: // Right
		text.TextAnchor = "end"
		if !txtPinXSet {
			textX = shapeW - rightMargin
		}
	default: // Center
		text.TextAnchor = "middle"
		if !txtPinXSet {
			textX = shapeW / 2
		}
	}

	// Calculate available width for text wrapping
	availableWidth := (shapeW - leftMargin - rightMargin) * ((b.scaleX + b.scaleY) / 2)
	charWidth := fontSizeSVG * 0.45 // approximate character width

	// Visio uses alphabetic baseline positioning with additional line-height padding.
	// For top-aligned text, add offset to match Visio's text leading.
	fontSizeInches := fontSizeSVG / 72.0
	switch vertAlign {
	case 0:
		text.Baseline = "hanging"
		// Visio adds slight leading above the first line
		textY -= fontSizeInches * 0.15
	case 2:
		text.Baseline = "alphabetic"
	default:
		// Middle alignment: use alphabetic baseline and offset Y
		text.Baseline = "alphabetic"
		// Add 0.3×font_size offset (in Visio coordinates, before scaling)
		textY -= (fontSizeSVG * 0.3) / b.scaleY
	}

	// Wrap text to fit within shape width
	lines := wrapTextLines(strings.TrimSpace(text.Content), availableWidth, charWidth)

	// Remove empty trailing lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	if len(lines) > 1 {
		text.Lines = lines
		// Line height is typically 1.2x font size
		text.LineHeight = fontSizeSVG * 1.2

		// Adjust Y position for multiline based on vertical alignment
		totalTextHeight := text.LineHeight * float64(len(lines))
		switch vertAlign {
		case 1: // Middle - center the text block
			textY += (totalTextHeight - text.LineHeight) / (2 * b.scaleY)
		case 2: // Bottom - offset up by total height
			textY += (totalTextHeight - text.LineHeight) / b.scaleY
		}
	}

	// Convert to SVG coordinates with group offset
	// X: Add group offset (accumulated across hierarchy)
	// Y: Transform to parent space then flip using parent height
	text.X = (textX + offsetX) * b.scaleX
	if negativeH {
		// For negative height shapes, use same transform as geometry
		text.Y = -(textY + offsetY) * b.scaleY
	} else {
		text.Y = (parentH - (textY + offsetY)) * b.scaleY
	}

	// Handle text rotation
	if txtAngle != 0 {
		angleDeg := -txtAngle * 180 / math.Pi
		text.Transform = fmt.Sprintf("rotate(%s %s %s)",
			fmtPrec(angleDeg, b.precision),
			fmtPrec(text.X, b.precision),
			fmtPrec(text.Y, b.precision))
	}

	return text
}

// wrapTextLines splits text into lines that fit within the given width.
func wrapTextLines(text string, maxWidth, charWidth float64) []string {
	if maxWidth <= 0 || charWidth <= 0 {
		return []string{text}
	}

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
		spaceWidth := charWidth * 0.6 // spaces are narrower than average chars

		for i, word := range words {
			wordWidth := float64(len(word)) * charWidth

			// Check if next word is short (like "&") - prefer keeping it with following word
			keepWithNext := len(word) <= 2 && i < len(words)-1

			if i == 0 {
				currentLine.WriteString(word)
				currentWidth = wordWidth
			} else if currentWidth+spaceWidth+wordWidth <= maxWidth && !keepWithNext {
				currentLine.WriteString(" ")
				currentLine.WriteString(word)
				currentWidth += spaceWidth + wordWidth
			} else if keepWithNext && currentWidth+spaceWidth+wordWidth <= maxWidth {
				// Short word fits, but check if it + next word would also fit
				nextWordWidth := float64(len(words[i+1])) * charWidth
				if currentWidth+spaceWidth+wordWidth+spaceWidth+nextWordWidth <= maxWidth {
					// Both fit, add this word
					currentLine.WriteString(" ")
					currentLine.WriteString(word)
					currentWidth += spaceWidth + wordWidth
				} else {
					// Won't fit with next word, break now to keep short word with next
					result = append(result, currentLine.String())
					currentLine.Reset()
					currentLine.WriteString(word)
					currentWidth = wordWidth
				}
			} else {
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

// InspectRenderTree returns a debug representation of the render tree.
func InspectRenderTree(node *RenderNode) string {
	var sb strings.Builder
	inspectNodeRecursive(node, &sb, 0)
	return sb.String()
}

func inspectNodeRecursive(node *RenderNode, sb *strings.Builder, depth int) {
	indent := strings.Repeat("  ", depth)

	sb.WriteString(fmt.Sprintf("%sNode: Shape=%s ZOrder=%d Visible=%v\n",
		indent, node.Shape.ID, node.ZOrder, node.Visible))

	if node.Style != nil {
		sb.WriteString(fmt.Sprintf("%s  Style: Fill=%s Stroke=%s LineWeight=%.2f\n",
			indent, node.Style.EffectiveFillColor(), node.Style.EffectiveLineColor(), node.Style.LineWeight))
		if node.Style.BeginArrow > 0 || node.Style.EndArrow > 0 {
			sb.WriteString(fmt.Sprintf("%s  Arrows: Begin=%d End=%d\n",
				indent, node.Style.BeginArrow, node.Style.EndArrow))
		}
	}

	if node.Transform != nil {
		sb.WriteString(fmt.Sprintf("%s  WorldBounds: (%.2f,%.2f) %.2fx%.2f\n",
			indent, node.Transform.WorldBounds.X, node.Transform.WorldBounds.Y,
			node.Transform.WorldBounds.Width, node.Transform.WorldBounds.Height))
	}

	if len(node.Geometry) > 0 {
		sb.WriteString(fmt.Sprintf("%s  Geometry: %d paths\n", indent, len(node.Geometry)))
	}

	if node.Text != nil {
		sb.WriteString(fmt.Sprintf("%s  Text: %q\n", indent, truncateString(node.Text.Content, 30)))
	}

	for _, child := range node.Children {
		inspectNodeRecursive(child, sb, depth+1)
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// BrandColor returns the most frequent non-white, non-black color detected during tree building.
func (b *RenderTreeBuilder) BrandColor() string {
	return detectBrandColor(b.colorFreq)
}
