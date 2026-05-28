package vsdx

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// Geometry represents the geometry section of a shape, defining its path.
type Geometry struct {
	xml   *etree.Element
	Cells []*GeometryCell
	Rows  map[string]*GeometryRow // indexed by IX attribute
	shape *Shape
}

// newGeometry creates a Geometry from an XML Section element with N="Geometry".
// geomIndex specifies which geometry section this is (IX attribute), used to inherit from
// the correct master geometry section.
func newGeometry(xml *etree.Element, shape *Shape, geomIndex int) *Geometry {
	g := &Geometry{
		xml:   xml,
		Cells: make([]*GeometryCell, 0),
		Rows:  make(map[string]*GeometryRow),
		shape: shape,
	}

	// Find the corresponding master geometry by index
	var masterGeom *Geometry
	masterShape := shape.MasterShape()
	if masterShape != nil && geomIndex >= 0 && geomIndex < len(masterShape.Geometries) {
		masterGeom = masterShape.Geometries[geomIndex]
	}

	// Inherit cells from master shape geometry (using correct geometry section)
	if masterGeom != nil {
		// Copy master cells (not share reference)
		g.Cells = append(g.Cells, masterGeom.Cells...)
	}

	// Add/overwrite with local cells
	for _, cellElem := range xml.SelectElements("Cell") {
		g.Cells = append(g.Cells, newGeometryCell(g, cellElem))
	}

	// Inherit rows from master shape geometry (using correct geometry section)
	if masterGeom != nil {
		for k, v := range masterGeom.Rows {
			g.Rows[k] = v
		}
	}

	// Add/overwrite with local rows
	for _, rowElem := range xml.SelectElements("Row") {
		index := rowElem.SelectAttrValue("IX", "")
		masterRow := g.Rows[index]
		gRow := newGeometryRow(g, rowElem, masterRow)
		g.Rows[gRow.Index()] = gRow
		if gRow.DelBool() {
			delete(g.Rows, gRow.Index())
		}
	}

	return g
}

// StartPoint returns the start position of the geometry as a Point.
func (g *Geometry) StartPoint() Point {
	x, y := g.StartPos()
	return Point{X: x, Y: y}
}

// StartPos returns the start position of the geometry based on the first MoveTo or RelMoveTo row.
func (g *Geometry) StartPos() (float64, float64) {
	for _, row := range g.Rows {
		rt := row.RowType()
		if rt == "MoveTo" || rt == "moveto" {
			return row.X(), row.Y()
		}
		if rt == "RelMoveTo" || rt == "relmoveto" {
			return g.shape.X(), g.shape.Y()
		}
	}
	return 0, 0
}

// Move updates absolute coordinate references in the geometry by the given deltas.
func (g *Geometry) Move(xDelta, yDelta float64) {
	for _, r := range g.Rows {
		rt := strings.ToLower(r.RowType())
		if rt == "moveto" || rt == "lineto" {
			if cell := r.Cells["X"]; cell != nil {
				v := toFloat(cell.Value())
				cell.SetValue(fmtFloat(v + xDelta))
			}
			if cell := r.Cells["Y"]; cell != nil {
				v := toFloat(cell.Value())
				cell.SetValue(fmtFloat(v + yDelta))
			}
		}
	}
}

// setRowCoords sets the X, Y coordinates of the nth row of the given type.
//
// Only LOCAL rows are considered (rows whose XML element lives inside this
// geometry section). Rows inherited from a master share their XML element
// with the master shape, and calling SetX/SetY on them would mutate the
// master geometry — not what callers want. Equally important: iteration is
// sorted by IX numerically so picking "index 0" is deterministic, instead
// of depending on Go's randomised map iteration order.
func (g *Geometry) setRowCoords(rowType string, x, y float64, index int) {
	type indexed struct {
		ix int
		r  *GeometryRow
	}
	var matching []indexed
	for ixStr, r := range g.Rows {
		if strings.ToLower(r.RowType()) != rowType {
			continue
		}
		if r.xml.Parent() != g.xml {
			continue // inherited from master, skip
		}
		ix, _ := strconv.Atoi(ixStr)
		matching = append(matching, indexed{ix, r})
	}
	sort.Slice(matching, func(i, j int) bool { return matching[i].ix < matching[j].ix })
	if index < len(matching) {
		matching[index].r.SetX(x)
		matching[index].r.SetY(y)
	}
}

