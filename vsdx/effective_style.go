package vsdx

import (
	"strings"
)

// EffectiveStyle holds fully resolved style properties for a shape.
// This is computed BEFORE SVG generation to ensure consistent rendering.
// Resolution order: local cell → master shape → style sheet → theme/QuickStyle → defaults.
type EffectiveStyle struct {
	// Line properties
	LineColor     string  // Resolved RGB color
	LineWeight    float64 // Points
	LinePattern   int     // 0=none, 1=solid, 2-24=patterns
	LineCap       int     // 0=round, 1=square, 2=extended
	LineColorTrans float64 // Transparency 0-1

	// Arrow properties
	BeginArrow     int     // Arrow type index (0=none, 1-45=types)
	EndArrow       int     // Arrow type index
	BeginArrowSize int     // Size index (0-6)
	EndArrowSize   int     // Size index
	BeginArrowSetback float64 // Path shortening in points
	EndArrowSetback   float64 // Path shortening in points

	// Fill properties
	FillForegnd     string  // Foreground fill color
	FillBkgnd       string  // Background fill color
	FillPattern     int     // Fill pattern (0=none, 1=solid, 2-24=patterns)
	FillForegndTrans float64 // Foreground transparency 0-1
	FillBkgndTrans   float64 // Background transparency 0-1

	// Shadow properties
	ShdwForegnd       string  // Shadow color
	ShdwForegndTrans  float64 // Shadow transparency
	ShapeShdwType     int     // Shadow type (0=none, 1=simple, 2=oblique)
	ShapeShdwOffsetX  float64 // X offset in points
	ShapeShdwOffsetY  float64 // Y offset in points
	ShapeShdwBlur     float64 // Blur radius in points
	ShapeShdwScaleFactor float64 // Scale factor

	// Text properties
	TextColor   string  // Text color
	FontSize    float64 // Font size in points
	FontID      int     // Font table index
	Bold        bool
	Italic      bool
	TextAlign   int // 0=left, 1=center, 2=right

	// Geometry visibility
	NoLine bool // Hide line
	NoFill bool // Hide fill
	NoShow bool // Hide entire shape

	// Source tracking (for debugging)
	LineColorSource string
	FillColorSource string
	ArrowSource     string
}

// ComputeEffectiveStyle resolves all style properties for a shape through
// the complete inheritance chain: local → master → style sheet → theme → defaults.
func (s *Shape) ComputeEffectiveStyle() *EffectiveStyle {
	es := &EffectiveStyle{
		// Defaults per MS-VSDX §2.2.5.3.3.1
		LineColor:     "#000000",
		LineWeight:    0.01, // 0.01 inches = 0.72 pts
		LinePattern:   1,    // Solid
		LineCap:       0,    // Round
		BeginArrowSize: 2,   // Default medium
		EndArrowSize:  2,
		FillForegnd:   "#FFFFFF",
		FillBkgnd:     "#FFFFFF",
		FillPattern:   1, // Solid
		ShdwForegnd:   "#808080",
		ShapeShdwScaleFactor: 1.0,
		TextColor:     "#000000",
		FontSize:      12,
		TextAlign:     0,
	}

	// Resolve each property through the inheritance chain
	es.resolveLineProperties(s)
	es.resolveArrowProperties(s)
	es.resolveFillProperties(s)
	es.resolveShadowProperties(s)
	es.resolveTextProperties(s)
	es.resolveVisibility(s)

	return es
}

// resolveLineProperties resolves line-related properties.
func (es *EffectiveStyle) resolveLineProperties(s *Shape) {
	// LineColor
	if color, source := s.resolveColorCell("LineColor"); color != "" {
		es.LineColor = color
		es.LineColorSource = source
	}

	// LineWeight (in inches, convert to points)
	if v := s.resolveNumericCell("LineWeight"); v > 0 {
		es.LineWeight = v * 72.0 // inches to points
	}

	// LinePattern
	if v := s.resolveIntCell("LinePattern"); v >= 0 {
		es.LinePattern = v
	}

	// LineCap
	if v := s.resolveIntCell("LineCap"); v >= 0 {
		es.LineCap = v
	}

	// LineColorTrans
	if v := s.resolveNumericCell("LineColorTrans"); v >= 0 {
		es.LineColorTrans = v
	}
}

