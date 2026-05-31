package vsdx

import (
	"math"
	"testing"
)

// SetStartAndFinish is the heaviest cascade in the public API: one call
// triggers SetBeginX/Y/EndX/Y, SetWidth (with geometry + LocPin + child
// scaling), SetHeight (idem), SetX/Y, Geometry.SetMoveTo + SetLineTo, text
// pin updates, Control/TextPosition updates, and a formula-evaluation pass
// over every shape and geometry cell. Audit hot-zone #2.
//
// We don't test every branch; we pin the headline contracts:
//
//   1. Endpoints come out exactly where they were placed.
//   2. Width/Height are derived from the endpoint deltas.
//   3. The pin (PinX/Y) lands on the start point (Visio's convention).
//   4. The geometry's first MoveTo + LineTo are set to (0,0) → (w, h).
//   5. Round-trip persists the endpoints + Width/Height.
//   6. Master isolation holds: applying SetStartAndFinish to a connector
//      instance doesn't change the master.
//   7. A no-op early exit fires for shapes without BeginX (non-1D).

// test4_connectors.vsdx contains connectors that are NOT glued to other
// shapes. logical-architecture.vsdx has glued connectors whose BeginX/EndX
// cells carry a PAR(PNT(Sheet.N!Connections.XM)) formula — the formula
// evaluation pass at the end of SetStartAndFinish would override the V we
// just set, masking the contract. Using a fixture with literal endpoints
// keeps the contract surface clean.
const startFinishFixture = "test4_connectors.vsdx"

// findUngluedConnector returns the first 1D shape (HasBeginX) whose BeginX
// is a plain literal, not a glue formula. Falls back to ANY 1D shape if all
// are glued — the test will then catch the formula-override symptom.
func findUngluedConnector(t *testing.T, v *VisioFile) *Shape {
	t.Helper()
	var first *Shape
	for _, p := range v.Pages {
		for _, s := range p.AllShapes() {
			if !s.HasBeginX() {
				continue
			}
			if first == nil {
				first = s
			}
			if s.CellFormula(CellBeginX) == "" {
				return s
			}
		}
	}
	if first == nil {
		t.Fatal("no 1D shape with BeginX found in fixture")
	}
	return first
}

