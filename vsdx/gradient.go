package vsdx

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// GradientStop represents a color stop in a gradient.
type GradientStop struct {
	Position float64 // 0.0 - 1.0
	Color    string  // #RRGGBB
	Trans    float64 // Transparency 0.0 - 1.0
}

// Gradient represents fill gradient settings for a shape.
type Gradient struct {
	Enabled bool
	Type    string  // "linear" or "radial"
	Angle   float64 // In radians (linear only)
	Stops   []GradientStop

	// RadialDir mirrors Visio's FillGradientDir for radial gradients —
	// it picks WHERE the radial centre sits inside the shape's bbox.
	//   7 = top-left corner    (center at 0,0; radius covers diagonal)
	//   8 = top-right corner
	//   9 = bottom-left corner
	//  10 = bottom-right corner
	//  11 = centre (true centered radial)
	// Defaults to 11 (centre) when Type == "radial" but RadialDir is unset.
	RadialDir int
}

// FillGradient returns the fill gradient settings for the shape.
// Returns nil if gradient fill is not enabled.
//
// In addition to the explicit FillGradientEnabled path, Visio's FillPattern
// numbers 25-40 are "fade" pseudo-patterns that actually render as linear
// gradients between the shape's FillBkgnd and FillForegnd colours. We
// synthesise a Gradient for those numbers so the renderer picks them up
// through the same code path as a real gradient section.
func (s *Shape) FillGradient() *Gradient {
	if g := patternFillGradient(s); g != nil {
		return g
	}

	enabled := s.CellValue("FillGradientEnabled")
	if enabled != "1" {
		return nil
	}

	g := &Gradient{
		Enabled: true,
		Type:    "linear",
	}

	// Get gradient angle (in radians).
	g.Angle = toFloat(s.CellValue("FillGradientAngle"))

	// Get gradient direction type. FillGradientDir values 7-11 are radial
	// variants with different centre positions; treat all as radial and
	// remember the direction so the SVG emitter can place the centre.
	gradDir := s.CellValue("FillGradientDir")
	if dirN := int(toFloat(gradDir)); dirN >= 7 && dirN <= 11 {
		g.Type = "radial"
		g.RadialDir = dirN
	}

	// Parse gradient stops from Section N="FillGradient".
	section := s.xml.FindElement("Section[@N='FillGradient']")
	if section == nil {
		section = s.xml.FindElement("Section[@N='FillGradientStops']")
	}
	if section == nil {
		// No gradient section - use fill colors as fallback.
		fillColor := resolveColor(s.FillColor())
		if fillColor == "" {
			fillColor = "#000000"
		}
		g.Stops = []GradientStop{
			{Position: 0, Color: fillColor, Trans: 0},
			{Position: 1, Color: fillColor, Trans: 0},
		}
		return g
	}

	// Parse each Row in the section.
	for _, row := range section.SelectElements("Row") {
		stop := GradientStop{}

		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			value := cell.SelectAttrValue("V", "")

			switch name {
			case "GradientStopColor", "Color":
				stop.Color = resolveColor(value)
			case "GradientStopPosition", "Position":
				stop.Position = toFloat(value)
			case "GradientStopTrans", "Trans":
				stop.Trans = toFloat(value)
			}
		}

		if stop.Color != "" {
			g.Stops = append(g.Stops, stop)
		}
	}

	// Sort stops by position.
	sort.Slice(g.Stops, func(i, j int) bool {
		return g.Stops[i].Position < g.Stops[j].Position
	})

	// Ensure we have at least 2 stops.
	if len(g.Stops) < 2 {
		fillColor := resolveColor(s.FillColor())
		if fillColor == "" {
			fillColor = "#000000"
		}
		if len(g.Stops) == 0 {
			g.Stops = []GradientStop{
				{Position: 0, Color: fillColor, Trans: 0},
				{Position: 1, Color: fillColor, Trans: 0},
			}
		} else {
			// Add a second stop at the opposite end.
			if g.Stops[0].Position == 0 {
				g.Stops = append(g.Stops, GradientStop{Position: 1, Color: g.Stops[0].Color, Trans: g.Stops[0].Trans})
			} else {
				g.Stops = append([]GradientStop{{Position: 0, Color: g.Stops[0].Color, Trans: g.Stops[0].Trans}}, g.Stops...)
			}
		}
	}

	return g
}

