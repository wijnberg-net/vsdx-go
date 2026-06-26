package vsdx

import "fmt"

// UngroupShape dissolves a group: each direct child is reparented into the
// group's own parent container (the page-level <Shapes>, or the enclosing
// group's <Shapes> when the group is nested), with its position converted from
// group-local to parent coordinates. The now-empty group shape is then removed.
//
// It returns the promoted child shapes (freshly bound to their new parent), in
// their original z-order.
//
// Only axis-aligned groups are supported: Angle == 0, FlipX == false,
// FlipY == false. For rotated or flipped groups it returns ErrUnsupportedUngroup
// without mutating the document — promoting children of a rotated group would
// require a full affine decomposition that this function deliberately does not
// perform (explicit failure over incorrect geometry).
//
// Returns ErrNotAGroup if g.ShapeType != "Group", and ErrShapeNotFound if g is
// nil. The group g is expected to be a shape on page p.
func (p *Page) UngroupShape(g *Shape) ([]*Shape, error) {
	if g == nil || g.xml == nil {
		return nil, ErrShapeNotFound
	}
	if g.ShapeType != "Group" {
		return nil, ErrNotAGroup
	}
	// Reject anything but a pure translation. For a rotated or flipped group the
	// group→parent transform mixes the child positions; correctly promoting them
	// needs an affine decompose-against-parent we don't do. Bail before touching
	// the document so the caller is left with the original, valid geometry.
	if g.Angle() != 0 || g.FlipX() || g.FlipY() {
		return nil, ErrUnsupportedUngroup
	}

	// container is the <Shapes> element physically holding the group element:
	// the page-level <Shapes> for a top-level group, or the enclosing group's
	// inner <Shapes> for a nested group. We move children here and remove the
	// group from here, so it stays correct in both cases.
	container := g.xml.Parent()
	if container == nil {
		return nil, fmt.Errorf("group %s has no parent container", g.ID)
	}

	// Group bottom-left corner in parent coordinates. For an axis-aligned,
	// unflipped group the group→child transform is a pure translation by this
	// amount (see Shape.ComputeShapeTransform in transform.go: with Angle==0 the
	// local transform reduces to Translate(PinX-LocPinX, PinY-LocPinY)). This is
	// the exact inverse of the (s.X()-minX, s.Y()-minY) shift GroupShapes applies.
	dx := g.X() - g.LocX()
	dy := g.Y() - g.LocY()

	children := g.ChildShapes() // snapshot; each wraps the live child element
	gidx := g.xml.Index()       // the group's slot in container, for z-order
	promoted := make([]*Shape, 0, len(children))

	for i, c := range children {
		// Pure translation: only PinX/PinY change. Width/Height/LocPin/geometry
		// and every other cell, section, sub-shape and unknown bit of XML are
		// left untouched because we move the existing element, never rebuild it.
		c.SetX(c.X() + dx)
		c.SetY(c.Y() + dy)
		// Insert just before the group. InsertChildAt detaches c from the
		// group's inner <Shapes> first. Each insert pushes the group one slot
		// to the right, so gidx+i always lands immediately before it — leaving
		// the promoted children in the group's former slot, in original order.
		container.InsertChildAt(gidx+i, c.xml)
		promoted = append(promoted, newShape(c.xml, g.Parent, p))
	}

	// Children are detached, so g.AllShapes() is now empty and
	// removeOrphanConnects only drops <Connect> elements bound to the group's
	// own ID. Connectors between the promoted children keep their IDs and so
	// stay valid. (We don't call g.Remove(): its Parent.removeChildShape path
	// can't remove a nested group, whose etree parent is the enclosing group's
	// <Shapes>, not the enclosing <Shape> element. Removing via the real
	// container is correct for both top-level and nested groups.)
	g.removeOrphanConnects()
	container.RemoveChild(g.xml)

	return promoted, nil
}
