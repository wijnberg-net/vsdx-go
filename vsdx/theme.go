package vsdx

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// Theme represents the visual theme applied to a Visio document.
type Theme struct {
	Name                string
	Colors              ThemeColors
	Fonts               ThemeFonts
	Effects             ThemeEffects
	Variants            []ThemeVariant
	ConnectorLineStyles []ConnectorLineStyle // Connector arrow/line style presets
	LineStyles          []LineStyleColor     // Non-connector line styles from fmtScheme/lnStyleLst
	FontStyles          []FontStyleColor     // Font styles from fontStylesGroup (indexed by QuickStyleFontMatrix)
}

// LineStyleColor represents a resolved line style color from the theme.
// These are indexed by QuickStyleLineMatrix for non-connector shapes.
type LineStyleColor struct {
	Color      string  // Resolved RGB hex color (e.g., "#C7C8C8"), empty if phClr
	IsPhClr    bool    // True if this uses placeholder color (shape's QuickStyleLineColor)
	PhClrTint  float64 // Tint to apply to phClr (0-1, 0=no tint)
	PhClrShade float64 // Shade to apply to phClr (0-1, 0=no shade)
}

// FontStyleColor represents a font style from the theme.
// These are indexed by QuickStyleFontMatrix.
type FontStyleColor struct {
	Color string // Resolved RGB hex color (e.g., "#FEFFFF")
}

// ConnectorLineStyle represents a connector line style from the theme.
// These are indexed by QuickStyleLineMatrix.
type ConnectorLineStyle struct {
	BeginArrow     int // Begin arrow type (0=none, 1-45=types)
	EndArrow       int // End arrow type
	BeginArrowSize int // Begin arrow size (0-6)
	EndArrowSize   int // End arrow size
	LinePattern    int // Line pattern (1=solid, etc.)
	Rounding       float64
}

// ThemeColors holds the color palette from the theme.
type ThemeColors struct {
	Dark1    string // dk1 - typically black
	Light1   string // lt1 - typically white
	Dark2    string // dk2 - secondary dark
	Light2   string // lt2 - secondary light
	Accent1  string // accent1
	Accent2  string // accent2
	Accent3  string // accent3
	Accent4  string // accent4
	Accent5  string // accent5
	Accent6  string // accent6
	Hlink    string // hyperlink color
	FolHlink string // followed hyperlink color
}

// ThemeFonts holds the font definitions from the theme.
type ThemeFonts struct {
	MajorLatin   string // major font for Latin text (headings)
	MinorLatin   string // minor font for Latin text (body)
	MajorAsian   string // major font for East Asian text
	MinorAsian   string // minor font for East Asian text
	MajorComplex string // major font for Complex Script text
	MinorComplex string // minor font for Complex Script text
}

// ThemeEffects holds the effect properties from the theme.
type ThemeEffects struct {
	// Shadow effects
	ShadowColor        string  // shadow color
	ShadowTransparency float64 // shadow transparency (0-1)
	ShadowOffsetX      float64 // shadow X offset in inches
	ShadowOffsetY      float64 // shadow Y offset in inches
	ShadowBlur         float64 // shadow blur radius

	// Glow effects
	GlowColor string  // glow color
	GlowSize  float64 // glow size in points

	// Bevel effects
	BevelTopType    int     // bevel top type (0=none, 1-12=types)
	BevelTopWidth   float64 // bevel top width
	BevelTopHeight  float64 // bevel top height
	BevelBottomType int     // bevel bottom type
	BevelMaterial   int     // material type (1-10)
	BevelLighting   int     // lighting type (1-15)

	// Reflection effects
	ReflectionSize float64 // reflection size (0-100)
	ReflectionDist float64 // distance from shape
	ReflectionBlur float64 // blur amount

	// Soft edges
	SoftEdgesSize float64 // soft edges size in points
}

// ThemeVariant represents a theme variant/variation.
type ThemeVariant struct {
	Index       int      // variant index (0-3)
	FillColor   string   // primary fill color (varColor1)
	FillColor2  string   // secondary fill color (varColor2)
	LineColor   string   // line color (varColor3)
	TextColor   string   // text color
	AccentColor string   // accent color (varColor5)
	VarColors   []string // all 7 varColors (varColor1-7) for indexed lookup
}

// Theme returns the document's theme, or nil if no theme is defined.
func (v *VisioFile) Theme() *Theme {
	// Look for theme file.
	themeData, ok := v.ZipFileContents["visio/theme/theme1.xml"]
	if !ok {
		// Try alternate location.
		themeData, ok = v.ZipFileContents["visio/theme1.xml"]
	}
	if !ok {
		return nil
	}

	themeDoc := etree.NewDocument()
	if err := themeDoc.ReadFromBytes(themeData); err != nil {
		return nil
	}

	theme := &Theme{}

	// Parse theme name.
	root := themeDoc.Root()
	if root != nil {
		theme.Name = root.SelectAttrValue("name", "")
	}

	// Parse color scheme.
	// Office themes use a:clrScheme element.
	clrScheme := themeDoc.FindElement("//a:clrScheme")
	if clrScheme == nil {
		clrScheme = themeDoc.FindElement("//clrScheme")
	}
	if clrScheme != nil {
		theme.Colors = parseThemeColors(clrScheme)
	}

	// Parse font scheme.
	fontScheme := themeDoc.FindElement("//a:fontScheme")
	if fontScheme == nil {
		fontScheme = themeDoc.FindElement("//fontScheme")
	}
	if fontScheme != nil {
		theme.Fonts = parseThemeFonts(fontScheme)
	}

	// Parse effects scheme.
	fmtScheme := themeDoc.FindElement("//a:fmtScheme")
	if fmtScheme == nil {
		fmtScheme = themeDoc.FindElement("//fmtScheme")
	}
	if fmtScheme != nil {
		theme.Effects = parseThemeEffects(fmtScheme)
	}

	// Parse theme variants (from Visio-specific elements).
	theme.Variants = parseThemeVariants(v, themeDoc)

	// Parse connector line styles (for arrow resolution).
	theme.ConnectorLineStyles = parseConnectorLineStyles(themeDoc)

	// Parse non-connector line styles (for line colors).
	theme.LineStyles = parseLineStyleColors(themeDoc, theme.Colors)

	// Parse font styles (for text colors indexed by QuickStyleFontMatrix).
	theme.FontStyles = parseFontStyleColors(themeDoc, theme.Colors)

	return theme
}

