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
	TextColor     string  // Text color
	FontSize      float64 // Font size in points
	FontID        int     // Font table index
	Bold          bool
	Italic        bool
	Underline     bool
	Strikethrough bool
	TextAlign     int // 0=left, 1=center, 2=right

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
	// Get text color - check Character section first, then QuickStyleFontMatrix, then QuickStyleFontColor
	if color := s.TextColor(); color != "" {
		es.TextColor = resolveColorStr(color)
	} else if fontColor := s.resolveFontStyleColor(); fontColor != "" {
		// Use QuickStyleFontMatrix to look up font color from theme fontStyles.
		es.TextColor = fontColor
	} else if qsColor := s.QuickStyleFontColor(); qsColor != "" {
		// Fallback to direct QuickStyleFontColor resolution.
		es.TextColor = qsColor
	}

	// Font size using existing method
	if size := s.TextSize(); size > 0 {
		es.FontSize = size * 72.0 // inches to points
	}

	// Font ID from Character section
	if font := s.CellValue("Char.Font"); font != "" {
		es.FontID = int(toFloat(font))
	}

	// Style (bold/italic/underline from Style cell bitmask)
	// Bit 1=Bold, 2=Italic, 4=Underline
	if style := s.CellValue("Char.Style"); style != "" {
		styleVal := int(toFloat(style))
		es.Bold = (styleVal & 1) != 0
		es.Italic = (styleVal & 2) != 0
		es.Underline = (styleVal & 4) != 0
	}

	// Strikethrough from Character section
	if strikethru := s.CellValue("Char.Strikethru"); strikethru != "" && strikethru != "0" {
		es.Strikethrough = true
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
		if color := resolveColorValue(s, v); color != "" {
			return color, "style"
		}
		// If style value is "Themed" or similar, fall through to theme resolution
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
		val := evaluateCellValue(s, v)
		if val >= 0 {
			return val
		}
		// If "Themed", fall through to theme resolution
	}

	// 2. Master shape
	if master := s.MasterShape(); master != nil {
		if v := master.CellValueLocal(cellName); v != "" {
			val := evaluateCellValue(master, v)
			if val >= 0 {
				return val
			}
			// If "Themed", fall through to theme resolution
		}
	}

	// 3. Style sheet
	if v := s.ResolveStyleValue(cellName); v != "" {
		val := evaluateCellValue(s, v)
		if val >= 0 {
			return val
		}
		// If "Themed", fall through to theme resolution
	}

	// 4. Theme - resolve LineWeight from theme line styles
	if cellName == "LineWeight" {
		if weight := s.resolveThemeLineWeight(); weight > 0 {
			return weight
		}
	}

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

	// Handle "Themed" keyword - resolve from theme
	if strings.EqualFold(value, "Themed") {
		return "" // Signal to caller to try theme resolution
	}

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
	vis := s.Page.vis
	if vis == nil {
		return ""
	}
	theme := vis.Theme()
	if theme == nil {
		return ""
	}

	// For LineColor, first try to resolve via QuickStyleLineMatrix + lnStyleLst
	if cellName == "LineColor" {
		lineMatrixIdx := s.resolveQuickStyleLineMatrix()

		// Handle special value 100 which means "use variation-based index"
		// Per MS-VSDX, QSLM=100 maps to the line style based on embellishment/variation.
		// Default to index 4 which points to LineStyles[3] (the neutral gray #C6C7C7).
		if lineMatrixIdx == 100 {
			// For non-connector shapes (QuickStyleType != 0), use the variation-based lineIdx.
			// Index 4 (1-indexed) = LineStyles[3] which typically has the gray stroke color.
			lineMatrixIdx = 4
		}

		// QuickStyleLineMatrix is 1-indexed (value 1 = first style at index 0)
		styleIdx := lineMatrixIdx - 1
		if styleIdx >= 0 && styleIdx < len(theme.LineStyles) {
			lineStyle := theme.LineStyles[styleIdx]
			if lineStyle.Color != "" {
				return lineStyle.Color
			}
			// Handle phClr (placeholder color) with tint/shade
			// phClr uses fill color if shape has explicit FillForegnd, otherwise QuickStyleLineColor
			if lineStyle.IsPhClr {
				var baseColor string
				// Only use fill if there's an explicit FillForegnd on the shape
				if explicitFill := s.CellValueLocal("FillForegnd"); explicitFill != "" {
					baseColor = resolveColor(explicitFill)
				} else {
					// Use QuickStyleLineColor for shapes without explicit fill
					qslc := s.CellValue("QuickStyleLineColor")
					if qslc != "" {
						baseColor = s.resolveQuickStyleColor(int(toFloat(qslc)), "line")
					} else {
						// No QuickStyleLineColor set - use varColor1 (primary accent) or accent1
						varIdx := int(toFloat(s.CellValue("QuickStyleVariation")))
						if varIdx >= 0 && varIdx < len(theme.Variants) && len(theme.Variants[varIdx].VarColors) > 0 {
							baseColor = theme.Variants[varIdx].VarColors[0] // varColor1
						} else if len(theme.Variants) > 0 && len(theme.Variants[0].VarColors) > 0 {
							baseColor = theme.Variants[0].VarColors[0]
						} else {
							baseColor = theme.Colors.Accent1
						}
					}
				}
				if baseColor != "" {
					// For connectors (1D shapes), don't apply tint/shade - use color directly
					if !s.IsConnector() {
						// Apply tint first, then shade for non-connectors
						if lineStyle.PhClrTint > 0 {
							baseColor = applyTint(baseColor, lineStyle.PhClrTint)
						}
						if lineStyle.PhClrShade > 0 {
							baseColor = applyShade(baseColor, lineStyle.PhClrShade)
						}
					}
					return baseColor
				}
			}
		}

		// For connectors with no explicit line style, use accent color
		if s.IsConnector() && theme.Colors.Accent1 != "" {
			return theme.Colors.Accent1
		}
	}

	// Fall back to QuickStyle color indices
	qs := s.QuickStyle()
	if qs == nil {
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

// resolveThemeLineWeight resolves line weight from theme line styles.
func (s *Shape) resolveThemeLineWeight() float64 {
	// For themed line weight, use standard 1 point (0.01389 inches)
	// This matches Visio's default themed connector line width
	return 0.01389
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

	// Check LineStyle reference for stylesheet (with inheritance)
	if s.xml != nil && s.Page != nil && s.Page.vis != nil {
		// First check master's LineStyle attribute
		master := s.MasterShape()
		if master != nil && master.xml != nil {
			if lineStyleID := master.xml.SelectAttrValue("LineStyle", ""); lineStyleID != "" {
				if ss := s.Page.vis.StyleSheetByID(lineStyleID); ss != nil {
					if v := ss.CellValueWithInheritance("QuickStyleLineMatrix"); v != "" {
						return int(toFloat(v))
					}
				}
			}
		}

		// Then check shape's own LineStyle
		if lineStyleID := s.xml.SelectAttrValue("LineStyle", ""); lineStyleID != "" {
			if ss := s.Page.vis.StyleSheetByID(lineStyleID); ss != nil {
				if v := ss.CellValueWithInheritance("QuickStyleLineMatrix"); v != "" {
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

// resolveFontStyleColor resolves the font color from theme fontStyles using QuickStyleFontMatrix.
// Per the theme structure, fontStyles are indexed by QuickStyleFontMatrix (1-indexed).
// The fontProps elements contain schemeClr references (e.g., lt1=white, phClr=shape color).
func (s *Shape) resolveFontStyleColor() string {
	vis := s.Page.vis
	if vis == nil {
		return ""
	}
	theme := vis.Theme()
	if theme == nil || len(theme.FontStyles) == 0 {
		return ""
	}

	// Get QuickStyleFontMatrix from shape or inheritance chain.
	fontMatrixIdx := -1

	// Check own cell first.
	if v := s.CellValue("QuickStyleFontMatrix"); v != "" {
		fontMatrixIdx = int(toFloat(v))
	}

	// Check master shape.
	if fontMatrixIdx < 0 {
		if master := s.MasterShape(); master != nil {
			if v := master.CellValue("QuickStyleFontMatrix"); v != "" {
				fontMatrixIdx = int(toFloat(v))
			}
		}
	}

	// Check TextStyle reference for stylesheet.
	if fontMatrixIdx < 0 && s.xml != nil && s.Page != nil && s.Page.vis != nil {
		if textStyleID := s.xml.SelectAttrValue("TextStyle", ""); textStyleID != "" {
			if ss := s.Page.vis.StyleSheetByID(textStyleID); ss != nil {
				if v := ss.CellValueWithInheritance("QuickStyleFontMatrix"); v != "" {
					fontMatrixIdx = int(toFloat(v))
				}
			}
		}
	}

	if fontMatrixIdx <= 0 {
		return ""
	}

	// fontStyles are 1-indexed, convert to 0-indexed.
	idx := fontMatrixIdx - 1
	if idx < 0 || idx >= len(theme.FontStyles) {
		return ""
	}

	return theme.FontStyles[idx].Color
}

// computeArrowSetback calculates how much to shorten the path for an arrow marker.
// The setback ensures the arrow body is visible and the tip reaches the shape edge.
// With refX=0, the arrow tip extends forward by the marker width, so we shorten
// the path by approximately the same amount.
func computeArrowSetback(arrowType, arrowSize int, lineWeight float64) float64 {
	if arrowType == 0 {
		return 0
	}

	// Size multipliers from MS-VSDX spec
	sizeMultipliers := []float64{0.5, 0.7, 1.0, 1.3, 1.6, 2.0, 2.5}
	sizeIdx := arrowSize
	if sizeIdx < 0 || sizeIdx >= len(sizeMultipliers) {
		sizeIdx = 2 // default medium
	}

	// Length multiplier per arrow type (matching visioArrowTypes in svg.go)
	// Type 13, 14 are longer arrows (1.5x), others are standard (1.0x)
	lengthMult := 1.0
	if arrowType == 13 || arrowType == 14 {
		lengthMult = 1.5
	}

	// Base visual arrow width (at 1pt stroke)
	// Visio uses ~7 visual units as minimum, scales up with stroke weight
	baseVisualWidth := 3.56 * sizeMultipliers[sizeIdx] * lengthMult
	minVisualWidth := 7.0 * sizeMultipliers[sizeIdx] * lengthMult

	// Calculate visual width: max(minimum, scaled by stroke weight)
	visualWidth := baseVisualWidth * lineWeight
	if visualWidth < minVisualWidth {
		visualWidth = minVisualWidth
	}

	// Return setback in points (matches markerWidth * strokeWidth for SVG)
	return visualWidth
}

// EffectiveLineColor returns the line color as CSS color.
func (es *EffectiveStyle) EffectiveLineColor() string {
	// LinePattern=0 means no line in Visio
	if es.LinePattern == 0 || es.LineColorTrans >= 1.0 || es.NoLine {
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
