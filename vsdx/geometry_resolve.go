package vsdx

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// GeometryResolver resolves VSDX geometry into SVG-ready path data.
// All coordinate transformations, arc conversions, and arrow setbacks
// are applied during resolution - not during SVG emission.
type GeometryResolver struct {
	shape      *Shape
	geom       *Geometry
	style      *EffectiveStyle
	localW     float64 // shape's local width
	localH     float64 // shape's local height
	offsetX    float64 // offset in parent space
	offsetY    float64 // offset in parent space
	scaleX     float64 // scale factor for X
	scaleY     float64 // scale factor for Y
	parentH    float64 // parent height for Y-flip
	negativeH  bool    // true if height was originally negative
	precision  int     // decimal precision for output
	geomIndex  int     // index of this geometry in shape (0=first)
	totalGeoms int     // total number of geometries in shape

	// inheritScaleX/Y bridge master geometry to the instance's coordinate
	// frame when the shape has no local geometry section and inherits
	// everything from its master. The master's geometry rows use absolute
	// coordinates in the master's local space (e.g. a 2.17" template's
	// LineTo lands at X=2.17); without scaling, that geometry is rendered
	// at master size even when the instance is 0.16" wide. We multiply
	// every absolute-coord row by these factors before localToSVG so the
	// inherited path fits the instance. Equal to 1.0 in the common case
	// (instance dimensions = master dimensions, or shape has local rows).
	inheritScaleX float64
	inheritScaleY float64
	// allInherited is set when the entire geometry section passed to the
	// resolver is the master's (no local override). In that case every row
	// is implicitly inherited even though row.xml.Parent() equals geom.xml
	// (because geom IS the master's section).
	allInherited bool
}

// ResolvedGeomResult contains the resolved geometry ready for SVG emission.
type ResolvedGeomResult struct {
	PathData      string  // SVG path data (d attribute)
	Fill          string  // fill color
	Stroke        string  // stroke color
	StrokeWidth   float64 // stroke width in SVG units
	FillOpacity   float64 // 0-1
	DashArray     string  // stroke-dasharray
	LineCap       string  // stroke-linecap
	VectorEffect  string  // vector-effect attribute
	MarkerStart   string  // marker-start URL
	MarkerEnd     string  // marker-end URL
	FilterID      string  // filter URL for shadows
	GradientID    string  // gradient URL for fills
	FillPatternID string  // fill pattern URL for hatching (patterns 2-24)
	NoFill        bool    // geometry-level NoFill
	NoLine        bool    // geometry-level NoLine
	NoShow        bool    // geometry-level NoShow
}

// ResolveGeometry resolves a geometry section into SVG-ready path data.
func ResolveGeometry(
	shape *Shape,
	geom *Geometry,
	style *EffectiveStyle,
	localW, localH, offsetX, offsetY float64,
	scaleX, scaleY, parentH float64,
	negativeH bool,
	precision int,
	geomIndex, totalGeoms int,
) *ResolvedGeomResult {
	return ResolveGeometryWithInherit(shape, geom, style,
		localW, localH, offsetX, offsetY, scaleX, scaleY, parentH,
		negativeH, precision, geomIndex, totalGeoms, 1.0, 1.0)
}

// ResolveGeometryWithInherit is the same as ResolveGeometry but takes
// per-axis scale factors that get applied to absolute geometry coordinates.
// The render tree builder uses this when a shape inherits its entire
// geometry section from a master and the instance dimensions don't match
// the master's, so the master's absolute coords (in master's local space)
// have to be projected into the instance's local space.
//
// Callers that don't care about master-inheritance scaling should pass
// inheritScaleX = inheritScaleY = 1.0 (or use ResolveGeometry).
func ResolveGeometryWithInherit(
	shape *Shape,
	geom *Geometry,
	style *EffectiveStyle,
	localW, localH, offsetX, offsetY float64,
	scaleX, scaleY, parentH float64,
	negativeH bool,
	precision int,
	geomIndex, totalGeoms int,
	inheritScaleX, inheritScaleY float64,
) *ResolvedGeomResult {
	return ResolveGeometryWithInheritAll(shape, geom, style,
		localW, localH, offsetX, offsetY, scaleX, scaleY, parentH,
		negativeH, precision, geomIndex, totalGeoms,
		inheritScaleX, inheritScaleY, false)
}

// ResolveGeometryWithInheritAll extends ResolveGeometryWithInherit with an
// "allInherited" flag for callers that hand us a geometry section that
// physically belongs to the master shape (so every row should be scaled by
// inheritScale, not just rows whose xml.Parent() differs from geom.xml —
// which would otherwise return false here because the geom IS the master's
// section).
func ResolveGeometryWithInheritAll(
	shape *Shape,
	geom *Geometry,
	style *EffectiveStyle,
	localW, localH, offsetX, offsetY float64,
	scaleX, scaleY, parentH float64,
	negativeH bool,
	precision int,
	geomIndex, totalGeoms int,
	inheritScaleX, inheritScaleY float64,
	allInherited bool,
) *ResolvedGeomResult {
	if geom == nil || len(geom.Rows) == 0 {
		return nil
	}

	resolver := &GeometryResolver{
		shape:         shape,
		geom:          geom,
		style:         style,
		localW:        localW,
		localH:        localH,
		offsetX:       offsetX,
		offsetY:       offsetY,
		scaleX:        scaleX,
		scaleY:        scaleY,
		parentH:       parentH,
		negativeH:     negativeH,
		precision:     precision,
		geomIndex:     geomIndex,
		totalGeoms:    totalGeoms,
		inheritScaleX: inheritScaleX,
		inheritScaleY: inheritScaleY,
		allInherited:  allInherited,
	}

	return resolver.resolve()
}

func (r *GeometryResolver) resolve() *ResolvedGeomResult {
	result := &ResolvedGeomResult{
		Fill:        r.style.EffectiveFillColor(),
		Stroke:      r.style.EffectiveLineColor(),
		StrokeWidth: r.style.LineWeight,
		FillOpacity: r.style.EffectiveFillOpacity(),
	}

	// LineCap: Visio 0=round, 1=square, 2=extended (flat/butt)
	switch r.style.LineCap {
	case 1:
		result.LineCap = "square"
	case 2:
		result.LineCap = "butt"
	default:
		result.LineCap = "round"
	}

	// Check geometry-level visibility flags
	for _, c := range r.geom.Cells {
		switch c.Name() {
		case "NoFill":
			result.NoFill = c.Value() == "1"
		case "NoLine":
			result.NoLine = c.Value() == "1"
		case "NoShow":
			result.NoShow = c.Value() == "1"
		}
	}

	if result.NoShow {
		return result
	}

	// Override fill/stroke based on geometry flags
	if result.NoFill {
		result.Fill = "none"
	}

	// Create fill pattern for hatching patterns (2-24)
	if r.style.FillPattern >= 2 && r.style.FillPattern <= 24 && !result.NoFill {
		patDef := createFillPatternDef(r.style.FillPattern, r.style.FillForegnd, r.style.FillBkgnd)
		if patDef != nil {
			result.FillPatternID = patDef.ID
		}
	}
	if result.NoLine {
		result.Stroke = "none"
	}

	// Resolve dash array
	if r.style.LinePattern > 1 && result.Stroke != "none" {
		result.DashArray = linePatternToSVG(r.style.LinePattern, result.StrokeWidth)
	}

	// Note: we intentionally do NOT set vector-effect="non-scaling-stroke".
	// Our render tree only applies translation transforms (no scaling), so
	// strokes already render at their declared width in user-space units -
	// matching Visio's output. Forcing non-scaling-stroke makes rsvg-convert
	// (and Chrome) treat stroke-width as device pixels, producing strokes
	// roughly 30% thinner than Visio's at the same viewBox scale.

	// Build path data
	pathData := r.buildPathData()
	if pathData == "" {
		return result
	}

	// Apply corner rounding if the shape has a Rounding cell > 0. MS-VSDX
	// §2.2.7.5: Rounding rounds every right-angle corner. We only handle
	// the simple axis-aligned rectangle case here (Visio's <rect rx ry>
	// equivalent); paths with other shapes pass through unchanged.
	if r.style.Rounding > 0 {
		ppi := 72.0
		if rounded := roundAxisAlignedRectangle(pathData, r.style.Rounding*ppi); rounded != "" {
			pathData = rounded
		}
	}

	// Apply arrow setbacks BEFORE finalizing path
	// This is critical: setbacks must be applied in resolved coordinate space
	// Include stroke width in marker ID to allow per-stroke-width marker sizing
	swKey := fmt.Sprintf("%.0f", r.style.LineWeight*10) // e.g., "30" for 3pt, "10" for 1pt
	if r.style.BeginArrow > 0 {
		setback := r.computeArrowSetbackInSVGUnits(r.style.BeginArrowSetback)
		if setback > 0 {
			pathData = r.shortenPathStart(pathData, setback)
		}
		result.MarkerStart = fmt.Sprintf("url(#arrow_%d_%d_%s_%s_start)",
			r.style.BeginArrow, r.style.BeginArrowSize, strings.TrimPrefix(result.Stroke, "#"), swKey)
	}
	if r.style.EndArrow > 0 {
		setback := r.computeArrowSetbackInSVGUnits(r.style.EndArrowSetback)
		if setback > 0 {
			pathData = r.shortenPathEnd(pathData, setback)
		}
		result.MarkerEnd = fmt.Sprintf("url(#arrow_%d_%d_%s_%s_end)",
			r.style.EndArrow, r.style.EndArrowSize, strings.TrimPrefix(result.Stroke, "#"), swKey)
	}

	result.PathData = pathData

	return result
}