// parseThemeColors extracts colors from a clrScheme element.
func parseThemeColors(clrScheme *etree.Element) ThemeColors {
	colors := ThemeColors{}

	getColor := func(name string) string {
		elem := clrScheme.FindElement(name)
		if elem == nil {
			elem = clrScheme.FindElement("a:" + name)
		}
		if elem == nil {
			return ""
		}
		// Look for srgbClr (RGB color) or sysClr (system color).
		if srgb := elem.FindElement("a:srgbClr"); srgb != nil {
			return "#" + strings.ToUpper(srgb.SelectAttrValue("val", ""))
		}
		if srgb := elem.FindElement("srgbClr"); srgb != nil {
			return "#" + strings.ToUpper(srgb.SelectAttrValue("val", ""))
		}
		if sys := elem.FindElement("a:sysClr"); sys != nil {
			return "#" + strings.ToUpper(sys.SelectAttrValue("lastClr", ""))
		}
		if sys := elem.FindElement("sysClr"); sys != nil {
			return "#" + strings.ToUpper(sys.SelectAttrValue("lastClr", ""))
		}
		return ""
	}

	colors.Dark1 = getColor("dk1")
	colors.Light1 = getColor("lt1")
	colors.Dark2 = getColor("dk2")
	colors.Light2 = getColor("lt2")
	colors.Accent1 = getColor("accent1")
	colors.Accent2 = getColor("accent2")
	colors.Accent3 = getColor("accent3")
	colors.Accent4 = getColor("accent4")
	colors.Accent5 = getColor("accent5")
	colors.Accent6 = getColor("accent6")
	colors.Hlink = getColor("hlink")
	colors.FolHlink = getColor("folHlink")

	return colors
}

// parseThemeFonts extracts fonts from a fontScheme element.
func parseThemeFonts(fontScheme *etree.Element) ThemeFonts {
	fonts := ThemeFonts{}

	getFontByScript := func(fontElem *etree.Element, script string) string {
		if fontElem == nil {
			return ""
		}
		// Try with namespace prefix
		elem := fontElem.FindElement("a:" + script)
		if elem == nil {
			elem = fontElem.FindElement(script)
		}
		if elem != nil {
			return elem.SelectAttrValue("typeface", "")
		}
		return ""
	}

	// Find major and minor font elements
	majorFont := fontScheme.FindElement("majorFont")
	if majorFont == nil {
		majorFont = fontScheme.FindElement("a:majorFont")
	}
	minorFont := fontScheme.FindElement("minorFont")
	if minorFont == nil {
		minorFont = fontScheme.FindElement("a:minorFont")
	}

	// Latin fonts
	fonts.MajorLatin = getFontByScript(majorFont, "latin")
	fonts.MinorLatin = getFontByScript(minorFont, "latin")

	// East Asian fonts
	fonts.MajorAsian = getFontByScript(majorFont, "ea")
	fonts.MinorAsian = getFontByScript(minorFont, "ea")

	// Complex Script fonts
	fonts.MajorComplex = getFontByScript(majorFont, "cs")
	fonts.MinorComplex = getFontByScript(minorFont, "cs")

	return fonts
}

