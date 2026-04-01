package vsdx

import (
	"sort"
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
func newGeometry(xml *etree.Element, shape *Shape) *Geometry {
	g := &Geometry{
		xml:   xml,
		Cells: make([]*GeometryCell, 0),
		Rows:  make(map[string]*GeometryRow),
		shape: shape,
	}

	// Inherit cells from master shape geometry
	masterShape := shape.MasterShape()
	if masterShape != nil && masterShape.Geometry != nil {
		// Copy master cells (not share reference)
		g.Cells = append(g.Cells, masterShape.Geometry.Cells...)
	}

	// Add/overwrite with local cells
	for _, cellElem := range xml.SelectElements("Cell") {
		g.Cells = append(g.Cells, newGeometryCell(g, cellElem))
	}

	// Inherit rows from master shape geometry
	if masterShape != nil && masterShape.Geometry != nil {
		for k, v := range masterShape.Geometry.Rows {
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
func (g *Geometry) setRowCoords(rowType string, x, y float64, index int) {
	var matching []*GeometryRow
	for _, r := range g.Rows {
		if strings.ToLower(r.RowType()) == rowType {
			matching = append(matching, r)
		}
	}
	if index < len(matching) {
		matching[index].SetX(x)
		matching[index].SetY(y)
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