// resolveArrowProperties resolves arrow/marker properties.
func (es *EffectiveStyle) resolveArrowProperties(s *Shape) {
	// BeginArrow
	if v := s.resolveIntCell("BeginArrow"); v >= 0 {
		es.BeginArrow = v
		es.ArrowSource = "cell"
	} else if v := s.resolveThemeArrow("Begin"); v > 0 {
		es.BeginArrow = v
		es.ArrowSource = "theme"
	}

	// EndArrow
	if v := s.resolveIntCell("EndArrow"); v >= 0 {
		es.EndArrow = v
	} else if v := s.resolveThemeArrow("End"); v > 0 {
		es.EndArrow = v
	}

	// BeginArrowSize
	if v := s.resolveIntCell("BeginArrowSize"); v >= 0 {
		es.BeginArrowSize = v
	}

	// EndArrowSize
	if v := s.resolveIntCell("EndArrowSize"); v >= 0 {
		es.EndArrowSize = v
	}

	// Compute arrow setbacks based on arrow type and size
	es.BeginArrowSetback = computeArrowSetback(es.BeginArrow, es.BeginArrowSize, es.LineWeight)
	es.EndArrowSetback = computeArrowSetback(es.EndArrow, es.EndArrowSize, es.LineWeight)
}

// resolveFillProperties resolves fill-related properties.
func (es *EffectiveStyle) resolveFillProperties(s *Shape) {
	// FillForegnd
	if color, source := s.resolveColorCell("FillForegnd"); color != "" {
		es.FillForegnd = color
		es.FillColorSource = source
	}

	// FillBkgnd
	if color, _ := s.resolveColorCell("FillBkgnd"); color != "" {
		es.FillBkgnd = color
	}

	// FillPattern
	if v := s.resolveIntCell("FillPattern"); v >= 0 {
		es.FillPattern = v
	}

	// FillForegndTrans
	if v := s.resolveNumericCell("FillForegndTrans"); v >= 0 {
		es.FillForegndTrans = v
	}

	// FillBkgndTrans
	if v := s.resolveNumericCell("FillBkgndTrans"); v >= 0 {
		es.FillBkgndTrans = v
	}
}

// resolveShadowProperties resolves shadow-related properties.
func (es *EffectiveStyle) resolveShadowProperties(s *Shape) {
	// ShdwForegnd
	if color, _ := s.resolveColorCell("ShdwForegnd"); color != "" {
		es.ShdwForegnd = color
	}

	// ShdwForegndTrans
	if v := s.resolveNumericCell("ShdwForegndTrans"); v >= 0 {
		es.ShdwForegndTrans = v
	}

	// ShapeShdwType
	if v := s.resolveIntCell("ShapeShdwType"); v >= 0 {
		es.ShapeShdwType = v
	}

	// ShapeShdwOffsetX (inches to points)
	if v := s.resolveNumericCell("ShapeShdwOffsetX"); v != 0 {
		es.ShapeShdwOffsetX = v * 72.0
	}

	// ShapeShdwOffsetY (inches to points)
	if v := s.resolveNumericCell("ShapeShdwOffsetY"); v != 0 {
		es.ShapeShdwOffsetY = v * 72.0
	}

	// ShapeShdwBlur (inches to points)
	if v := s.resolveNumericCell("ShapeShdwBlur"); v >= 0 {
		es.ShapeShdwBlur = v * 72.0
	}

	// ShapeShdwScaleFactor
	if v := s.resolveNumericCell("ShapeShdwScaleFactor"); v > 0 {
		es.ShapeShdwScaleFactor = v
	}
}