// parseThemeEffects extracts effect properties from a fmtScheme element.
func parseThemeEffects(fmtScheme *etree.Element) ThemeEffects {
	effects := ThemeEffects{}

	// Look for effect style list.
	effectStyleLst := fmtScheme.FindElement("effectStyleLst")
	if effectStyleLst == nil {
		effectStyleLst = fmtScheme.FindElement("a:effectStyleLst")
	}
	if effectStyleLst == nil {
		return effects
	}

	// effectStyleLst typically contains 6 entries. The first 3 are usually
	// empty (subtle/no shadow); the later entries carry the actual outer
	// shadow definitions referenced via QuickStyleEffectsMatrix. Scan for
	// the first entry that has an outerShdw so the theme shadow is
	// discoverable without QuickStyle matrix resolution.
	var effectLst *etree.Element
	for _, child := range effectStyleLst.ChildElements() {
		tag := child.Tag
		if tag != "effectStyle" {
			continue
		}
		lst := child.FindElement("effectLst")
		if lst == nil {
			lst = child.FindElement("a:effectLst")
		}
		if lst == nil {
			continue
		}
		if lst.FindElement("outerShdw") != nil || lst.FindElement("a:outerShdw") != nil {
			effectLst = lst
			break
		}
		// Remember the first non-nil effectLst as fallback (for glow/reflection/etc).
		if effectLst == nil {
			effectLst = lst
		}
	}
	if effectLst == nil {
		return effects
	}

	// Parse outer shadow (OOXML <a:outerShdw> in theme1.xml).
	// - blurRad / dist are in EMUs (914400 per inch, 12700 per point).
	// - dir is the shadow direction in 60000ths of a degree, measured
	//   clockwise from the +X axis, so the X/Y offsets must be derived
	//   from dist + dir, not from dist alone.
	// - alpha (under the color child) is in thousandths (e.g. 22000 = 22%).
	outerShdw := effectLst.FindElement("outerShdw")
	if outerShdw == nil {
		outerShdw = effectLst.FindElement("a:outerShdw")
	}
	if outerShdw != nil {
		effects.ShadowBlur = parseEmu(outerShdw.SelectAttrValue("blurRad", "0"))
		dist := parseEmu(outerShdw.SelectAttrValue("dist", "0"))
		dirDeg := toFloat(outerShdw.SelectAttrValue("dir", "0")) / 60000.0
		rad := dirDeg * math.Pi / 180.0
		effects.ShadowOffsetX = dist * math.Cos(rad)
		// Theme dir is measured clockwise (screen convention) so a positive
		// sin value already means "downward" in SVG coordinates. The shape
		// cell convention however stores OffsetY upward-positive, so we
		// keep the sign positive here and flip when needed at use site.
		effects.ShadowOffsetY = dist * math.Sin(rad)
		// Extract color + alpha. The outerShdw can use either srgbClr (explicit
		// color) or schemeClr (typically "phClr" placeholder - resolved per-shape
		// via QuickStyleShadowColor). For our theme-level fields we only store
		// the explicit srgb color; transparency lives on both color element types.
		var clrElem *etree.Element
		for _, child := range outerShdw.ChildElements() {
			switch child.Tag {
			case "srgbClr":
				effects.ShadowColor = "#" + child.SelectAttrValue("val", "000000")
				clrElem = child
			case "schemeClr":
				clrElem = child
			}
		}
		if clrElem != nil {
			if alpha := clrElem.FindElement("alpha"); alpha != nil {
				effects.ShadowTransparency = 1.0 - toFloat(alpha.SelectAttrValue("val", "100000"))/100000.0
			}
		}
	}

	// Parse glow.
	glow := effectLst.FindElement("glow")
	if glow == nil {
		glow = effectLst.FindElement("a:glow")
	}
	if glow != nil {
		effects.GlowSize = parseEmu(glow.SelectAttrValue("rad", "0"))
		if srgb := glow.FindElement("a:srgbClr"); srgb != nil {
			effects.GlowColor = "#" + srgb.SelectAttrValue("val", "000000")
		} else if srgb := glow.FindElement("srgbClr"); srgb != nil {
			effects.GlowColor = "#" + srgb.SelectAttrValue("val", "000000")
		}
	}

	// Parse soft edge.
	softEdge := effectLst.FindElement("softEdge")
	if softEdge == nil {
		softEdge = effectLst.FindElement("a:softEdge")
	}
	if softEdge != nil {
		effects.SoftEdgesSize = parseEmu(softEdge.SelectAttrValue("rad", "0"))
	}

	// Parse reflection.
	reflection := effectLst.FindElement("reflection")
	if reflection == nil {
		reflection = effectLst.FindElement("a:reflection")
	}
	if reflection != nil {
		effects.ReflectionBlur = parseEmu(reflection.SelectAttrValue("blurRad", "0"))
		effects.ReflectionDist = parseEmu(reflection.SelectAttrValue("dist", "0"))
		effects.ReflectionSize = toFloat(reflection.SelectAttrValue("stA", "0")) / 1000 // Percentage
	}

	return effects
}

// parseEmu parses an EMU (English Metric Unit) value to points.
// 1 point = 12700 EMUs
func parseEmu(s string) float64 {
	v := toFloat(s)
	return v / 12700.0
}

// parseThemeVariants extracts theme variants from Visio-specific elements.
// Variant colors are stored in vt:variationClrSchemeLst within the theme's extension list.
// MS-VSDX spec: variant colors define alternate color schemes (typically 4 variants).
func parseThemeVariants(vis *VisioFile, themeDoc *etree.Document) []ThemeVariant {
	var variants []ThemeVariant

	// Look for variation color scheme list in theme extensions.
	// Path: a:theme/a:themeElements/a:clrScheme/a:extLst/a:ext/vt:variationClrSchemeLst
	varList := themeDoc.FindElement("//vt:variationClrSchemeLst")
	if varList == nil {
		// Try without namespace prefix.
		varList = themeDoc.FindElement("//variationClrSchemeLst")
	}
	if varList == nil {
		// Also check for separate variant file (legacy format).
		variantData, ok := vis.ZipFileContents["visio/theme/variant1.xml"]
		if ok {
			varDoc := etree.NewDocument()
			if err := varDoc.ReadFromBytes(variantData); err == nil {
				varList = varDoc.FindElement("//vt:variationClrSchemeLst")
				if varList == nil {
					varList = varDoc.FindElement("//variationClrSchemeLst")
				}
			}
		}
	}

	if varList == nil {
		return variants
	}

	// Parse each variationClrScheme element.
	varSchemes := varList.SelectElements("vt:variationClrScheme")
	if len(varSchemes) == 0 {
		varSchemes = varList.SelectElements("variationClrScheme")
	}

	for i, varScheme := range varSchemes {
		variant := ThemeVariant{Index: i}

		// Parse all variant colors (varColor1 through varColor7).
		// Store all 7 for indexed lookup (QuickStyleFillColor 200-206 maps to varColor1-7).
		variant.VarColors = make([]string, 7)
		for j := 1; j <= 7; j++ {
			variant.VarColors[j-1] = parseVariantColor(varScheme, fmt.Sprintf("varColor%d", j))
		}

		// Also store named mappings for convenience.
		// varColor1 is typically the primary fill, varColor2-7 are accent colors.
		variant.FillColor = variant.VarColors[0]   // varColor1
		variant.FillColor2 = variant.VarColors[1]  // varColor2
		variant.LineColor = variant.VarColors[2]   // varColor3
		variant.AccentColor = variant.VarColors[4] // varColor5

		// Check monotone attribute.
		if varScheme.SelectAttrValue("monotone", "") == "1" {
			// Monotone variants use same color for line and fill.
			if variant.LineColor == "" {
				variant.LineColor = variant.FillColor
			}
		}

		variants = append(variants, variant)
	}

	return variants
}