// computeArrowSetbackInSVGUnits converts the setback from points to SVG coordinate units.
// The setback in EffectiveStyle is in points, but we need it in the same units as the path.
func (r *GeometryResolver) computeArrowSetbackInSVGUnits(setbackPts float64) float64 {
	if setbackPts <= 0 {
		return 0
	}
	// Convert points to SVG units using the average scale factor
	// Points are 1/72 inch, and our scale converts inches to SVG units
	avgScale := (r.scaleX + r.scaleY) / 2
	return (setbackPts / 72.0) * avgScale
}

// rowScaleX / rowScaleY return the appropriate scale factor for a row's
// absolute coordinates. Rows whose XML lives in the master's geometry
// section (inherited) get the master-to-instance scale; rows in the local
// geometry section keep coords as-is.
func (r *GeometryResolver) rowScaleX(row *GeometryRow) float64 {
	if r.isInherited(row) {
		return r.inheritScaleX
	}
	return 1.0
}
func (r *GeometryResolver) rowScaleY(row *GeometryRow) float64 {
	if r.isInherited(row) {
		return r.inheritScaleY
	}
	return 1.0
}
func (r *GeometryResolver) isInherited(row *GeometryRow) bool {
	if r.allInherited {
		return true
	}
	if row == nil || r.geom == nil || row.xml == nil || r.geom.xml == nil {
		return false
	}
	return row.xml.Parent() != r.geom.xml
}

// absX / absY scale an absolute geometry coordinate to the instance's local
// space, accounting for whether the row came from the master or the local
// section. Identity when inheritScale is 1.0.
func (r *GeometryResolver) absX(v float64) float64 { return v * r.inheritScaleX }
func (r *GeometryResolver) absY(v float64) float64 { return v * r.inheritScaleY }

// localToSVG transforms local Visio coordinates to SVG coordinates.
func (r *GeometryResolver) localToSVG(visioX, visioY float64) (float64, float64) {
	svgX := visioX * r.scaleX
	var svgY float64
	if r.negativeH {
		svgY = -visioY * r.scaleY
	} else {
		svgY = (r.parentH - visioY) * r.scaleY
	}
	return svgX, svgY
}

func (r *GeometryResolver) buildPathData() string {
	sortedRows := r.sortGeometryRows()
	if len(sortedRows) == 0 {
		return ""
	}

	var d strings.Builder
	var prevX, prevY float64
	var startX, startY float64

	for _, row := range sortedRows {
		rt := strings.ToLower(row.RowType())
		switch rt {
		case "moveto":
			sx, sy := row.X()*r.rowScaleX(row), row.Y()*r.rowScaleY(row)
			svgX, svgY := r.localToSVG(sx+r.offsetX, sy+r.offsetY)
			d.WriteString(fmt.Sprintf("M%s %s", r.fmtNum(svgX), r.fmtNum(svgY)))
			prevX, prevY = svgX, svgY
			startX, startY = svgX, svgY

		case "lineto":
			sx, sy := row.X()*r.rowScaleX(row), row.Y()*r.rowScaleY(row)
			svgX, svgY := r.localToSVG(sx+r.offsetX, sy+r.offsetY)
			d.WriteString(fmt.Sprintf("L%s %s", r.fmtNum(svgX), r.fmtNum(svgY)))
			prevX, prevY = svgX, svgY

		case "relmoveto":
			rx, ry := row.X(), row.Y()
			absX := rx*r.localW + r.offsetX
			absY := ry*r.localH + r.offsetY
			svgX, svgY := r.localToSVG(absX, absY)
			d.WriteString(fmt.Sprintf("M%s %s", r.fmtNum(svgX), r.fmtNum(svgY)))
			startX, startY = svgX, svgY
			prevX, prevY = svgX, svgY

		case "rellineto":
			rx, ry := row.X(), row.Y()
			absX := rx*r.localW + r.offsetX
			absY := ry*r.localH + r.offsetY
			svgX, svgY := r.localToSVG(absX, absY)
			d.WriteString(fmt.Sprintf("L%s %s", r.fmtNum(svgX), r.fmtNum(svgY)))
			prevX, prevY = svgX, svgY

		case "arcto":
			sx, sy := row.X()*r.rowScaleX(row), row.Y()*r.rowScaleY(row)
			svgX, svgY := r.localToSVG(sx+r.offsetX, sy+r.offsetY)
			bow := r.cellFloat(row, "A")
			arcSVG := r.arcToSVG(prevX, prevY, svgX, svgY, bow*((r.scaleX+r.scaleY)/2))
			d.WriteString(arcSVG)
			prevX, prevY = svgX, svgY

		case "ellipticalarcto":
			rsX := r.rowScaleX(row)
			rsY := r.rowScaleY(row)
			ex, ey := row.X()*rsX, row.Y()*rsY
			svgEndX, svgEndY := r.localToSVG(ex+r.offsetX, ey+r.offsetY)
			cpX := r.cellFloat(row, "A")*rsX + r.offsetX
			cpY := r.cellFloat(row, "B")*rsY + r.offsetY
			rotAngleDeg := r.cellFloat(row, "C")
			aspectRatio := r.cellFloat(row, "D")

			startX := (prevX/r.scaleX - r.offsetX)
			startY := r.parentH - (prevY/r.scaleY) - r.offsetY

			arcSVG := r.ellipticalArcToSVG(startX, startY, cpX-r.offsetX, cpY-r.offsetY, ex, ey,
				aspectRatio, rotAngleDeg)
			if arcSVG != "" {
				d.WriteString(arcSVG)
			} else {
				d.WriteString(fmt.Sprintf("L%s %s", r.fmtNum(svgEndX), r.fmtNum(svgEndY)))
			}
			prevX, prevY = svgEndX, svgEndY

		case "relellipticalarcto":
			rx, ry := row.X(), row.Y()
			absX := rx*r.localW + r.offsetX
			absY := ry*r.localH + r.offsetY
			svgX, svgY := r.localToSVG(absX, absY)
			cpRX := r.cellFloat(row, "A")
			cpRY := r.cellFloat(row, "B")
			rotAngleDeg := r.cellFloat(row, "C")
			aspectRatio := r.cellFloat(row, "D")
			cpX := cpRX*r.localW + r.offsetX
			cpY := cpRY*r.localH + r.offsetY

			startX := (prevX/r.scaleX - r.offsetX)
			startY := r.parentH - (prevY/r.scaleY) - r.offsetY

			arcSVG := r.ellipticalArcToSVG(startX, startY, cpX-r.offsetX, cpY-r.offsetY,
				absX-r.offsetX, absY-r.offsetY, aspectRatio, rotAngleDeg)
			if arcSVG != "" {
				d.WriteString(arcSVG)
			} else {
				d.WriteString(fmt.Sprintf("L%s %s", r.fmtNum(svgX), r.fmtNum(svgY)))
			}
			prevX, prevY = svgX, svgY

		case "relcubbezto":
			rx, ry := row.X(), row.Y()
			absX := rx*r.localW + r.offsetX
			absY := ry*r.localH + r.offsetY
			svgX, svgY := r.localToSVG(absX, absY)
			cp1X := r.cellFloat(row, "A")*r.localW + r.offsetX
			cp1Y := r.cellFloat(row, "B")*r.localH + r.offsetY
			cp2X := r.cellFloat(row, "C")*r.localW + r.offsetX
			cp2Y := r.cellFloat(row, "D")*r.localH + r.offsetY
			cp1SvgX, cp1SvgY := r.localToSVG(cp1X, cp1Y)
			cp2SvgX, cp2SvgY := r.localToSVG(cp2X, cp2Y)
			d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
				r.fmtNum(cp1SvgX), r.fmtNum(cp1SvgY),
				r.fmtNum(cp2SvgX), r.fmtNum(cp2SvgY),
				r.fmtNum(svgX), r.fmtNum(svgY)))
			prevX, prevY = svgX, svgY

		case "nurbsto":
			sx, sy := row.X()*r.rowScaleX(row), row.Y()*r.rowScaleY(row)
			svgX, svgY := r.localToSVG(sx+r.offsetX, sy+r.offsetY)
			eFormula := r.cellString(row, "E")
			nurbsInfo := parseNURBSData(eFormula)
			// Cell A = knot value of the LAST control point (the END's knot).
			// This is the actual interior-knot position for Boehm B-spline →
			// Bezier conversion (NOT a uniformly-spaced midpoint).
			if nurbsInfo != nil {
				nurbsInfo.lastCPKnot = r.cellFloat(row, "A")
			}

			nurbsSVG, _, _, actualEndX, actualEndY := r.nurbsToSVGWithCP(prevX, prevY, svgX, svgY, sx+r.offsetX, sy+r.offsetY, nurbsInfo)
			d.WriteString(nurbsSVG)
			prevX, prevY = actualEndX, actualEndY

		case "polylineto":
			sx, sy := row.X()*r.rowScaleX(row), row.Y()*r.rowScaleY(row)
			svgX, svgY := r.localToSVG(sx+r.offsetX, sy+r.offsetY)
			aFormula := r.cellString(row, "A")
			pts := parsePolylinePoints(aFormula, r.localW, r.localH, r.offsetX, r.offsetY)

			for _, pt := range pts {
				ptX, ptY := r.localToSVG(pt.x, pt.y)
				d.WriteString(fmt.Sprintf("L%s %s", r.fmtNum(ptX), r.fmtNum(ptY)))
			}
			d.WriteString(fmt.Sprintf("L%s %s", r.fmtNum(svgX), r.fmtNum(svgY)))
			prevX, prevY = svgX, svgY

		case "infiniteline":
			// InfiniteLine: two points defining an infinite line; render as line segment
			rsX := r.rowScaleX(row)
			rsY := r.rowScaleY(row)
			sx := r.cellFloat(row, "X")*rsX + r.offsetX
			sy := r.cellFloat(row, "Y")*rsY + r.offsetY
			ax := r.cellFloat(row, "A")*rsX + r.offsetX
			ay := r.cellFloat(row, "B")*rsY + r.offsetY
			svgX1, svgY1 := r.localToSVG(sx, sy)
			svgX2, svgY2 := r.localToSVG(ax, ay)
			d.WriteString(fmt.Sprintf("M%s %s L%s %s",
				r.fmtNum(svgX1), r.fmtNum(svgY1),
				r.fmtNum(svgX2), r.fmtNum(svgY2)))
			prevX, prevY = svgX2, svgY2

		case "ellipse":
			// Ellipse cells are all absolute coordinates in the shape's local
			// frame, so they all pick up the per-row inheritance scale.
			rsX := r.rowScaleX(row)
			rsY := r.rowScaleY(row)
			cx := r.cellFloat(row, "X")*rsX + r.offsetX
			cy := r.cellFloat(row, "Y")*rsY + r.offsetY
			svgCx, svgCy := r.localToSVG(cx, cy)
			ax := r.cellFloat(row, "A") * rsX
			ay := r.cellFloat(row, "B") * rsY
			cx2 := r.cellFloat(row, "C") * rsX
			cy2 := r.cellFloat(row, "D") * rsY
			rx := math.Sqrt(math.Pow(ax-cx+r.offsetX, 2)+math.Pow(ay-cy+r.offsetY, 2)) * ((r.scaleX + r.scaleY) / 2)
			ry := math.Sqrt(math.Pow(cx2-cx+r.offsetX, 2)+math.Pow(cy2-cy+r.offsetY, 2)) * ((r.scaleX + r.scaleY) / 2)
			if rx < 0.001 {
				rx = 0.001
			}
			if ry < 0.001 {
				ry = 0.001
			}
			d.WriteString(fmt.Sprintf("M%s %s A%s %s 0 1 0 %s %s A%s %s 0 1 0 %s %s Z",
				r.fmtNum(svgCx-rx), r.fmtNum(svgCy),
				r.fmtNum(rx), r.fmtNum(ry),
				r.fmtNum(svgCx+rx), r.fmtNum(svgCy),
				r.fmtNum(rx), r.fmtNum(ry),
				r.fmtNum(svgCx-rx), r.fmtNum(svgCy)))
			prevX, prevY = svgCx-rx, svgCy
		}
		_ = startX
		_ = startY
	}

	// Note: SVG markers with orient="auto" automatically calculate direction
	// from the path tangent at the endpoint, so no direction stub is needed.
	// The marker tip position is controlled by refX in the marker definition.

	return d.String()
}

