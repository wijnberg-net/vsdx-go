package vsdx

import (
	"math"
	"testing"
)

// a regression case: targeting geometry sections beyond IX=0 used to require digging
// directly into shape.Geometries (a public slice). GeometryAt(idx) is the
// discoverable, bounds-checked accessor.

func TestEC005_GeometryAt_RetrievesIndexBeyondZero(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	s := v.GetPage(0).AddShape()

	// AddGeometry creates a fresh section at idx = len(Geometries).
	g0 := s.AddGeometry()
	g1 := s.AddGeometry()
	g2 := s.AddGeometry()

	if got := s.GeometryAt(0); got != g0 {
		t.Errorf("GeometryAt(0) = %p, want %p", got, g0)
	}
	if got := s.GeometryAt(1); got != g1 {
		t.Errorf("GeometryAt(1) = %p, want %p", got, g1)
	}
	if got := s.GeometryAt(2); got != g2 {
		t.Errorf("GeometryAt(2) = %p, want %p", got, g2)
	}
	if got := s.GeometryAt(3); got != nil {
		t.Errorf("GeometryAt(3) = %p, want nil (out of range)", got)
	}
	if got := s.GeometryAt(-1); got != nil {
		t.Errorf("GeometryAt(-1) = %p, want nil (negative index)", got)
	}
}

func TestEC005_MutateGeometryAtBeyondZero(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	s := v.GetPage(0).AddShape()
	s.AddGeometry()
	g1 := s.AddGeometry()
	g1.AddMoveTo(0, 0)
	g1.AddLineTo(0, 0)

	// Mutate the SECOND section's first LineTo via the new accessor.
	s.GeometryAt(1).SetLineTo(3.5, 4.5, 0)

	for _, r := range s.GeometryAt(1).Rows {
		if r.RowType() == "LineTo" {
			if math.Abs(r.X()-3.5) > 1e-9 || math.Abs(r.Y()-4.5) > 1e-9 {
				t.Errorf("LineTo in section IX=1 = (%v, %v), want (3.5, 4.5)", r.X(), r.Y())
			}
		}
	}
}
