package vsdx

import (
	"bytes"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strings"
)

// SVGOptions controls the output of ShapeToSVG.
type SVGOptions struct {
	Width      float64 // output viewBox width in px (default: 100)
	Height     float64 // output viewBox height in px (default: 100)
	Precision  int     // decimal places for coordinates (default: 2)
	BrandColor string  // override brand color detection (default: auto-detect)
}

// SVGOption is a functional option for ShapeToSVG.
type SVGOption func(*SVGOptions)

// WithSize sets the output viewBox dimensions.
func WithSize(w, h float64) SVGOption {
	return func(o *SVGOptions) { o.Width = w; o.Height = h }
}

// WithPrecision sets the coordinate decimal precision.
func WithPrecision(p int) SVGOption {
	return func(o *SVGOptions) { o.Precision = p }
}

// WithBrandColor overrides brand color auto-detection.
func WithBrandColor(c string) SVGOption {
	return func(o *SVGOptions) { o.BrandColor = c }
}

// SVGResult contains the SVG output and detected metadata.
type SVGResult struct {
	SVG        []byte  // clean SVG markup
	BrandColor string  // auto-detected or overridden brand color (#RRGGBB)
	Width      float64 // actual viewBox width (may differ from requested if aspect-ratio preserved)
	Height     float64 // actual viewBox height
}

// ShapeToSVG converts a Visio shape (typically a master shape) to SVG bytes.
// The shape's geometry sections are converted to SVG paths with styling.
// Group shapes recursively include all child shape geometry.
func ShapeToSVG(shape *Shape, opts ...SVGOption) (*SVGResult, error) {
	o := &SVGOptions{
		Width:     100,
		Height:    100,
		Precision: 2,
	}
	for _, opt := range opts {
		opt(o)
	}

	// Collect all sub-shapes to render.
	// For groups: use child shapes. For simple shapes: just this shape.
	subShapes := collectRenderableShapes(shape)
	if len(subShapes) == 0 {
		return nil, fmt.Errorf("shape has no geometry to convert")
	}

	// Determine the bounding box in shape-local coordinates (inches).
	shapeW := shape.Width()
	shapeH := shape.Height()
	if shapeW <= 0 {
		shapeW = 1
	}
	if shapeH <= 0 {
		shapeH = 1
	}

	// Uniform scale: fit the shape proportionally into the requested output box.
	// This preserves the original aspect ratio instead of stretching.
	scale := o.Width / shapeW
	if s := o.Height / shapeH; s < scale {
		scale = s
	}
	scaleX := scale
	scaleY := scale
	// Actual output dimensions after uniform scaling.
	outW := shapeW * scale
	outH := shapeH * scale

	// Color frequency map for brand color detection.
	colorFreq := make(map[string]int)

	var pathElements []string
	var maxStrokeWidth float64
	for _, ss := range subShapes {
		pathSVG, sw := renderSubShape(ss, shape, scaleX, scaleY, o, colorFreq)
		if pathSVG != "" {
			pathElements = append(pathElements, pathSVG)
			if sw > maxStrokeWidth {
				maxStrokeWidth = sw
			}
		}
	}

	if len(pathElements) == 0 {
		return nil, fmt.Errorf("no geometry paths produced from shape")
	}

	// Detect brand color.
	brandColor := o.BrandColor
	if brandColor == "" {
		brandColor = detectBrandColor(colorFreq)
	}

	// Pad viewBox by half the max stroke width so strokes aren't clipped.
	pad := maxStrokeWidth / 2
	vbX := -pad
	vbY := -pad
	vbW := outW + 2*pad
	vbH := outH + 2*pad

	// Build SVG document.
	var svg strings.Builder
	svg.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s %s %s %s">`,
		fmtPrec(vbX, o.Precision), fmtPrec(vbY, o.Precision),
		fmtPrec(vbW, o.Precision), fmtPrec(vbH, o.Precision)))
	svg.WriteByte('\n')
	for _, p := range pathElements {
		svg.WriteString(p)
		svg.WriteByte('\n')
	}
	svg.WriteString("</svg>\n")

	return &SVGResult{
		SVG:        []byte(svg.String()),
		BrandColor: brandColor,
		Width:      outW,
		Height:     outH,
	}, nil
}

