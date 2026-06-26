package vsdx

import (
	"math"
	"testing"

	"github.com/beevik/etree"
)

// Page.UngroupShape dissolves a <Shape Type="Group">: it promotes each direct
// child back into the group's own parent container, converting positions from
// group-local to parent coordinates, then removes the now-empty group. It is
// the inverse of GroupShapes (foreign.go).
//
// The contracts below pin the behaviour spec from the feature request:
//
//   1. Axis-aligned ungroup is the exact inverse of GroupShapes (coords + parent).
//   2. Zero data loss — every cell/section/sub-shape/unknown element survives,
//      because the element is moved, never rebuilt.
//   3. Promoted children occupy the group's former z-order slot, in order.
//   4. Connectors between children survive; connectors bound to the group go.
//   5. Nested groups: ungroup one level, the inner group stays a group.
//   6. Rotated/flipped groups are rejected WITHOUT mutating the document.
//   7. Non-group / nil inputs return sentinel errors.
//   8. The result survives a save + reopen round-trip.

// --- helpers ---

func approxEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// pageShapesElem returns the page-level <Shapes> container element.
func pageShapesElem(p *Page) *etree.Element {
	return p.xml.Root().SelectElement("Shapes")
}

// topLevelIDs returns the IDs of the page's top-level shapes in document order.
func topLevelIDs(p *Page) []string {
	var ids []string
	for _, s := range p.ChildShapes() {
		ids = append(ids, s.ID)
	}
	return ids
}

// childIDsOf returns the IDs of a group's direct children in document order.
func childIDsOf(s *Shape) []string {
	var ids []string
	for _, c := range s.ChildShapes() {
		ids = append(ids, c.ID)
	}
	return ids
}

// addConnectElem injects a raw <Connect FromSheet=.. ToSheet=..> into the page.
func addConnectElem(p *Page, from, to string) {
	e := etree.NewElement("Connect")
	e.CreateAttr("FromSheet", from)
	e.CreateAttr("ToSheet", to)
	p.AddConnect(&Connect{xml: e, page: p})
}

// serializeWithoutPins serializes a shape's XML subtree with its direct PinX
// and PinY cells stripped. Those two are the ONLY cells ungroup is allowed to
// change, so stripping them lets us assert everything else is byte-identical.
func serializeWithoutPins(t *testing.T, el *etree.Element) string {
	t.Helper()
	cp := el.Copy()
	for _, c := range cp.SelectElements("Cell") {
		switch c.SelectAttrValue("N", "") {
		case "PinX", "PinY":
			cp.RemoveChild(c)
		}
	}
	doc := etree.NewDocument()
	doc.SetRoot(cp)
	s, err := doc.WriteToString()
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return s
}

func serializeDoc(t *testing.T, doc *etree.Document) string {
	t.Helper()
	s, err := doc.WriteToString()
	if err != nil {
		t.Fatalf("serialize doc: %v", err)
	}
	return s
}

// --- 1. Inverse of GroupShapes ---

