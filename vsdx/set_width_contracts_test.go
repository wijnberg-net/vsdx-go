package vsdx

import (
	"strings"
	"testing"
)

// This file demonstrates the mutation-test framework on SetWidth, the
// single mutator that produced four of the six bugs found in the Phase 1
// audit. Each test below addresses one contract category and would have
// caught at least one historical regression.
//
// The fixture is logical-architecture.vsdx because it contains the full
// variety: a top-level shape with local geometry (Rounded Rectangle.5),
// shapes that inherit geometry from a master (Can/Can.15/Can.17), and
// shapes that have child shapes (Can has a sub-shape body).

const setWidthFixture = "testdata/architecture-sample.vsdx"

// 1) Set-and-read. Trivial but pins the basic contract.
func TestSetWidthContract_SetAndRead(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	s.SetWidth(3.5)
	if got := s.Width(); got != 3.5 {
		t.Errorf("Width() after SetWidth(3.5) = %v, want 3.5", got)
	}
}

// 2) Side effects. SetWidth must scale LocPinX (so the bbox.left stays put)
// but must NOT touch Y, Height, LocY, FillColor, etc.
func TestSetWidthContract_SideEffects(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	before := snapshotShape(s)

	s.SetWidth(s.Width() * 2.0)
	after := snapshotShape(s)

	// W and LocX should both change; everything else (including RowCount —
	// scaling existing rows shouldn't add or remove any) must be unchanged.
	assertOnlyTheseFieldsChanged(t, before, after, []string{"W", "LocX"})
}

// 3) Round-trip XML. Mutate, save, reopen — the new Width must persist.
func TestSetWidthContract_RoundTripXML(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	AssertRoundTripXML(t, data,
		func(v *VisioFile) {
			findShapeByName(t, v, "Rounded Rectangle.5").SetWidth(3.5)
		},
		func(t *testing.T, v *VisioFile) {
			s := findShapeByName(t, v, "Rounded Rectangle.5")
			if got := s.Width(); got != 3.5 {
				t.Errorf("after round-trip Width() = %v, want 3.5", got)
			}
		},
	)
}

// 5) Idempotence. Calling SetWidth(2.5) twice in a row must yield the same
// observable state as calling it once. Without this, geometry rows could
// scale twice and corrupt the path.
func TestSetWidthContract_Idempotent(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	AssertIdempotent(t, data,
		func(v *VisioFile) {
			findShapeByName(t, v, "Rounded Rectangle.5").SetWidth(2.5)
		},
		func(v *VisioFile) string {
			s := findShapeByName(t, v, "Rounded Rectangle.5")
			return snapshotShape(s).XMLHash
		},
	)
}

// 5b) Order independence (the simple two-mutator case). SetWidth scales
// LocPinX proportionally; the question is whether a SetWidth followed by
// SetX produces the same state as SetX followed by SetWidth. The answer
// should be YES — these mutate orthogonal cells.
func TestSetWidthContract_OrderWithSetX(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	mutators := []func(*VisioFile){
		func(v *VisioFile) {
			findShapeByName(t, v, "Rounded Rectangle.5").SetWidth(2.5)
		},
		func(v *VisioFile) {
			findShapeByName(t, v, "Rounded Rectangle.5").SetX(5.0)
		},
	}
	AssertOrderIndependent(t, data, mutators, func(v *VisioFile) any {
		s := findShapeByName(t, v, "Rounded Rectangle.5")
		// Compare a tuple of geometric state — string for DeepEqual.
		return [4]float64{s.X(), s.Width(), s.LocX(), s.LocY()}
	})
}

// 6) Inheritance — master isolation. Resizing one Can instance must not
// change Width, geometry, or anything on a sibling Can instance that
// references the same master.
func TestSetWidthContract_MasterIsolation(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	AssertMasterIsolation(t, data, "Can.15", func(s *Shape) {
		s.SetWidth(s.Width() * 2.0)
	})
}

