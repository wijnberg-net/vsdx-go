package vsdx

import "github.com/beevik/etree"

// Connect represents a connection between two shapes in a Visio page.
type Connect struct {
	xml     *etree.Element
	page    *Page
	FromID  string // FromSheet - ref to the connector shape
	ToID    string // ToSheet - ref to the shape where the connector terminates
	FromRel string // FromCell - e.g., "EndX", "BeginX"
	ToRel   string // ToCell - e.g., "PinX"
}

func newConnect(xml *etree.Element, page *Page) *Connect {
	return &Connect{
		xml:     xml,
		page:    page,
		FromID:  xml.SelectAttrValue("FromSheet", ""),
		ToID:    xml.SelectAttrValue("ToSheet", ""),
		FromRel: xml.SelectAttrValue("FromCell", ""),
		ToRel:   xml.SelectAttrValue("ToCell", ""),
	}
}

// ShapeID returns the ID of the shape where the connector terminates (convenience for ToID).
func (c *Connect) ShapeID() string {
	return c.ToID
}

// ConnectorShapeID returns the ID of the connector shape (convenience for FromID).
func (c *Connect) ConnectorShapeID() string {
	return c.FromID
}

// Shape returns the Shape object where the connector terminates.
func (c *Connect) Shape() *Shape {
	return c.page.FindShapeByID(c.ShapeID())
}

// ConnectorShape returns the connector Shape object.
func (c *Connect) ConnectorShape() *Shape {
	return c.page.FindShapeByID(c.ConnectorShapeID())
}
