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
}

// ResolvedGeomResult contains the resolved geometry ready for SVG emission.
type ResolvedGeomResult struct {
	PathData     string  // SVG path data (d attribute)
	Fill         string  // fill color
	Stroke       string  // stroke color
	StrokeWidth  float64 // stroke width in SVG units
	FillOpacity  float64 // 0-1
	DashArray    string  // stroke-dasharray
	LineCap      string  // stroke-linecap
	VectorEffect string  // vector-effect attribute
	MarkerStart  string  // marker-start URL
	MarkerEnd    string  // marker-end URL
	FilterID     string  // filter URL for shadows
	GradientID   string  // gradient URL for fills
	NoFill       bool    // geometry-level NoFill
	NoLine       bool    // geometry-level NoLine
	NoShow       bool    // geometry-level NoShow
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
	if geom == nil || len(geom.Rows) == 0 {
		return nil
	}

	resolver := &GeometryResolver{
		shape:      shape,
		geom:       geom,
		style:      style,
		localW:     localW,
		localH:     localH,
		offsetX:    offsetX,
		offsetY:    offsetY,
		scaleX:     scaleX,
		scaleY:     scaleY,
		parentH:    parentH,
		negativeH:  negativeH,
		precision:  precision,
		geomIndex:  geomIndex,
		totalGeoms: totalGeoms,
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
	if result.NoLine {
		result.Stroke = "none"
	}

	// Resolve dash array
	if r.style.LinePattern > 1 && result.Stroke != "none" {
		result.DashArray = linePatternToSVG(r.style.LinePattern, result.StrokeWidth)
	}

	// Vector effect for non-scaling stroke
	if result.Stroke != "none" {
		result.VectorEffect = "non-scaling-stroke"
	}

	// Build path data
	pathData := r.buildPathData()
	if pathData == "" {
		return result
	}

	// Apply arrow setbacks BEFORE finalizing path
	// This is critical: setbacks must be applied in resolved coordinate space
	if r.style.BeginArrow > 0 {
		setback := r.computeArrowSetbackInSVGUnits(r.style.BeginArrowSetback)
		if setback > 0 {
			pathData = r.shortenPathStart(pathData, setback)
		}
		result.MarkerStart = fmt.Sprintf("url(#arrow_%d_%d_%s_start)",
			r.style.BeginArrow, r.style.BeginArrowSize, strings.TrimPrefix(result.Stroke, "#"))
	}
	if r.style.EndArrow > 0 {
		setback := r.computeArrowSetbackInSVGUnits(r.style.EndArrowSetback)
		if setback > 0 {
			pathData = r.shortenPathEnd(pathData, setback)
		}
		result.MarkerEnd = fmt.Sprintf("url(#arrow_%d_%d_%s_end)",
			r.style.EndArrow, r.style.EndArrowSize, strings.TrimPrefix(result.Stroke, "#"))
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
			sx, sy := row.X(), row.Y()
			svgX, svgY := r.localToSVG(sx+r.offsetX, sy+r.offsetY)
			d.WriteString(fmt.Sprintf("M%s %s", r.fmtNum(svgX), r.fmtNum(svgY)))
			prevX, prevY = svgX, svgY
			startX, startY = svgX, svgY

		case "lineto":
			sx, sy := row.X(), row.Y()
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
			sx, sy := row.X(), row.Y()
			svgX, svgY := r.localToSVG(sx+r.offsetX, sy+r.offsetY)
			bow := r.cellFloat(row, "A")
			arcSVG := r.arcToSVG(prevX, prevY, svgX, svgY, bow*((r.scaleX+r.scaleY)/2))
			d.WriteString(arcSVG)
			prevX, prevY = svgX, svgY

		case "ellipticalarcto":
			ex, ey := row.X(), row.Y()
			svgEndX, svgEndY := r.localToSVG(ex+r.offsetX, ey+r.offsetY)
			cpX := r.cellFloat(row, "A") + r.offsetX
			cpY := r.cellFloat(row, "B") + r.offsetY
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
			sx, sy := row.X(), row.Y()
			svgX, svgY := r.localToSVG(sx+r.offsetX, sy+r.offsetY)
			eFormula := r.cellString(row, "E")
			nurbsInfo := parseNURBSData(eFormula)

			nurbsSVG := r.nurbsToSVG(prevX, prevY, svgX, svgY, sx+r.offsetX, sy+r.offsetY, nurbsInfo)
			d.WriteString(nurbsSVG)
			prevX, prevY = svgX, svgY

		case "polylineto":
			sx, sy := row.X(), row.Y()
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
			sx := r.cellFloat(row, "X") + r.offsetX
			sy := r.cellFloat(row, "Y") + r.offsetY
			ax := r.cellFloat(row, "A") + r.offsetX
			ay := r.cellFloat(row, "B") + r.offsetY
			svgX1, svgY1 := r.localToSVG(sx, sy)
			svgX2, svgY2 := r.localToSVG(ax, ay)
			d.WriteString(fmt.Sprintf("M%s %s L%s %s",
				r.fmtNum(svgX1), r.fmtNum(svgY1),
				r.fmtNum(svgX2), r.fmtNum(svgY2)))
			prevX, prevY = svgX2, svgY2

		case "ellipse":
			cx := r.cellFloat(row, "X") + r.offsetX
			cy := r.cellFloat(row, "Y") + r.offsetY
			svgCx, svgCy := r.localToSVG(cx, cy)
			ax := r.cellFloat(row, "A")
			ay := r.cellFloat(row, "B")
			cx2 := r.cellFloat(row, "C")
			cy2 := r.cellFloat(row, "D")
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
	if nurbsInfo == nil || len(nurbsInfo.cps) == 0 {
		return fmt.Sprintf("L%s %s", r.fmtNum(svgX), r.fmtNum(svgY))
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
		// Degree 3, 2 interior control points + start + end = 4 total control points
		// Build the full B-spline control polygon and fit Bezier curve
		startLocalX := prevX / r.scaleX
		var startLocalY float64
		if r.negativeH {
			startLocalY = -prevY / r.scaleY
		} else {
			startLocalY = r.parentH - prevY/r.scaleY
		}

		bsplineCtrl := [][2]float64{
			{startLocalX, startLocalY},
			{absPoints[0][0], absPoints[0][1]},
			{absPoints[1][0], absPoints[1][1]},
			{endX, endY},
		}

		// Use knotLast to properly convert B-spline to Bezier
		bezierCtrl := r.fitBezierToBSpline(bsplineCtrl, nurbsInfo.knotLast)
		if bezierCtrl != nil && len(bezierCtrl) == 4 {
			cp1SvgX, cp1SvgY := r.localToSVG(bezierCtrl[1][0], bezierCtrl[1][1])
			cp2SvgX, cp2SvgY := r.localToSVG(bezierCtrl[2][0], bezierCtrl[2][1])
			d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
				r.fmtNum(cp1SvgX), r.fmtNum(cp1SvgY),
				r.fmtNum(cp2SvgX), r.fmtNum(cp2SvgY),
				r.fmtNum(svgX), r.fmtNum(svgY)))
		} else {
			// Fallback to direct use of control points
			cp1SvgX, cp1SvgY := r.localToSVG(absPoints[0][0], absPoints[0][1])
			cp2SvgX, cp2SvgY := r.localToSVG(absPoints[1][0], absPoints[1][1])
			d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
				r.fmtNum(cp1SvgX), r.fmtNum(cp1SvgY),
				r.fmtNum(cp2SvgX), r.fmtNum(cp2SvgY),
				r.fmtNum(svgX), r.fmtNum(svgY)))
		}
	} else if len(nurbsInfo.cps) == 1 {
		// Degree 2, 1 control point → quadratic Bezier
		cpSvgX, cpSvgY := r.localToSVG(absPoints[0][0], absPoints[0][1])
		d.WriteString(fmt.Sprintf("Q%s %s %s %s",
			r.fmtNum(cpSvgX), r.fmtNum(cpSvgY),
			r.fmtNum(svgX), r.fmtNum(svgY)))
	} else if len(nurbsInfo.cps) >= 3 {
		// For 3+ interior control points, use B-spline to multi-Bezier conversion
		// Build full control polygon: start + interior points + end
		startLocalX := prevX / r.scaleX
		var startLocalY float64
		if r.negativeH {
			startLocalY = -prevY / r.scaleY
		} else {
			startLocalY = r.parentH - prevY/r.scaleY
		}

		// Build control polygon with all points
		n := len(nurbsInfo.cps) + 2 // start + interior + end
		ctrl := make([][2]float64, n)
		ctrl[0] = [2]float64{startLocalX, startLocalY}
		for i, pt := range absPoints {
			ctrl[i+1] = [2]float64{pt[0], pt[1]}
		}
		ctrl[n-1] = [2]float64{endX, endY}

		// Convert B-spline to multiple Bezier curves
		beziers := r.bsplineToMultiBezier(ctrl, nurbsInfo.knotLast)
		d.WriteString(beziers)
	} else {
		d.WriteString(fmt.Sprintf("L%s %s", r.fmtNum(svgX), r.fmtNum(svgY)))
	}

	return d.String()
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
	}

	return pathData
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

		// Follow Visio's approach: shorten curve slightly and add line segment
		// This preserves the curve shape better than modifying control points
		dx := endX - cp2X
		dy := endY - cp2Y
		length := math.Sqrt(dx*dx + dy*dy)
		if length > shortenBy {
			ratio := shortenBy / length
			newEndX := endX - dx*ratio
			newEndY := endY - dy*ratio

			// Keep original control points, just shorten to new endpoint
			newArgs := fmt.Sprintf("%s %s %s %s %s %s",
				r.fmtNum(cp1X), r.fmtNum(cp1Y),
				r.fmtNum(cp2X), r.fmtNum(cp2Y),
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