// gradientToSVGDef generates an SVG gradient definition (<linearGradient> or <radialGradient>).
// Returns the gradient element string and its ID.
func gradientToSVGDef(g *Gradient, id string, precision int) string {
	if g == nil || len(g.Stops) < 2 {
		return ""
	}

	var svg strings.Builder

	if g.Type == "radial" {
		// Position the radial centre based on RadialDir. Visio's "radial
		// from corner" (FillGradientDir = 7-10) puts the gradient origin
		// at the bbox corner and uses radius 1.4 (140% of the bbox) so the
		// gradient covers the diagonal. RadialDir = 11 (or unset) is the
		// true centred radial.
		cx, cy, r := "50%", "50%", "50%"
		switch g.RadialDir {
		case 7: // top-left corner
			cx, cy, r = "0%", "0%", "140%"
		case 8: // top-right corner
			cx, cy, r = "100%", "0%", "140%"
		case 9: // bottom-left corner
			cx, cy, r = "0%", "100%", "140%"
		case 10: // bottom-right corner
			cx, cy, r = "100%", "100%", "140%"
		}
		svg.WriteString(fmt.Sprintf(`<radialGradient id="%s" cx="%s" cy="%s" r="%s" fx="%s" fy="%s">`,
			id, cx, cy, r, cx, cy))
	} else {
		// Linear gradient: place start/end on a unit circle around (50%,50%).
		// Visio's FillGradientAngle is measured clockwise from the X-axis in
		// screen-space (NOT counter-clockwise from Visio's Y-up frame).
		//   angle=0    → horizontal L→R
		//   angle=π/2  → top→bottom (Visio's "vertical" gradient flows down)
		//   angle=π/4  → top-left → bottom-right diagonal
		// That matches MS Office convention (PowerPoint, Visio UI). Got
		// confirmed against the comprehensive corpus where Visio's
		// `gradientTransform="rotate(45 0.5 0.5)"` rotates clockwise.
		// The earlier formula assumed Y-up math convention and so flipped
		// the diagonal for any non-cardinal angle.
		c := math.Cos(g.Angle)
		s := math.Sin(g.Angle)
		x1 := 50 - 50*c
		y1 := 50 - 50*s
		x2 := 50 + 50*c
		y2 := 50 + 50*s

		svg.WriteString(fmt.Sprintf(`<linearGradient id="%s" x1="%s%%" y1="%s%%" x2="%s%%" y2="%s%%">`,
			id,
			fmtPrec(x1, precision), fmtPrec(y1, precision),
			fmtPrec(x2, precision), fmtPrec(y2, precision)))
	}

	// Add color stops.
	for _, stop := range g.Stops {
		offset := stop.Position * 100
		color := stop.Color
		if color == "" {
			color = "#000000"
		}
		opacity := 1 - stop.Trans
		if opacity < 0 {
			opacity = 0
		}
		if opacity > 1 {
			opacity = 1
		}

		if opacity < 1 {
			svg.WriteString(fmt.Sprintf(`<stop offset="%s%%" stop-color="%s" stop-opacity="%s"/>`,
				fmtPrec(offset, precision), color, fmtPrec(opacity, precision)))
		} else {
			svg.WriteString(fmt.Sprintf(`<stop offset="%s%%" stop-color="%s"/>`,
				fmtPrec(offset, precision), color))
		}
	}

	if g.Type == "radial" {
		svg.WriteString("</radialGradient>")
	} else {
		svg.WriteString("</linearGradient>")
	}

	return svg.String()
}

// patternFillGradient maps FillPattern numbers 25-40 — Visio's "fade"
// pseudo-patterns — onto explicit Gradient objects using the shape's
// FillForegnd / FillBkgnd colours. Returns nil for FillPattern outside
// the gradient range or when FillPattern is 0/1/hatching (which the
// hatch-pattern emitter handles).
//
// Confirmed against the comprehensive corpus where Visio's resave
// turns FillPattern=25 into a horizontal fore→back gradient and
// FillPattern=26 into a back→fore→back centre bar.
func patternFillGradient(s *Shape) *Gradient {
	pat := int(toFloat(s.CellValue("FillPattern")))
	if pat < 25 || pat > 40 {
		return nil
	}
	fore := resolveColor(s.CellValue("FillForegnd"))
	if fore == "" {
		fore = "#000000"
	}
	back := resolveColor(s.CellValue("FillBkgnd"))
	if back == "" {
		back = "#FFFFFF"
	}

	g := &Gradient{Enabled: true, Type: "linear"}
	switch pat {
	case 25: // Linear fade fore → back, horizontal L→R
		g.Stops = []GradientStop{
			{Position: 0, Color: fore},
			{Position: 1, Color: back},
		}
	case 26: // Linear back → fore → back, horizontal centre bar
		g.Stops = []GradientStop{
			{Position: 0, Color: back},
			{Position: 0.5, Color: fore},
			{Position: 1, Color: back},
		}
	default:
		return nil
	}
	return g
}