// SetMoveTo sets the coordinates of a MoveTo row at the given index (0-based).
func (g *Geometry) SetMoveTo(x, y float64, moveToIndex int) {
	g.setRowCoords("moveto", x, y, moveToIndex)
}

// SetLineTo sets the coordinates of a LineTo row at the given index (0-based).
func (g *Geometry) SetLineTo(x, y float64, lineToIndex int) {
	g.setRowCoords("lineto", x, y, lineToIndex)
}

// nextIX returns the next available IX value for a new row.
func (g *Geometry) nextIX() string {
	maxIX := 0
	for k := range g.Rows {
		if ix, err := strconv.Atoi(k); err == nil && ix > maxIX {
			maxIX = ix
		}
	}
	return strconv.Itoa(maxIX + 1)
}

// addRow creates a new geometry row with X and Y cells.
func (g *Geometry) addRow(rowType string, x, y float64) {
	ix := g.nextIX()
	addGeoRowXML(g.xml, rowType, ix, fmtFloat(x), fmtFloat(y))
	rowElem := g.xml.SelectElements("Row")
	last := rowElem[len(rowElem)-1]
	g.Rows[ix] = newGeometryRow(g, last, nil)
}

// AddMoveTo adds a MoveTo row with absolute coordinates.
func (g *Geometry) AddMoveTo(x, y float64) { g.addRow("MoveTo", x, y) }

// AddLineTo adds a LineTo row with absolute coordinates.
func (g *Geometry) AddLineTo(x, y float64) { g.addRow("LineTo", x, y) }

// AddRelMoveTo adds a RelMoveTo row with coordinates relative to shape bounds (0-1 range).
func (g *Geometry) AddRelMoveTo(x, y float64) { g.addRow("RelMoveTo", x, y) }

// AddRelLineTo adds a RelLineTo row with coordinates relative to shape bounds (0-1 range).
func (g *Geometry) AddRelLineTo(x, y float64) { g.addRow("RelLineTo", x, y) }

// AddArcTo adds an ArcTo row with absolute coordinates and bow (arc bulge).
func (g *Geometry) AddArcTo(x, y, bow float64) {
	ix := g.nextIX()
	row := g.xml.CreateElement("Row")
	row.CreateAttr("T", "ArcTo")
	row.CreateAttr("IX", ix)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "A", fmtFloat(bow), "")
	g.Rows[ix] = newGeometryRow(g, row, nil)
}

// AddEllipse adds an Ellipse row defining an ellipse by its center and two control points.
// (x, y) is the center, (a, b) is a point on the ellipse along the major axis,
// (c, d) is a point on the ellipse along the minor axis.
func (g *Geometry) AddEllipse(x, y, a, b, c, d float64) {
	ix := g.nextIX()
	row := g.xml.CreateElement("Row")
	row.CreateAttr("T", "Ellipse")
	row.CreateAttr("IX", ix)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "A", fmtFloat(a), "")
	addCellXML(row, "B", fmtFloat(b), "")
	addCellXML(row, "C", fmtFloat(c), "")
	addCellXML(row, "D", fmtFloat(d), "")
	g.Rows[ix] = newGeometryRow(g, row, nil)
}

// AddEllipticalArcTo adds an EllipticalArcTo row.
// (x, y) is the endpoint, a is the control point X, b is the control point Y,
// c is the major/minor ratio, d is the angle of the major axis.
func (g *Geometry) AddEllipticalArcTo(x, y, a, b, c, d float64) {
	ix := g.nextIX()
	row := g.xml.CreateElement("Row")
	row.CreateAttr("T", "EllipticalArcTo")
	row.CreateAttr("IX", ix)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "A", fmtFloat(a), "")
	addCellXML(row, "B", fmtFloat(b), "")
	addCellXML(row, "C", fmtFloat(c), "")
	addCellXML(row, "D", fmtFloat(d), "")
	g.Rows[ix] = newGeometryRow(g, row, nil)
}