// renderableShape bundles a shape geometry section with its position relative to the parent group.
// A single shape with multiple geometry sections produces multiple renderableShapes.
type renderableShape struct {
	shape          *Shape
	geom           *Geometry // specific geometry section to render
	offsetX        float64   // X offset in parent group's coordinate space (inches)
	offsetY        float64   // Y offset in parent group's coordinate space (inches)
	localW, localH float64   // shape's own width/height
}

// shapeHasGeometry returns true if the shape has at least one non-empty geometry section.
func shapeHasGeometry(s *Shape) bool {
	for _, g := range s.Geometries {
		if len(g.Rows) > 0 {
			return true
		}
	}
	return false
}

// collectRenderableShapes returns all leaf shapes with geometry.
// Each geometry section within a shape is returned as a separate renderable.
func collectRenderableShapes(shape *Shape) []renderableShape {
	if shape.ShapeType == "Group" {
		var result []renderableShape
		for _, child := range shape.ChildShapes() {
			childW := child.Width()
			childH := child.Height()
			if childW <= 0 {
				childW = shape.Width()
			}
			if childH <= 0 {
				childH = shape.Height()
			}

			// Child position relative to group's local space.
			// PinX/PinY is center position; LocPinX/LocPinY is center offset within shape.
			ox := child.X() - child.LocX()
			oy := child.Y() - child.LocY()

			if child.ShapeType == "Group" {
				// Recursive: collect from nested group
				for _, nested := range collectRenderableShapes(child) {
					nested.offsetX += ox
					nested.offsetY += oy
					result = append(result, nested)
				}
			} else if shapeHasGeometry(child) {
				for _, g := range child.Geometries {
					if len(g.Rows) == 0 {
						continue
					}
					result = append(result, renderableShape{
						shape:   child,
						geom:    g,
						offsetX: ox,
						offsetY: oy,
						localW:  childW,
						localH:  childH,
					})
				}
			}
		}
		return result
	}

	// Simple shape — emit one renderable per geometry section.
	var result []renderableShape
	for _, g := range shape.Geometries {
		if len(g.Rows) > 0 {
			result = append(result, renderableShape{
				shape:   shape,
				geom:    g,
				offsetX: 0,
				offsetY: 0,
				localW:  shape.Width(),
				localH:  shape.Height(),
			})
		}
	}
	return result
}

