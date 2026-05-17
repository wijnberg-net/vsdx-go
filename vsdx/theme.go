package vsdx

import (
	"strings"

	"github.com/beevik/etree"
)

// Theme represents the visual theme applied to a Visio document.
type Theme struct {
	Name   string
	Colors ThemeColors
	Fonts  ThemeFonts
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
	MajorLatin string // major font for Latin text (headings)
	MinorLatin string // minor font for Latin text (body)
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

	getFont := func(name string) string {
		elem := fontScheme.FindElement(name)
		if elem == nil {
			elem = fontScheme.FindElement("a:" + name)
		}
		if elem == nil {
			return ""
		}
		if latin := elem.FindElement("a:latin"); latin != nil {
			return latin.SelectAttrValue("typeface", "")
		}
		if latin := elem.FindElement("latin"); latin != nil {
			return latin.SelectAttrValue("typeface", "")
		}
		return ""
	}

	fonts.MajorLatin = getFont("majorFont")
	fonts.MinorLatin = getFont("minorFont")

	return fonts
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
