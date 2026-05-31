package vsdx

import (
	"math"
	"testing"
)

// Page.GroupShapes creates a new <Shape Type="Group"> around the supplied
// shapes, computing a padded axis-aligned bounding box. The audit pointed at
// a specific subtlety in [foreign.go:172-174](foreign.go):
//
//   // Use Width()/2 rather than LocX() because LocPinX's cached V attribute
//   // can be stale after resize.
//
// This is a pragmatic workaround that ASSUMES LocPinX == Width/2 for every
// grouped shape. It works for symmetric shapes (Rounded Rectangle: LocX =
// 0.5 * Width by master formula). It silently drifts for shapes with an
// off-center pin — the bbox is computed at the wrong place.
//
// The contracts below pin three behaviours:
//
//   1. ShapeType is "Group" and ID is allocated.
//   2. Source shapes' IDs are preserved (connectors reference shapes by ID
//      globally, so renaming would break them).
//   3. Bbox math: for centered-pin shapes the bbox is tight. For off-center
//      pin shapes we explicitly accept the current Width/2 approximation —
//      that is the documented compromise. If a future fix uses an effective
//      LocPin lookup, both the test AND the comment in foreign.go must
//      change together.

func newBlankFile(t *testing.T) *VisioFile {
	t.Helper()
	v, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func addRect(p *Page, x, y, w, h, lx, ly float64) *Shape {
	s := p.AddShape()
	s.SetX(x)
	s.SetY(y)
	s.SetWidth(w)
	s.SetHeight(h)
	s.SetLocX(lx)
	s.SetLocY(ly)
	return s
}

func TestGroupShapesContract_GroupCreated(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	s1 := addRect(p, 1, 1, 1, 1, 0.5, 0.5)
	s2 := addRect(p, 3, 3, 1, 1, 0.5, 0.5)
	g := p.GroupShapes([]*Shape{s1, s2}, 0)
	if g == nil {
		t.Fatal("GroupShapes returned nil")
	}
	if g.ShapeType != "Group" {
		t.Errorf("ShapeType = %q, want %q", g.ShapeType, "Group")
	}
	if g.ID == "" {
		t.Error("group has empty ID")
	}
}

// Source shape IDs must survive grouping. Connectors reference grouped
// shapes by global ID; renaming would break every Connect element.
func TestGroupShapesContract_ChildIDsPreserved(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	s1 := addRect(p, 1, 1, 1, 1, 0.5, 0.5)
	s2 := addRect(p, 3, 3, 1, 1, 0.5, 0.5)
	id1, id2 := s1.ID, s2.ID

	p.GroupShapes([]*Shape{s1, s2}, 0)

	if found := p.FindShapeByID(id1); found == nil {
		t.Errorf("child %s lost its ID after grouping", id1)
	}
	if found := p.FindShapeByID(id2); found == nil {
		t.Errorf("child %s lost its ID after grouping", id2)
	}
}

// Bbox math: for shapes whose pin is centered (LocX == Width/2), the bbox
// is the union of their visible rectangles, plus padding.
func TestGroupShapesContract_BboxCenteredPins(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	// Three rectangles spanning x:[1,4], y:[1,4]. All centered pins.
	addRect(p, 1.5, 1.5, 1.0, 1.0, 0.5, 0.5) // covers (1,1)-(2,2)
	addRect(p, 3.5, 1.5, 1.0, 1.0, 0.5, 0.5) // covers (3,1)-(4,2)
	addRect(p, 2.5, 3.5, 1.0, 1.0, 0.5, 0.5) // covers (2,3)-(3,4)

	shapes := p.ChildShapes()
	g := p.GroupShapes(shapes, 0)

	// Expected bbox: x in [1, 4], y in [1, 4]. width=3, height=3, center=(2.5, 2.5).
	if math.Abs(g.Width()-3.0) > 1e-9 {
		t.Errorf("group Width = %v, want 3.0", g.Width())
	}
	if math.Abs(g.Height()-3.0) > 1e-9 {
		t.Errorf("group Height = %v, want 3.0", g.Height())
	}
	if math.Abs(g.X()-2.5) > 1e-9 {
		t.Errorf("group X = %v, want 2.5", g.X())
	}
	if math.Abs(g.Y()-2.5) > 1e-9 {
		t.Errorf("group Y = %v, want 2.5", g.Y())
	}
}

// Bbox math: for shapes whose pin is OFF-CENTER (LocX != Width/2), the
// bbox must use (pin - LocX, pin - LocX + Width) — not (pin ± Width/2).
// For a shape with pin at the LEFT edge (LocX=0), the visible rect is
// (pin, pin+Width). a regression case closed the prior Width/2 approximation; this
// test now pins the tight bbox.
func TestGroupShapesContract_BboxOffCenterPin(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	p := v.GetPage(0)
	// Pin at the LEFT edge: LocX=0, so visible rect = (5, 6).
	addRect(p, 5.0, 5.0, 1.0, 1.0, 0.0, 0.0)

	g := p.GroupShapes(p.ChildShapes(), 0)
	// Expected: bbox = (5, 6), width=1, center=5.5.
	if math.Abs(g.X()-5.5) > 1e-9 {
		t.Errorf("group X = %v, want 5.5 (center of bbox 5..6)", g.X())
	}
	if math.Abs(g.Width()-1.0) > 1e-9 {
		t.Errorf("group Width = %v, want 1.0", g.Width())
	}
}

// Round-trip: a grouped page survives save+reopen with all children intact.
func TestGroupShapesContract_RoundTripXML(t *testing.T) {
	v := newBlankFile(t)
	p := v.GetPage(0)
	s1 := addRect(p, 1, 1, 1, 1, 0.5, 0.5)
	s2 := addRect(p, 3, 3, 1, 1, 0.5, 0.5)
	g := p.GroupShapes([]*Shape{s1, s2}, 0.1)
	groupID := g.ID
	id1, id2 := s1.ID, s2.ID

	out, err := v.SaveVsdxBytes()
	if err != nil {
		v.Close()
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	v.Close()

	v2 := openFromBytes(t, out)
	defer v2.Close()
	p2 := v2.GetPage(0)
	g2 := p2.FindShapeByID(groupID)
	if g2 == nil {
		t.Fatalf("group %s not found after round-trip", groupID)
	}
	if g2.ShapeType != "Group" {
		t.Errorf("after round-trip ShapeType = %q, want %q", g2.ShapeType, "Group")
	}
	if p2.FindShapeByID(id1) == nil {
		t.Errorf("child %s not found after round-trip", id1)
	}
	if p2.FindShapeByID(id2) == nil {
		t.Errorf("child %s not found after round-trip", id2)
	}
}

// Empty group: GroupShapes([]*Shape{}, 0) must return nil cleanly.
func TestGroupShapesContract_EmptyReturnsNil(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	if g := v.GetPage(0).GroupShapes([]*Shape{}, 0); g != nil {
		t.Errorf("GroupShapes([]) = %v, want nil", g)
	}
}
