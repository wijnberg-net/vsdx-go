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
	Angle   float64 // In radians
	Stops   []GradientStop
}

// FillGradient returns the fill gradient settings for the shape.
// Returns nil if gradient fill is not enabled.
func (s *Shape) FillGradient() *Gradient {
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

	// Get gradient direction type.
	gradDir := s.CellValue("FillGradientDir")
	if gradDir == "7" {
		g.Type = "radial"
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
		svg.WriteString(fmt.Sprintf(`<radialGradient id="%s" cx="50%%" cy="50%%" r="50%%" fx="50%%" fy="50%%">`, id))
	} else {
		// Linear gradient: place start/end on a unit circle around (50%,50%),
		// then take a SVG-Y-down direction vector matching Visio's angle.
		// Visio: angle=0 → horizontal L→R, angle=π/2 → vertical bottom→top
		// (Y-up). In SVG (Y-down) the bottom→top direction has negative Y.
		c := math.Cos(g.Angle)
		s := math.Sin(g.Angle)
		x1 := 50 - 50*c
		y1 := 50 + 50*s
		x2 := 50 + 50*c
		y2 := 50 - 50*s

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
