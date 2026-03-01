package vsdx

import "github.com/beevik/etree"

// Cell represents a Cell element in a vsdx XML file.
// A Cell has a name (N attribute), value (V attribute), and optional formula (F attribute).
type Cell struct {
	xml   *etree.Element
	shape *Shape
}

func newCell(xml *etree.Element, shape *Shape) *Cell {
	return &Cell{xml: xml, shape: shape}
}

// Value returns the V attribute of the Cell.
func (c *Cell) Value() string {
	return c.xml.SelectAttrValue("V", "")
}

// SetValue sets the V attribute of the Cell.
func (c *Cell) SetValue(v string) {
	c.xml.CreateAttr("V", v)
}

// Formula returns the F attribute of the Cell.
func (c *Cell) Formula() string {
	return c.xml.SelectAttrValue("F", "")
}

// SetFormula sets the F attribute of the Cell.
func (c *Cell) SetFormula(f string) {
	c.xml.CreateAttr("F", f)
}

// Name returns the N attribute of the Cell.
func (c *Cell) Name() string {
	return c.xml.SelectAttrValue("N", "")
}

func (c *Cell) String() string {
	return "Cell: name=" + c.Name() + " val=" + c.Value() + " func=" + c.Formula()
}
