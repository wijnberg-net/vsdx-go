package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"wijnberg.net/vsdx-go/vsdx"
)

// Arrow marker definitions
var arrowDefs = map[int]string{
	1:  "M0 0 L10 5 L0 10 z",             // Triangle filled
	2:  "M0 0 L10 5 L0 10 L2 5 z",        // Stealth
	3:  "M0 0 L10 5 L0 10",               // Triangle open
	4:  "M0 0 L10 5 M0 10 L10 5",         // Line arrow
	5:  "M5 0 A5 5 0 1 1 5 10 A5 5 0 1 1 5 0 z", // Circle filled
	13: "M0 0 L10 5 L0 10 z",             // Standard arrow (same as triangle)
}

// nurbsControlPoint is an interior control point parsed from a NURBS formula.
type nurbsControlPoint struct {
	x, y float64
}

// nurbsData holds parsed NURBS formula data including coordinate type flags.
type nurbsData struct {
	xType int                 // 0 = proportional (0-1), 1 = absolute (inches)
	yType int                 // 0 = proportional (0-1), 1 = absolute (inches)
	cps   []nurbsControlPoint // interior control points
}

// parseNURBSData extracts data from a NURBS() formula string.
// Format: NURBS(knotLast, degree, xType, yType, x1,y1,knot1,weight1, x2,y2,knot2,weight2, ...)
func parseNURBSData(formula string) *nurbsData {
	formula = strings.TrimSpace(formula)
	upper := strings.ToUpper(formula)
	if !strings.HasPrefix(upper, "NURBS(") || !strings.HasSuffix(formula, ")") {
		return nil
	}
	inner := formula[6 : len(formula)-1]

	parts := strings.Split(inner, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if len(parts) < 8 {
		return nil
	}

	xType := int(toFloat(parts[2]))
	yType := int(toFloat(parts[3]))

	var cps []nurbsControlPoint
	for i := 4; i+3 < len(parts); i += 4 {
		x := toFloat(parts[i])
		y := toFloat(parts[i+1])
		cps = append(cps, nurbsControlPoint{x: x, y: y})
	}
	return &nurbsData{xType: xType, yType: yType, cps: cps}
}

// toFloat parses a string to float64, returning 0 on error.
func toFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// Helper to check if geometry cell has value "1"
func geomCellBool(geom *vsdx.Geometry, name string) bool {
	for _, c := range geom.Cells {
		if c.Name() == name && c.Value() == "1" {
			return true
		}
	}
	return false
}

// Helper to convert cell value to float
func cellFloat(row *vsdx.GeometryRow, name string) float64 {
	if c := row.Cells[name]; c != nil {
		v := c.Value()
		if v == "" {
			return 0
		}
		var f float64
		fmt.Sscanf(v, "%f", &f)
		return f
	}
	return 0
}

// fmtPrec formats a float with given precision
func fmtPrec(v float64, prec int) string {
	return fmt.Sprintf("%.*f", prec, v)
}

// escapeXML escapes special XML characters
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// PageToSVG renders a full page to SVG similar to Microsoft Visio's export.
func PageToSVG(page *vsdx.Page) string {
	pageW := page.Width()  // inches
	pageH := page.Height() // inches

	// Convert to points (72 points per inch) - Visio uses points in SVG export
	pageWPt := pageW * 72
	pageHPt := pageH * 72

	// Collect shapes and arrow markers
	shapes := page.ChildShapes()
	arrowMarkers := collectArrowMarkers(shapes)

	var svg strings.Builder

	// SVG header matching Visio's format
	svg.WriteString(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"
     width="%.4fin" height="%.4fin" viewBox="0 0 %.3f %.3f">
`, pageW, pageH, pageWPt, pageHPt))

	// Add defs section with filters and markers
	svg.WriteString("  <defs>\n")
	// Shadow filter
	svg.WriteString(`    <filter id="shadow_filter"><feGaussianBlur stdDeviation="2"/></filter>
`)

	// Add marker definitions if any arrows are used
	for markerID, arrowType := range arrowMarkers {
		pathData := arrowDefs[arrowType]
		if pathData == "" {
			pathData = arrowDefs[13] // fallback to standard arrow
		}
		// Determine if marker is filled
		filled := strings.HasSuffix(pathData, "z")
		fill := "#000000"
		stroke := "none"
		if !filled {
			fill = "none"
			stroke = "#000000"
		}
		svg.WriteString(fmt.Sprintf("    <marker id=\"%s\" viewBox=\"0 0 10 10\" refX=\"10\" refY=\"5\" markerWidth=\"4\" markerHeight=\"4\" orient=\"auto\">\n", markerID))
		svg.WriteString(fmt.Sprintf("      <path d=\"%s\" fill=\"%s\" stroke=\"%s\"/>\n", pathData, fill, stroke))
		svg.WriteString("    </marker>\n")
	}
	svg.WriteString("  </defs>\n")

	// Sort shapes by ID for consistent output
	sort.Slice(shapes, func(i, j int) bool {
		return shapes[i].ID < shapes[j].ID
	})

	for _, shape := range shapes {
		shapeSVG := renderShapeOnPage(shape, pageHPt, 72, 3)
		svg.WriteString(shapeSVG)
	}

	svg.WriteString("</svg>\n")
	return svg.String()
}

// collectArrowMarkers finds all arrow markers used in shapes
func collectArrowMarkers(shapes []*vsdx.Shape) map[string]int {
	markers := make(map[string]int)
	for _, shape := range shapes {
		collectShapeArrows(shape, markers)
	}
	return markers
}

func collectShapeArrows(shape *vsdx.Shape, markers map[string]int) {
	// Check for begin arrow
	if v := shape.CellValue("BeginArrow"); v != "" {
		if arrowType, err := strconv.Atoi(v); err == nil && arrowType > 0 {
			markers[fmt.Sprintf("arrow-start-%d", arrowType)] = arrowType
		}
	}
	// Check for end arrow
	if v := shape.CellValue("EndArrow"); v != "" {
		if arrowType, err := strconv.Atoi(v); err == nil && arrowType > 0 {
			markers[fmt.Sprintf("arrow-end-%d", arrowType)] = arrowType
		}
	}
	// Check children
	for _, child := range shape.ChildShapes() {
		collectShapeArrows(child, markers)
	}
}

// renderShapeOnPage renders a shape positioned on the page.
func renderShapeOnPage(shape *vsdx.Shape, pageHPt, scale float64, prec int) string {
	var svg strings.Builder

	// Get shape position and dimensions in inches
	pinX := shape.X()     // center X in inches
	pinY := shape.Y()     // center Y in inches (from bottom)
	locPinX := shape.LocX()
	locPinY := shape.LocY()
	width := shape.Width()
	height := shape.Height()

	// Use absolute values for layout calculations but remember sign
	absWidth := math.Abs(width)
	if absWidth == 0 {
		absWidth = 0.001
	}
	absHeight := math.Abs(height)
	if absHeight == 0 {
		absHeight = 0.001
	}

	// Calculate top-left corner in page coordinates (inches)
	// In Visio, Y=0 is at bottom; in SVG, Y=0 is at top
	tlX := pinX - locPinX
	tlY := pinY - locPinY

	// Convert to SVG coordinates (points, Y-flipped)
	svgX := tlX * scale
	svgY := pageHPt - (tlY+absHeight)*scale

	// Get rotation angle
	angle := shape.Angle()
	angleDeg := angle * 180 / math.Pi

	// Build transform
	transform := fmt.Sprintf("translate(%s,%s)", fmtPrec(svgX, prec), fmtPrec(svgY, prec))
	if angleDeg != 0 {
		// Rotate around local pin
		cx := locPinX * scale
		cy := absHeight*scale - locPinY*scale
		transform += fmt.Sprintf(" rotate(%s,%s,%s)", fmtPrec(-angleDeg, prec), fmtPrec(cx, prec), fmtPrec(cy, prec))
	}

	svg.WriteString(fmt.Sprintf(`<g id="shape%s" transform="%s">`, shape.ID, transform))
	svg.WriteByte('\n')

	// Render shadow first (if shape has geometry)
	if len(shape.Geometries) > 0 {
		shadowSVG := renderGeometryShadow(shape, scale, prec)
		svg.WriteString(shadowSVG)
	}

	// Render this shape's own geometry (groups can have geometry too)
	if len(shape.Geometries) > 0 {
		geomSVG := renderGeometry(shape, scale, prec)
		svg.WriteString(geomSVG)
	}

	// Render children
	for _, child := range shape.ChildShapes() {
		childSVG := renderChildShape(child, absHeight*scale, scale, prec)
		svg.WriteString(childSVG)
	}

	// Render text
	textSVG := renderText(shape, scale, prec)
	svg.WriteString(textSVG)

	svg.WriteString("</g>\n")
	return svg.String()
}

// renderChildShape renders a child shape within a group.
func renderChildShape(shape *vsdx.Shape, parentHPt, scale float64, prec int) string {
	var svg strings.Builder

	pinX := shape.X()
	pinY := shape.Y()
	locPinX := shape.LocX()
	locPinY := shape.LocY()
	height := shape.Height()

	absHeight := math.Abs(height)
	if absHeight == 0 {
		absHeight = 0.001
	}

	tlX := pinX - locPinX
	tlY := pinY - locPinY

	svgX := tlX * scale
	svgY := parentHPt - (tlY+absHeight)*scale

	angle := shape.Angle()
	angleDeg := angle * 180 / math.Pi

	transform := fmt.Sprintf("translate(%s,%s)", fmtPrec(svgX, prec), fmtPrec(svgY, prec))
	if angleDeg != 0 {
		cx := locPinX * scale
		cy := absHeight*scale - locPinY*scale
		transform += fmt.Sprintf(" rotate(%s,%s,%s)", fmtPrec(-angleDeg, prec), fmtPrec(cx, prec), fmtPrec(cy, prec))
	}

	svg.WriteString(fmt.Sprintf(`  <g id="shape%s" transform="%s">`, shape.ID, transform))
	svg.WriteByte('\n')

	// Render shadow first
	if len(shape.Geometries) > 0 {
		shadowSVG := renderGeometryShadow(shape, scale, prec)
		svg.WriteString(shadowSVG)
	}

	// Render this shape's own geometry
	if len(shape.Geometries) > 0 {
		geomSVG := renderGeometry(shape, scale, prec)
		svg.WriteString(geomSVG)
	}

	// Render children
	for _, child := range shape.ChildShapes() {
		childSVG := renderChildShape(child, absHeight*scale, scale, prec)
		svg.WriteString(childSVG)
	}

	textSVG := renderText(shape, scale, prec)
	svg.WriteString(textSVG)

	svg.WriteString("  </g>\n")
	return svg.String()
}

// getArrowSetback calculates the path shortening distance for arrow markers.
// Visio shortens connector paths by the arrow extent so markers fit precisely.
func getArrowSetback(shape *vsdx.Shape) (beginSetback, endSetback float64) {
	// Arrow setback formula: setback = arrowExtent * scaleFactor
	// Default arrow type 4 with size 2: extent=2, scale=3.52, setback=7.04 pts
	// Arrow size cells: 0=very small, 1=small, 2=medium(default), 3=large, 4=very large, 5=jumbo
	arrowScales := map[int]float64{
		0: 2.0,  // very small
		1: 2.5,  // small
		2: 3.52, // medium (default)
		3: 4.5,  // large
		4: 5.5,  // very large
		5: 6.5,  // jumbo
	}

	if v := shape.CellValue("BeginArrow"); v != "" {
		if arrowType, err := strconv.Atoi(v); err == nil && arrowType > 0 {
			size := 2 // default
			if sizeStr := shape.CellValue("BeginArrowSize"); sizeStr != "" {
				if s, err := strconv.Atoi(sizeStr); err == nil {
					size = s
				}
			}
			scaleFactor := arrowScales[size]
			if scaleFactor == 0 {
				scaleFactor = 3.52
			}
			beginSetback = 2.0 * scaleFactor // arrow path extent is 2 units
		}
	}

	if v := shape.CellValue("EndArrow"); v != "" {
		if arrowType, err := strconv.Atoi(v); err == nil && arrowType > 0 {
			size := 2 // default
			if sizeStr := shape.CellValue("EndArrowSize"); sizeStr != "" {
				if s, err := strconv.Atoi(sizeStr); err == nil {
					size = s
				}
			}
			scaleFactor := arrowScales[size]
			if scaleFactor == 0 {
				scaleFactor = 3.52
			}
			endSetback = 2.0 * scaleFactor
		}
	}

	return beginSetback, endSetback
}

// renderGeometry renders the geometry sections of a shape.
func renderGeometry(shape *vsdx.Shape, scale float64, prec int) string {
	var svg strings.Builder

	width := math.Abs(shape.Width())
	if width == 0 {
		width = 0.001
	}
	height := math.Abs(shape.Height())
	if height == 0 {
		height = 0.001
	}

	// Calculate arrow setback for connectors
	beginSetback, endSetback := getArrowSetback(shape)

	// Get fill and stroke colors
	fillColor := shape.FillColor()
	lineColor := shape.LineColor()
	lineWeight := shape.LineWeight() * scale

	if fillColor == "" {
		fillColor = "none"
	}
	if lineColor == "" {
		// Use theme accent color for connectors (shapes with no fill and a line)
		// Detect connector by checking BeginX/EndX cells (connectors have these)
		if shape.CellValue("BeginX") != "" || shape.CellValue("EndX") != "" {
			lineColor = "#5b9bd5" // Theme accent4 - default connector color
		} else {
			lineColor = "#000000"
		}
	}
	if lineWeight == 0 {
		lineWeight = 0.75 // default 1pt in 72dpi
	}

	for _, geom := range shape.Geometries {
		if geomCellBool(geom, "NoShow") {
			continue
		}

		var pathD strings.Builder
		rows := geom.SortedRows()
		var prevX, prevY float64

		// For simple line connectors, collect points and apply arrow setback
		isSimpleLine := len(rows) == 2 && rows[0].RowType() == "MoveTo" && rows[1].RowType() == "LineTo"

		for i, row := range rows {
			x := row.X() * scale
			y := (height - row.Y()) * scale

			// Apply arrow setback for simple line connectors
			if isSimpleLine && (beginSetback > 0 || endSetback > 0) {
				startX := rows[0].X() * scale
				startY := (height - rows[0].Y()) * scale
				endX := rows[1].X() * scale
				endY := (height - rows[1].Y()) * scale

				dx := endX - startX
				dy := endY - startY
				lineLen := math.Sqrt(dx*dx + dy*dy)
				if lineLen > 0.001 {
					ux := dx / lineLen // unit vector
					uy := dy / lineLen

					if i == 0 { // MoveTo - apply begin setback
						x = startX + ux*beginSetback
						y = startY + uy*beginSetback
					} else { // LineTo - apply end setback
						x = endX - ux*endSetback
						y = endY - uy*endSetback
					}
				}
			}

			switch row.RowType() {
			case "MoveTo":
				pathD.WriteString(fmt.Sprintf("M%s %s ", fmtPrec(x, prec), fmtPrec(y, prec)))
				prevX, prevY = x, y
			case "LineTo":
				pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(x, prec), fmtPrec(y, prec)))
				prevX, prevY = x, y
			case "RelMoveTo":
				// RelMoveTo uses fractions 0-1 of shape width/height
				absX := row.X() * width * scale
				absY := (height - row.Y()*height) * scale
				pathD.WriteString(fmt.Sprintf("M%s %s ", fmtPrec(absX, prec), fmtPrec(absY, prec)))
				prevX, prevY = absX, absY
			case "RelLineTo":
				absX := row.X() * width * scale
				absY := (height - row.Y()*height) * scale
				pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(absX, prec), fmtPrec(absY, prec)))
				prevX, prevY = absX, absY
			case "ArcTo":
				// ArcTo: A is the bow (bulge) value
				bow := cellFloat(row, "A") * scale
				arcSVG := arcToSVG(prevX, prevY, x, y, bow, prec)
				pathD.WriteString(arcSVG + " ")
				prevX, prevY = x, y
			case "Ellipse":
				// Ellipse row: X,Y = center, A,B = point on major axis, C,D = point on minor axis
				cx := row.X() * scale
				cy := (height - row.Y()) * scale
				// A,B define a point on the X-axis of the ellipse
				aX := cellFloat(row, "A") * scale
				aY := (height - cellFloat(row, "B")) * scale
				// C,D define a point on the Y-axis of the ellipse
				cX := cellFloat(row, "C") * scale
				cY := (height - cellFloat(row, "D")) * scale
				// Calculate radii
				rx := math.Sqrt((aX-cx)*(aX-cx) + (aY-cy)*(aY-cy))
				ry := math.Sqrt((cX-cx)*(cX-cx) + (cY-cy)*(cY-cy))
				// Output as SVG ellipse (but we need path for proper closing)
				// Use two arc commands to draw full ellipse
				if rx > 0 && ry > 0 {
					pathD.WriteString(fmt.Sprintf("M%s %s A%s %s 0 1 1 %s %s A%s %s 0 1 1 %s %s ",
						fmtPrec(cx+rx, prec), fmtPrec(cy, prec),
						fmtPrec(rx, prec), fmtPrec(ry, prec),
						fmtPrec(cx-rx, prec), fmtPrec(cy, prec),
						fmtPrec(rx, prec), fmtPrec(ry, prec),
						fmtPrec(cx+rx, prec), fmtPrec(cy, prec)))
				}
			case "EllipticalArcTo", "RelEllipticalArcTo":
				// EllipticalArcTo: X,Y = end point, A,B = control point, C = rotation, D = aspect ratio (rx/ry)
				endX := x
				endY := y
				cpX := cellFloat(row, "A") * scale
				cpY := (height - cellFloat(row, "B")) * scale
				aspectRatio := cellFloat(row, "D")
				if aspectRatio == 0 {
					aspectRatio = 1
				}
				// Calculate arc parameters from control point
				midX := (prevX + endX) / 2
				midY := (prevY + endY) / 2
				dx := cpX - midX
				dy := cpY - midY
				dist := math.Sqrt(dx*dx + dy*dy)
				if dist < 0.01 {
					// Control point at midpoint - straight line
					pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(endX, prec), fmtPrec(endY, prec)))
				} else {
					// Calculate radii from chord length and aspect ratio
					// Chord is the horizontal span, rx = chord/2, ry = rx/aspectRatio
					chordLen := math.Sqrt((endX-prevX)*(endX-prevX) + (endY-prevY)*(endY-prevY))
					rx := chordLen / 2
					ry := rx / aspectRatio // aspectRatio = rx/ry, so ry = rx/aspectRatio
					if rx > 0 && ry > 0 {
						// Determine sweep direction based on control point position
						cross := (endX-prevX)*(cpY-prevY) - (endY-prevY)*(cpX-prevX)
						sweep := 0
						if cross < 0 {
							sweep = 1
						}
						// Use large-arc flag = 1 for semicircle caps
						largeArc := 1
						pathD.WriteString(fmt.Sprintf("A%s %s 0 %d %d %s %s ",
							fmtPrec(rx, prec), fmtPrec(ry, prec), largeArc, sweep,
							fmtPrec(endX, prec), fmtPrec(endY, prec)))
					} else {
						pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(endX, prec), fmtPrec(endY, prec)))
					}
				}
				prevX, prevY = endX, endY
			case "NURBSTo":
				// Get the E cell formula containing NURBS control points
				eFormula := ""
				if c := row.Cells["E"]; c != nil {
					eFormula = c.Value()
					// Also check Formula attribute if Value is empty
					if eFormula == "" {
						eFormula = c.Formula()
					}
				}
				nurbsInfo := parseNURBSData(eFormula)

				if nurbsInfo != nil && len(nurbsInfo.cps) == 2 {
					// Degree 3, 2 interior control points → cubic Bezier
					var cp1AbsX, cp1AbsY, cp2AbsX, cp2AbsY float64
					if nurbsInfo.xType == 0 {
						cp1AbsX = nurbsInfo.cps[0].x * width * scale
						cp2AbsX = nurbsInfo.cps[1].x * width * scale
					} else {
						cp1AbsX = nurbsInfo.cps[0].x * scale
						cp2AbsX = nurbsInfo.cps[1].x * scale
					}
					if nurbsInfo.yType == 0 {
						cp1AbsY = (height - nurbsInfo.cps[0].y*height) * scale
						cp2AbsY = (height - nurbsInfo.cps[1].y*height) * scale
					} else {
						cp1AbsY = (height - nurbsInfo.cps[0].y) * scale
						cp2AbsY = (height - nurbsInfo.cps[1].y) * scale
					}
					pathD.WriteString(fmt.Sprintf("C%s %s %s %s %s %s ",
						fmtPrec(cp1AbsX, prec), fmtPrec(cp1AbsY, prec),
						fmtPrec(cp2AbsX, prec), fmtPrec(cp2AbsY, prec),
						fmtPrec(x, prec), fmtPrec(y, prec)))
				} else if nurbsInfo != nil && len(nurbsInfo.cps) >= 1 {
					// Single control point - use quadratic bezier
					var cpAbsX, cpAbsY float64
					if nurbsInfo.xType == 0 {
						cpAbsX = nurbsInfo.cps[0].x * width * scale
					} else {
						cpAbsX = nurbsInfo.cps[0].x * scale
					}
					if nurbsInfo.yType == 0 {
						cpAbsY = (height - nurbsInfo.cps[0].y*height) * scale
					} else {
						cpAbsY = (height - nurbsInfo.cps[0].y) * scale
					}
					pathD.WriteString(fmt.Sprintf("Q%s %s %s %s ",
						fmtPrec(cpAbsX, prec), fmtPrec(cpAbsY, prec),
						fmtPrec(x, prec), fmtPrec(y, prec)))
				} else {
					// Fallback to line if no valid NURBS data
					pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(x, prec), fmtPrec(y, prec)))
				}
				prevX, prevY = x, y
			case "PolylineTo":
				pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(x, prec), fmtPrec(y, prec)))
				prevX, prevY = x, y
			}
		}

		// Close path if not NoFill
		if !geomCellBool(geom, "NoFill") && pathD.Len() > 0 {
			pathD.WriteString("Z")
		}

		path := pathD.String()
		if path == "" {
			continue
		}

		fillAttr := fillColor
		if geomCellBool(geom, "NoFill") {
			fillAttr = "none"
		}

		strokeAttr := lineColor
		if geomCellBool(geom, "NoLine") {
			strokeAttr = "none"
		}

		// Check for arrow markers
		var markerAttrs string
		if v := shape.CellValue("BeginArrow"); v != "" {
			if arrowType, err := strconv.Atoi(v); err == nil && arrowType > 0 {
				markerAttrs += fmt.Sprintf(` marker-start="url(#arrow-start-%d)"`, arrowType)
			}
		}
		if v := shape.CellValue("EndArrow"); v != "" {
			if arrowType, err := strconv.Atoi(v); err == nil && arrowType > 0 {
				markerAttrs += fmt.Sprintf(` marker-end="url(#arrow-end-%d)"`, arrowType)
			}
		}

		svg.WriteString(fmt.Sprintf(`    <path d="%s" fill="%s" stroke="%s" stroke-width="%s"%s/>`,
			strings.TrimSpace(path), fillAttr, strokeAttr, fmtPrec(lineWeight, prec), markerAttrs))
		svg.WriteByte('\n')
	}

	return svg.String()
}

// renderGeometryShadow renders a shadow copy of the shape's geometry.
func renderGeometryShadow(shape *vsdx.Shape, scale float64, prec int) string {
	var svg strings.Builder

	width := math.Abs(shape.Width())
	if width == 0 {
		width = 0.001
	}
	height := math.Abs(shape.Height())
	if height == 0 {
		height = 0.001
	}

	// Shadow styling - matches Visio's default theme shadow
	shadowColor := "#5b9bd5"
	shadowOpacity := "0.22"
	// Shadow offset: Visio uses approx (0.35, -1.97) in points
	shadowOffsetX := -1.97 // translate X (negative because Visio's shadow goes left/down)
	shadowOffsetY := 0.35  // translate Y

	for _, geom := range shape.Geometries {
		if geomCellBool(geom, "NoShow") {
			continue
		}

		var pathD strings.Builder
		rows := geom.SortedRows()
		var prevX, prevY float64

		for _, row := range rows {
			x := row.X() * scale
			y := (height - row.Y()) * scale

			switch row.RowType() {
			case "MoveTo":
				pathD.WriteString(fmt.Sprintf("M%s %s ", fmtPrec(x, prec), fmtPrec(y, prec)))
				prevX, prevY = x, y
			case "LineTo":
				pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(x, prec), fmtPrec(y, prec)))
				prevX, prevY = x, y
			case "RelMoveTo":
				absX := row.X() * width * scale
				absY := (height - row.Y()*height) * scale
				pathD.WriteString(fmt.Sprintf("M%s %s ", fmtPrec(absX, prec), fmtPrec(absY, prec)))
				prevX, prevY = absX, absY
			case "RelLineTo":
				absX := row.X() * width * scale
				absY := (height - row.Y()*height) * scale
				pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(absX, prec), fmtPrec(absY, prec)))
				prevX, prevY = absX, absY
			case "ArcTo":
				bow := cellFloat(row, "A") * scale
				arcSVG := arcToSVG(prevX, prevY, x, y, bow, prec)
				pathD.WriteString(arcSVG + " ")
				prevX, prevY = x, y
			case "NURBSTo":
				eFormula := ""
				if c := row.Cells["E"]; c != nil {
					eFormula = c.Value()
					if eFormula == "" {
						eFormula = c.Formula()
					}
				}
				nurbsInfo := parseNURBSData(eFormula)
				if nurbsInfo != nil && len(nurbsInfo.cps) == 2 {
					var cp1AbsX, cp1AbsY, cp2AbsX, cp2AbsY float64
					if nurbsInfo.xType == 0 {
						cp1AbsX = nurbsInfo.cps[0].x * width * scale
						cp2AbsX = nurbsInfo.cps[1].x * width * scale
					} else {
						cp1AbsX = nurbsInfo.cps[0].x * scale
						cp2AbsX = nurbsInfo.cps[1].x * scale
					}
					if nurbsInfo.yType == 0 {
						cp1AbsY = (height - nurbsInfo.cps[0].y*height) * scale
						cp2AbsY = (height - nurbsInfo.cps[1].y*height) * scale
					} else {
						cp1AbsY = (height - nurbsInfo.cps[0].y) * scale
						cp2AbsY = (height - nurbsInfo.cps[1].y) * scale
					}
					pathD.WriteString(fmt.Sprintf("C%s %s %s %s %s %s ",
						fmtPrec(cp1AbsX, prec), fmtPrec(cp1AbsY, prec),
						fmtPrec(cp2AbsX, prec), fmtPrec(cp2AbsY, prec),
						fmtPrec(x, prec), fmtPrec(y, prec)))
				} else {
					pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(x, prec), fmtPrec(y, prec)))
				}
				prevX, prevY = x, y
			case "EllipticalArcTo", "RelEllipticalArcTo":
				// Same calculation as main geometry rendering
				endX := x
				endY := y
				cpX := cellFloat(row, "A") * scale
				cpY := (height - cellFloat(row, "B")) * scale
				aspectRatio := cellFloat(row, "D")
				if aspectRatio == 0 {
					aspectRatio = 1
				}
				midX := (prevX + endX) / 2
				midY := (prevY + endY) / 2
				dx := cpX - midX
				dy := cpY - midY
				dist := math.Sqrt(dx*dx + dy*dy)
				if dist < 0.01 {
					pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(endX, prec), fmtPrec(endY, prec)))
				} else {
					chordLen := math.Sqrt((endX-prevX)*(endX-prevX) + (endY-prevY)*(endY-prevY))
					rx := chordLen / 2
					ry := rx / aspectRatio
					if rx > 0 && ry > 0 {
						cross := (endX-prevX)*(cpY-prevY) - (endY-prevY)*(cpX-prevX)
						sweep := 0
						if cross < 0 {
							sweep = 1
						}
						largeArc := 1
						pathD.WriteString(fmt.Sprintf("A%s %s 0 %d %d %s %s ",
							fmtPrec(rx, prec), fmtPrec(ry, prec), largeArc, sweep,
							fmtPrec(endX, prec), fmtPrec(endY, prec)))
					} else {
						pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(endX, prec), fmtPrec(endY, prec)))
					}
				}
				prevX, prevY = endX, endY
			case "Ellipse":
				cx := row.X() * scale
				cy := (height - row.Y()) * scale
				aX := cellFloat(row, "A") * scale
				aY := (height - cellFloat(row, "B")) * scale
				cX := cellFloat(row, "C") * scale
				cY := (height - cellFloat(row, "D")) * scale
				rx := math.Sqrt((aX-cx)*(aX-cx) + (aY-cy)*(aY-cy))
				ry := math.Sqrt((cX-cx)*(cX-cx) + (cY-cy)*(cY-cy))
				if rx > 0 && ry > 0 {
					pathD.WriteString(fmt.Sprintf("M%s %s A%s %s 0 1 1 %s %s A%s %s 0 1 1 %s %s ",
						fmtPrec(cx+rx, prec), fmtPrec(cy, prec),
						fmtPrec(rx, prec), fmtPrec(ry, prec),
						fmtPrec(cx-rx, prec), fmtPrec(cy, prec),
						fmtPrec(rx, prec), fmtPrec(ry, prec),
						fmtPrec(cx+rx, prec), fmtPrec(cy, prec)))
				}
			default:
				pathD.WriteString(fmt.Sprintf("L%s %s ", fmtPrec(x, prec), fmtPrec(y, prec)))
				prevX, prevY = x, y
			}
		}

		if !geomCellBool(geom, "NoFill") && pathD.Len() > 0 {
			pathD.WriteString("Z")
		}

		path := pathD.String()
		if path == "" {
			continue
		}

		// Shadow path with offset transform and blur filter
		svg.WriteString(fmt.Sprintf(`    <path d="%s" fill="%s" fill-opacity="%s" stroke="%s" stroke-opacity="%s" filter="url(#shadow_filter)" transform="translate(%s,%s)"/>`,
			strings.TrimSpace(path), shadowColor, shadowOpacity, shadowColor, shadowOpacity, fmtPrec(shadowOffsetX, prec), fmtPrec(shadowOffsetY, prec)))
		svg.WriteByte('\n')
	}

	return svg.String()
}

// arcToSVG converts a Visio ArcTo (bow-based) to SVG arc command
func arcToSVG(x1, y1, x2, y2, bow float64, prec int) string {
	if math.Abs(bow) < 0.001 {
		// No bow - just a line
		return fmt.Sprintf("L%s %s", fmtPrec(x2, prec), fmtPrec(y2, prec))
	}

	// Calculate arc parameters from bow
	dx := x2 - x1
	dy := y2 - y1
	chordLen := math.Sqrt(dx*dx + dy*dy)
	if chordLen < 0.001 {
		return ""
	}

	// Bow is the perpendicular distance from chord midpoint to arc
	// Calculate radius from bow and chord length
	// bow = r - sqrt(r^2 - (chord/2)^2)
	// Solving: r = (bow^2 + (chord/2)^2) / (2*bow)
	halfChord := chordLen / 2
	r := (bow*bow + halfChord*halfChord) / (2 * math.Abs(bow))

	// Large arc flag: if |bow| > chord/2, arc is > 180 degrees
	largeArc := 0
	if math.Abs(bow) > halfChord {
		largeArc = 1
	}

	// Sweep flag: positive bow = clockwise in SVG (counterclockwise in Visio coords after Y-flip)
	sweep := 0
	if bow > 0 {
		sweep = 1
	}

	return fmt.Sprintf("A%s %s 0 %d %d %s %s",
		fmtPrec(r, prec), fmtPrec(r, prec),
		largeArc, sweep,
		fmtPrec(x2, prec), fmtPrec(y2, prec))
}

// renderText renders the text content of a shape.
func renderText(shape *vsdx.Shape, scale float64, prec int) string {
	text := shape.Text()
	if text == "" {
		return ""
	}

	width := math.Abs(shape.Width())
	height := math.Abs(shape.Height())

	// Center text in shape
	x := width * scale / 2
	y := height * scale / 2

	// Get font size
	fontSize := shape.TextSize() * scale
	if fontSize == 0 {
		fontSize = 12
	}

	// Get text color
	textColor := shape.TextColor()
	if textColor == "" {
		textColor = "#000000"
	}

	// Escape XML entities
	text = escapeXML(strings.TrimSpace(text))

	return fmt.Sprintf(`    <text x="%s" y="%s" text-anchor="middle" dominant-baseline="middle" font-size="%s" fill="%s">%s</text>
`, fmtPrec(x, prec), fmtPrec(y, prec), fmtPrec(fontSize, prec), textColor, text)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: render_page <vsdx-file>")
		os.Exit(1)
	}

	vsdxPath := os.Args[1]

	vf, err := vsdx.Open(vsdxPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer vf.Close()

	for i, page := range vf.Pages {
		svg := PageToSVG(page)

		base := strings.TrimSuffix(filepath.Base(vsdxPath), ".vsdx")
		outPath := filepath.Join(filepath.Dir(vsdxPath), fmt.Sprintf("%s-page%d-generated.svg", base, i+1))
		os.WriteFile(outPath, []byte(svg), 0644)
		fmt.Printf("Wrote: %s\n", outPath)
	}
}
