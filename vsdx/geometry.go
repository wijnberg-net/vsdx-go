package vsdx

import (
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
		for _, cell := range masterShape.Geometry.Cells {
			g.Cells = append(g.Cells, cell)
		}
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

// GeometryCell represents a Cell element within a Geometry section or row.
type GeometryCell struct {
	parent interface{} // *Geometry or *GeometryRow
	xml    *etree.Element
}

func newGeometryCell(parent interface{}, xml *etree.Element) *GeometryCell {
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