// AddRelEllipticalArcTo adds a RelEllipticalArcTo row with relative coordinates (0-1 range).
// (x, y) is the endpoint, a is the control point X, b is the control point Y,
// c is the major/minor ratio, d is the angle of the major axis.
func (g *Geometry) AddRelEllipticalArcTo(x, y, a, b, c, d float64) {
	ix := g.nextIX()
	row := g.xml.CreateElement("Row")
	row.CreateAttr("T", "RelEllipticalArcTo")
	row.CreateAttr("IX", ix)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "A", fmtFloat(a), "")
	addCellXML(row, "B", fmtFloat(b), "")
	addCellXML(row, "C", fmtFloat(c), "")
	addCellXML(row, "D", fmtFloat(d), "")
	g.Rows[ix] = newGeometryRow(g, row, nil)
}

// AddRelCubBezTo adds a RelCubBezTo row for a relative cubic Bezier curve.
// (x, y) is the endpoint, (a, b) is the first control point, (c, d) is the second control point.
// All coordinates are relative to shape bounds (0-1 range).
func (g *Geometry) AddRelCubBezTo(x, y, a, b, c, d float64) {
	ix := g.nextIX()
	row := g.xml.CreateElement("Row")
	row.CreateAttr("T", "RelCubBezTo")
	row.CreateAttr("IX", ix)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "A", fmtFloat(a), "")
	addCellXML(row, "B", fmtFloat(b), "")
	addCellXML(row, "C", fmtFloat(c), "")
	addCellXML(row, "D", fmtFloat(d), "")
	g.Rows[ix] = newGeometryRow(g, row, nil)
}

// AddRelQuadBezTo adds a RelQuadBezTo row for a relative quadratic Bezier curve.
// (x, y) is the endpoint, (a, b) is the control point.
// All coordinates are relative to shape bounds (0-1 range).
func (g *Geometry) AddRelQuadBezTo(x, y, a, b float64) {
	ix := g.nextIX()
	row := g.xml.CreateElement("Row")
	row.CreateAttr("T", "RelQuadBezTo")
	row.CreateAttr("IX", ix)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "A", fmtFloat(a), "")
	addCellXML(row, "B", fmtFloat(b), "")
	g.Rows[ix] = newGeometryRow(g, row, nil)
}

// AddNURBSTo adds a NURBSTo row for a NURBS curve segment.
// (x, y) is the endpoint, a is the second-to-last knot, b is the last weight,
// c is the first knot, d is the first weight, e is the NURBS formula string.
func (g *Geometry) AddNURBSTo(x, y, a, b, c, d float64, e string) {
	ix := g.nextIX()
	row := g.xml.CreateElement("Row")
	row.CreateAttr("T", "NURBSTo")
	row.CreateAttr("IX", ix)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "A", fmtFloat(a), "")
	addCellXML(row, "B", fmtFloat(b), "")
	addCellXML(row, "C", fmtFloat(c), "")
	addCellXML(row, "D", fmtFloat(d), "")
	addCellXML(row, "E", e, "")
	g.Rows[ix] = newGeometryRow(g, row, nil)
}

// AddPolylineTo adds a PolylineTo row for a polyline defined by a formula string.
// (x, y) is the endpoint, a is the polyline formula (e.g., "POLYLINE(...)").
func (g *Geometry) AddPolylineTo(x, y float64, a string) {
	ix := g.nextIX()
	row := g.xml.CreateElement("Row")
	row.CreateAttr("T", "PolylineTo")
	row.CreateAttr("IX", ix)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "A", a, "")
	g.Rows[ix] = newGeometryRow(g, row, nil)
}

// AddSplineStart adds a SplineStart row that begins a spline.
// (x, y) is the second control point, a is the second knot,
// b is the first knot, c is the last knot, d is the degree.
func (g *Geometry) AddSplineStart(x, y, a, b, c float64, d int) {
	ix := g.nextIX()
	row := g.xml.CreateElement("Row")
	row.CreateAttr("T", "SplineStart")
	row.CreateAttr("IX", ix)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "A", fmtFloat(a), "")
	addCellXML(row, "B", fmtFloat(b), "")
	addCellXML(row, "C", fmtFloat(c), "")
	addCellXML(row, "D", strconv.Itoa(d), "")
	g.Rows[ix] = newGeometryRow(g, row, nil)
}

// AddSplineKnot adds a SplineKnot row that continues a spline.
// (x, y) is the control point, a is the knot value.
func (g *Geometry) AddSplineKnot(x, y, a float64) {
	ix := g.nextIX()
	row := g.xml.CreateElement("Row")
	row.CreateAttr("T", "SplineKnot")
	row.CreateAttr("IX", ix)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "A", fmtFloat(a), "")
	g.Rows[ix] = newGeometryRow(g, row, nil)
}

