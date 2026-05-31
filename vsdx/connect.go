package vsdx

import (
	"fmt"

	"github.com/beevik/etree"
)

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

func (c *Connect) String() string {
	return fmt.Sprintf("Connect: from=%s to=%s connector_id=%s shape_id=%s", c.FromID, c.ToID, c.ConnectorShapeID(), c.ShapeID())
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

// Remove strips this <Connect> element from the page's <Connects> section.
// Neither the connector shape nor the terminal shape is touched — the
// connector keeps its current geometry, only the binding disappears. After
// Remove, this *Connect is detached and further method calls are no-ops.
func (c *Connect) Remove() {
	if c.xml == nil {
		return
	}
	if parent := c.xml.Parent(); parent != nil {
		parent.RemoveChild(c.xml)
	}
	c.xml = nil
}