func (r *GeometryResolver) sortGeometryRows() []*GeometryRow {
	sorted := make([]*GeometryRow, 0, len(r.geom.Rows))
	for _, row := range r.geom.Rows {
		sorted = append(sorted, row)
	}
	sort.Slice(sorted, func(i, j int) bool {
		a, _ := parseIX(sorted[i].Index())
		b, _ := parseIX(sorted[j].Index())
		return a < b
	})
	return sorted
}

func (r *GeometryResolver) cellFloat(row *GeometryRow, name string) float64 {
	if c, ok := row.Cells[name]; ok {
		return toFloat(c.Value())
	}
	return 0
}

func (r *GeometryResolver) cellString(row *GeometryRow, name string) string {
	if c, ok := row.Cells[name]; ok {
		if v := c.Value(); v != "" {
			return v
		}
		return c.Formula()
	}
	return ""
}

func (r *GeometryResolver) fmtNum(v float64) string {
	s := fmt.Sprintf("%.*f", r.precision, v)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// arcToSVG converts a Visio ArcTo to SVG arc command.
func (r *GeometryResolver) arcToSVG(x1, y1, x2, y2, bow float64) string {
	if math.Abs(bow) < 1e-6 {
		return fmt.Sprintf("L%s %s", r.fmtNum(x2), r.fmtNum(y2))
	}

	dx := x2 - x1
	dy := y2 - y1
	chord := math.Sqrt(dx*dx + dy*dy)
	if chord < 1e-10 {
		return ""
	}

	absBow := math.Abs(bow)
	radius := (chord*chord/4 + bow*bow) / (2 * absBow)

	largeArc := 0
	if absBow > radius {
		largeArc = 1
	}

	sweep := 0
	if bow < 0 {
		sweep = 1
	}

	return fmt.Sprintf("A%s %s 0 %d %d %s %s",
		r.fmtNum(radius), r.fmtNum(radius),
		largeArc, sweep,
		r.fmtNum(x2), r.fmtNum(y2))
}

// ellipticalArcToSVG converts a Visio EllipticalArcTo to SVG arc command.
func (r *GeometryResolver) ellipticalArcToSVG(startX, startY, cpX, cpY, endX, endY, aspectRatio, rotAngleDeg float64) string {
	svgEndX, svgEndY := r.localToSVG(endX+r.offsetX, endY+r.offsetY)

	dx := endX - startX
	dy := endY - startY
	chordLen := math.Sqrt(dx*dx + dy*dy)
	if chordLen < 1e-10 {
		return ""
	}

	midX := (startX + endX) / 2
	midY := (startY + endY) / 2
	distToCP := math.Sqrt(math.Pow(cpX-midX, 2) + math.Pow(cpY-midY, 2))
	bow := distToCP * ((r.scaleX + r.scaleY) / 2)

	crossZ := (endX-startX)*(cpY-startY) - (endY-startY)*(cpX-startX)
	sweep := 0
	if crossZ > 0 {
		sweep = 1
	}

	if bow < 1e-6 {
		return fmt.Sprintf("L%s %s", r.fmtNum(svgEndX), r.fmtNum(svgEndY))
	}

	if aspectRatio <= 0 {
		aspectRatio = 1.0
	}

	var rx, ry float64
	minRx := chordLen / 2
	ry = bow
	rx = ry * aspectRatio

	if rx < minRx {
		scale := minRx / rx * 1.01
		rx = rx * scale
		ry = ry * scale
	}

	largeArc := 0
	if bow > ry {
		largeArc = 1
	}

	return fmt.Sprintf("A%s %s %s %d %d %s %s",
		r.fmtNum(rx), r.fmtNum(ry),
		r.fmtNum(rotAngleDeg), largeArc, sweep,
		r.fmtNum(svgEndX), r.fmtNum(svgEndY))
}

// nurbsToSVG converts NURBS data to SVG bezier curves.
// Uses B-spline evaluation and Bezier fitting for accurate curve representation.
func (r *GeometryResolver) nurbsToSVG(prevX, prevY, svgX, svgY, endX, endY float64, nurbsInfo *nurbsData) string {
	svg, _, _, _, _ := r.nurbsToSVGWithCP(prevX, prevY, svgX, svgY, endX, endY, nurbsInfo)
	return svg
}

// nurbsToSVGWithCP converts NURBS data to SVG and returns the last control point and actual endpoint.
// Returns: (svg, lastCPX, lastCPY, actualEndX, actualEndY)
func (r *GeometryResolver) nurbsToSVGWithCP(prevX, prevY, svgX, svgY, endX, endY float64, nurbsInfo *nurbsData) (string, float64, float64, float64, float64) {
	// Default control point is the previous point (for straight lines)
	lastCPX, lastCPY := prevX, prevY
	actualEndX, actualEndY := svgX, svgY
	if nurbsInfo == nil || len(nurbsInfo.cps) == 0 {
		return fmt.Sprintf("L%s %s", r.fmtNum(svgX), r.fmtNum(svgY)), lastCPX, lastCPY, actualEndX, actualEndY
	}

	// Convert control points to absolute local coordinates
	absPoints := make([][2]float64, len(nurbsInfo.cps))
	for i, cp := range nurbsInfo.cps {
		if nurbsInfo.xType == 0 {
			absPoints[i][0] = cp.x*r.localW + r.offsetX
		} else {
			absPoints[i][0] = cp.x + r.offsetX
		}
		if nurbsInfo.yType == 0 {
			absPoints[i][1] = cp.y*r.localH + r.offsetY
		} else {
			absPoints[i][1] = cp.y + r.offsetY
		}
	}

	var d strings.Builder

	if len(nurbsInfo.cps) == 2 {
		// Two-control-point NURBS: map directly to a cubic Bezier.
		// - Bezier CP1 = formula CP1
		// - Bezier CP2 = formula CP2
		// - Bezier endpoint = the NURBSTo row's (X, Y), which is the curve's
		//   true logical end (used by Visio for connector endpoints, arrow
		//   placement, etc.). Earlier code rebuilt the endpoint from
		//   (CP1.x, CP2.y) when yType=1, but that synthetic endpoint sits
		//   ~10 path-units before the row's X/Y for typical right-leading
		//   connectors, leaving the marker tip floating short of the target
		//   shape after the arrow setback is applied.

		// Convert formula CPs to SVG coordinates
		cp1SvgX, cp1SvgY := r.localToSVG(absPoints[0][0], absPoints[0][1])
		cp2SvgX, cp2SvgY := r.localToSVG(absPoints[1][0], absPoints[1][1])

		actualEndSvgX, actualEndSvgY := svgX, svgY

		// Guard against a degenerate Bezier where the trailing control point
		// coincides with the endpoint. SVG's orient="auto" then has no
		// defined tangent and rsvg-convert/Chrome fall back to the marker's
		// default (right-pointing) orientation - the rotated-arrow symptom
		// seen on straight vertical connectors. Emitting a line preserves
		// the visual geometry while giving markers a well-defined direction.
		const degenerateEps = 1e-3
		if math.Abs(cp2SvgX-actualEndSvgX) < degenerateEps && math.Abs(cp2SvgY-actualEndSvgY) < degenerateEps {
			d.WriteString(fmt.Sprintf("L%s %s",
				r.fmtNum(actualEndSvgX), r.fmtNum(actualEndSvgY)))
		} else {
			d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
				r.fmtNum(cp1SvgX), r.fmtNum(cp1SvgY),
				r.fmtNum(cp2SvgX), r.fmtNum(cp2SvgY),
				r.fmtNum(actualEndSvgX), r.fmtNum(actualEndSvgY)))
		}
		lastCPX, lastCPY = cp2SvgX, cp2SvgY
		actualEndX, actualEndY = actualEndSvgX, actualEndSvgY
	} else if len(nurbsInfo.cps) == 1 {
		// Degree 2, 1 control point → quadratic Bezier
		cpSvgX, cpSvgY := r.localToSVG(absPoints[0][0], absPoints[0][1])
		d.WriteString(fmt.Sprintf("Q%s %s %s %s",
			r.fmtNum(cpSvgX), r.fmtNum(cpSvgY),
			r.fmtNum(svgX), r.fmtNum(svgY)))
		lastCPX, lastCPY = cpSvgX, cpSvgY
	} else if len(nurbsInfo.cps) >= 3 {
		// Visio's 3-CP NURBS connectors all have interior knots = 0 in their
		// formula data. Mathematically this stacks the start-clamp knot
		// multiplicity, degenerating the curve so that Visio's intended
		// shape depends on whether the interior control points stay near
		// the start-end straight line (gentle arc) or fly off well past it
		// (a wavy / S-shaped connector).
		//
		// Our uniform-knot LSQ-fit produces a symmetric two-segment Bezier
		// that loops the first segment back through CP1 - acceptable for
		// the wavy case (where the wave is intended) but visibly wrong for
		// the gentle-arc case (where it manifests as an S-curve dip near
		// the start). Distinguish the two cases by looking at whether the
		// interior CP X values stay within the start-end bounding box; if
		// they do, collapse to a single cubic Bezier using the outermost
		// interior CPs as control points.
		// The gentle-arc single-Bezier collapse only applies when the
		// formula's interior control points have a NON-MONOTONIC X
		// sequence (i.e. X goes back-and-forth). That's the geometric
		// signature of the "loop-back" CP polygon that makes our
		// uniform-knot LSQ multi-Bezier fit emit a visible S-curve at
		// the start (concrete example: shape 30 Provisioning ↔ Registry,
		// CPs X = 0.121, 0.066, 0.516 - dip at CP2).
		//
		// When the CP X values are monotonic AND inside the start-end
		// X bounding box, the LSQ multi-Bezier fit produces a curve
		// shape that matches Visio's golden output very closely
		// (concrete example: shape 24 Event Processing → Warm Storage,
		// CPs X = 0.25, 1, 1 - monotonic, L-shape rendering correct).
		gentleArc := false
		if len(nurbsInfo.cps) >= 2 {
			startLocalX := prevX / r.scaleX
			minX, maxX := startLocalX, endX
			if minX > maxX {
				minX, maxX = maxX, minX
			}
			span := maxX - minX
			tol := span * 0.10

			// Check that all CPs are inside the bounding box.
			inBounds := true
			for _, p := range absPoints {
				if p[0] < minX-tol || p[0] > maxX+tol {
					inBounds = false
					break
				}
			}

			// Check monotonic X progression in the CP polygon
			// (start → CP[0] → CP[1] → ... → end).
			monotonic := true
			prev := startLocalX
			ascending := endX >= startLocalX
			for _, p := range absPoints {
				if ascending && p[0]+tol < prev {
					monotonic = false
					break
				}
				if !ascending && p[0]-tol > prev {
					monotonic = false
					break
				}
				prev = p[0]
			}

			// Single-Bezier collapse only for in-bounds AND non-monotonic.
			gentleArc = inBounds && !monotonic
		}

		if gentleArc {
			// Match Visio's curve tangent at start/end empirically.
			//
			// For Visio's degenerate-knot NURBS (interior knots all = 0),
			// the rendered start tangent direction lands close to the
			// direction from the MoveTo point to the AVERAGE of the first
			// two interior control points - NOT either CP individually.
			// (Concrete example: shape 30 in logical-architecture has
			//  Visio tangent angle -38.2° at start, where -38.8° comes
			//  from avg(CP[0],CP[1]), -66° from CP[0] alone, 0° from CP[1]
			//  alone.) The end tangent direction matches the formula's
			//  last interior CP, so we use that directly.
			//
			// Result: Bezier cp1 = average of the first two interior CPs,
			// Bezier cp2 = the last interior CP. This gives both tangents
			// near Visio's rendered direction without needing a proper
			// NURBS-knot-aware curve evaluator.
			n := len(absPoints)
			var cp1X, cp1Y float64
			if n >= 2 {
				cp1X = (absPoints[0][0] + absPoints[1][0]) / 2
				cp1Y = (absPoints[0][1] + absPoints[1][1]) / 2
			} else {
				cp1X = absPoints[0][0]
				cp1Y = absPoints[0][1]
			}
			cpFirstSvgX, cpFirstSvgY := r.localToSVG(cp1X, cp1Y)
			cpLastSvgX, cpLastSvgY := r.localToSVG(absPoints[n-1][0], absPoints[n-1][1])
			d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
				r.fmtNum(cpFirstSvgX), r.fmtNum(cpFirstSvgY),
				r.fmtNum(cpLastSvgX), r.fmtNum(cpLastSvgY),
				r.fmtNum(svgX), r.fmtNum(svgY)))
			lastCPX, lastCPY = cpLastSvgX, cpLastSvgY
		} else {
			// Build control polygon with all points
			startLocalX := prevX / r.scaleX
			var startLocalY float64
			if r.negativeH {
				startLocalY = -prevY / r.scaleY
			} else {
				startLocalY = r.parentH - prevY/r.scaleY
			}

			n := len(nurbsInfo.cps) + 2 // start + interior + end
			ctrl := make([][2]float64, n)
			ctrl[0] = [2]float64{startLocalX, startLocalY}
			for i, pt := range absPoints {
				ctrl[i+1] = [2]float64{pt[0], pt[1]}
			}
			ctrl[n-1] = [2]float64{endX, endY}

			// For the common 5-CP cubic case with one interior knot, do the
			// exact Boehm B-spline → Bezier conversion using r = A/knotLast
			// (where A is the last control point's knot value from cell A).
			// This reproduces Visio's exact rendered curve, not just an LSQ
			// approximation - giving pixel-perfect connector curves for the
			// monotonic-X 3-CP NURBS connectors (shape 24, 25, 27, ...).
			if n == 5 && nurbsInfo.knotLast > 0 && nurbsInfo.lastCPKnot > 0 && nurbsInfo.lastCPKnot < nurbsInfo.knotLast {
				ratio := nurbsInfo.lastCPKnot / nurbsInfo.knotLast
				beziers := r.boehmCubicBspline5(ctrl, ratio)
				d.WriteString(beziers)
			} else {
				beziers := r.bsplineToMultiBezier(ctrl, nurbsInfo.knotLast)
				d.WriteString(beziers)
			}
			lastCPX, lastCPY = r.localToSVG(absPoints[len(absPoints)-1][0], absPoints[len(absPoints)-1][1])
		}
	} else {
		d.WriteString(fmt.Sprintf("L%s %s", r.fmtNum(svgX), r.fmtNum(svgY)))
	}

	return d.String(), lastCPX, lastCPY, actualEndX, actualEndY
}

