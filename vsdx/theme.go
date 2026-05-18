package vsdx

import (
	"strings"

	"github.com/beevik/etree"
)

// Theme represents the visual theme applied to a Visio document.
type Theme struct {
	Name     string
	Colors   ThemeColors
	Fonts    ThemeFonts
	Effects  ThemeEffects
	Variants []ThemeVariant
}

// ThemeColors holds the color palette from the theme.
type ThemeColors struct {
	Dark1   string // dk1 - typically black
	Light1  string // lt1 - typically white
	Dark2   string // dk2 - secondary dark
	Light2  string // lt2 - secondary light
	Accent1 string // accent1
	Accent2 string // accent2
	Accent3 string // accent3
	Accent4 string // accent4
	Accent5 string // accent5
	Accent6 string // accent6
	Hlink   string // hyperlink color
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
	Index       int    // variant index (0-3)
	FillColor   string // primary fill color
	FillColor2  string // secondary fill color
	LineColor   string // line color
	TextColor   string // text color
	AccentColor string // accent color
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

	// Get the first effect style (typically the default).
	effectStyle := effectStyleLst.FindElement("effectStyle")
	if effectStyle == nil {
		effectStyle = effectStyleLst.FindElement("a:effectStyle")
	}
	if effectStyle == nil {
		return effects
	}

	effectLst := effectStyle.FindElement("effectLst")
	if effectLst == nil {
		effectLst = effectStyle.FindElement("a:effectLst")
	}
	if effectLst == nil {
		return effects
	}

	// Parse outer shadow.
	outerShdw := effectLst.FindElement("outerShdw")
	if outerShdw == nil {
		outerShdw = effectLst.FindElement("a:outerShdw")
	}
	if outerShdw != nil {
		effects.ShadowBlur = parseEmu(outerShdw.SelectAttrValue("blurRad", "0"))
		effects.ShadowOffsetX = parseEmu(outerShdw.SelectAttrValue("dist", "0"))
		// Extract color from shadow.
		if srgb := outerShdw.FindElement("a:srgbClr"); srgb != nil {
			effects.ShadowColor = "#" + srgb.SelectAttrValue("val", "000000")
		} else if srgb := outerShdw.FindElement("srgbClr"); srgb != nil {
			effects.ShadowColor = "#" + srgb.SelectAttrValue("val", "000000")
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

		// Parse variant colors (varColor1 through varColor7).
		// varColor1 is typically the primary fill, varColor2-7 are accent colors.
		variant.FillColor = parseVariantColor(varScheme, "varColor1")
		variant.FillColor2 = parseVariantColor(varScheme, "varColor2")
		variant.LineColor = parseVariantColor(varScheme, "varColor3")
		variant.AccentColor = parseVariantColor(varScheme, "varColor5")

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

// QuickStyleFillColor returns the fill color determined by QuickStyle settings.
func (s *Shape) QuickStyleFillColor() string {
	qsType := s.CellValue("QuickStyleType")
	qsVar := s.CellValue("QuickStyleVariation")
	qsFill := s.CellValue("QuickStyleFillColor")

	// If explicit QuickStyleFillColor is set, use it.
	if qsFill != "" {
		return s.ResolveThemeColor("QuickStyleFillColor")
	}

	// Otherwise, derive from type and variation.
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

	// Map QuickStyleType to accent colors.
	typeIdx := int(toFloat(qsType))
	if typeIdx >= 0 && typeIdx <= 5 {
		return theme.ThemeColor(4 + typeIdx) // accent1-6 are indices 4-9
	}

	return ""
}

// QuickStyleLineColor returns the line color determined by QuickStyle settings.
func (s *Shape) QuickStyleLineColor() string {
	qsLine := s.CellValue("QuickStyleLineColor")
	if qsLine != "" {
		return s.ResolveThemeColor("QuickStyleLineColor")
	}
	return ""
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