// parseVariantColor extracts a color from a variant color element.
func parseVariantColor(parent *etree.Element, name string) string {
	// Try with vt: namespace prefix.
	elem := parent.FindElement("vt:" + name)
	if elem == nil {
		elem = parent.FindElement(name)
	}
	if elem == nil {
		return ""
	}

	// Look for srgbClr child (with or without namespace).
	srgb := elem.FindElement("a:srgbClr")
	if srgb == nil {
		srgb = elem.FindElement("srgbClr")
	}
	if srgb != nil {
		val := srgb.SelectAttrValue("val", "")
		if val != "" {
			return "#" + strings.ToUpper(val)
		}
	}

	// Also check for schemeClr reference.
	schemeClr := elem.FindElement("a:schemeClr")
	if schemeClr == nil {
		schemeClr = elem.FindElement("schemeClr")
	}
	if schemeClr != nil {
		// Return the scheme color reference (would need theme context to resolve).
		return schemeClr.SelectAttrValue("val", "")
	}

	return ""
}

// parseConnectorLineStyles extracts connector line styles from the theme.
// These define arrow types for connectors based on QuickStyleLineMatrix.
// Path: //vt:lineStyles/vt:fmtConnectorSchemeLineStyles/vt:lineStyle/vt:lineEx
func parseConnectorLineStyles(themeDoc *etree.Document) []ConnectorLineStyle {
	var styles []ConnectorLineStyle

	// Find the connector line styles section
	lineStyles := themeDoc.FindElement("//vt:fmtConnectorSchemeLineStyles")
	if lineStyles == nil {
		// Try without namespace prefix
		lineStyles = themeDoc.FindElement("//fmtConnectorSchemeLineStyles")
	}
	if lineStyles == nil {
		return styles
	}

	// Parse each lineStyle element
	for _, ls := range lineStyles.SelectElements("vt:lineStyle") {
		style := ConnectorLineStyle{
			BeginArrowSize: 2, // default
			EndArrowSize:   2, // default
			LinePattern:    1, // solid
		}

		// Find the lineEx child which contains the actual values
		lineEx := ls.FindElement("vt:lineEx")
		if lineEx == nil {
			lineEx = ls.FindElement("lineEx")
		}
		if lineEx != nil {
			// start = begin arrow type
			if v := lineEx.SelectAttrValue("start", ""); v != "" {
				style.BeginArrow = int(toFloat(v))
			}
			// end = end arrow type
			if v := lineEx.SelectAttrValue("end", ""); v != "" {
				style.EndArrow = int(toFloat(v))
			}
			// startSize = begin arrow size
			if v := lineEx.SelectAttrValue("startSize", ""); v != "" {
				style.BeginArrowSize = int(toFloat(v))
			}
			// endSize = end arrow size
			if v := lineEx.SelectAttrValue("endSize", ""); v != "" {
				style.EndArrowSize = int(toFloat(v))
			}
			// pattern = line pattern
			if v := lineEx.SelectAttrValue("pattern", ""); v != "" {
				style.LinePattern = int(toFloat(v))
			}
			// rndg = rounding
			if v := lineEx.SelectAttrValue("rndg", ""); v != "" {
				style.Rounding = toFloat(v)
			}
		}

		styles = append(styles, style)
	}

	return styles
}

// parseLineStyleColors extracts line style colors from the theme's fmtScheme/lnStyleLst.
// These are used for non-connector shapes via QuickStyleLineMatrix.
func parseLineStyleColors(themeDoc *etree.Document, colors ThemeColors) []LineStyleColor {
	var styles []LineStyleColor

	// Find lnStyleLst in fmtScheme
	lnStyleLst := themeDoc.FindElement("//a:fmtScheme/a:lnStyleLst")
	if lnStyleLst == nil {
		lnStyleLst = themeDoc.FindElement("//fmtScheme/lnStyleLst")
	}
	if lnStyleLst == nil {
		return styles
	}

	// Parse each ln element
	for _, ln := range lnStyleLst.SelectElements("a:ln") {
		style := LineStyleColor{}

		// Find solidFill child
		solidFill := ln.FindElement("a:solidFill")
		if solidFill == nil {
			solidFill = ln.FindElement("solidFill")
		}
		if solidFill != nil {
			// Check for phClr (placeholder color)
			schemeClr := solidFill.FindElement("a:schemeClr")
			if schemeClr == nil {
				schemeClr = solidFill.FindElement("schemeClr")
			}
			if schemeClr != nil && schemeClr.SelectAttrValue("val", "") == "phClr" {
				style.IsPhClr = true
				// Capture tint/shade for phClr
				if tint := schemeClr.FindElement("a:tint"); tint != nil {
					if v := tint.SelectAttrValue("val", ""); v != "" {
						style.PhClrTint = toFloat(v) / 100000.0
					}
				} else if tint := schemeClr.FindElement("tint"); tint != nil {
					if v := tint.SelectAttrValue("val", ""); v != "" {
						style.PhClrTint = toFloat(v) / 100000.0
					}
				}
				if shade := schemeClr.FindElement("a:shade"); shade != nil {
					if v := shade.SelectAttrValue("val", ""); v != "" {
						style.PhClrShade = toFloat(v) / 100000.0
					}
				} else if shade := schemeClr.FindElement("shade"); shade != nil {
					if v := shade.SelectAttrValue("val", ""); v != "" {
						style.PhClrShade = toFloat(v) / 100000.0
					}
				}
			} else {
				style.Color = parseSchemeColor(solidFill, colors)
			}
		}

		styles = append(styles, style)
	}

	return styles
}