func (r *GeometryResolver) detectLShape(absPoints [][2]float64) bool {
	if len(absPoints) != 3 {
		return false
	}
	eps := 0.001
	sameX01 := math.Abs(absPoints[0][0]-absPoints[1][0]) < eps
	sameY12 := math.Abs(absPoints[1][1]-absPoints[2][1]) < eps
	sameY01 := math.Abs(absPoints[0][1]-absPoints[1][1]) < eps
	sameX12 := math.Abs(absPoints[1][0]-absPoints[2][0]) < eps
	return (sameX01 && sameY12) || (sameY01 && sameX12)
}

func (r *GeometryResolver) generateSCurve(prevX, prevY, svgX, svgY float64, absPoints [][2]float64) string {
	// Convert prev point back to local coords for calculation
	// Account for negativeH in the inverse Y transform
	startX := prevX / r.scaleX
	endX := svgX / r.scaleX
	var startY, endY float64
	if r.negativeH {
		startY = -prevY / r.scaleY
		endY = -svgY / r.scaleY
	} else {
		startY = r.parentH - prevY/r.scaleY
		endY = r.parentH - svgY/r.scaleY
	}

	eps := 0.001
	sameX01 := math.Abs(absPoints[0][0]-absPoints[1][0]) < eps
	cornerX, cornerY := absPoints[1][0], absPoints[1][1]
	midX := (startX + endX) / 2
	midY := (startY + endY) / 2

	var cp1X, cp1Y, cp2X, cp2Y, cp3X, cp3Y, cp4X, cp4Y float64
	if sameX01 {
		cp1X = startX
		cp1Y = startY + (cornerY-startY)*0.5
		cp2X = startX + (midX-startX)*0.2
		cp2Y = midY
		cp3X = endX - (endX-midX)*0.2
		cp3Y = midY
		cp4X = endX
		cp4Y = endY - (endY-cornerY)*0.5
	} else {
		cp1X = startX + (cornerX-startX)*0.5
		cp1Y = startY
		cp2X = midX
		cp2Y = startY + (midY-startY)*0.2
		cp3X = midX
		cp3Y = endY - (endY-midY)*0.2
		cp4X = endX - (endX-cornerX)*0.5
		cp4Y = endY
	}

	cp1SvgX, cp1SvgY := r.localToSVG(cp1X, cp1Y)
	cp2SvgX, cp2SvgY := r.localToSVG(cp2X, cp2Y)
	midSvgX, midSvgY := r.localToSVG(midX, midY)
	cp3SvgX, cp3SvgY := r.localToSVG(cp3X, cp3Y)
	cp4SvgX, cp4SvgY := r.localToSVG(cp4X, cp4Y)

	return fmt.Sprintf("C%s %s %s %s %s %sC%s %s %s %s %s %s",
		r.fmtNum(cp1SvgX), r.fmtNum(cp1SvgY),
		r.fmtNum(cp2SvgX), r.fmtNum(cp2SvgY),
		r.fmtNum(midSvgX), r.fmtNum(midSvgY),
		r.fmtNum(cp3SvgX), r.fmtNum(cp3SvgY),
		r.fmtNum(cp4SvgX), r.fmtNum(cp4SvgY),
		r.fmtNum(svgX), r.fmtNum(svgY))
}