// resolveTextProperties resolves text-related properties.
func (es *EffectiveStyle) resolveTextProperties(s *Shape) {
	// Get text color using existing method
	if color := s.TextColor(); color != "" {
		es.TextColor = resolveColorStr(color)
	}

	// Font size using existing method
	if size := s.TextSize(); size > 0 {
		es.FontSize = size * 72.0 // inches to points
	}

	// Font ID from Character section
	if font := s.CellValue("Char.Font"); font != "" {
		es.FontID = int(toFloat(font))
	}

	// Style (bold/italic from Style cell bitmask)
	if style := s.CellValue("Char.Style"); style != "" {
		styleVal := int(toFloat(style))
		es.Bold = (styleVal & 1) != 0
		es.Italic = (styleVal & 2) != 0
	}

	// Paragraph alignment
	if align := s.CellValue("Para.HorzAlign"); align != "" {
		es.TextAlign = int(toFloat(align))
	}
}

// resolveVisibility resolves visibility flags.
func (es *EffectiveStyle) resolveVisibility(s *Shape) {
	// NoLine
	if v := s.resolveIntCell("NoLine"); v > 0 {
		es.NoLine = true
	}

	// NoFill
	if v := s.resolveIntCell("NoFill"); v > 0 {
		es.NoFill = true
	}

	// NoShow
	if v := s.resolveIntCell("NoShow"); v > 0 {
		es.NoShow = true
	}
}

// resolveColorCell resolves a color cell value through inheritance chain.
// Returns the resolved RGB color and the source ("local", "master", "style", "theme", "default").
func (s *Shape) resolveColorCell(cellName string) (string, string) {
	// 1. Local cell
	if v := s.CellValueLocal(cellName); v != "" {
		return resolveColorValue(s, v), "local"
	}

	// 2. Master shape
	if master := s.MasterShape(); master != nil {
		if v := master.CellValueLocal(cellName); v != "" {
			return resolveColorValue(master, v), "master"
		}
	}

	// 3. Style sheet
	if v := s.ResolveStyleValue(cellName); v != "" {
		return resolveColorValue(s, v), "style"
	}

	// 4. Theme/QuickStyle
	if color := s.resolveThemeColor(cellName); color != "" {
		return color, "theme"
	}

	return "", "default"
}

// CellValueLocal returns the cell value without inheritance.
func (s *Shape) CellValueLocal(name string) string {
	if s.xml == nil {
		return ""
	}
	for _, cell := range s.xml.SelectElements("Cell") {
		if cell.SelectAttrValue("N", "") == name {
			return cell.SelectAttrValue("V", "")
		}
	}
	return ""
}

// resolveNumericCell resolves a numeric cell value through inheritance.
func (s *Shape) resolveNumericCell(cellName string) float64 {
	// 1. Local cell
	if v := s.CellValueLocal(cellName); v != "" {
		return evaluateCellValue(s, v)
	}

	// 2. Master shape
	if master := s.MasterShape(); master != nil {
		if v := master.CellValueLocal(cellName); v != "" {
			return evaluateCellValue(master, v)
		}
	}

	// 3. Style sheet
	if v := s.ResolveStyleValue(cellName); v != "" {
		return evaluateCellValue(s, v)
	}

	// 4. Theme - handled specially for some cells

	return -1 // Indicates not found
}

// resolveIntCell resolves an integer cell value through inheritance.
func (s *Shape) resolveIntCell(cellName string) int {
	v := s.resolveNumericCell(cellName)
	if v >= 0 {
		return int(v)
	}
	return -1
}

// evaluateCellValue evaluates a cell value that may contain a formula.
// Returns -1 if value is "Themed" or other non-numeric to indicate theme lookup needed.
func evaluateCellValue(s *Shape, value string) float64 {
	value = strings.TrimSpace(value)

	// Handle "Themed" keyword - this means the value should come from theme
	if strings.EqualFold(value, "Themed") {
		return -1 // Signal that theme lookup is needed
	}

	// Handle THEMEVAL
	if strings.HasPrefix(strings.ToUpper(value), "THEMEVAL(") {
		// Extract default value if present
		inner := value[9:]
		if idx := strings.LastIndex(inner, ","); idx > 0 {
			// THEMEVAL(ref, default) - return default
			defVal := strings.TrimSuffix(strings.TrimSpace(inner[idx+1:]), ")")
			return toFloat(defVal)
		}
		return -1 // No default, signal theme lookup needed
	}

	// Handle THEMEGUARD
	if strings.HasPrefix(strings.ToUpper(value), "THEMEGUARD(") {
		inner := value[11:]
		inner = strings.TrimSuffix(inner, ")")
		return evaluateCellValue(s, inner)
	}

	return toFloat(value)
}