// renderSubShape converts a single sub-shape's geometry to an SVG <path> element.
// renderSubShape converts a single geometry section to an SVG <path>.
// Returns the SVG string and the computed stroke width (for viewBox padding).
func renderSubShape(ss renderableShape, parent *Shape, scaleX, scaleY float64, o *SVGOptions, colorFreq map[string]int) (string, float64) {
	s := ss.shape
	geom := ss.geom
	if geom == nil || len(geom.Rows) == 0 {
		return "", 0
	}

	parentW := parent.Width()
	parentH := parent.Height()
	if parentW <= 0 {
		parentW = 1 //nolint:ineffassign // kept for symmetry with parentH guard
	}
	if parentH <= 0 {
		parentH = 1
	}

	// Sort geometry rows by IX.
	sortedRows := sortGeometryRows(geom.Rows)

	// Build SVG path data.
	var d strings.Builder
	var prevX, prevY float64 // track current point in SVG space

	for _, row := range sortedRows {
		rt := strings.ToLower(row.RowType())
		switch rt {
		case "moveto":
			// Absolute coordinates in shape's local space (inches)
			sx, sy := row.X(), row.Y()
			svgX, svgY := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)
			d.WriteString(fmt.Sprintf("M%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "lineto":
			sx, sy := row.X(), row.Y()
			svgX, svgY := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)
			d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "relmoveto":
			// RelMoveTo: coordinates are 0..1 fractions of shape's own width/height
			rx, ry := row.X(), row.Y()
			absX := rx*ss.localW + ss.offsetX
			absY := ry*ss.localH + ss.offsetY
			svgX, svgY := toSVGCoords(absX, absY, parentH, scaleX, scaleY)
			d.WriteString(fmt.Sprintf("M%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "rellineto":
			rx, ry := row.X(), row.Y()
			absX := rx*ss.localW + ss.offsetX
			absY := ry*ss.localH + ss.offsetY
			svgX, svgY := toSVGCoords(absX, absY, parentH, scaleX, scaleY)
			d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "arcto":
			sx, sy := row.X(), row.Y()
			svgX, svgY := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)
			bow := cellFloat(row, "A")

			arcSVG := arcToSVG(prevX, prevY, svgX, svgY, bow*((scaleX+scaleY)/2), o.Precision)
			d.WriteString(arcSVG)
			prevX, prevY = svgX, svgY

		case "ellipticalarcto":
			sx, sy := row.X(), row.Y()
			svgX, svgY := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)
			// A = control point X, B = control point Y, C = aspect ratio, D = angle
			cpX := cellFloat(row, "A")
			cpY := cellFloat(row, "B")
			_ = cpX
			_ = cpY
			// Approximate as line for now — elliptical arcs are complex and rare in stencils
			d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			prevX, prevY = svgX, svgY

		case "nurbsto":
			sx, sy := row.X(), row.Y()
			svgX, svgY := toSVGCoords(sx+ss.offsetX, sy+ss.offsetY, parentH, scaleX, scaleY)

			// Parse NURBS formula from E cell for control points.
			eFormula := cellString(row, "E")
			cps := parseNURBSControlPoints(eFormula)

			if len(cps) == 2 {
				// Degree 3, 2 interior control points → cubic Bezier.
				// Control point coordinates are proportional (0-1) of shape dimensions
				// when xType=0/yType=0, or absolute when xType=1/yType=1.
				// Visio uses the same proportional space as RelMoveTo.
				cp1AbsX := cps[0].x*ss.localW + ss.offsetX
				cp1AbsY := cps[0].y*ss.localH + ss.offsetY
				cp2AbsX := cps[1].x*ss.localW + ss.offsetX
				cp2AbsY := cps[1].y*ss.localH + ss.offsetY
				cp1SvgX, cp1SvgY := toSVGCoords(cp1AbsX, cp1AbsY, parentH, scaleX, scaleY)
				cp2SvgX, cp2SvgY := toSVGCoords(cp2AbsX, cp2AbsY, parentH, scaleX, scaleY)
				d.WriteString(fmt.Sprintf("C%s %s %s %s %s %s",
					fmtPrec(cp1SvgX, o.Precision), fmtPrec(cp1SvgY, o.Precision),
					fmtPrec(cp2SvgX, o.Precision), fmtPrec(cp2SvgY, o.Precision),
					fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			} else if len(cps) == 1 {
				// Degree 2, 1 control point → quadratic Bezier.
				cpAbsX := cps[0].x*ss.localW + ss.offsetX
				cpAbsY := cps[0].y*ss.localH + ss.offsetY
				cpSvgX, cpSvgY := toSVGCoords(cpAbsX, cpAbsY, parentH, scaleX, scaleY)
				d.WriteString(fmt.Sprintf("Q%s %s %s %s",
					fmtPrec(cpSvgX, o.Precision), fmtPrec(cpSvgY, o.Precision),
					fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			} else {
				// Fallback: straight line to endpoint.
				d.WriteString(fmt.Sprintf("L%s %s", fmtPrec(svgX, o.Precision), fmtPrec(svgY, o.Precision)))
			}
			prevX, prevY = svgX, svgY

		default:
			// Unknown row type: skip
		}
	}

	pathData := d.String()
	if pathData == "" {
		return "", 0
	}

	// Check if path should be closed.
	noFill := geomCellBool(geom, "NoFill")
	noLine := geomCellBool(geom, "NoLine")
	noShow := geomCellBool(geom, "NoShow")
	if noShow {
		return "", 0
	}

	// Build style attributes.
	fill := resolveColor(s.FillColor())
	stroke := resolveColor(s.LineColor())
	lineWeight := s.LineWeight()
	opacity := ""

	// Visio default: when FillForegnd is unresolvable but the shape has
	// an explicit FillPattern (solid) or FillForegndTrans, apply the Visio
	// default color index 0 (black) — the shape intends to be filled.
	fillPattern := s.CellValue("FillPattern")
	if fill == "" {
		if fillPattern == "1" || s.CellValue("FillForegndTrans") != "" {
			fill = "#000000"
		}
	}

	// FillPattern=0 means "no fill" regardless of FillForegnd value.
	if noFill || fill == "" || fillPattern == "0" {
		fill = "none"
	}

	// LinePattern=0 means "no line" regardless of other line properties.
	// Empty LineColor means no explicit line was set — don't invent one.
	linePattern := s.CellValue("LinePattern")
	if noLine || linePattern == "0" || stroke == "" {
		stroke = "none"
	}

	// Track colors for brand detection.
	if fill != "none" && fill != "" {
		colorFreq[strings.ToUpper(fill)]++
	}
	if stroke != "none" && stroke != "" {
		colorFreq[strings.ToUpper(stroke)]++
	}

	// Scale line weight.
	strokeWidth := lineWeight * ((scaleX + scaleY) / 2)
	if strokeWidth <= 0 && stroke != "none" {
		strokeWidth = 1
	}

	// Fill opacity.
	fillTrans := s.CellValue("FillForegndTrans")
	if fillTrans != "" {
		transVal := toFloat(fillTrans)
		if transVal > 0 {
			opacity = fmt.Sprintf(` fill-opacity="%s"`, fmtPrec(1-transVal, o.Precision))
		}
	}

	return fmt.Sprintf(`  <path d="%s" fill="%s" stroke="%s" stroke-width="%s"%s/>`,
		pathData, fill, stroke, fmtPrec(strokeWidth, o.Precision), opacity), strokeWidth
}

// toSVGCoords transforms Visio coordinates (inches, Y-up, bottom-left origin)
// to SVG coordinates (pixels, Y-down, top-left origin).
func toSVGCoords(visioX, visioY, parentHeight, scaleX, scaleY float64) (float64, float64) {
	// Flip Y: SVG Y = (parentHeight - visioY) * scale
	svgX := visioX * scaleX
	svgY := (parentHeight - visioY) * scaleY
	return svgX, svgY
}

// arcToSVG converts a Visio ArcTo (with bow) to an SVG arc command.
// bow is the distance from the chord midpoint to the arc, scaled to SVG units.
// Positive bow = arc bows to the left of the direction of travel.
func arcToSVG(x1, y1, x2, y2, bow float64, precision int) string {
	if math.Abs(bow) < 1e-6 {
		// Zero bow = straight line
		return fmt.Sprintf("L%s %s", fmtPrec(x2, precision), fmtPrec(y2, precision))
	}

	dx := x2 - x1
	dy := y2 - y1
	chord := math.Sqrt(dx*dx + dy*dy)
	if chord < 1e-10 {
		return ""
	}

	// radius = (chord²/4 + bow²) / (2 * |bow|)
	absBow := math.Abs(bow)
	radius := (chord*chord/4 + bow*bow) / (2 * absBow)

	// Large arc: if the arc subtends more than 180°
	largeArc := 0
	if absBow > radius {
		largeArc = 1
	}

	// Sweep direction: bow > 0 means left of direction = counterclockwise in Visio (Y-up),
	// which maps to clockwise in SVG (Y-down), so sweep=1.
	// After Y-flip, the bow sign is effectively inverted.
	sweep := 0
	if bow < 0 {
		sweep = 1
	}

	return fmt.Sprintf("A%s %s 0 %d %d %s %s",
		fmtPrec(radius, precision), fmtPrec(radius, precision),
		largeArc, sweep,
		fmtPrec(x2, precision), fmtPrec(y2, precision))
}

// sortGeometryRows returns geometry rows sorted by their IX attribute.
func sortGeometryRows(rows map[string]*GeometryRow) []*GeometryRow {
	sorted := make([]*GeometryRow, 0, len(rows))
	for _, r := range rows {
		sorted = append(sorted, r)
	}
	sort.Slice(sorted, func(i, j int) bool {
		a, _ := parseIX(sorted[i].Index())
		b, _ := parseIX(sorted[j].Index())
		return a < b
	})
	return sorted
}

func parseIX(s string) (int, bool) {
	v := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			v = v*10 + int(c-'0')
		} else {
			return v, false
		}
	}
	return v, len(s) > 0
}

// cellFloat returns the float64 value of a named cell in a geometry row.
func cellFloat(row *GeometryRow, name string) float64 {
	if c, ok := row.Cells[name]; ok {
		return toFloat(c.Value())
	}
	return 0
}

// geomCellBool returns true if a geometry-level cell has value "1".
func geomCellBool(geom *Geometry, name string) bool {
	for _, c := range geom.Cells {
		if c.Name() == name && c.Value() == "1" {
			return true
		}
	}
	return false
}

// resolveColor normalizes a Visio color to #RRGGBB format.
// Visio stores colors as "#RRGGBB", "RGB(r,g,b)", theme index, or empty.
func resolveColor(c string) string {
	if c == "" {
		return ""
	}
	c = strings.TrimSpace(c)

	// Already hex
	if strings.HasPrefix(c, "#") {
		// Normalize to uppercase
		return strings.ToUpper(c)
	}

	// RGB(r,g,b) format
	if strings.HasPrefix(strings.ToUpper(c), "RGB(") && strings.HasSuffix(c, ")") {
		inner := c[4 : len(c)-1]
		parts := strings.Split(inner, ",")
		if len(parts) == 3 {
			r := int(toFloat(strings.TrimSpace(parts[0])))
			g := int(toFloat(strings.TrimSpace(parts[1])))
			b := int(toFloat(strings.TrimSpace(parts[2])))
			return fmt.Sprintf("#%02X%02X%02X", clamp(r), clamp(g), clamp(b))
		}
	}

	// Visio indexed color (integer) — map to standard Visio color palette.
	if len(c) > 0 && c[0] >= '0' && c[0] <= '9' {
		idx := int(toFloat(c))
		if idx >= 0 && idx < len(visioColorPalette) {
			return visioColorPalette[idx]
		}
		return ""
	}

	// Visio theme reference (e.g., "Themed") — cannot resolve without theme data
	return ""
}

// visioColorPalette maps Visio indexed colors (0-23) to hex values.
// Defined by the MS-VSDX spec §2.2.5.3.3.1 default color table.
var visioColorPalette = []string{
	"#000000", // 0  Black
	"#FFFFFF", // 1  White
	"#FF0000", // 2  Red
	"#00FF00", // 3  Green
	"#0000FF", // 4  Blue
	"#FFFF00", // 5  Yellow
	"#FF00FF", // 6  Magenta
	"#00FFFF", // 7  Cyan
	"#800000", // 8  Dark Red
	"#008000", // 9  Dark Green
	"#000080", // 10 Dark Blue
	"#808000", // 11 Dark Yellow (Olive)
	"#800080", // 12 Purple
	"#008080", // 13 Teal
	"#C0C0C0", // 14 Silver (Light Gray)
	"#E0E0E0", // 15 Ice Blue (Lighter Gray)
	"#8080FF", // 16 Light Blue
	"#FF80FF", // 17 Light Magenta
	"#80FFFF", // 18 Light Cyan
	"#FFFF80", // 19 Light Yellow
	"#0000A0", // 20 Dark Blue 2
	"#FF8000", // 21 Orange
	"#80FF00", // 22 Chartreuse
	"#00FF80", // 23 Spring Green
}

func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// detectBrandColor finds the most frequent non-white, non-black color.
func detectBrandColor(freq map[string]int) string {
	var best string
	var bestCount int
	for color, count := range freq {
		upper := strings.ToUpper(color)
		// Skip white/near-white and black/near-black
		if upper == "#FFFFFF" || upper == "#000000" || upper == "#FFF" || upper == "#000" {
			continue
		}
		if count > bestCount {
			bestCount = count
			best = color
		}
	}
	return best
}

// nurbsControlPoint is an interior control point parsed from a NURBS formula.
type nurbsControlPoint struct {
	x, y float64
}

// parseNURBSControlPoints extracts interior control points from a NURBS() formula string.
// Format: NURBS(knotLast, degree, xType, yType, x1,y1,knot1,weight1, x2,y2,knot2,weight2, ...)
// Returns the (x,y) pairs of interior control points.
func parseNURBSControlPoints(formula string) []nurbsControlPoint {
	// Strip "NURBS(" prefix and ")" suffix.
	formula = strings.TrimSpace(formula)
	upper := strings.ToUpper(formula)
	if !strings.HasPrefix(upper, "NURBS(") || !strings.HasSuffix(formula, ")") {
		return nil
	}
	inner := formula[6 : len(formula)-1]

	// Split by comma and trim spaces.
	parts := strings.Split(inner, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// Need at least 4 header values (knotLast, degree, xType, yType) + 4 per control point.
	if len(parts) < 8 {
		return nil
	}

	// Control points start at index 4, in groups of 4: x, y, knot, weight.
	var cps []nurbsControlPoint
	for i := 4; i+3 < len(parts); i += 4 {
		x := toFloat(parts[i])
		y := toFloat(parts[i+1])
		cps = append(cps, nurbsControlPoint{x: x, y: y})
	}
	return cps
}

// cellString returns the string value of a named cell in a geometry row.
func cellString(row *GeometryRow, name string) string {
	if c, ok := row.Cells[name]; ok {
		return c.Value()
	}
	// Also check the F (formula) attribute — some cells store NURBS in F, not V.
	if c, ok := row.Cells[name]; ok {
		if f := c.Formula(); f != "" {
			return f
		}
	}
	return ""
}

// fmtPrec formats a float with the given number of decimal places,
// stripping trailing zeros for cleaner SVG output.
func fmtPrec(v float64, prec int) string {
	s := fmt.Sprintf("%.*f", prec, v)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

// ForeignImage contains embedded image data extracted from a Foreign-type shape.
type ForeignImage struct {
	Data        []byte  // raw image bytes (EMF, PNG, JPEG, etc.)
	ForeignType string  // "EnhMetaFile", "Bitmap", "MetaFile"
	Compression string  // "PNG", "JPEG", "" (uncompressed)
	Extension   string  // file extension: "emf", "png", "jpeg", "wmf"
	Width       float64 // image width in inches (from ImgWidth cell)
	Height      float64 // image height in inches (from ImgHeight cell)
}

// ForeignImageData extracts the embedded image data from a Foreign-type shape.
// Returns nil if the shape has no ForeignData element or if the image cannot be resolved.
//
// Cisco Visio stencils store device icons as embedded EMF (Enhanced MetaFile) images.
// The resolution chain is: Shape ForeignData → Rel rId → page/master rels → media path → zip bytes.
func (s *Shape) ForeignImageData() *ForeignImage {
	fd := s.xml.FindElement("ForeignData")
	if fd == nil {
		return nil
	}

	foreignType := fd.SelectAttrValue("ForeignType", "")
	compression := fd.SelectAttrValue("CompressionType", "")

	// Resolve the relationship ID to a media path.
	relElem := fd.FindElement("Rel")
	if relElem == nil {
		return nil
	}
	relID := relElem.SelectAttrValue("id", "")
	if relID == "" {
		return nil
	}

	// Look up the target path in the page/master rels.
	page := s.Page
	if page == nil || page.RelsXML == nil {
		return nil
	}

	var targetPath string
	for _, rel := range page.RelsXML.Root().SelectElements("Relationship") {
		if rel.SelectAttrValue("Id", "") == relID {
			targetPath = rel.SelectAttrValue("Target", "")
			break
		}
	}
	if targetPath == "" {
		return nil
	}

	// Resolve relative path (e.g., "../media/image1.emf") to ZIP path.
	mediaPath := resolveMediaPath(page.filename, targetPath)

	// Look up in zip contents.
	vis := page.vis
	if vis == nil {
		return nil
	}
	data, ok := vis.ZipFileContents[mediaPath]
	if !ok {
		return nil
	}

	// Determine file extension.
	ext := extensionFromPath(mediaPath)
	if ext == "" {
		ext = extensionFromForeignType(foreignType, compression)
	}

	return &ForeignImage{
		Data:        data,
		ForeignType: foreignType,
		Compression: compression,
		Extension:   ext,
		Width:       toFloat(s.CellValue("ImgWidth")),
		Height:      toFloat(s.CellValue("ImgHeight")),
	}
}

// resolveMediaPath resolves a relative target path against the page's directory.
// e.g., page="visio/masters/master1.xml", target="../media/image1.emf" → "visio/media/image1.emf"
func resolveMediaPath(pagePath, target string) string {
	if !strings.Contains(target, "..") {
		// Absolute or same-directory path.
		dir := pagePath[:strings.LastIndex(pagePath, "/")+1]
		return dir + target
	}

	// Split and resolve ".." segments.
	base := strings.Split(pagePath[:strings.LastIndex(pagePath, "/")], "/")
	parts := strings.Split(target, "/")

	for _, p := range parts {
		if p == ".." {
			if len(base) > 0 {
				base = base[:len(base)-1]
			}
		} else if p != "." && p != "" {
			base = append(base, p)
		}
	}
	return strings.Join(base, "/")
}

// extensionFromPath extracts the file extension from a path.
func extensionFromPath(path string) string {
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		return strings.ToLower(path[idx+1:])
	}
	return ""
}

// extensionFromForeignType maps ForeignType + CompressionType to a file extension.
func extensionFromForeignType(foreignType, compression string) string {
	switch strings.ToLower(foreignType) {
	case "enhmetafile":
		return "emf"
	case "metafile":
		return "wmf"
	case "bitmap":
		switch strings.ToLower(compression) {
		case "png":
			return "png"
		case "jpeg":
			return "jpeg"
		default:
			return "bmp"
		}
	}
	return ""
}

// ConvertEMFToSVG converts EMF (Enhanced MetaFile) bytes to SVG using emf2svg-conv.
// Requires the emf2svg package to be installed (Debian: apt install emf2svg).
// Returns clean SVG bytes suitable for web display.
func ConvertEMFToSVG(emfData []byte) ([]byte, error) {
	cmd := exec.Command("emf2svg-conv", "-i", "/dev/stdin", "-o", "/dev/stdout")
	cmd.Stdin = bytes.NewReader(emfData)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("emf2svg-conv: %w (stderr: %s)", err, stderr.String())
	}

	svgBytes := stdout.Bytes()
	if len(svgBytes) == 0 {
		return nil, fmt.Errorf("emf2svg-conv produced empty output")
	}

	return svgBytes, nil
}

// MasterToSVG converts a master page's primary shape to SVG.
// It handles both geometry-based shapes and Foreign (EMF) shapes.
//
// For geometry shapes: uses ShapeToSVG to convert Visio geometry to SVG paths.
// For EMF shapes: extracts the embedded EMF and converts via emf2svg-conv.
//
// Returns the SVG result with brand color metadata.
func MasterToSVG(master *Page, opts ...SVGOption) (*SVGResult, error) {
	shapes := master.ChildShapes()
	if len(shapes) == 0 {
		return nil, fmt.Errorf("master %q has no shapes", master.Name())
	}

	shape := shapes[0]

	// Try geometry-based conversion first (works for shapes with real path data).
	if shape.ShapeType != "Foreign" {
		return ShapeToSVG(shape, opts...)
	}

	// Foreign shape — extract embedded image.
	img := shape.ForeignImageData()
	if img == nil {
		// Fall back to geometry (some Foreign shapes also have geometry).
		return ShapeToSVG(shape, opts...)
	}

	switch img.Extension {
	case "emf":
		svgBytes, err := ConvertEMFToSVG(img.Data)
		if err != nil {
			return nil, fmt.Errorf("converting EMF for %q: %w", master.Name(), err)
		}
		// Detect brand color from the generated SVG.
		brandColor := detectSVGBrandColor(string(svgBytes))
		return &SVGResult{SVG: svgBytes, BrandColor: brandColor}, nil

	case "svg":
		// Already SVG — return as-is.
		return &SVGResult{SVG: img.Data, BrandColor: detectSVGBrandColor(string(img.Data))}, nil

	default:
		return nil, fmt.Errorf("unsupported foreign image type %q for %q (need EMF or SVG)", img.Extension, master.Name())
	}
}

// detectSVGBrandColor scans SVG markup for the most frequent non-B&W fill/stroke color.
func detectSVGBrandColor(svg string) string {
	freq := make(map[string]int)

	// Simple scan for fill="..." and stroke="..." hex colors.
	for _, attr := range []string{"fill=", "stroke="} {
		idx := 0
		for {
			pos := strings.Index(svg[idx:], attr)
			if pos < 0 {
				break
			}
			pos += idx + len(attr)
			if pos < len(svg) && svg[pos] == '"' {
				end := strings.Index(svg[pos+1:], "\"")
				if end > 0 {
					color := svg[pos+1 : pos+1+end]
					if strings.HasPrefix(color, "#") && (len(color) == 7 || len(color) == 4) {
						freq[strings.ToUpper(color)]++
					}
				}
			}
			idx = pos + 1
		}
	}

	return detectBrandColor(freq)
}