// 6b) Inheritance — after SetWidth on an inheriting instance, the instance
// must own a local Geometry section. Without this the resize would still
// silently target the master.
func TestSetWidthContract_CreatesLocalGeometry(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Can.15")
	AssertLocalGeometryCreated(t, s, func(s *Shape) {
		s.SetWidth(s.Width() * 2.0)
	})
}

// As a sanity check that the framework would catch a real regression, the
// next test deliberately uses a mutator with broken behaviour (no-op) and
// asserts the test would fail. We can't put this in the suite directly —
// it'd just fail — so the assertion is the inverted form: an empty mutator
// should produce zero side effects.
func TestSetWidthContract_FrameworkNegativeControl(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	before := snapshotShape(s)
	// Intentionally no mutation.
	after := snapshotShape(s)
	if diffs := diffShapeSnapshots(before, after); len(diffs) > 0 {
		t.Errorf("framework noise: empty mutation produced diffs %v", diffs)
	}
	// And the XML hash equality matters for Idempotent — check it's stable.
	if before.XMLHash == "" || before.XMLHash != after.XMLHash {
		t.Errorf("XMLHash unstable: before=%q after=%q", before.XMLHash, after.XMLHash)
	}
	// Silence unused-import warning if we ever drop strings.
	_ = strings.Contains
}

// 7) Multi-cell geometry scaling. For row types with more than one
// coordinate cell per axis (Ellipse: X+A+C on X, Y+B+D on Y) SetWidth and
// SetHeight must scale ALL of them. Regression: an earlier scaleGeometryAxis
// only touched the cell literally named "X" / "Y", leaving A/B/C/D stale.
// On a Can stencil this caused the lip ellipse to render at the master's
// original size on top of a body that scaled correctly — the resized cylinder
// rendered visually larger than the dragged bbox.
func TestSetWidthContract_ScalesAllEllipseCoordCells(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Can.15")

	ellipseBefore := findEllipseRow(t, s)
	x0 := toFloat(ellipseBefore.Cells["X"].Value())
	a0 := toFloat(ellipseBefore.Cells["A"].Value())
	c0 := toFloat(ellipseBefore.Cells["C"].Value())
	y0 := toFloat(ellipseBefore.Cells["Y"].Value())
	b0 := toFloat(ellipseBefore.Cells["B"].Value())
	d0 := toFloat(ellipseBefore.Cells["D"].Value())
	w0 := s.Width()
	h0 := s.Height()

	s.SetWidth(w0 * 2.5)
	s.SetHeight(h0 * 1.75)

	ellipseAfter := findEllipseRow(t, s)
	checkScaled := func(name string, before, after, scale float64) {
		want := before * scale
		// Tolerate normal float jitter.
		if !nearlyEqual(after, want, 1e-9) {
			t.Errorf("Ellipse cell %s: before=%v after=%v want %v (scale %v)",
				name, before, after, want, scale)
		}
	}
	checkScaled("X", x0, toFloat(ellipseAfter.Cells["X"].Value()), 2.5)
	checkScaled("A", a0, toFloat(ellipseAfter.Cells["A"].Value()), 2.5)
	checkScaled("C", c0, toFloat(ellipseAfter.Cells["C"].Value()), 2.5)
	checkScaled("Y", y0, toFloat(ellipseAfter.Cells["Y"].Value()), 1.75)
	checkScaled("B", b0, toFloat(ellipseAfter.Cells["B"].Value()), 1.75)
	checkScaled("D", d0, toFloat(ellipseAfter.Cells["D"].Value()), 1.75)
}

func findEllipseRow(t *testing.T, s *Shape) *GeometryRow {
	t.Helper()
	for _, g := range s.Geometries {
		for _, r := range g.Rows {
			if strings.EqualFold(r.RowType(), "Ellipse") {
				return r
			}
		}
	}
	t.Fatalf("shape %q has no Ellipse geometry row", s.ShapeName)
	return nil
}

func nearlyEqual(a, b, eps float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= eps
}