// AddInfiniteLine adds an InfiniteLine row that defines an infinite line.
// (x, y) is a point on the line, (a, b) is a second point on the line.
func (g *Geometry) AddInfiniteLine(x, y, a, b float64) {
	ix := g.nextIX()
	row := g.xml.CreateElement("Row")
	row.CreateAttr("T", "InfiniteLine")
	row.CreateAttr("IX", ix)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "A", fmtFloat(a), "")
	addCellXML(row, "B", fmtFloat(b), "")
	g.Rows[ix] = newGeometryRow(g, row, nil)
}

// GeometryRow represents a row within a Geometry section.
// Each row has a type (T attribute) and index (IX attribute), and contains Cell elements.
type GeometryRow struct {
	geometry *Geometry
	xml      *etree.Element
	Cells    map[string]*GeometryCell
}

func newGeometryRow(geometry *Geometry, xml *etree.Element, masterRow *GeometryRow) *GeometryRow {
	r := &GeometryRow{
		geometry: geometry,
		xml:      xml,
		Cells:    make(map[string]*GeometryCell),
	}

	// Inherit cells from master row
	if masterRow != nil {
		for k, v := range masterRow.Cells {
			r.Cells[k] = v
		}
	}

	// Add/overwrite with local cells
	for _, cellElem := range xml.SelectElements("Cell") {
		gCell := newGeometryCell(r, cellElem)
		r.Cells[gCell.Name()] = gCell
	}

	return r
}

// RowType returns the T attribute of the row (e.g., "MoveTo", "LineTo", "RelMoveTo").
func (r *GeometryRow) RowType() string {
	return r.xml.SelectAttrValue("T", "")
}

// Index returns the IX attribute of the row.
func (r *GeometryRow) Index() string {
	return r.xml.SelectAttrValue("IX", "")
}

// X returns the X cell value as float64, or 0.0 if not present.
func (r *GeometryRow) X() float64 {
	if cell := r.Cells["X"]; cell != nil {
		return toFloat(cell.Value())
	}
	return 0
}

// Y returns the Y cell value as float64, or 0.0 if not present.
func (r *GeometryRow) Y() float64 {
	if cell := r.Cells["Y"]; cell != nil {
		return toFloat(cell.Value())
	}
	return 0
}

// DelBool returns true if the row is marked as deleted (Del attribute).
func (r *GeometryRow) DelBool() bool {
	return r.xml.SelectAttrValue("Del", "") != ""
}

// --- GeometryRow setters ---

// SetRowType sets the T attribute of the row.
func (r *GeometryRow) SetRowType(v string) {
	r.xml.CreateAttr("T", v)
}

// SetIndex sets the IX attribute of the row.
func (r *GeometryRow) SetIndex(v string) {
	r.xml.CreateAttr("IX", v)
}

// SetX sets the X cell value, creating it if needed.
func (r *GeometryRow) SetX(v float64) {
	xCell := r.Cells["X"]
	if xCell == nil {
		cellElem := etree.NewElement("Cell")
		cellElem.CreateAttr("N", "X")
		r.xml.AddChild(cellElem)
		xCell = newGeometryCell(r, cellElem)
		r.Cells["X"] = xCell
	}
	xCell.SetValue(fmtFloat(v))
}

// SetY sets the Y cell value, creating it if needed.
func (r *GeometryRow) SetY(v float64) {
	yCell := r.Cells["Y"]
	if yCell == nil {
		cellElem := etree.NewElement("Cell")
		cellElem.CreateAttr("N", "Y")
		r.xml.AddChild(cellElem)
		yCell = newGeometryCell(r, cellElem)
		r.Cells["Y"] = yCell
	}
	yCell.SetValue(fmtFloat(v))
}

// SetDelBool sets or removes the Del attribute on the row.
func (r *GeometryRow) SetDelBool(v bool) {
	if v {
		r.xml.CreateAttr("Del", "1")
	} else {
		r.xml.RemoveAttr("Del")
	}
}

