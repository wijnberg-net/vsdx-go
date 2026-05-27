package vsdx

import (
	"fmt"
	"strings"
)

// Shadow represents shadow settings for a shape.
type Shadow struct {
	Type       int     // 0=none, 1=simple, 2=oblique
	OffsetX    float64 // X offset in inches
	OffsetY    float64 // Y offset in inches
	Color      string  // Shadow color (#RRGGBB)
	Opacity    float64 // Opacity 0.0 - 1.0
	Blur       float64 // Blur radius in inches
	Scale      float64 // Scale factor for oblique shadows
	ObliqueAng float64 // Oblique angle in radians
}

// ShapeShadow returns the shadow settings for the shape.
// Returns nil if shadow is not enabled (ShapeShdwType=0).
// Note: MS-VSDX has two sets of shadow cells:
// - "Shdw*" cells for color/pattern (ShdwForegnd, ShdwForegndTrans)
// - "ShapeShdw*" cells for geometry (ShapeShdwType, ShapeShdwOffsetX/Y, etc.)
func (s *Shape) ShapeShadow() *Shadow {
	// Check ShapeShdwType (shape-level shadow type)
	shdwType := int(toFloat(s.CellValue("ShapeShdwType")))
	if shdwType == 0 {
		return nil
	}

	shadow := &Shadow{
		Type: shdwType,
	}

	// Get shadow offsets from ShapeShdw* cells (in inches)
	shadow.OffsetX = toFloat(s.CellValue("ShapeShdwOffsetX"))
	shadow.OffsetY = toFloat(s.CellValue("ShapeShdwOffsetY"))

	// Get shadow color from Shdw* cells
	shadow.Color = resolveColor(s.CellValue("ShdwForegnd"))
	if shadow.Color == "" {
		shadow.Color = "#000000" // Default black shadow
	}

	// Get shadow transparency (convert to opacity).
	trans := toFloat(s.CellValue("ShdwForegndTrans"))
	shadow.Opacity = 1.0 - trans
	if shadow.Opacity < 0 {
		shadow.Opacity = 0
	}
	if shadow.Opacity > 1 {
		shadow.Opacity = 1
	}
	// Default shadow opacity if not specified.
	if shadow.Opacity == 1.0 && s.CellValue("ShdwForegndTrans") == "" {
		shadow.Opacity = 0.5
	}

	// Note: ShapeShdwBlur is ignored because Visio's SVG export renders
	// sharp shadows (via duplicated paths) rather than blurred ones.
	// Setting blur=0 matches the golden SVG output more closely.
	shadow.Blur = 0

	// Get oblique shadow settings
	shadow.Scale = toFloat(s.CellValue("ShapeShdwScaleFactor"))
	if shadow.Scale == 0 {
		shadow.Scale = 1.0
	}
	shadow.ObliqueAng = toFloat(s.CellValue("ShapeShdwObliqueAngle"))

	return shadow
}

// shadowToSVGFilter generates an SVG <filter> element for a shadow.
// Returns the filter element string.
func shadowToSVGFilter(shadow *Shadow, id string, scaleX, scaleY float64, precision int) string {
	if shadow == nil || shadow.Type == 0 {
		return ""
	}

	var svg strings.Builder

	// Scale offsets to SVG units.
	avgScale := (scaleX + scaleY) / 2
	dx := shadow.OffsetX * avgScale
	dy := -shadow.OffsetY * avgScale // Flip Y for SVG
	blur := shadow.Blur * avgScale

	// Calculate filter region to contain shadow (extend beyond shape bounds).
	// Account for offset and blur.
	maxOffset := max(abs(dx), abs(dy)) + blur*2
	filterExtend := maxOffset / 100 // As percentage

	svg.WriteString(fmt.Sprintf(`<filter id="%s" x="%s%%" y="%s%%" width="%s%%" height="%s%%">`,
		id,
		fmtPrec(-50-filterExtend*100, precision),
		fmtPrec(-50-filterExtend*100, precision),
		fmtPrec(200+filterExtend*200, precision),
		fmtPrec(200+filterExtend*200, precision)))

	// For simple shadows, use feDropShadow or feGaussianBlur + feOffset combination.
	svg.WriteString(fmt.Sprintf(`<feDropShadow dx="%s" dy="%s" stdDeviation="%s" flood-color="%s" flood-opacity="%s"/>`,
		fmtPrec(dx, precision),
		fmtPrec(dy, precision),
		fmtPrec(blur, precision),
		shadow.Color,
		fmtPrec(shadow.Opacity, precision)))

	svg.WriteString("</filter>")

	return svg.String()
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