// resolveColorValue resolves a color value that may be a formula, index, or RGB.
func resolveColorValue(s *Shape, value string) string {
	value = strings.TrimSpace(value)

	// Handle THEMEVAL
	if strings.HasPrefix(strings.ToUpper(value), "THEMEVAL(") {
		return s.resolveThemeVal(value)
	}

	// Handle THEMEGUARD
	if strings.HasPrefix(strings.ToUpper(value), "THEMEGUARD(") {
		inner := value[11:]
		inner = strings.TrimSuffix(inner, ")")
		return resolveColorValue(s, inner)
	}

	return resolveColor(value)
}

// resolveThemeColor resolves a color from theme/QuickStyle settings.
func (s *Shape) resolveThemeColor(cellName string) string {
	qs := s.QuickStyle()
	if qs == nil {
		return ""
	}

	vis := s.Page.vis
	if vis == nil {
		return ""
	}
	theme := vis.Theme()
	if theme == nil {
		return ""
	}

	var colorIdx int
	switch cellName {
	case "LineColor":
		colorIdx = qs.LineColor
	case "FillForegnd":
		colorIdx = qs.FillColor
	case "ShdwForegnd":
		colorIdx = qs.ShadowColor
	default:
		return ""
	}

	// Map QuickStyle color index to theme color
	// Per MS-VSDX §2.2.7.4.3: indices 0-8 map to theme colors
	return s.resolveQuickStyleColor(colorIdx, cellName)
}

// resolveThemeArrow resolves arrow type from theme connector properties.
// This implements THEMEVAL() resolution for BeginArrow/EndArrow cells.
func (s *Shape) resolveThemeArrow(position string) int {
	// Check if this is a connector shape
	if !s.IsConnector() {
		return 0
	}

	vis := s.Page.vis
	if vis == nil {
		return 0
	}

	// Get the QuickStyleLineMatrix index from the shape's style chain
	// This determines which connector line style preset to use
	lineMatrixIdx := s.resolveQuickStyleLineMatrix()
	if lineMatrixIdx < 0 {
		return 0
	}

	// Get theme and look up the connector line style
	theme := vis.Theme()
	if theme == nil || len(theme.ConnectorLineStyles) == 0 {
		return 0
	}

	// QuickStyleLineMatrix is 1-indexed in stylesheets, convert to 0-indexed
	idx := lineMatrixIdx
	if idx >= len(theme.ConnectorLineStyles) {
		idx = len(theme.ConnectorLineStyles) - 1
	}
	if idx < 0 {
		return 0
	}

	style := theme.ConnectorLineStyles[idx]
	if position == "Begin" {
		return style.BeginArrow
	}
	return style.EndArrow
}

// resolveQuickStyleLineMatrix returns the QuickStyleLineMatrix index for this shape.
// Follows the resolution chain: shape → master → line style sheet.
func (s *Shape) resolveQuickStyleLineMatrix() int {
	// Check own cell first
	if v := s.CellValue("QuickStyleLineMatrix"); v != "" {
		return int(toFloat(v))
	}

	// Check master shape
	if master := s.MasterShape(); master != nil {
		if v := master.CellValue("QuickStyleLineMatrix"); v != "" {
			return int(toFloat(v))
		}
	}

	// Check LineStyle reference for stylesheet
	if s.xml != nil && s.Page != nil && s.Page.vis != nil {
		// First check master's LineStyle attribute
		master := s.MasterShape()
		if master != nil && master.xml != nil {
			if lineStyleID := master.xml.SelectAttrValue("LineStyle", ""); lineStyleID != "" {
				if ss := s.Page.vis.StyleSheetByID(lineStyleID); ss != nil {
					if v := ss.CellValue("QuickStyleLineMatrix"); v != "" {
						return int(toFloat(v))
					}
				}
			}
		}

		// Then check shape's own LineStyle
		if lineStyleID := s.xml.SelectAttrValue("LineStyle", ""); lineStyleID != "" {
			if ss := s.Page.vis.StyleSheetByID(lineStyleID); ss != nil {
				if v := ss.CellValue("QuickStyleLineMatrix"); v != "" {
					return int(toFloat(v))
				}
			}
		}
	}

	return -1
}