func (r *GeometryResolver) generateMultiBezier(prevX, prevY, svgX, svgY float64, absPoints [][2]float64) string {
	var d strings.Builder

	if len(absPoints) == 4 {
		midX := (absPoints[1][0] + absPoints[2][0]) / 2
		midY := (absPoints[1][1] + absPoints[2][1]) / 2

		cp0SvgX, cp0SvgY := r.localToSVG(absPoints[0][0], absPoints[0][1])
		cp1SvgX, cp1SvgY := r.localToSVG(absPoints[1][0], absPoints[1][1])
		midSvgX, midSvgY := r.localToSVG(midX, midY)
		d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
			r.fmtNum(cp0SvgX), r.fmtNum(cp0SvgY),
			r.fmtNum(cp1SvgX), r.fmtNum(cp1SvgY),
			r.fmtNum(midSvgX), r.fmtNum(midSvgY)))

		cp2SvgX, cp2SvgY := r.localToSVG(absPoints[2][0], absPoints[2][1])
		cp3SvgX, cp3SvgY := r.localToSVG(absPoints[3][0], absPoints[3][1])
		d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
			r.fmtNum(cp2SvgX), r.fmtNum(cp2SvgY),
			r.fmtNum(cp3SvgX), r.fmtNum(cp3SvgY),
			r.fmtNum(svgX), r.fmtNum(svgY)))
	} else if len(absPoints) == 3 {
		cp0SvgX, cp0SvgY := r.localToSVG(absPoints[0][0], absPoints[0][1])
		mid01X := (absPoints[0][0] + absPoints[1][0]) / 2
		mid01Y := (absPoints[0][1] + absPoints[1][1]) / 2
		mid01SvgX, mid01SvgY := r.localToSVG(mid01X, mid01Y)
		cp1SvgX, cp1SvgY := r.localToSVG(absPoints[1][0], absPoints[1][1])
		d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
			r.fmtNum(cp0SvgX), r.fmtNum(cp0SvgY),
			r.fmtNum(mid01SvgX), r.fmtNum(mid01SvgY),
			r.fmtNum(cp1SvgX), r.fmtNum(cp1SvgY)))

		mid12X := (absPoints[1][0] + absPoints[2][0]) / 2
		mid12Y := (absPoints[1][1] + absPoints[2][1]) / 2
		mid12SvgX, mid12SvgY := r.localToSVG(mid12X, mid12Y)
		cp2SvgX, cp2SvgY := r.localToSVG(absPoints[2][0], absPoints[2][1])
		d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
			r.fmtNum(mid12SvgX), r.fmtNum(mid12SvgY),
			r.fmtNum(cp2SvgX), r.fmtNum(cp2SvgY),
			r.fmtNum(svgX), r.fmtNum(svgY)))
	} else {
		// Fallback: use first and last as control points
		cp1SvgX, cp1SvgY := r.localToSVG(absPoints[0][0], absPoints[0][1])
		lastIdx := len(absPoints) - 1
		cp2SvgX, cp2SvgY := r.localToSVG(absPoints[lastIdx][0], absPoints[lastIdx][1])
		d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
			r.fmtNum(cp1SvgX), r.fmtNum(cp1SvgY),
			r.fmtNum(cp2SvgX), r.fmtNum(cp2SvgY),
			r.fmtNum(svgX), r.fmtNum(svgY)))
	}

	return d.String()
}

// fitBezierToBSpline samples a clamped cubic B-spline and fits a cubic Bezier.
// This provides better curve approximation than directly using B-spline control points.
func (r *GeometryResolver) fitBezierToBSpline(ctrl [][2]float64, knotLast float64) [][2]float64 {
	if len(ctrl) != 4 {
		return nil
	}

	// For a clamped cubic B-spline with 4 control points,
	// build the knot vector: [0,0,0,0, knotLast,knotLast,knotLast,knotLast]
	knots := []float64{0, 0, 0, 0, knotLast, knotLast, knotLast, knotLast}

	// Sample the B-spline at key parameter values
	// For a clamped curve, endpoints are interpolated
	samples := make([][2]float64, 5)
	tValues := []float64{0, knotLast * 0.25, knotLast * 0.5, knotLast * 0.75, knotLast * 0.999}

	for i, t := range tValues {
		x, y := evalBSpline(3, t, knots, ctrl)
		samples[i] = [2]float64{x, y}
	}

	// Fit cubic Bezier to the 5 samples using least squares
	// For a cubic Bezier: P(t) = (1-t)³P0 + 3(1-t)²t·P1 + 3(1-t)t²·P2 + t³·P3
	// P0 and P3 are fixed at samples[0] and samples[4]
	// We solve for P1 and P2 that minimize error at t=0.25, 0.5, 0.75

	P0 := samples[0]
	P3 := samples[4]

	// Bernstein basis at t = 0.25, 0.5, 0.75
	t1, t2, t3 := 0.25, 0.5, 0.75

	// Build the system for P1 and P2
	// At each t: 3(1-t)²t·P1 + 3(1-t)t²·P2 = P(t) - (1-t)³P0 - t³P3

	b10 := 3 * (1 - t1) * (1 - t1) * t1 // 3(1-t1)²t1
	b11 := 3 * (1 - t1) * t1 * t1        // 3(1-t1)t1²
	b20 := 3 * (1 - t2) * (1 - t2) * t2
	b21 := 3 * (1 - t2) * t2 * t2
	b30 := 3 * (1 - t3) * (1 - t3) * t3
	b31 := 3 * (1 - t3) * t3 * t3

	// RHS for x and y
	rx1 := samples[1][0] - math.Pow(1-t1, 3)*P0[0] - math.Pow(t1, 3)*P3[0]
	ry1 := samples[1][1] - math.Pow(1-t1, 3)*P0[1] - math.Pow(t1, 3)*P3[1]
	rx2 := samples[2][0] - math.Pow(1-t2, 3)*P0[0] - math.Pow(t2, 3)*P3[0]
	ry2 := samples[2][1] - math.Pow(1-t2, 3)*P0[1] - math.Pow(t2, 3)*P3[1]
	rx3 := samples[3][0] - math.Pow(1-t3, 3)*P0[0] - math.Pow(t3, 3)*P3[0]
	ry3 := samples[3][1] - math.Pow(1-t3, 3)*P0[1] - math.Pow(t3, 3)*P3[1]

	// Solve 2x2 least squares for each coordinate
	// Normal equations: A'A x = A'b
	// A = [[b10,b11], [b20,b21], [b30,b31]]
	ata00 := b10*b10 + b20*b20 + b30*b30
	ata01 := b10*b11 + b20*b21 + b30*b31
	ata11 := b11*b11 + b21*b21 + b31*b31

	atbx0 := b10*rx1 + b20*rx2 + b30*rx3
	atbx1 := b11*rx1 + b21*rx2 + b31*rx3
	atby0 := b10*ry1 + b20*ry2 + b30*ry3
	atby1 := b11*ry1 + b21*ry2 + b31*ry3

	det := ata00*ata11 - ata01*ata01
	if math.Abs(det) < 1e-10 {
		return nil
	}

	P1x := (ata11*atbx0 - ata01*atbx1) / det
	P2x := (ata00*atbx1 - ata01*atbx0) / det
	P1y := (ata11*atby0 - ata01*atby1) / det
	P2y := (ata00*atby1 - ata01*atby0) / det

	return [][2]float64{
		P0,
		{P1x, P1y},
		{P2x, P2y},
		P3,
	}
}