// parseFontStyleColors extracts font style colors from the theme's fontStylesGroup.
// These are used for text colors via QuickStyleFontMatrix.
// Path: //vt:fontStylesGroup/vt:fontStyles/vt:fontProps/vt:color
func parseFontStyleColors(themeDoc *etree.Document, colors ThemeColors) []FontStyleColor {
	var styles []FontStyleColor

	// Find fontStyles in fontStylesGroup (not connectorFontStyles).
	fontStylesGroup := themeDoc.FindElement("//vt:fontStylesGroup")
	if fontStylesGroup == nil {
		fontStylesGroup = themeDoc.FindElement("//fontStylesGroup")
	}
	if fontStylesGroup == nil {
		return styles
	}

	// Get the fontStyles element (not connectorFontStyles).
	fontStyles := fontStylesGroup.FindElement("vt:fontStyles")
	if fontStyles == nil {
		fontStyles = fontStylesGroup.FindElement("fontStyles")
	}
	if fontStyles == nil {
		return styles
	}

	// Parse each fontProps element.
	for _, fp := range fontStyles.SelectElements("vt:fontProps") {
		style := FontStyleColor{}

		// Find the color element.
		colorElem := fp.FindElement("vt:color")
		if colorElem == nil {
			colorElem = fp.FindElement("color")
		}
		if colorElem != nil {
			style.Color = parseSchemeColor(colorElem, colors)
		}

		styles = append(styles, style)
	}

	// Also try without vt: namespace.
	if len(styles) == 0 {
		for _, fp := range fontStyles.SelectElements("fontProps") {
			style := FontStyleColor{}
			colorElem := fp.FindElement("color")
			if colorElem != nil {
				style.Color = parseSchemeColor(colorElem, colors)
			}
			styles = append(styles, style)
		}
	}

	return styles
}

// parseSchemeColor parses a schemeClr element and resolves it to an RGB color.
// Handles shade/tint transformations.
func parseSchemeColor(parent *etree.Element, colors ThemeColors) string {
	schemeClr := parent.FindElement("a:schemeClr")
	if schemeClr == nil {
		schemeClr = parent.FindElement("schemeClr")
	}
	if schemeClr == nil {
		// Try srgbClr directly
		if srgb := parent.FindElement("a:srgbClr"); srgb != nil {
			return "#" + strings.ToUpper(srgb.SelectAttrValue("val", ""))
		}
		if srgb := parent.FindElement("srgbClr"); srgb != nil {
			return "#" + strings.ToUpper(srgb.SelectAttrValue("val", ""))
		}
		return ""
	}

	// Get the scheme color reference
	schemeVal := schemeClr.SelectAttrValue("val", "")
	var baseColor string

	switch schemeVal {
	case "dk1":
		baseColor = colors.Dark1
	case "lt1":
		baseColor = colors.Light1
	case "dk2":
		baseColor = colors.Dark2
	case "lt2":
		baseColor = colors.Light2
	case "accent1":
		baseColor = colors.Accent1
	case "accent2":
		baseColor = colors.Accent2
	case "accent3":
		baseColor = colors.Accent3
	case "accent4":
		baseColor = colors.Accent4
	case "accent5":
		baseColor = colors.Accent5
	case "accent6":
		baseColor = colors.Accent6
	case "phClr":
		// Placeholder color - depends on the shape's fill
		// For now, return empty to use shape's color
		return ""
	default:
		return ""
	}

	if baseColor == "" {
		return ""
	}

	// Check for shade transformation
	if shade := schemeClr.FindElement("a:shade"); shade != nil {
		if v := shade.SelectAttrValue("val", ""); v != "" {
			shadeVal := toFloat(v) / 100000.0 // val is in 1/100000ths
			baseColor = applyShade(baseColor, shadeVal)
		}
	} else if shade := schemeClr.FindElement("shade"); shade != nil {
		if v := shade.SelectAttrValue("val", ""); v != "" {
			shadeVal := toFloat(v) / 100000.0
			baseColor = applyShade(baseColor, shadeVal)
		}
	}

	// Check for tint transformation
	if tint := schemeClr.FindElement("a:tint"); tint != nil {
		if v := tint.SelectAttrValue("val", ""); v != "" {
			tintVal := toFloat(v) / 100000.0
			baseColor = applyTint(baseColor, tintVal)
		}
	} else if tint := schemeClr.FindElement("tint"); tint != nil {
		if v := tint.SelectAttrValue("val", ""); v != "" {
			tintVal := toFloat(v) / 100000.0
			baseColor = applyTint(baseColor, tintVal)
		}
	}

	return baseColor
}

// applyShade applies a shade transformation to a color.
// Shade darkens a color using sRGB gamma-corrected math.
// DrawingML shade transformations use gamma 2.2 for perceptual correctness.
// shadeVal is a fraction (0-1) where lower values = darker.
func applyShade(color string, shadeVal float64) string {
	r, g, b := parseHexColor(color)
	if r < 0 {
		return color
	}

	// Apply gamma-corrected shade transformation.
	// In sRGB color space, the effective multiplier is shade^(1/gamma).
	const gamma = 2.2
	factor := math.Pow(shadeVal, 1.0/gamma)

	r = int(math.Round(float64(r) * factor))
	g = int(math.Round(float64(g) * factor))
	b = int(math.Round(float64(b) * factor))

	// Clamp to valid range
	if r > 255 {
		r = 255
	}
	if g > 255 {
		g = 255
	}
	if b > 255 {
		b = 255
	}

	return formatHexColor(r, g, b)
}

// applyTint applies a tint transformation to a color.
// Tint lightens a color by moving RGB values toward white.
// tintVal is a fraction (0-1) where higher values = lighter.
func applyTint(color string, tintVal float64) string {
	r, g, b := parseHexColor(color)
	if r < 0 {
		return color
	}

	// Tint moves color toward white
	r = int(float64(r) + (255.0-float64(r))*tintVal)
	g = int(float64(g) + (255.0-float64(g))*tintVal)
	b = int(float64(b) + (255.0-float64(b))*tintVal)

	// Clamp to valid range
	if r > 255 {
		r = 255
	}
	if g > 255 {
		g = 255
	}
	if b > 255 {
		b = 255
	}

	return formatHexColor(r, g, b)
}

