package vsdx

import "github.com/beevik/etree"

// Cell represents a Cell element in a vsdx XML file.
// A Cell has a name (N attribute), value (V attribute), optional formula (F attribute),
// optional unit (U attribute), and optional error (E attribute).
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

// Unit returns the U attribute of the Cell (e.g., "IN", "CM", "PT").
func (c *Cell) Unit() string {
	return c.xml.SelectAttrValue("U", "")
}

// SetUnit sets the U attribute of the Cell.
func (c *Cell) SetUnit(u string) {
	if u == "" {
		c.xml.RemoveAttr("U")
	} else {
		c.xml.CreateAttr("U", u)
	}
}

// Error returns the E attribute of the Cell (error code if formula evaluation failed).
func (c *Cell) Error() string {
	return c.xml.SelectAttrValue("E", "")
}

// SetError sets the E attribute of the Cell.
func (c *Cell) SetError(e string) {
	if e == "" {
		c.xml.RemoveAttr("E")
	} else {
		c.xml.CreateAttr("E", e)
	}
}

// HasError returns true if the cell has an error value.
func (c *Cell) HasError() bool {
	return c.xml.SelectAttrValue("E", "") != ""
}

func (c *Cell) String() string {
	s := "Cell: name=" + c.Name() + " val=" + c.Value() + " func=" + c.Formula()
	if u := c.Unit(); u != "" {
		s += " unit=" + u
	}
	if e := c.Error(); e != "" {
		s += " err=" + e
	}
	return s
}