// evalBSpline evaluates a B-spline at parameter t using De Boor's algorithm.
func evalBSpline(degree int, t float64, knots []float64, ctrl [][2]float64) (float64, float64) {
	n := len(ctrl)
	if n == 0 {
		return 0, 0
	}

	// Find the knot span
	k := degree
	for k < n && t >= knots[k+1] && k+1 < len(knots)-1 {
		k++
	}

	// De Boor's algorithm
	d := make([][2]float64, degree+1)
	for j := 0; j <= degree; j++ {
		idx := j + k - degree
		if idx < 0 || idx >= n {
			d[j] = [2]float64{0, 0}
		} else {
			d[j] = ctrl[idx]
		}
	}

	for r := 1; r <= degree; r++ {
		for j := degree; j >= r; j-- {
			alpha := 0.0
			left := knots[j+k-degree]
			right := knots[j+1+k-r]
			if right-left > 1e-10 {
				alpha = (t - left) / (right - left)
			}
			d[j][0] = (1-alpha)*d[j-1][0] + alpha*d[j][0]
			d[j][1] = (1-alpha)*d[j-1][1] + alpha*d[j][1]
		}
	}

	return d[degree][0], d[degree][1]
}

// bsplineToMultiBezier converts a clamped cubic B-spline to one or more Bezier curves.
// For n control points, produces (n-3) Bezier segments.
func (r *GeometryResolver) bsplineToMultiBezier(ctrl [][2]float64, knotLast float64) string {
	n := len(ctrl)
	if n < 4 {
		// Not enough points for cubic B-spline
		if n >= 2 {
			endX, endY := r.localToSVG(ctrl[n-1][0], ctrl[n-1][1])
			return fmt.Sprintf("L%s %s", r.fmtNum(endX), r.fmtNum(endY))
		}
		return ""
	}

	// For n control points, we have (n - degree) spans
	// Build clamped knot vector
	degree := 3
	numKnots := n + degree + 1
	knots := make([]float64, numKnots)

	// First (degree+1) knots are 0
	for i := 0; i <= degree; i++ {
		knots[i] = 0
	}
	// Last (degree+1) knots are knotLast
	for i := n; i < numKnots; i++ {
		knots[i] = knotLast
	}
	// Interior knots evenly spaced
	numInterior := n - degree - 1
	if numInterior > 0 {
		step := knotLast / float64(numInterior+1)
		for i := 0; i < numInterior; i++ {
			knots[degree+1+i] = step * float64(i+1)
		}
	}

	// Number of Bezier segments = n - degree
	numSegments := n - degree
	var d strings.Builder

	for seg := 0; seg < numSegments; seg++ {
		// Each segment spans from knots[seg+degree] to knots[seg+degree+1]
		tStart := knots[seg+degree]
		tEnd := knots[seg+degree+1]

		if tEnd-tStart < 1e-10 {
			continue
		}

		// Sample this segment at 5 points
		samples := make([][2]float64, 5)
		for i := 0; i < 5; i++ {
			t := tStart + (tEnd-tStart)*float64(i)/4.0
			if i == 4 {
				t = tEnd - 1e-10 // Avoid exactly hitting the knot
			}
			x, y := evalBSpline(degree, t, knots, ctrl)
			samples[i] = [2]float64{x, y}
		}

		// Fit cubic Bezier to these samples
		P0 := samples[0]
		P3 := samples[4]

		// Use least squares to find P1, P2
		t1, t2, t3 := 0.25, 0.5, 0.75

		b10 := 3 * (1 - t1) * (1 - t1) * t1
		b11 := 3 * (1 - t1) * t1 * t1
		b20 := 3 * (1 - t2) * (1 - t2) * t2
		b21 := 3 * (1 - t2) * t2 * t2
		b30 := 3 * (1 - t3) * (1 - t3) * t3
		b31 := 3 * (1 - t3) * t3 * t3

		rx1 := samples[1][0] - math.Pow(1-t1, 3)*P0[0] - math.Pow(t1, 3)*P3[0]
		ry1 := samples[1][1] - math.Pow(1-t1, 3)*P0[1] - math.Pow(t1, 3)*P3[1]
		rx2 := samples[2][0] - math.Pow(1-t2, 3)*P0[0] - math.Pow(t2, 3)*P3[0]
		ry2 := samples[2][1] - math.Pow(1-t2, 3)*P0[1] - math.Pow(t2, 3)*P3[1]
		rx3 := samples[3][0] - math.Pow(1-t3, 3)*P0[0] - math.Pow(t3, 3)*P3[0]
		ry3 := samples[3][1] - math.Pow(1-t3, 3)*P0[1] - math.Pow(t3, 3)*P3[1]

		ata00 := b10*b10 + b20*b20 + b30*b30
		ata01 := b10*b11 + b20*b21 + b30*b31
		ata11 := b11*b11 + b21*b21 + b31*b31

		atbx0 := b10*rx1 + b20*rx2 + b30*rx3
		atbx1 := b11*rx1 + b21*rx2 + b31*rx3
		atby0 := b10*ry1 + b20*ry2 + b30*ry3
		atby1 := b11*ry1 + b21*ry2 + b31*ry3

		det := ata00*ata11 - ata01*ata01
		if math.Abs(det) < 1e-10 {
			// Fallback to line
			endSvgX, endSvgY := r.localToSVG(P3[0], P3[1])
			d.WriteString(fmt.Sprintf("L%s %s", r.fmtNum(endSvgX), r.fmtNum(endSvgY)))
			continue
		}

		P1x := (ata11*atbx0 - ata01*atbx1) / det
		P2x := (ata00*atbx1 - ata01*atbx0) / det
		P1y := (ata11*atby0 - ata01*atby1) / det
		P2y := (ata00*atby1 - ata01*atby0) / det

		// Convert to SVG coordinates
		cp1SvgX, cp1SvgY := r.localToSVG(P1x, P1y)
		cp2SvgX, cp2SvgY := r.localToSVG(P2x, P2y)
		endSvgX, endSvgY := r.localToSVG(P3[0], P3[1])

		d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
			r.fmtNum(cp1SvgX), r.fmtNum(cp1SvgY),
			r.fmtNum(cp2SvgX), r.fmtNum(cp2SvgY),
			r.fmtNum(endSvgX), r.fmtNum(endSvgY)))
	}

	return d.String()
}

// shortenPathStart shortens the path at the start by the given amount.
func (r *GeometryResolver) shortenPathStart(pathData string, shortenBy float64) string {
	if shortenBy <= 0 || pathData == "" {
		return pathData
	}

	// Find M command and first drawing command
	pathData = strings.TrimSpace(pathData)
	if !strings.HasPrefix(pathData, "M") {
		return pathData
	}

	// Find end of M command
	mEndIdx := 1
	for mEndIdx < len(pathData) {
		c := pathData[mEndIdx]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			break
		}
		mEndIdx++
	}

	mCmd := pathData[:mEndIdx]
	rest := pathData[mEndIdx:]

	// Parse M coordinates
	coords := strings.Fields(strings.TrimPrefix(mCmd, "M"))
	if len(coords) < 2 {
		return pathData
	}
	startX, _ := parseFloat(coords[0])
	startY, _ := parseFloat(coords[1])

	// Find next command
	if len(rest) == 0 {
		return pathData
	}

	nextCmd := rest[0]
	cmdEnd := 1
	for cmdEnd < len(rest) {
		c := rest[cmdEnd]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			break
		}
		cmdEnd++
	}

	if nextCmd == 'L' {
		// LineTo - shorten by moving start point
		cmdCoords := strings.Fields(rest[1:cmdEnd])
		if len(cmdCoords) >= 2 {
			endX, _ := parseFloat(cmdCoords[0])
			endY, _ := parseFloat(cmdCoords[1])
			dx := endX - startX
			dy := endY - startY
			length := math.Sqrt(dx*dx + dy*dy)
			if length > shortenBy {
				ratio := shortenBy / length
				newStartX := startX + dx*ratio
				newStartY := startY + dy*ratio
				return fmt.Sprintf("M%s %s%s", r.fmtNum(newStartX), r.fmtNum(newStartY), rest)
			}
		}
	} else if nextCmd == 'C' {
		// CurveTo - shorten by moving start point along tangent
		cmdCoords := strings.Fields(rest[1:cmdEnd])
		if len(cmdCoords) >= 6 {
			cp1X, _ := parseFloat(cmdCoords[0])
			cp1Y, _ := parseFloat(cmdCoords[1])
			cp2X, _ := parseFloat(cmdCoords[2])
			cp2Y, _ := parseFloat(cmdCoords[3])
			endX, _ := parseFloat(cmdCoords[4])
			endY, _ := parseFloat(cmdCoords[5])

			// Tangent at start is (cp1 - start)
			dx := cp1X - startX
			dy := cp1Y - startY
			segmentLen := math.Sqrt(dx*dx + dy*dy)

			if segmentLen > 0.01 {
				// When the requested setback exceeds the start-to-cp1
				// chord, the previous implementation clamped to 90% of
				// segmentLen which left the marker tip stranded inside
				// the source shape (because the line was not actually
				// shortened far enough). Properly handle this by
				// subdividing the Bezier with de Casteljau at the
				// parameter t whose chord distance from start equals
				// shortenBy - this yields a correctly-shortened curve
				// whose new start sits the full setback distance from
				// the original start along the curve.
				newStartX, newStartY, newCp1X, newCp1Y, newCp2X, newCp2Y, ok :=
					subdivideCubicByChord(startX, startY, cp1X, cp1Y, cp2X, cp2Y, endX, endY, shortenBy)
				if !ok {
					// Fallback: linear move along start tangent.
					dx /= segmentLen
					dy /= segmentLen
					newStartX = startX + dx*shortenBy
					newStartY = startY + dy*shortenBy
					newCp1X = cp1X
					newCp1Y = cp1Y
					newCp2X = cp2X
					newCp2Y = cp2Y
				}

				newCurve := fmt.Sprintf("C%s %s %s %s %s %s",
					r.fmtNum(newCp1X), r.fmtNum(newCp1Y),
					r.fmtNum(newCp2X), r.fmtNum(newCp2Y),
					r.fmtNum(endX), r.fmtNum(endY))
				return fmt.Sprintf("M%s %s%s%s", r.fmtNum(newStartX), r.fmtNum(newStartY), newCurve, rest[cmdEnd:])
			}
		}
	}

	return pathData
}