// parseHexColor parses a hex color string like "#RGB" or "#RRGGBB".
// Returns -1 for all components if parsing fails.
func parseHexColor(color string) (r, g, b int) {
	color = strings.TrimPrefix(color, "#")
	if len(color) == 3 {
		// Short form #RGB -> #RRGGBB
		color = string(color[0]) + string(color[0]) +
			string(color[1]) + string(color[1]) +
			string(color[2]) + string(color[2])
	}
	if len(color) != 6 {
		return -1, -1, -1
	}

	val, err := strconv.ParseInt(color, 16, 32)
	if err != nil {
		return -1, -1, -1
	}

	r = int((val >> 16) & 0xFF)
	g = int((val >> 8) & 0xFF)
	b = int(val & 0xFF)
	return
}

// formatHexColor formats RGB values as a hex color string.
func formatHexColor(r, g, b int) string {
	// Use fmt-style formatting with zero-padding to ensure 6 hex digits.
	hex := strconv.FormatInt(int64(r<<16|g<<8|b), 16)
	for len(hex) < 6 {
		hex = "0" + hex
	}
	return "#" + strings.ToUpper(hex)
}

// ThemeColor returns a color from the theme by index.
// Index 0-11 map to the standard theme color slots.
func (t *Theme) ThemeColor(index int) string {
	if t == nil {
		return ""
	}
	switch index {
	case 0:
		return t.Colors.Dark1
	case 1:
		return t.Colors.Light1
	case 2:
		return t.Colors.Dark2
	case 3:
		return t.Colors.Light2
	case 4:
		return t.Colors.Accent1
	case 5:
		return t.Colors.Accent2
	case 6:
		return t.Colors.Accent3
	case 7:
		return t.Colors.Accent4
	case 8:
		return t.Colors.Accent5
	case 9:
		return t.Colors.Accent6
	case 10:
		return t.Colors.Hlink
	case 11:
		return t.Colors.FolHlink
	default:
		return ""
	}
}

// ResolveThemeColor resolves a color value that may reference the theme.
// If the color is a theme reference (e.g., "THEMEVAL()"), looks up in theme.
// Otherwise returns the color as-is.
func (s *Shape) ResolveThemeColor(cellName string) string {
	value := s.CellValue(cellName)
	if value == "" {
		return ""
	}

	// Check for THEMEVAL function reference.
	if strings.HasPrefix(strings.ToUpper(value), "THEMEVAL(") {
		return s.resolveThemeVal(value)
	}

	// Check for QuickStyle reference.
	if strings.HasPrefix(strings.ToUpper(value), "THEMEGUARD(") {
		// Extract the inner value.
		inner := value[11:]
		if idx := strings.Index(inner, ")"); idx > 0 {
			inner = inner[:idx]
		}
		if strings.HasPrefix(strings.ToUpper(inner), "THEMEVAL(") {
			return s.resolveThemeVal(inner)
		}
		return resolveColor(inner)
	}

	return resolveColor(value)
}

// resolveThemeVal resolves a THEMEVAL() function reference.
func (s *Shape) resolveThemeVal(value string) string {
	// Extract theme index from THEMEVAL(index) or THEMEVAL("name").
	inner := value[9:] // Remove "THEMEVAL("
	if idx := strings.Index(inner, ")"); idx > 0 {
		inner = inner[:idx]
	}
	inner = strings.Trim(inner, "\"' ")

	// Get document theme.
	vis := s.Page.vis
	if vis == nil {
		return ""
	}
	theme := vis.Theme()
	if theme == nil {
		return ""
	}

	// Try to parse as index.
	if index, err := parseInt(inner); err == nil {
		return theme.ThemeColor(index)
	}

	// Try to match by name.
	switch strings.ToLower(inner) {
	case "dk1", "dark1":
		return theme.Colors.Dark1
	case "lt1", "light1":
		return theme.Colors.Light1
	case "dk2", "dark2":
		return theme.Colors.Dark2
	case "lt2", "light2":
		return theme.Colors.Light2
	case "accent1":
		return theme.Colors.Accent1
	case "accent2":
		return theme.Colors.Accent2
	case "accent3":
		return theme.Colors.Accent3
	case "accent4":
		return theme.Colors.Accent4
	case "accent5":
		return theme.Colors.Accent5
	case "accent6":
		return theme.Colors.Accent6
	case "hlink":
		return theme.Colors.Hlink
	case "folhlink":
		return theme.Colors.FolHlink
	}

	return ""
}

// parseInt parses a string to int, returning error if not a valid number.
func parseInt(s string) (int, error) {
	var v int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			v = v*10 + int(c-'0')
		} else {
			return 0, &FileError{Path: s, Err: ErrInvalidFileType}
		}
	}
	return v, nil
}

// QuickStyle holds all 7 quick style slices per MS-VSDX §2.2.7.4.3.
type QuickStyle struct {
	Type          int // QuickStyleType: determines if connector or effect scheme is used
	Variation     int // QuickStyleVariation: selects variant (0-3)
	LineMatrix    int // QuickStyleLineMatrix: line style slice (0-5)
	FillMatrix    int // QuickStyleFillMatrix: fill style slice (0-5)
	EffectsMatrix int // QuickStyleEffectsMatrix: effects style slice (0-5)
	FontMatrix    int // QuickStyleFontMatrix: font style slice (0-5)
	LineColor     int // QuickStyleLineColor: color index (0-8)
	FillColor     int // QuickStyleFillColor: color index (0-8)
	ShadowColor   int // QuickStyleShadowColor: color index (0-8)
	FontColor     int // QuickStyleFontColor: color index (0-8)
}

// QuickStyle returns the quick style settings for this shape.
func (s *Shape) QuickStyle() *QuickStyle {
	return &QuickStyle{
		Type:          int(toFloat(s.CellValue(CellQuickStyleType))),
		Variation:     int(toFloat(s.CellValue(CellQuickStyleVariation))),
		LineMatrix:    int(toFloat(s.CellValue(CellQuickStyleLineMatrix))),
		FillMatrix:    int(toFloat(s.CellValue(CellQuickStyleFillMatrix))),
		EffectsMatrix: int(toFloat(s.CellValue(CellQuickStyleEffectsMatrix))),
		FontMatrix:    int(toFloat(s.CellValue(CellQuickStyleFontMatrix))),
		LineColor:     int(toFloat(s.CellValue(CellQuickStyleLineColor))),
		FillColor:     int(toFloat(s.CellValue(CellQuickStyleFillColor))),
		ShadowColor:   int(toFloat(s.CellValue(CellQuickStyleShadowColor))),
		FontColor:     int(toFloat(s.CellValue(CellQuickStyleFontColor))),
	}
}