// Group two shapes, ungroup, and assert the children return to their exact
// original X/Y/Width/Height and land back in the page-level <Shapes>.
func TestUngroupContract_InverseOfGroup(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	s1 := addRect(p, 1.5, 2.5, 1.0, 1.0, 0.5, 0.5)
	s2 := addRect(p, 4.0, 3.0, 2.0, 1.0, 1.0, 0.5)
	id1, id2 := s1.ID, s2.ID
	x1, y1, w1, h1 := s1.X(), s1.Y(), s1.Width(), s1.Height()
	x2, y2, w2, h2 := s2.X(), s2.Y(), s2.Width(), s2.Height()

	g := p.GroupShapes([]*Shape{s1, s2}, 0)
	if g == nil {
		t.Fatal("GroupShapes returned nil")
	}

	promoted, err := p.UngroupShape(g)
	if err != nil {
		t.Fatalf("UngroupShape: %v", err)
	}
	if len(promoted) != 2 {
		t.Fatalf("promoted = %d shapes, want 2", len(promoted))
	}

	// Group is gone.
	if p.FindShapeByID(g.ID) != nil {
		t.Errorf("group %s still present after ungroup", g.ID)
	}

	r1 := p.FindShapeByID(id1)
	r2 := p.FindShapeByID(id2)
	if r1 == nil || r2 == nil {
		t.Fatalf("children lost after ungroup: r1=%v r2=%v", r1, r2)
	}

	// Coordinates restored exactly.
	for _, tc := range []struct {
		name           string
		r              *Shape
		wx, wy, ww, wh float64
	}{
		{"s1", r1, x1, y1, w1, h1},
		{"s2", r2, x2, y2, w2, h2},
	} {
		if !approxEq(tc.r.X(), tc.wx) || !approxEq(tc.r.Y(), tc.wy) {
			t.Errorf("%s pos = (%v,%v), want (%v,%v)", tc.name, tc.r.X(), tc.r.Y(), tc.wx, tc.wy)
		}
		if !approxEq(tc.r.Width(), tc.ww) || !approxEq(tc.r.Height(), tc.wh) {
			t.Errorf("%s size = (%v,%v), want (%v,%v)", tc.name, tc.r.Width(), tc.r.Height(), tc.ww, tc.wh)
		}
	}

	// Back in the page-level <Shapes>.
	pageShapes := pageShapesElem(p)
	if r1.xml.Parent() != pageShapes || r2.xml.Parent() != pageShapes {
		t.Errorf("children not reparented to page-level <Shapes>")
	}

	// Returned shapes are bound to the page-level container (same parent the
	// normal traversal API would give them).
	for i, ps := range promoted {
		if ps.xml.Parent() != pageShapes {
			t.Errorf("promoted[%d] not under page <Shapes>", i)
		}
	}
}

// --- 2. Zero data loss, including unknown XML ---