// CreateRowXML creates a new Row XML element and inserts it into the geometry section.
func (r *GeometryRow) CreateRowXML(rowType, ix string) *etree.Element {
	if rowType == "" || ix == "" {
		return nil
	}
	row := etree.NewElement("Row")
	row.CreateAttr("T", rowType)
	row.CreateAttr("IX", ix)

	// Get all existing indexes and insert in sorted order
	var indexes []string
	for _, rowElem := range r.geometry.xml.SelectElements("Row") {
		if idx := rowElem.SelectAttrValue("IX", ""); idx != "" {
			indexes = append(indexes, idx)
		}
	}
	if contains(indexes, ix) {
		return nil // Index already exists
	}
	indexes = append(indexes, ix)
	sort.Strings(indexes)

	// Find insertion position
	pos := 0
	for i, idx := range indexes {
		if idx == ix {
			pos = i
			break
		}
	}
	r.geometry.xml.InsertChildAt(pos, row)
	r.geometry.Rows[ix] = r
	r.xml = row
	return row
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GeometryCellParent is the interface for types that can contain geometry cells.
type GeometryCellParent interface {
	geometryCellParent()
}

func (*Geometry) geometryCellParent()    {}
func (*GeometryRow) geometryCellParent() {}

// GeometryCell represents a Cell element within a Geometry section or row.
type GeometryCell struct {
	parent GeometryCellParent // *Geometry or *GeometryRow
	xml    *etree.Element
}

func newGeometryCell(parent GeometryCellParent, xml *etree.Element) *GeometryCell {
	return &GeometryCell{parent: parent, xml: xml}
}

// Value returns the V attribute value.
func (c *GeometryCell) Value() string {
	return c.xml.SelectAttrValue("V", "")
}

// SetValue sets the V attribute.
func (c *GeometryCell) SetValue(v string) {
	c.xml.CreateAttr("V", v)
}

// Formula returns the F attribute value.
func (c *GeometryCell) Formula() string {
	return c.xml.SelectAttrValue("F", "")
}

// Name returns the N attribute value.
func (c *GeometryCell) Name() string {
	return c.xml.SelectAttrValue("N", "")
}

// Func returns the F attribute value (alias for Formula, as F represents a function/formula).
func (c *GeometryCell) Func() string {
	return c.Formula()
}

// --- GeometryCell setters ---

// SetFormula sets the F attribute.
func (c *GeometryCell) SetFormula(f string) {
	c.xml.CreateAttr("F", f)
}

// SetName sets the N attribute.
func (c *GeometryCell) SetName(n string) {
	c.xml.CreateAttr("N", n)
}

// CreateCellXML creates a new Cell XML element and adds it to the parent row.
func (c *GeometryCell) CreateCellXML(name string) *etree.Element {
	cell := etree.NewElement("Cell")
	cell.CreateAttr("N", name)
	if parentRow, ok := c.parent.(*GeometryRow); ok {
		parentRow.xml.AddChild(cell)
		parentRow.Cells[name] = c
	}
	c.xml = cell
	return cell
}

// pathSegment represents a segment in a geometry path for length/position calculations.
type pathSegment struct {
	startX, startY float64 // start point
	endX, endY     float64 // end point
	length         float64 // segment length
	rowType        string  // MoveTo, LineTo, ArcTo, etc.
	bow            float64 // for ArcTo: arc bulge
}

// SortedRows returns the geometry rows sorted by IX (index) attribute numerically.
func (g *Geometry) SortedRows() []*GeometryRow {
	// Collect all indices
	indices := make([]string, 0, len(g.Rows))
	for ix := range g.Rows {
		indices = append(indices, ix)
	}

	// Sort numerically (IX values are integers as strings)
	sort.Slice(indices, func(i, j int) bool {
		ni, _ := strconv.Atoi(indices[i])
		nj, _ := strconv.Atoi(indices[j])
		return ni < nj
	})

	// Return rows in sorted order
	rows := make([]*GeometryRow, 0, len(indices))
	for _, ix := range indices {
		rows = append(rows, g.Rows[ix])
	}
	return rows
}

// buildPathSegments constructs the path segments from geometry rows.
func (g *Geometry) buildPathSegments() []pathSegment {
	rows := g.SortedRows()
	if len(rows) == 0 {
		return nil
	}

	segments := make([]pathSegment, 0, len(rows))
	var curX, curY float64

	// Get shape dimensions for relative coordinates
	var width, height float64 = 1, 1
	if g.shape != nil {
		width = g.shape.Width()
		height = g.shape.Height()
		if width == 0 {
			width = 1
		}
		if height == 0 {
			height = 1
		}
	}

	for _, row := range rows {
		rt := strings.ToLower(row.RowType())
		x, y := row.X(), row.Y()

		switch rt {
		case "moveto":
			curX, curY = x, y
		case "relmoveto":
			curX, curY = x*width, y*height
		case "lineto":
			length := math.Hypot(x-curX, y-curY)
			segments = append(segments, pathSegment{
				startX: curX, startY: curY,
				endX: x, endY: y,
				length:  length,
				rowType: "LineTo",
			})
			curX, curY = x, y
		case "rellineto":
			absX, absY := x*width, y*height
			length := math.Hypot(absX-curX, absY-curY)
			segments = append(segments, pathSegment{
				startX: curX, startY: curY,
				endX: absX, endY: absY,
				length:  length,
				rowType: "RelLineTo",
			})
			curX, curY = absX, absY
		case "arcto":
			// ArcTo has a bow (A cell) that defines arc bulge
			bow := 0.0
			if aCell := row.Cells["A"]; aCell != nil {
				bow = toFloat(aCell.Value())
			}
			// Arc length approximation: chord length * factor based on bow
			chord := math.Hypot(x-curX, y-curY)
			arcLen := chord
			if bow != 0 && chord > 0 {
				// Arc length = 2 * radius * arcsin(chord / (2 * radius))
				// where radius = (chord^2 + 4*bow^2) / (8*|bow|)
				radius := (chord*chord + 4*bow*bow) / (8 * math.Abs(bow))
				if radius > 0 {
					halfChord := chord / 2
					if halfChord <= radius {
						arcLen = 2 * radius * math.Asin(halfChord/radius)
					}
				}
			}
			segments = append(segments, pathSegment{
				startX: curX, startY: curY,
				endX: x, endY: y,
				length:  arcLen,
				rowType: "ArcTo",
				bow:     bow,
			})
			curX, curY = x, y
		case "ellipticalarctoc", "relellipticalarctoc":
			// Simplified: use chord length as approximation
			absX, absY := x, y
			if rt == "relellipticalarctoc" {
				absX, absY = x*width, y*height
			}
			length := math.Hypot(absX-curX, absY-curY)
			segments = append(segments, pathSegment{
				startX: curX, startY: curY,
				endX: absX, endY: absY,
				length:  length,
				rowType: rt,
			})
			curX, curY = absX, absY
		case "relcubbezto", "relquadbezto", "nurbsto", "polylineto", "splinestart", "splineknot":
			// Simplified: use chord length as approximation for curves
			absX, absY := x, y
			if strings.HasPrefix(rt, "rel") {
				absX, absY = x*width, y*height
			}
			length := math.Hypot(absX-curX, absY-curY)
			segments = append(segments, pathSegment{
				startX: curX, startY: curY,
				endX: absX, endY: absY,
				length:  length,
				rowType: rt,
			})
			curX, curY = absX, absY
		}
	}
	return segments
}

// PathLength returns the total length of the geometry path.
func (g *Geometry) PathLength() float64 {
	segments := g.buildPathSegments()
	total := 0.0
	for _, seg := range segments {
		total += seg.length
	}
	return total
}

// PointAlongPath returns the (x, y) coordinates at a given travel fraction (0-1) along the path.
// offset specifies a perpendicular offset from the path.
func (g *Geometry) PointAlongPath(travel, offset float64) (x, y float64) {
	segments := g.buildPathSegments()
	if len(segments) == 0 {
		return 0, 0
	}

	totalLen := 0.0
	for _, seg := range segments {
		totalLen += seg.length
	}
	if totalLen == 0 {
		return segments[0].startX, segments[0].startY
	}

	targetDist := travel * totalLen
	cumDist := 0.0

	for _, seg := range segments {
		if cumDist+seg.length >= targetDist || &seg == &segments[len(segments)-1] {
			// Point is in this segment
			if seg.length == 0 {
				x, y = seg.startX, seg.startY
			} else {
				t := (targetDist - cumDist) / seg.length
				if t < 0 {
					t = 0
				}
				if t > 1 {
					t = 1
				}

				if seg.rowType == "ArcTo" && seg.bow != 0 {
					// Arc interpolation
					x, y = interpolateArc(seg.startX, seg.startY, seg.endX, seg.endY, seg.bow, t)
				} else {
					// Linear interpolation
					x = seg.startX + t*(seg.endX-seg.startX)
					y = seg.startY + t*(seg.endY-seg.startY)
				}
			}

			// Apply perpendicular offset
			if offset != 0 {
				angle := math.Atan2(seg.endY-seg.startY, seg.endX-seg.startX)
				perpAngle := angle + math.Pi/2
				x += offset * math.Cos(perpAngle)
				y += offset * math.Sin(perpAngle)
			}
			return x, y
		}
		cumDist += seg.length
	}

	// Return end point
	last := segments[len(segments)-1]
	return last.endX, last.endY
}

// AngleAlongPath returns the angle (in radians) of the path tangent at a given travel fraction (0-1).
func (g *Geometry) AngleAlongPath(travel float64) float64 {
	segments := g.buildPathSegments()
	if len(segments) == 0 {
		return 0
	}

	totalLen := 0.0
	for _, seg := range segments {
		totalLen += seg.length
	}
	if totalLen == 0 {
		return 0
	}

	targetDist := travel * totalLen
	cumDist := 0.0

	for _, seg := range segments {
		if cumDist+seg.length >= targetDist || &seg == &segments[len(segments)-1] {
			// Angle in this segment
			if seg.rowType == "ArcTo" && seg.bow != 0 {
				// Arc tangent angle at position t
				t := 0.5
				if seg.length > 0 {
					t = (targetDist - cumDist) / seg.length
				}
				return arcTangentAngle(seg.startX, seg.startY, seg.endX, seg.endY, seg.bow, t)
			}
			// Linear segment tangent
			return math.Atan2(seg.endY-seg.startY, seg.endX-seg.startX)
		}
		cumDist += seg.length
	}

	// Return angle of last segment
	last := segments[len(segments)-1]
	return math.Atan2(last.endY-last.startY, last.endX-last.startX)
}

// interpolateArc returns the point at parameter t (0-1) along an arc defined by endpoints and bow.
func interpolateArc(x1, y1, x2, y2, bow, t float64) (float64, float64) {
	if bow == 0 {
		return x1 + t*(x2-x1), y1 + t*(y2-y1)
	}

	// Calculate arc center and radius
	midX, midY := (x1+x2)/2, (y1+y2)/2
	chord := math.Hypot(x2-x1, y2-y1)
	if chord == 0 {
		return x1, y1
	}

	// Perpendicular direction from midpoint
	perpX := -(y2 - y1) / chord
	perpY := (x2 - x1) / chord

	// Sagitta (distance from chord midpoint to arc)
	// bow is the sagitta value
	radius := (chord*chord + 4*bow*bow) / (8 * math.Abs(bow))

	// Center is on perpendicular from midpoint
	dist := radius - math.Abs(bow)
	if bow > 0 {
		dist = -dist
	}
	centerX := midX + dist*perpX
	centerY := midY + dist*perpY

	// Angles from center to start and end points
	startAngle := math.Atan2(y1-centerY, x1-centerX)
	endAngle := math.Atan2(y2-centerY, x2-centerX)

	// Handle angle wrapping for arc direction
	if bow > 0 {
		if endAngle > startAngle {
			endAngle -= 2 * math.Pi
		}
	} else {
		if endAngle < startAngle {
			endAngle += 2 * math.Pi
		}
	}

	// Interpolate angle
	angle := startAngle + t*(endAngle-startAngle)
	return centerX + radius*math.Cos(angle), centerY + radius*math.Sin(angle)
}

// arcTangentAngle returns the tangent angle at parameter t along an arc.
func arcTangentAngle(x1, y1, x2, y2, bow, t float64) float64 {
	if bow == 0 {
		return math.Atan2(y2-y1, x2-x1)
	}

	// Get point slightly before and after t to compute tangent
	dt := 0.001
	t1, t2 := t-dt, t+dt
	if t1 < 0 {
		t1 = 0
	}
	if t2 > 1 {
		t2 = 1
	}

	px1, py1 := interpolateArc(x1, y1, x2, y2, bow, t1)
	px2, py2 := interpolateArc(x1, y1, x2, y2, bow, t2)

	return math.Atan2(py2-py1, px2-px1)
}
