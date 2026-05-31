package vsdx

import (
	"math"
	"testing"
)

// Shape.Move translates the shape by (dx, dy). Implementation:
//
//	if s.Geometry != nil { s.Geometry.Move(dx, dy) }
//	if s.HasBeginX() { s.SetBeginX(s.BeginX() + dx); s.SetBeginY(s.BeginY() + dy) }
//	s.SetX(s.X() + dx); s.SetY(s.Y() + dy)
//
// The audit flagged two gaps:
//
//   1. Connector EndX/EndY are NOT updated, even though BeginX/Y are. So a
//      Move on a 1D connector translates the begin point but leaves the end
//      anchored, stretching the line.
//   2. Geometry.Move only handles MoveTo and LineTo rows. ArcTo,
//      EllipticalArcTo, NURBSTo and friends are silently skipped — for
//      mixed-path shapes only some of the geometry follows the move.
//
// The contract tests below pin both behaviours so future fixes either
// (a) make Move complete or (b) update the assertions when scope is
// formally narrowed.

const moveFixture = setWidthFixture

// 1) Set-and-read on the canonical case: a plain 2D rectangle.
func TestMoveContract_SetAndRead(t *testing.T) {
	data := loadFixtureBytes(t, moveFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	x0, y0 := s.X(), s.Y()
	s.Move(1.0, 2.0)
	if got := s.X(); math.Abs(got-(x0+1.0)) > 1e-9 {
		t.Errorf("X() = %v, want %v", got, x0+1.0)
	}
	if got := s.Y(); math.Abs(got-(y0+2.0)) > 1e-9 {
		t.Errorf("Y() = %v, want %v", got, y0+2.0)
	}
}

// 2) Side effects. For a non-connector shape Move should change only X, Y.
func TestMoveContract_SideEffects(t *testing.T) {
	data := loadFixtureBytes(t, moveFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	before := snapshotShape(s)
	s.Move(1.0, 2.0)
	after := snapshotShape(s)
	assertOnlyTheseFieldsChanged(t, before, after, []string{"X", "Y"})
}

// 3) Round-trip XML. Position survives save+reopen.
func TestMoveContract_RoundTripXML(t *testing.T) {
	data := loadFixtureBytes(t, moveFixture)
	var x0, y0 float64
	AssertRoundTripXML(t, data,
		func(v *VisioFile) {
			s := findShapeByName(t, v, "Rounded Rectangle.5")
			x0, y0 = s.X(), s.Y()
			s.Move(1.0, 2.0)
		},
		func(t *testing.T, v *VisioFile) {
			s := findShapeByName(t, v, "Rounded Rectangle.5")
			if math.Abs(s.X()-(x0+1.0)) > 1e-9 || math.Abs(s.Y()-(y0+2.0)) > 1e-9 {
				t.Errorf("after round-trip Move did not persist: X=%v Y=%v want %v %v",
					s.X(), s.Y(), x0+1.0, y0+2.0)
			}
		},
	)
}

// 5) Composition: Move(1,0); Move(2,0) must produce the same end state as
// Move(3,0). This is the additive contract — without it, repeated Moves
// could compound or drift due to floating-point accumulation.
func TestMoveContract_Additive(t *testing.T) {
	data := loadFixtureBytes(t, moveFixture)

	// Path A: two moves.
	v1 := openFromBytes(t, data)
	s1 := findShapeByName(t, v1, "Rounded Rectangle.5")
	s1.Move(1.0, 0.0)
	s1.Move(2.0, 0.0)
	endA := [2]float64{s1.X(), s1.Y()}
	v1.Close()

	// Path B: one combined move.
	v2 := openFromBytes(t, data)
	s2 := findShapeByName(t, v2, "Rounded Rectangle.5")
	s2.Move(3.0, 0.0)
	endB := [2]float64{s2.X(), s2.Y()}
	v2.Close()

	if math.Abs(endA[0]-endB[0]) > 1e-9 || math.Abs(endA[1]-endB[1]) > 1e-9 {
		t.Errorf("Move not additive: 1+2 -> %v, 3 -> %v", endA, endB)
	}
}

// 5b) Move(0, 0) must be an idempotent no-op.
func TestMoveContract_ZeroDeltaIsNoOp(t *testing.T) {
	data := loadFixtureBytes(t, moveFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	before := snapshotShape(s)
	s.Move(0.0, 0.0)
	after := snapshotShape(s)
	if diffs := diffShapeSnapshots(before, after); len(diffs) > 0 {
		t.Errorf("Move(0,0) was not a no-op: %v", diffs)
	}
}

// 6) Inheritance: master isolation under Move.
func TestMoveContract_MasterIsolation(t *testing.T) {
	data := loadFixtureBytes(t, moveFixture)
	AssertMasterIsolation(t, data, "Can.15", func(s *Shape) {
		s.Move(1.0, 1.0)
	})
}

// CONNECTOR ENDPOINT TEST — the audit's specific finding. A 1D connector
// has BOTH BeginX/Y and EndX/Y endpoints. Move must translate BOTH by the
// delta; otherwise the connector stretches or skews. Vsdx-go currently
// updates BeginX/Y only — this test is expected to FAIL, surfacing the bug.
func TestMoveContract_ConnectorBothEndpointsMove(t *testing.T) {
	data := loadFixtureBytes(t, moveFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	// Find a connector — any shape with BeginX.
	conn := findShape(t, v, func(s *Shape) bool { return s.HasBeginX() })

	bx0, by0 := conn.BeginX(), conn.BeginY()
	ex0, ey0 := conn.EndX(), conn.EndY()
	conn.Move(1.5, -0.5)

	if got := conn.BeginX(); math.Abs(got-(bx0+1.5)) > 1e-9 {
		t.Errorf("BeginX = %v, want %v", got, bx0+1.5)
	}
	if got := conn.BeginY(); math.Abs(got-(by0-0.5)) > 1e-9 {
		t.Errorf("BeginY = %v, want %v", got, by0-0.5)
	}
	if got := conn.EndX(); math.Abs(got-(ex0+1.5)) > 1e-9 {
		t.Errorf("EndX = %v, want %v (END endpoint did not follow Move)", got, ex0+1.5)
	}
	if got := conn.EndY(); math.Abs(got-(ey0-0.5)) > 1e-9 {
		t.Errorf("EndY = %v, want %v (END endpoint did not follow Move)", got, ey0-0.5)
	}
}