// IsConnector returns true if the shape is a connector/dynamic connector.
func (s *Shape) IsConnector() bool {
	// Check shape type or style inheritance
	if s.xml == nil {
		return false
	}

	// Check for 1D endpoints
	if s.CellValue("BeginX") != "" && s.CellValue("EndX") != "" {
		return true
	}

	// Check style inheritance from "Connector" style
	lineStyle := s.xml.SelectAttrValue("LineStyle", "")
	if lineStyle != "" && s.Page != nil && s.Page.vis != nil {
		ss := s.Page.vis.StyleSheetByID(lineStyle)
		if ss != nil && ss.NameU == "Connector" {
			return true
		}
	}

	return false
}

// computeArrowSetback calculates how much to shorten the path for an arrow marker.
// This ensures arrows don't extend beyond the path endpoint.
func computeArrowSetback(arrowType, arrowSize int, lineWeight float64) float64 {
	if arrowType == 0 {
		return 0
	}

	// Arrow size multipliers (MS-VSDX arrow size semantics)
	// Index 0-6 correspond to specific size multipliers
	sizeMultipliers := []float64{0.5, 0.75, 1.0, 1.5, 2.0, 3.0, 4.0}
	sizeMult := 1.0
	if arrowSize >= 0 && arrowSize < len(sizeMultipliers) {
		sizeMult = sizeMultipliers[arrowSize]
	}

	// Base setback depends on arrow type category
	// Standard arrows (1-14): triangular, based on arrow length
	// Circle arrows (15-22): diameter-based
	// Other specialized arrows have their own metrics

	var baseSetback float64
	switch {
	case arrowType >= 1 && arrowType <= 14:
		// Triangular arrows: setback = arrow length
		// Default arrow length is approximately 4x line weight
		baseSetback = lineWeight * 4.0 * sizeMult
	case arrowType >= 15 && arrowType <= 22:
		// Circle/dot arrows: setback = radius
		baseSetback = lineWeight * 2.0 * sizeMult
	case arrowType >= 23 && arrowType <= 45:
		// Specialized arrows: estimate
		baseSetback = lineWeight * 3.0 * sizeMult
	default:
		baseSetback = lineWeight * 4.0 * sizeMult
	}

	return baseSetback
}

// EffectiveLineColor returns the line color as CSS color.
func (es *EffectiveStyle) EffectiveLineColor() string {
	if es.LineColorTrans >= 1.0 || es.NoLine {
		return "none"
	}
	return es.LineColor
}

// EffectiveFillColor returns the fill color as CSS color.
func (es *EffectiveStyle) EffectiveFillColor() string {
	if es.FillPattern == 0 || es.FillForegndTrans >= 1.0 || es.NoFill {
		return "none"
	}
	// If no fill color was explicitly resolved (FillColorSource is empty),
	// treat as unfilled - the default is no fill, not white.
	if es.FillColorSource == "" {
		return "none"
	}
	return es.FillForegnd
}

// EffectiveLineOpacity returns the line opacity (0-1).
func (es *EffectiveStyle) EffectiveLineOpacity() float64 {
	return 1.0 - es.LineColorTrans
}

// EffectiveFillOpacity returns the fill opacity (0-1).
func (es *EffectiveStyle) EffectiveFillOpacity() float64 {
	return 1.0 - es.FillForegndTrans
}

// HasShadow returns true if shadow should be rendered.
func (es *EffectiveStyle) HasShadow() bool {
	return es.ShapeShdwType > 0 && es.ShdwForegndTrans < 1.0
}

// ShadowOpacity returns the shadow opacity (0-1).
func (es *EffectiveStyle) ShadowOpacity() float64 {
	return 1.0 - es.ShdwForegndTrans
}

// resolveColorStr resolves a color value which may be an index, RGB, or named color.
// This is the effective style version that calls the main resolveColor in svg.go.
func resolveColorStr(value string) string {
	return resolveColor(value)
}