func TestUngroupContract_PreservesUnknownData(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	child := addRect(p, 2.0, 2.0, 1.0, 1.0, 0.5, 0.5)

	// A custom/unknown cell.
	child.SetCellValue("MyCustomCell", "keep-me")
	// An extra geometry section with a row.
	geo := child.xml.CreateElement("Section")
	geo.CreateAttr("N", "Geometry")
	geo.CreateAttr("IX", "1")
	row := geo.CreateElement("Row")
	row.CreateAttr("T", "RelMoveTo")
	row.CreateAttr("IX", "1")
	addCellXML(row, "X", "0.25", "")
	addCellXML(row, "Y", "0.75", "")
	// An entirely unrecognised child element with text + attrs.
	unknown := child.xml.CreateElement("VisiGoExtension")
	unknown.CreateAttr("flavor", "round-trip")
	unknown.SetText("opaque-payload")

	x0, y0 := child.X(), child.Y()
	before := serializeWithoutPins(t, child.xml)

	g := p.GroupShapes([]*Shape{child}, 0)
	if _, err := p.UngroupShape(g); err != nil {
		t.Fatalf("UngroupShape: %v", err)
	}

	// child.xml is the SAME element (moved, never rebuilt).
	after := serializeWithoutPins(t, child.xml)
	if before != after {
		t.Errorf("subtree changed (excluding PinX/PinY):\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
	// PinX/PinY restored to their original values.
	if !approxEq(child.X(), x0) || !approxEq(child.Y(), y0) {
		t.Errorf("pin = (%v,%v), want (%v,%v)", child.X(), child.Y(), x0, y0)
	}
}

// --- 3. Z-order: children land in the group's former slot, in order ---

// Build a page whose document order is [A, G, B] where G groups {P, Q}.
// Ungroup G; the page must become [A, P, Q, B].
func TestUngroupContract_ZOrderPreserved(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	a := addRect(p, 1, 1, 1, 1, 0.5, 0.5)
	pp := addRect(p, 2, 2, 1, 1, 0.5, 0.5)
	qq := addRect(p, 3, 3, 1, 1, 0.5, 0.5)
	// Grouping appends the group at the end and removes P, Q -> page: [A, G].
	g := p.GroupShapes([]*Shape{pp, qq}, 0)
	// Add B after the group -> page: [A, G, B], so G sits in the middle.
	b := addRect(p, 4, 4, 1, 1, 0.5, 0.5)

	want := []string{a.ID, pp.ID, qq.ID, b.ID}

	if _, err := p.UngroupShape(g); err != nil {
		t.Fatalf("UngroupShape: %v", err)
	}

	got := topLevelIDs(p)
	if len(got) != len(want) {
		t.Fatalf("top-level order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("top-level order = %v, want %v", got, want)
		}
	}
}

// --- 4. Connectors ---

// A <Connect> joining two grouped children survives ungroup; a <Connect> bound
// to the group shape itself is removed with the group.
func TestUngroupContract_ConnectorsHandled(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	c1 := addRect(p, 1, 1, 1, 1, 0.5, 0.5)
	c2 := addRect(p, 3, 3, 1, 1, 0.5, 0.5)
	ext := addRect(p, 6, 6, 1, 1, 0.5, 0.5) // stand-in connector shape

	// Connector between the two shapes that are about to be grouped.
	addConnectElem(p, c1.ID, c2.ID)

	g := p.GroupShapes([]*Shape{c1, c2}, 0)

	// Connector bound to the group shape itself.
	addConnectElem(p, ext.ID, g.ID)

	if _, err := p.UngroupShape(g); err != nil {
		t.Fatalf("UngroupShape: %v", err)
	}

	var hasChildConn, hasGroupConn bool
	for _, c := range p.Connects() {
		if c.FromID == c1.ID && c.ToID == c2.ID {
			hasChildConn = true
		}
		if c.FromID == g.ID || c.ToID == g.ID {
			hasGroupConn = true
		}
	}
	if !hasChildConn {
		t.Error("connector between grouped children did not survive ungroup")
	}
	if hasGroupConn {
		t.Error("connector bound to the group was not removed")
	}
}

// --- 5. Nested group: ungroup the OUTER group ---

func TestUngroupContract_NestedUngroupOuter(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	in1 := addRect(p, 1, 1, 1, 1, 0.5, 0.5)
	in2 := addRect(p, 2, 2, 1, 1, 0.5, 0.5)
	innerG := p.GroupShapes([]*Shape{in1, in2}, 0)
	innerChildIDs := childIDsOf(innerG) // capture before any further nesting

	extra := addRect(p, 5, 5, 1, 1, 0.5, 0.5)
	innerX, innerY := innerG.X(), innerG.Y() // page coords before outer grouping

	outerG := p.GroupShapes([]*Shape{innerG, extra}, 0)

	promoted, err := p.UngroupShape(outerG)
	if err != nil {
		t.Fatalf("UngroupShape: %v", err)
	}
	if len(promoted) != 2 {
		t.Fatalf("promoted = %d, want 2", len(promoted))
	}

	// Outer group removed.
	if p.FindShapeByID(outerG.ID) != nil {
		t.Error("outer group still present after ungroup")
	}

	// Inner group is back on the page, still a group, children intact.
	ri := p.FindShapeByID(innerG.ID)
	if ri == nil {
		t.Fatal("inner group lost after ungroup")
	}
	if ri.ShapeType != "Group" {
		t.Errorf("inner group ShapeType = %q, want %q", ri.ShapeType, "Group")
	}
	if ri.xml.Parent() != pageShapesElem(p) {
		t.Error("inner group not reparented to page-level <Shapes>")
	}
	gotInnerChildren := childIDsOf(ri)
	if len(gotInnerChildren) != len(innerChildIDs) {
		t.Errorf("inner group children = %v, want %v", gotInnerChildren, innerChildIDs)
	}

	// Inner group position restored to its pre-outer-grouping page coords.
	if !approxEq(ri.X(), innerX) || !approxEq(ri.Y(), innerY) {
		t.Errorf("inner group pos = (%v,%v), want (%v,%v)", ri.X(), ri.Y(), innerX, innerY)
	}
}

// --- 6. Nested group: ungroup the INNER group (robust removal) ---

// Dissolving a nested group must remove the group element from the ENCLOSING
// group's <Shapes> (its real etree parent) and land the grandchildren one
// level up. This exercises the container-based removal that a naive
// g.Remove() would get wrong for nested groups.
func TestUngroupContract_NestedUngroupInner(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	in1 := addRect(p, 1, 1, 1, 1, 0.5, 0.5)
	in2 := addRect(p, 2, 2, 1, 1, 0.5, 0.5)
	innerG := p.GroupShapes([]*Shape{in1, in2}, 0)
	extra := addRect(p, 5, 5, 1, 1, 0.5, 0.5)
	outerG := p.GroupShapes([]*Shape{innerG, extra}, 0)

	// Translation the inner group contributes to its children, captured while
	// the inner group is still nested inside the outer group.
	dx := innerG.X() - innerG.LocX()
	dy := innerG.Y() - innerG.LocY()
	in1X, in1Y := in1.X(), in1.Y() // inner-group-local coords

	promoted, err := p.UngroupShape(innerG)
	if err != nil {
		t.Fatalf("UngroupShape(inner): %v", err)
	}
	if len(promoted) != 2 {
		t.Fatalf("promoted = %d, want 2", len(promoted))
	}

	// Inner group element is gone entirely.
	if p.FindShapeByID(innerG.ID) != nil {
		t.Error("inner group still present after ungroup")
	}

	// Grandchildren are now direct children of the OUTER group.
	outerChildIDs := childIDsOf(outerG)
	if !contains(outerChildIDs, in1.ID) || !contains(outerChildIDs, in2.ID) {
		t.Errorf("grandchildren not promoted into outer group: outer children = %v", outerChildIDs)
	}
	if contains(outerChildIDs, innerG.ID) {
		t.Error("inner group element still a child of the outer group")
	}

	// Coordinates converted into the outer group's local space.
	r1 := outerG.FindShapeByID(in1.ID)
	if r1 == nil {
		t.Fatal("promoted grandchild not found under outer group")
	}
	if !approxEq(r1.X(), in1X+dx) || !approxEq(r1.Y(), in1Y+dy) {
		t.Errorf("grandchild pos = (%v,%v), want (%v,%v)", r1.X(), r1.Y(), in1X+dx, in1Y+dy)
	}
}

// --- 7. Rejection: rotated / flipped groups, no mutation ---

func TestUngroupContract_RejectsRotated(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	s1 := addRect(p, 1, 1, 1, 1, 0.5, 0.5)
	s2 := addRect(p, 3, 3, 1, 1, 0.5, 0.5)
	g := p.GroupShapes([]*Shape{s1, s2}, 0)
	g.SetCellValue(CellAngle, "1.5707963267948966") // 90 degrees in radians

	before := serializeDoc(t, p.xml)
	got, err := p.UngroupShape(g)
	if err != ErrUnsupportedUngroup {
		t.Errorf("err = %v, want ErrUnsupportedUngroup", err)
	}
	if got != nil {
		t.Errorf("returned %v, want nil", got)
	}
	if after := serializeDoc(t, p.xml); after != before {
		t.Error("document mutated despite rejecting a rotated group")
	}
}

func TestUngroupContract_RejectsFlipped(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	s1 := addRect(p, 1, 1, 1, 1, 0.5, 0.5)
	s2 := addRect(p, 3, 3, 1, 1, 0.5, 0.5)
	g := p.GroupShapes([]*Shape{s1, s2}, 0)
	g.SetCellValue(CellFlipX, "1")

	before := serializeDoc(t, p.xml)
	got, err := p.UngroupShape(g)
	if err != ErrUnsupportedUngroup {
		t.Errorf("err = %v, want ErrUnsupportedUngroup", err)
	}
	if got != nil {
		t.Errorf("returned %v, want nil", got)
	}
	if after := serializeDoc(t, p.xml); after != before {
		t.Error("document mutated despite rejecting a flipped group")
	}
}

// --- 8. Bad input ---

func TestUngroupContract_NotAGroup(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	s := addRect(p, 1, 1, 1, 1, 0.5, 0.5)

	before := serializeDoc(t, p.xml)
	got, err := p.UngroupShape(s)
	if err != ErrNotAGroup {
		t.Errorf("err = %v, want ErrNotAGroup", err)
	}
	if got != nil {
		t.Errorf("returned %v, want nil", got)
	}
	if after := serializeDoc(t, p.xml); after != before {
		t.Error("document mutated on non-group input")
	}
}

func TestUngroupContract_NilGroup(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	got, err := p.UngroupShape(nil)
	if err != ErrShapeNotFound {
		t.Errorf("err = %v, want ErrShapeNotFound", err)
	}
	if got != nil {
		t.Errorf("returned %v, want nil", got)
	}
}

// --- 9. Empty group ---

// A group with zero children is removed and an empty (non-nil) slice returned.
func TestUngroupContract_EmptyGroup(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	s1 := addRect(p, 2, 2, 1, 1, 0.5, 0.5)
	g := p.GroupShapes([]*Shape{s1}, 0)

	// Empty the group's inner <Shapes> directly (RemoveChild works here because
	// the child's real etree parent IS the inner <Shapes>).
	inner := g.xml.FindElement("Shapes")
	for _, ch := range inner.SelectElements("Shape") {
		inner.RemoveChild(ch)
	}

	promoted, err := p.UngroupShape(g)
	if err != nil {
		t.Fatalf("UngroupShape: %v", err)
	}
	if promoted == nil {
		t.Error("promoted slice is nil, want empty non-nil slice")
	}
	if len(promoted) != 0 {
		t.Errorf("promoted = %d, want 0", len(promoted))
	}
	if p.FindShapeByID(g.ID) != nil {
		t.Error("empty group not removed")
	}
}

// --- 10. Save + reopen round-trip (proxy for the Visio re-open check) ---

func TestUngroupContract_RoundTripSaveReopen(t *testing.T) {
	v := newBlankFile(t)
	p := v.GetPage(0)
	s1 := addRect(p, 1.5, 2.5, 1.0, 1.0, 0.5, 0.5)
	s2 := addRect(p, 4.0, 3.0, 2.0, 1.0, 1.0, 0.5)
	id1, id2 := s1.ID, s2.ID
	x1, y1 := s1.X(), s1.Y()
	x2, y2 := s2.X(), s2.Y()

	g := p.GroupShapes([]*Shape{s1, s2}, 0.1)
	gid := g.ID
	if _, err := p.UngroupShape(g); err != nil {
		v.Close()
		t.Fatalf("UngroupShape: %v", err)
	}

	out, err := v.SaveVsdxBytes()
	if err != nil {
		v.Close()
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	v.Close()

	v2 := openFromBytes(t, out)
	defer v2.Close()
	p2 := v2.GetPage(0)

	if p2.FindShapeByID(gid) != nil {
		t.Errorf("group %s survived round-trip", gid)
	}
	r1 := p2.FindShapeByID(id1)
	r2 := p2.FindShapeByID(id2)
	if r1 == nil || r2 == nil {
		t.Fatalf("children lost on round-trip: r1=%v r2=%v", r1, r2)
	}
	if !approxEq(r1.X(), x1) || !approxEq(r1.Y(), y1) {
		t.Errorf("s1 pos after round-trip = (%v,%v), want (%v,%v)", r1.X(), r1.Y(), x1, y1)
	}
	if !approxEq(r2.X(), x2) || !approxEq(r2.Y(), y2) {
		t.Errorf("s2 pos after round-trip = (%v,%v), want (%v,%v)", r2.X(), r2.Y(), x2, y2)
	}
}