// boehmCubicBspline5 converts a clamped cubic B-spline with exactly 5 control
// points and one interior knot at parameter t_m = ratio * knotLast into two
// connected cubic Bezier segments using Boehm's knot insertion algorithm.
//
// The knot vector is [0,0,0,0, t_m, T,T,T,T] with T = knotLast and t_m at
// position ratio (where Visio sets t_m = cell A's knot value, NOT a uniform
// midpoint).
//
// For r = t_m/T = ratio:
//
//	Q2 = (1-r)*P1 + r*P2
//	Q3 = (1-r)*P2 + r*P3
//	R3 = (1-r)*Q2 + r*Q3      (= curve value at t = t_m)
//
// First Bezier:  P0, P1, Q2, R3
// Second Bezier: R3, Q3, P3, P4
//
// This is the exact decomposition Visio uses, so the rendered curve matches
// pixel-for-pixel rather than the approximate LSQ-fit from sample points.
func (r *GeometryResolver) boehmCubicBspline5(ctrl [][2]float64, ratio float64) string {
	if len(ctrl) != 5 {
		return ""
	}
	// Clamp ratio to (0, 1) to avoid degenerate output.
	if ratio <= 1e-6 {
		ratio = 1e-6
	}
	if ratio >= 1.0-1e-6 {
		ratio = 1.0 - 1e-6
	}
	// ctrl[0] is the path start; it's already at the path cursor via the
	// preceding command. The two emitted Beziers consume control points 1..4
	// plus the derived split point.
	_ = ctrl[0]
	p1, p2, p3, p4 := ctrl[1], ctrl[2], ctrl[3], ctrl[4]
	one := 1.0 - ratio
	q2 := [2]float64{one*p1[0] + ratio*p2[0], one*p1[1] + ratio*p2[1]}
	q3 := [2]float64{one*p2[0] + ratio*p3[0], one*p2[1] + ratio*p3[1]}
	r3 := [2]float64{one*q2[0] + ratio*q3[0], one*q2[1] + ratio*q3[1]}

	// First Bezier: p0, p1, q2, r3
	c1x0, c1y0 := r.localToSVG(p1[0], p1[1])
	c1x1, c1y1 := r.localToSVG(q2[0], q2[1])
	endX1, endY1 := r.localToSVG(r3[0], r3[1])

	// Second Bezier: r3 → q3 → p3 → p4
	c2x0, c2y0 := r.localToSVG(q3[0], q3[1])
	c2x1, c2y1 := r.localToSVG(p3[0], p3[1])
	endX2, endY2 := r.localToSVG(p4[0], p4[1])

	return fmt.Sprintf("C%s %s %s %s %s %sC%s %s %s %s %s %s",
		r.fmtNum(c1x0), r.fmtNum(c1y0),
		r.fmtNum(c1x1), r.fmtNum(c1y1),
		r.fmtNum(endX1), r.fmtNum(endY1),
		r.fmtNum(c2x0), r.fmtNum(c2y0),
		r.fmtNum(c2x1), r.fmtNum(c2y1),
		r.fmtNum(endX2), r.fmtNum(endY2))
}

// subdivideCubicByChordFromEnd splits a cubic Bezier at the parameter t whose
// chord distance from the END point equals targetDist, returning the control
// points of the FIRST sub-curve (from 0 to t) - i.e. the curve with its end
// shortened by setback. Same role as subdivideCubicByChord, just operating
// from the t=1 side instead of t=0.
func subdivideCubicByChordFromEnd(x0, y0, x1, y1, x2, y2, x3, y3, targetDist float64) (
	sx, sy, c1x, c1y, c2x, c2y, ex, ey float64, ok bool) {
	if targetDist <= 0 {
		return x0, y0, x1, y1, x2, y2, x3, y3, false
	}
	bez := func(t float64) (float64, float64) {
		u := 1 - t
		uu := u * u
		uuu := uu * u
		tt := t * t
		ttt := tt * t
		bx := uuu*x0 + 3*uu*t*x1 + 3*u*tt*x2 + ttt*x3
		by := uuu*y0 + 3*uu*t*y1 + 3*u*tt*y2 + ttt*y3
		return bx, by
	}
	distFromEnd := func(t float64) float64 {
		bx, by := bez(t)
		dx := bx - x3
		dy := by - y3
		return math.Sqrt(dx*dx + dy*dy)
	}

	// Check that the target is reachable.
	if distFromEnd(0) < targetDist {
		return x0, y0, x1, y1, x2, y2, x3, y3, false
	}

	// distFromEnd is monotonically increasing as t decreases from 1.
	lo, hi := 0.0, 1.0
	for i := 0; i < 40; i++ {
		mid := (lo + hi) / 2
		if distFromEnd(mid) > targetDist {
			lo = mid
		} else {
			hi = mid
		}
	}
	t := (lo + hi) / 2

	// De Casteljau split at t. Keep FIRST sub-curve (from t=0 to t).
	lerp := func(a, b, k float64) float64 { return a + (b-a)*k }
	p01x, p01y := lerp(x0, x1, t), lerp(y0, y1, t)
	p12x, p12y := lerp(x1, x2, t), lerp(y1, y2, t)
	p23x, p23y := lerp(x2, x3, t), lerp(y2, y3, t)
	p012x, p012y := lerp(p01x, p12x, t), lerp(p01y, p12y, t)
	p123x, p123y := lerp(p12x, p23x, t), lerp(p12y, p23y, t)
	p0123x, p0123y := lerp(p012x, p123x, t), lerp(p012y, p123y, t)

	// First sub-curve: P0, p01, p012, p0123
	return x0, y0, p01x, p01y, p012x, p012y, p0123x, p0123y, true
}

// subdivideCubicByChord splits a cubic Bezier at the parameter t whose chord
// distance from the start point equals targetDist, and returns the control
// points of the SECOND sub-curve (from t to 1). This is the correct way to
// shorten the start of a cubic Bezier by a given setback distance: the new
// start lies the full setback along the curve and the new control points
// preserve the original curve shape past that point.
//
// Returns ok=false when the curve cannot be shortened (e.g. the entire chord
// length is below targetDist), letting the caller fall back to a simpler
// tangent-based shift.
func subdivideCubicByChord(x0, y0, x1, y1, x2, y2, x3, y3, targetDist float64) (sx, sy, c1x, c1y, c2x, c2y float64, ok bool) {
	if targetDist <= 0 {
		return x0, y0, x1, y1, x2, y2, false
	}
	// Walk t from 0 to 1 with binary search until |B(t) - B(0)| == targetDist.
	// The chord from B(0) is monotonically (mostly) increasing in t for the
	// non-degenerate curves we deal with here.
	bez := func(t float64) (float64, float64) {
		u := 1 - t
		uu := u * u
		uuu := uu * u
		tt := t * t
		ttt := tt * t
		bx := uuu*x0 + 3*uu*t*x1 + 3*u*tt*x2 + ttt*x3
		by := uuu*y0 + 3*uu*t*y1 + 3*u*tt*y2 + ttt*y3
		return bx, by
	}
	dist := func(t float64) float64 {
		bx, by := bez(t)
		dx := bx - x0
		dy := by - y0
		return math.Sqrt(dx*dx + dy*dy)
	}

	// Check that the target is reachable along the chord.
	if dist(1) < targetDist {
		return x0, y0, x1, y1, x2, y2, false
	}

	lo, hi := 0.0, 1.0
	for i := 0; i < 40; i++ {
		mid := (lo + hi) / 2
		if dist(mid) < targetDist {
			lo = mid
		} else {
			hi = mid
		}
	}
	t := (lo + hi) / 2

	// De Casteljau split at t.
	// Linear interpolations on the control polygon.
	lerp := func(a, b, k float64) float64 { return a + (b-a)*k }
	p01x, p01y := lerp(x0, x1, t), lerp(y0, y1, t)
	p12x, p12y := lerp(x1, x2, t), lerp(y1, y2, t)
	p23x, p23y := lerp(x2, x3, t), lerp(y2, y3, t)
	p012x, p012y := lerp(p01x, p12x, t), lerp(p01y, p12y, t)
	p123x, p123y := lerp(p12x, p23x, t), lerp(p12y, p23y, t)
	p0123x, p0123y := lerp(p012x, p123x, t), lerp(p012y, p123y, t)

	// Second sub-curve: B(t), p123, p23, P3.
	return p0123x, p0123y, p123x, p123y, p23x, p23y, true
}

