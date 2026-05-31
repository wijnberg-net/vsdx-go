package vsdx

import (
	"math"
	"testing"
)

// a regression case: Geometry.Move used to handle only MoveTo and LineTo, silently
// skipping ArcTo, EllipticalArcTo, NURBSTo, Polyline, Spline, InfiniteLine,
// Ellipse. The new implementation drives the per-row coord cells from
// moveCoordCells(). These tests construct each row type programmatically
// and assert that Move translates the cells we expect — and leaves scalar
// cells (bow, weight, angle) alone.

func newGeometryWithRow(t *testing.T) (*VisioFile, *Shape, *Geometry) {
	t.Helper()
	v := newBlankFile(t)
	page := v.GetPage(0)
	s := page.AddShape()
	s.SetX(0)
	s.SetY(0)
	s.SetWidth(1)
	s.SetHeight(1)
	g := s.AddGeometry()
	return v, s, g
}

func cellValueOf(t *testing.T, r *GeometryRow, name string) float64 {
	t.Helper()
	c, ok := r.Cells[name]
	if !ok || c == nil {
		t.Fatalf("row has no cell %q", name)
	}
	return toFloat(c.Value())
}

// row-type → (expected x-cells, expected y-cells)
func TestGeometryMoveContract_ArcTo(t *testing.T) {
	v, _, g := newGeometryWithRow(t)
	defer v.Close()
	g.AddArcTo(2.0, 3.0, 0.5) // X=2, Y=3, A=0.5 (bow scalar)

	g.Move(1.0, 1.0)

	var r *GeometryRow
	for _, row := range g.Rows {
		if row.RowType() == "ArcTo" {
			r = row
		}
	}
	if math.Abs(cellValueOf(t, r, "X")-3.0) > 1e-9 {
		t.Errorf("X = %v, want 3", cellValueOf(t, r, "X"))
	}
	if math.Abs(cellValueOf(t, r, "Y")-4.0) > 1e-9 {
		t.Errorf("Y = %v, want 4", cellValueOf(t, r, "Y"))
	}
	if math.Abs(cellValueOf(t, r, "A")-0.5) > 1e-9 {
		t.Errorf("A (bow) = %v, want 0.5 (scalar must NOT translate)", cellValueOf(t, r, "A"))
	}
}

func TestGeometryMoveContract_EllipticalArcTo(t *testing.T) {
	v, _, g := newGeometryWithRow(t)
	defer v.Close()
	// X=2, Y=3, A=4 (ctl X), B=5 (ctl Y), C=2 (ratio), D=0.5 (angle)
	g.AddEllipticalArcTo(2.0, 3.0, 4.0, 5.0, 2.0, 0.5)
	g.Move(1.0, 1.0)
	var r *GeometryRow
	for _, row := range g.Rows {
		if row.RowType() == "EllipticalArcTo" {
			r = row
		}
	}
	if math.Abs(cellValueOf(t, r, "X")-3.0) > 1e-9 {
		t.Errorf("X = %v, want 3", cellValueOf(t, r, "X"))
	}
	if math.Abs(cellValueOf(t, r, "Y")-4.0) > 1e-9 {
		t.Errorf("Y = %v, want 4", cellValueOf(t, r, "Y"))
	}
	if math.Abs(cellValueOf(t, r, "A")-5.0) > 1e-9 {
		t.Errorf("A (ctl X) = %v, want 5", cellValueOf(t, r, "A"))
	}
	if math.Abs(cellValueOf(t, r, "B")-6.0) > 1e-9 {
		t.Errorf("B (ctl Y) = %v, want 6", cellValueOf(t, r, "B"))
	}
	if math.Abs(cellValueOf(t, r, "C")-2.0) > 1e-9 {
		t.Errorf("C (ratio) = %v, want 2 (scalar)", cellValueOf(t, r, "C"))
	}
	if math.Abs(cellValueOf(t, r, "D")-0.5) > 1e-9 {
		t.Errorf("D (angle) = %v, want 0.5 (scalar)", cellValueOf(t, r, "D"))
	}
}