// SetQuickStyle sets the quick style settings for this shape.
func (s *Shape) SetQuickStyle(qs *QuickStyle) {
	if qs == nil {
		return
	}
	s.SetCellValue(CellQuickStyleType, fmtInt(qs.Type))
	s.SetCellValue(CellQuickStyleVariation, fmtInt(qs.Variation))
	s.SetCellValue(CellQuickStyleLineMatrix, fmtInt(qs.LineMatrix))
	s.SetCellValue(CellQuickStyleFillMatrix, fmtInt(qs.FillMatrix))
	s.SetCellValue(CellQuickStyleEffectsMatrix, fmtInt(qs.EffectsMatrix))
	s.SetCellValue(CellQuickStyleFontMatrix, fmtInt(qs.FontMatrix))
	s.SetCellValue(CellQuickStyleLineColor, fmtInt(qs.LineColor))
	s.SetCellValue(CellQuickStyleFillColor, fmtInt(qs.FillColor))
	s.SetCellValue(CellQuickStyleShadowColor, fmtInt(qs.ShadowColor))
	s.SetCellValue(CellQuickStyleFontColor, fmtInt(qs.FontColor))
}

// fmtInt formats an integer for XML output.
func fmtInt(i int) string {
	return strconv.Itoa(i)
}

// QuickStyleFillColor returns the fill color determined by QuickStyle settings.
// This resolves the QuickStyleFillColor index through the theme's color scheme
// and variant per MS-VSDX §2.2.7.4.3.
func (s *Shape) QuickStyleFillColor() string {
	qsFillRaw := s.CellValue(CellQuickStyleFillColor)

	// If explicit QuickStyleFillColor is set, resolve through theme.
	if qsFillRaw != "" {
		return s.resolveQuickStyleColor(int(toFloat(qsFillRaw)), "fill")
	}

	// Otherwise, derive from type and variation.
	qsType := s.CellValue(CellQuickStyleType)
	qsVar := s.CellValue(CellQuickStyleVariation)
	if qsType == "" && qsVar == "" {
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

	// Map QuickStyleType to accent colors (indices 0-5 map to accent1-6).
	typeIdx := int(toFloat(qsType))
	if typeIdx >= 0 && typeIdx <= 5 {
		return theme.ThemeColor(4 + typeIdx) // accent1-6 are indices 4-9
	}

	return ""
}

// QuickStyleLineColor returns the line color determined by QuickStyle settings.
func (s *Shape) QuickStyleLineColor() string {
	qsLineRaw := s.CellValue(CellQuickStyleLineColor)
	if qsLineRaw != "" {
		return s.resolveQuickStyleColor(int(toFloat(qsLineRaw)), "line")
	}
	return ""
}

// QuickStyleShadowColor returns the shadow color determined by QuickStyle settings.
func (s *Shape) QuickStyleShadowColor() string {
	qsShadowRaw := s.CellValue(CellQuickStyleShadowColor)
	if qsShadowRaw != "" {
		return s.resolveQuickStyleColor(int(toFloat(qsShadowRaw)), "shadow")
	}
	return ""
}

// QuickStyleFontColor returns the font color determined by QuickStyle settings.
func (s *Shape) QuickStyleFontColor() string {
	qsFontRaw := s.CellValue(CellQuickStyleFontColor)
	if qsFontRaw != "" {
		return s.resolveQuickStyleColor(int(toFloat(qsFontRaw)), "font")
	}
	return ""
}

// resolveQuickStyleColor resolves a QuickStyle color index through the theme.
// Per MS-VSDX §2.2.7.4.3, color indices 0-8 map to:
// 0: dark1, 1: light1, 2: dark2, 3: light2, 4-9: accent1-6
// Special values:
// 100-107: Line style colors from lnStyleLst
// 200-206: Variant colors (varColor1-7) from variationClrScheme
func (s *Shape) resolveQuickStyleColor(colorIdx int, colorType string) string {
	vis := s.Page.vis
	if vis == nil {
		return ""
	}
	theme := vis.Theme()
	if theme == nil {
		return ""
	}

	// Get the variation index for variant-specific colors.
	varIdx := int(toFloat(s.CellValue(CellQuickStyleVariation)))

	// Handle variant color indices 200-206 (varColor1-7).
	// These map to the VarColors array in the current variant.
	if colorIdx >= 200 && colorIdx <= 206 {
		varColorIdx := colorIdx - 200 // 0-6 maps to varColor1-7
		if varIdx >= 0 && varIdx < len(theme.Variants) {
			variant := &theme.Variants[varIdx]
			if varColorIdx < len(variant.VarColors) && variant.VarColors[varColorIdx] != "" {
				return variant.VarColors[varColorIdx]
			}
		}
		// Fallback: use the first variant if available.
		if len(theme.Variants) > 0 {
			variant := &theme.Variants[0]
			if varColorIdx < len(variant.VarColors) && variant.VarColors[varColorIdx] != "" {
				return variant.VarColors[varColorIdx]
			}
		}
		return ""
	}

	// For standard indices, check if variant has specific color for this type.
	if varIdx >= 0 && varIdx < len(theme.Variants) {
		variant := &theme.Variants[varIdx]
		// Check if variant has specific color for this type.
		switch colorType {
		case "fill":
			if variant.FillColor != "" {
				return variant.FillColor
			}
		case "line":
			if variant.LineColor != "" {
				return variant.LineColor
			}
		}
	}

	// Fall back to base theme colors by index.
	return theme.ThemeColor(colorIdx)
}

// ThemeVariant returns the theme variant by index (0-3).
func (t *Theme) ThemeVariant(index int) *ThemeVariant {
	if t == nil || index < 0 || index >= len(t.Variants) {
		return nil
	}
	return &t.Variants[index]
}

// ThemeEffects returns the theme effects, or nil if not defined.
func (t *Theme) ThemeEffects() *ThemeEffects {
	if t == nil {
		return nil
	}
	return &t.Effects
}

// ResolveFillPattern resolves a fill pattern value that may reference the theme.
func (s *Shape) ResolveFillPattern() int {
	value := s.CellValue("FillPattern")
	if value == "" {
		return 1 // Default solid fill
	}

	// Check for THEMEVAL function reference.
	if strings.HasPrefix(strings.ToUpper(value), "THEMEVAL(") {
		// Theme fill patterns are typically 0 (none) or 1 (solid).
		return 1
	}

	return int(toFloat(value))
}

// ResolveLinePattern resolves a line pattern value that may reference the theme.
func (s *Shape) ResolveLinePattern() int {
	value := s.CellValue("LinePattern")
	if value == "" {
		return 1 // Default solid line
	}

	// Check for THEMEVAL function reference.
	if strings.HasPrefix(strings.ToUpper(value), "THEMEVAL(") {
		// Theme line patterns are typically 1 (solid).
		return 1
	}

	return int(toFloat(value))
}

// ThemeShadow returns the theme shadow properties for a shape.
func (s *Shape) ThemeShadow() (color string, offsetX, offsetY, blur float64) {
	vis := s.Page.vis
	if vis == nil {
		return "", 0, 0, 0
	}
	theme := vis.Theme()
	if theme == nil {
		return "", 0, 0, 0
	}

	effects := theme.ThemeEffects()
	if effects == nil {
		return "", 0, 0, 0
	}

	return effects.ShadowColor, effects.ShadowOffsetX, effects.ShadowOffsetY, effects.ShadowBlur
}

// ThemeGlow returns the theme glow properties for a shape.
func (s *Shape) ThemeGlow() (color string, size float64) {
	vis := s.Page.vis
	if vis == nil {
		return "", 0
	}
	theme := vis.Theme()
	if theme == nil {
		return "", 0
	}

	effects := theme.ThemeEffects()
	if effects == nil {
		return "", 0
	}

	return effects.GlowColor, effects.GlowSize
}

// ThemeSoftEdges returns the theme soft edges size for a shape.
func (s *Shape) ThemeSoftEdges() float64 {
	vis := s.Page.vis
	if vis == nil {
		return 0
	}
	theme := vis.Theme()
	if theme == nil {
		return 0
	}

	effects := theme.ThemeEffects()
	if effects == nil {
		return 0
	}

	return effects.SoftEdgesSize
}

// ThemeBevel returns the theme bevel properties for a shape.
func (s *Shape) ThemeBevel() (topType int, topWidth, topHeight float64, material, lighting int) {
	vis := s.Page.vis
	if vis == nil {
		return 0, 0, 0, 0, 0
	}
	theme := vis.Theme()
	if theme == nil {
		return 0, 0, 0, 0, 0
	}

	effects := theme.ThemeEffects()
	if effects == nil {
		return 0, 0, 0, 0, 0
	}

	return effects.BevelTopType, effects.BevelTopWidth, effects.BevelTopHeight,
		effects.BevelMaterial, effects.BevelLighting
}

// ThemeReflection returns the theme reflection properties for a shape.
func (s *Shape) ThemeReflection() (size, dist, blur float64) {
	vis := s.Page.vis
	if vis == nil {
		return 0, 0, 0
	}
	theme := vis.Theme()
	if theme == nil {
		return 0, 0, 0
	}

	effects := theme.ThemeEffects()
	if effects == nil {
		return 0, 0, 0
	}

	return effects.ReflectionSize, effects.ReflectionDist, effects.ReflectionBlur
}

// VariantFonts holds variant-specific font overrides.
type VariantFonts struct {
	MajorLatin   string
	MinorLatin   string
	MajorAsian   string
	MinorAsian   string
	MajorComplex string
	MinorComplex string
}

// ThemeVariantFonts returns font overrides for a specific variant index.
// If the variant has no font overrides, returns the base theme fonts.
func (t *Theme) ThemeVariantFonts(variantIndex int) *VariantFonts {
	if t == nil {
		return nil
	}

	// Start with base theme fonts
	fonts := &VariantFonts{
		MajorLatin:   t.Fonts.MajorLatin,
		MinorLatin:   t.Fonts.MinorLatin,
		MajorAsian:   t.Fonts.MajorAsian,
		MinorAsian:   t.Fonts.MinorAsian,
		MajorComplex: t.Fonts.MajorComplex,
		MinorComplex: t.Fonts.MinorComplex,
	}

	// Check if there are variant-specific overrides
	if variantIndex >= 0 && variantIndex < len(t.Variants) {
		// Variants can override fonts, but in standard Visio themes,
		// they typically only override colors. The base fonts are used.
	}

	return fonts
}

// VariantCount returns the number of variants in the theme.
func (t *Theme) VariantCount() int {
	if t == nil {
		return 0
	}
	return len(t.Variants)
}

// ApplyVariant applies a theme variant's colors to a shape.
func (s *Shape) ApplyVariant(variantIndex int) {
	vis := s.Page.vis
	if vis == nil {
		return
	}
	theme := vis.Theme()
	if theme == nil {
		return
	}

	variant := theme.ThemeVariant(variantIndex)
	if variant == nil {
		return
	}

	// Apply variant colors to shape
	if variant.FillColor != "" {
		s.SetCellValue("FillForegnd", variant.FillColor)
	}
	if variant.LineColor != "" {
		s.SetCellValue("LineColor", variant.LineColor)
	}
	if variant.TextColor != "" {
		// Apply text color through character formatting
		s.SetTextColor(variant.TextColor)
	}
}