// shortenPathEnd shortens the path at the end by the given amount.
func (r *GeometryResolver) shortenPathEnd(pathData string, shortenBy float64) string {
	if shortenBy <= 0 || pathData == "" {
		return pathData
	}

	pathData = strings.TrimSpace(pathData)

	// Find the last command
	lastCmdIdx := -1
	for i := len(pathData) - 1; i >= 0; i-- {
		c := pathData[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			lastCmdIdx = i
			break
		}
	}
	if lastCmdIdx < 0 {
		return pathData
	}

	cmd := pathData[lastCmdIdx]
	args := strings.TrimSpace(pathData[lastCmdIdx+1:])
	prefix := pathData[:lastCmdIdx]

	parts := strings.Fields(args)

	switch cmd {
	case 'L':
		if len(parts) < 2 {
			return pathData
		}
		endX, _ := parseFloat(parts[len(parts)-2])
		endY, _ := parseFloat(parts[len(parts)-1])
		prevX, prevY := r.findPrevPoint(prefix)

		dx := endX - prevX
		dy := endY - prevY
		length := math.Sqrt(dx*dx + dy*dy)
		if length > shortenBy {
			ratio := (length - shortenBy) / length
			newX := prevX + dx*ratio
			newY := prevY + dy*ratio
			return prefix + fmt.Sprintf("L%s %s", r.fmtNum(newX), r.fmtNum(newY))
		}

	case 'C':
		if len(parts) < 6 {
			return pathData
		}
		cp1X, _ := parseFloat(parts[len(parts)-6])
		cp1Y, _ := parseFloat(parts[len(parts)-5])
		cp2X, _ := parseFloat(parts[len(parts)-4])
		cp2Y, _ := parseFloat(parts[len(parts)-3])
		endX, _ := parseFloat(parts[len(parts)-2])
		endY, _ := parseFloat(parts[len(parts)-1])

		// Find the start of this cubic from the preceding command/M to do
		// proper de Casteljau subdivision. Same approach as shortenPathStart:
		// shortening by a tangent-direction shift moves the endpoint along the
		// straight chord rather than along the curve, which becomes visibly
		// wrong when the setback exceeds the segment chord length.
		startX, startY := r.findPrevPoint(prefix)
		newStartX, newStartY, newCp1X, newCp1Y, newCp2X, newCp2Y, newEndX, newEndY, ok :=
			subdivideCubicByChordFromEnd(startX, startY, cp1X, cp1Y, cp2X, cp2Y, endX, endY, shortenBy)
		_ = newStartX
		_ = newStartY
		if ok {
			newArgs := fmt.Sprintf("%s %s %s %s %s %s",
				r.fmtNum(newCp1X), r.fmtNum(newCp1Y),
				r.fmtNum(newCp2X), r.fmtNum(newCp2Y),
				r.fmtNum(newEndX), r.fmtNum(newEndY))
			return prefix + "C" + newArgs
		}

		// Fallback: linear move along end tangent (legacy behaviour).
		dx := endX - cp2X
		dy := endY - cp2Y
		segmentLen := math.Sqrt(dx*dx + dy*dy)
		effectiveShorten := shortenBy
		if segmentLen > 0.01 && segmentLen < shortenBy {
			effectiveShorten = segmentLen * 0.9
		}
		if segmentLen > 0.01 {
			dx = dx / segmentLen
			dy = dy / segmentLen
			newEndX := endX - dx*effectiveShorten
			newEndY := endY - dy*effectiveShorten
			cp2Ratio := effectiveShorten / segmentLen
			if cp2Ratio > 0.5 {
				cp2Ratio = 0.5
			}
			newCp2X := cp2X - dx*effectiveShorten*cp2Ratio
			newCp2Y := cp2Y - dy*effectiveShorten*cp2Ratio
			newArgs := fmt.Sprintf("%s %s %s %s %s %s",
				r.fmtNum(cp1X), r.fmtNum(cp1Y),
				r.fmtNum(newCp2X), r.fmtNum(newCp2Y),
				r.fmtNum(newEndX), r.fmtNum(newEndY))
			return prefix + "C" + newArgs
		}
	}

	return pathData
}

func (r *GeometryResolver) findPrevPoint(pathData string) (float64, float64) {
	// Find the last coordinate pair in the path
	var lastX, lastY float64

	i := len(pathData) - 1
	for i >= 0 {
		c := pathData[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			args := strings.TrimSpace(pathData[i+1:])
			parts := strings.Fields(args)
			if len(parts) >= 2 {
				lastX, _ = parseFloat(parts[len(parts)-2])
				lastY, _ = parseFloat(parts[len(parts)-1])
			}
			break
		}
		i--
	}

	return lastX, lastY
}

func parseFloat(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

// roundAxisAlignedRectangle rewrites a 5-point closed rectangle path
// (1 MoveTo + 4 LineTo segments forming an axis-aligned box) into a
// rounded-rectangle path with arcs at each corner. Returns "" if the
// path is not a recognizable axis-aligned rectangle.
//
// radius is in the same units as the path (SVG pixels at 72 ppi).
// The corner radius is automatically clamped to half the smallest side.
func roundAxisAlignedRectangle(pathData string, radius float64) string {
	if radius <= 0 {
		return ""
	}

	// Tokenise: split on whitespace and on letters that introduce commands.
	// We're looking for the pattern: M x y L x y L x y L x y L x y [Z]
	type point struct{ x, y float64 }
	var pts []point
	var cmd byte
	idx := 0
	skipWS := func() {
		for idx < len(pathData) && (pathData[idx] == ' ' || pathData[idx] == ',' || pathData[idx] == '\t' || pathData[idx] == '\n') {
			idx++
		}
	}
	readNum := func() (float64, bool) {
		skipWS()
		start := idx
		for idx < len(pathData) {
			c := pathData[idx]
			if (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '+' || c == 'e' || c == 'E' {
				idx++
				continue
			}
			break
		}
		if start == idx {
			return 0, false
		}
		return parseFloat(pathData[start:idx])
	}

	for idx < len(pathData) {
		skipWS()
		if idx >= len(pathData) {
			break
		}
		c := pathData[idx]
		if c == 'M' || c == 'L' || c == 'Z' || c == 'z' {
			cmd = c
			idx++
			if cmd == 'Z' || cmd == 'z' {
				continue
			}
			x, ok1 := readNum()
			y, ok2 := readNum()
			if !ok1 || !ok2 {
				return ""
			}
			pts = append(pts, point{x, y})
			continue
		}
		// Unknown command → not a simple rectangle; bail.
		return ""
	}

	// Must have exactly 5 points (M + 4 L, closing the rectangle).
	if len(pts) != 5 {
		return ""
	}

	// Closure: last point equals first.
	const eps = 1e-6
	if math.Abs(pts[4].x-pts[0].x) > eps || math.Abs(pts[4].y-pts[0].y) > eps {
		return ""
	}

	// Each of the 4 segments must be axis-aligned.
	for i := 0; i < 4; i++ {
		dx := math.Abs(pts[i+1].x - pts[i].x)
		dy := math.Abs(pts[i+1].y - pts[i].y)
		if dx > eps && dy > eps {
			return ""
		}
	}

	// Bounding box.
	minX, minY := pts[0].x, pts[0].y
	maxX, maxY := pts[0].x, pts[0].y
	for _, p := range pts[1:] {
		if p.x < minX {
			minX = p.x
		}
		if p.x > maxX {
			maxX = p.x
		}
		if p.y < minY {
			minY = p.y
		}
		if p.y > maxY {
			maxY = p.y
		}
	}
	w, h := maxX-minX, maxY-minY
	if w <= eps || h <= eps {
		return ""
	}

	rad := radius
	if rad > w/2 {
		rad = w / 2
	}
	if rad > h/2 {
		rad = h / 2
	}

	// Emit CW rounded rectangle starting at the top of the top-left arc.
	// Sweep flag 1 = clockwise (matches positive-Y-down screen coords).
	fmtN := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	return "M" + fmtN(minX+rad) + " " + fmtN(minY) +
		" L" + fmtN(maxX-rad) + " " + fmtN(minY) +
		" A" + fmtN(rad) + " " + fmtN(rad) + " 0 0 1 " + fmtN(maxX) + " " + fmtN(minY+rad) +
		" L" + fmtN(maxX) + " " + fmtN(maxY-rad) +
		" A" + fmtN(rad) + " " + fmtN(rad) + " 0 0 1 " + fmtN(maxX-rad) + " " + fmtN(maxY) +
		" L" + fmtN(minX+rad) + " " + fmtN(maxY) +
		" A" + fmtN(rad) + " " + fmtN(rad) + " 0 0 1 " + fmtN(minX) + " " + fmtN(maxY-rad) +
		" L" + fmtN(minX) + " " + fmtN(minY+rad) +
		" A" + fmtN(rad) + " " + fmtN(rad) + " 0 0 1 " + fmtN(minX+rad) + " " + fmtN(minY) +
		" Z"
}