func TestGeometryMoveContract_Ellipse(t *testing.T) {
	v, _, g := newGeometryWithRow(t)
	defer v.Close()
	// Center X=2 Y=3, major (A=4, B=5), minor (C=6, D=7)
	g.AddEllipse(2.0, 3.0, 4.0, 5.0, 6.0, 7.0)
	g.Move(1.0, 1.0)
	var r *GeometryRow
	for _, row := range g.Rows {
		if row.RowType() == "Ellipse" {
			r = row
		}
	}
	for _, c := range []struct {
		name string
		want float64
	}{{"X", 3}, {"Y", 4}, {"A", 5}, {"B", 6}, {"C", 7}, {"D", 8}} {
		got := cellValueOf(t, r, c.name)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("Ellipse %s = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestGeometryMoveContract_NURBSTo(t *testing.T) {
	v, _, g := newGeometryWithRow(t)
	defer v.Close()
	g.AddNURBSTo(2.0, 3.0, 0.5, 1.0, 0.0, 1.0, "NURBS(...)")
	g.Move(1.0, 1.0)
	var r *GeometryRow
	for _, row := range g.Rows {
		if row.RowType() == "NURBSTo" {
			r = row
		}
	}
	if math.Abs(cellValueOf(t, r, "X")-3.0) > 1e-9 {
		t.Errorf("X = %v, want 3", cellValueOf(t, r, "X"))
	}
	if math.Abs(cellValueOf(t, r, "Y")-4.0) > 1e-9 {
		t.Errorf("Y = %v, want 4", cellValueOf(t, r, "Y"))
	}
	// A, B, C, D are knots/weights — must not translate.
	if math.Abs(cellValueOf(t, r, "A")-0.5) > 1e-9 {
		t.Errorf("A (knot) translated to %v, want 0.5", cellValueOf(t, r, "A"))
	}
}

func TestGeometryMoveContract_InfiniteLine(t *testing.T) {
	v, _, g := newGeometryWithRow(t)
	defer v.Close()
	// Point 1 = (2, 3), point 2 = (5, 7)
	g.AddInfiniteLine(2.0, 3.0, 5.0, 7.0)
	g.Move(1.0, 1.0)
	var r *GeometryRow
	for _, row := range g.Rows {
		if row.RowType() == "InfiniteLine" {
			r = row
		}
	}
	for _, c := range []struct {
		name string
		want float64
	}{{"X", 3}, {"Y", 4}, {"A", 6}, {"B", 8}} {
		got := cellValueOf(t, r, c.name)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("InfiniteLine %s = %v, want %v", c.name, got, c.want)
		}
	}
}

// Relative row types must NOT translate — their X/Y are fractions of
// shape.Width/Height. Translating them would scale the curve by the
// move amount, distorting it.
func TestGeometryMoveContract_RelLineToUnchanged(t *testing.T) {
	v, _, g := newGeometryWithRow(t)
	defer v.Close()
	g.AddRelLineTo(0.5, 0.5)
	g.Move(1.0, 1.0)
	var r *GeometryRow
	for _, row := range g.Rows {
		if row.RowType() == "RelLineTo" {
			r = row
		}
	}
	if math.Abs(cellValueOf(t, r, "X")-0.5) > 1e-9 {
		t.Errorf("RelLineTo X translated to %v, want 0.5 (relative)", cellValueOf(t, r, "X"))
	}
	if math.Abs(cellValueOf(t, r, "Y")-0.5) > 1e-9 {
		t.Errorf("RelLineTo Y translated to %v, want 0.5 (relative)", cellValueOf(t, r, "Y"))
	}
}

// Mixed-path regression: a real connector geometry has MoveTo + LineTo +
// NURBSTo combined. Move must translate all three consistently, not just
// MoveTo/LineTo as the pre-fix code did.
func TestGeometryMoveContract_MixedPathAllTranslate(t *testing.T) {
	v, _, g := newGeometryWithRow(t)
	defer v.Close()
	g.AddMoveTo(1.0, 1.0)
	g.AddLineTo(2.0, 2.0)
	g.AddNURBSTo(3.0, 3.0, 0.5, 1.0, 0.0, 1.0, "NURBS(...)")
	g.Move(10.0, 10.0)

	var mt, lt, nt *GeometryRow
	for _, row := range g.Rows {
		switch row.RowType() {
		case "MoveTo":
			mt = row
		case "LineTo":
			lt = row
		case "NURBSTo":
			nt = row
		}
	}
	if math.Abs(cellValueOf(t, mt, "X")-11) > 1e-9 || math.Abs(cellValueOf(t, mt, "Y")-11) > 1e-9 {
		t.Errorf("MoveTo not translated: (%v, %v)", cellValueOf(t, mt, "X"), cellValueOf(t, mt, "Y"))
	}
	if math.Abs(cellValueOf(t, lt, "X")-12) > 1e-9 || math.Abs(cellValueOf(t, lt, "Y")-12) > 1e-9 {
		t.Errorf("LineTo not translated: (%v, %v)", cellValueOf(t, lt, "X"), cellValueOf(t, lt, "Y"))
	}
	if math.Abs(cellValueOf(t, nt, "X")-13) > 1e-9 || math.Abs(cellValueOf(t, nt, "Y")-13) > 1e-9 {
		t.Errorf("NURBSTo not translated: (%v, %v) — a regression case regression",
			cellValueOf(t, nt, "X"), cellValueOf(t, nt, "Y"))
	}
}