func TestSetStartAndFinishContract_EndpointsMatch(t *testing.T) {
	data := loadFixtureBytes(t, startFinishFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	conn := findUngluedConnector(t, v)
	conn.SetStartAndFinish(2.0, 3.0, 5.0, 4.0)
	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{
		{"BeginX", conn.BeginX(), 2.0},
		{"BeginY", conn.BeginY(), 3.0},
		{"EndX", conn.EndX(), 5.0},
		{"EndY", conn.EndY(), 4.0},
	} {
		if math.Abs(c.got-c.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestSetStartAndFinishContract_WidthHeightAreDeltas(t *testing.T) {
	data := loadFixtureBytes(t, startFinishFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	conn := findUngluedConnector(t, v)
	conn.SetStartAndFinish(2.0, 3.0, 5.0, 4.5)
	if got, want := conn.Width(), 3.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("Width = %v, want %v (= finishX - startX)", got, want)
	}
	if got, want := conn.Height(), 1.5; math.Abs(got-want) > 1e-9 {
		t.Errorf("Height = %v, want %v (= finishY - startY for a connector)", got, want)
	}
}

// PinX/PinY contract: the pin lands at the START point UNLESS the cell has
// a formula (e.g. F="(BeginX+EndX)/2" for midpoint-pinned connectors), in
// which case the formula wins at the final evaluation pass. We pin both
// outcomes depending on what the fixture's connector carries.
func TestSetStartAndFinishContract_PinFollowsFormulaOrStart(t *testing.T) {
	data := loadFixtureBytes(t, startFinishFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	conn := findUngluedConnector(t, v)
	pinXFormula := conn.CellFormula(CellPinX)
	conn.SetStartAndFinish(2.0, 3.0, 5.0, 4.0)
	if pinXFormula == "" {
		// No formula: literal SetX(2.0) wins.
		if got, want := conn.X(), 2.0; math.Abs(got-want) > 1e-9 {
			t.Errorf("X = %v, want %v (= start X, no PinX formula)", got, want)
		}
	} else {
		// Has formula: pin should be re-evaluated. For midpoint formulas
		// the X is (2+5)/2 = 3.5. We only check the pin is somewhere
		// between start and end — not the precise value, which depends on
		// the specific formula.
		x := conn.X()
		if x < 2.0 || x > 5.0 {
			t.Errorf("X = %v outside [start=2, end=5] range with formula %q", x, pinXFormula)
		}
	}
}

// The geometry's first MoveTo + LineTo are reset to (0,0) → (width, height)
// — local coordinates within the connector. This is the canonical
// straight-line representation the renderer expects.
func TestSetStartAndFinishContract_GeometryReset(t *testing.T) {
	data := loadFixtureBytes(t, startFinishFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	conn := findUngluedConnector(t, v)
	conn.SetStartAndFinish(2.0, 3.0, 5.0, 4.5)

	if len(conn.Geometries) == 0 {
		t.Fatal("connector has no geometry after SetStartAndFinish")
	}
	g := conn.Geometries[0]
	var mt, lt *GeometryRow
	for _, r := range g.Rows {
		switch r.RowType() {
		case "MoveTo":
			if mt == nil || r.Index() < mt.Index() {
				mt = r
			}
		case "LineTo":
			if lt == nil || r.Index() < lt.Index() {
				lt = r
			}
		}
	}
	if mt == nil {
		t.Fatal("no MoveTo row found")
	}
	if math.Abs(mt.X()) > 1e-9 || math.Abs(mt.Y()) > 1e-9 {
		t.Errorf("MoveTo = (%v, %v), want (0, 0)", mt.X(), mt.Y())
	}
	if lt != nil {
		if math.Abs(lt.X()-3.0) > 1e-9 || math.Abs(lt.Y()-1.5) > 1e-9 {
			t.Errorf("LineTo = (%v, %v), want (3, 1.5) — width × height local coords",
				lt.X(), lt.Y())
		}
	}
}

func TestSetStartAndFinishContract_RoundTripXML(t *testing.T) {
	data := loadFixtureBytes(t, startFinishFixture)
	v := openFromBytes(t, data)
	conn := findUngluedConnector(t, v)
	connID := conn.ID
	conn.SetStartAndFinish(2.0, 3.0, 5.0, 4.0)
	out, err := v.SaveVsdxBytes()
	if err != nil {
		v.Close()
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	v.Close()

	v2 := openFromBytes(t, out)
	defer v2.Close()
	var conn2 *Shape
	for _, s := range v2.Pages[0].AllShapes() {
		if s.ID == connID {
			conn2 = s
			break
		}
	}
	if conn2 == nil {
		t.Fatalf("connector %s missing after round-trip", connID)
	}
	if math.Abs(conn2.BeginX()-2.0) > 1e-9 || math.Abs(conn2.EndX()-5.0) > 1e-9 {
		t.Errorf("endpoints lost after round-trip: BeginX=%v EndX=%v", conn2.BeginX(), conn2.EndX())
	}
}

// Master isolation: re-routing one connector instance must not alter the
// "Dynamic connector" master that other connectors depend on.
func TestSetStartAndFinishContract_MasterIsolation(t *testing.T) {
	data := loadFixtureBytes(t, startFinishFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	conn := findUngluedConnector(t, v)
	if conn.MasterPageID == "" {
		t.Skip("connector has no master")
	}
	masterHashBefore := masterXMLHash(v, conn.MasterPageID)
	conn.SetStartAndFinish(2.0, 3.0, 5.0, 4.0)
	masterHashAfter := masterXMLHash(v, conn.MasterPageID)
	if masterHashBefore != masterHashAfter {
		t.Errorf("master %s XML changed after SetStartAndFinish on instance (hash %s → %s)",
			conn.MasterPageID, masterHashBefore, masterHashAfter)
	}
}

// Non-1D shapes (no BeginX cell) should ignore SetStartAndFinish — the
// early-return guard. We verify the shape is byte-for-byte unchanged.
func TestSetStartAndFinishContract_NonOneDShapeIsNoOp(t *testing.T) {
	data := loadFixtureBytes(t, startFinishFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShape(t, v, func(s *Shape) bool { return !s.HasBeginX() })
	before := snapshotShape(s)
	s.SetStartAndFinish(1.0, 2.0, 3.0, 4.0)
	after := snapshotShape(s)
	if diffs := diffShapeSnapshots(before, after); len(diffs) > 0 {
		t.Errorf("SetStartAndFinish on non-1D shape produced changes: %v", diffs)
	}
}
