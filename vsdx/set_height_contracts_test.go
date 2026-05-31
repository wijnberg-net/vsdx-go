package vsdx

import "testing"

// SetHeight mirrors SetWidth on the Y axis. Same code path (scaleGeometryAxis
// + scaleChildShapeAxis), just with absVal() applied because Visio connectors
// store negative Height when EndY < BeginY. We rerun the SetWidth contract
// matrix on the Y axis and add one extra test for sign-preservation.

const setHeightFixture = setWidthFixture // re-use logical-architecture.vsdx

func TestSetHeightContract_SetAndRead(t *testing.T) {
	data := loadFixtureBytes(t, setHeightFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	s.SetHeight(3.0)
	if got := s.Height(); got != 3.0 {
		t.Errorf("Height() = %v, want 3.0", got)
	}
}

func TestSetHeightContract_SideEffects(t *testing.T) {
	data := loadFixtureBytes(t, setHeightFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	before := snapshotShape(s)
	s.SetHeight(s.Height() * 2.0)
	after := snapshotShape(s)
	assertOnlyTheseFieldsChanged(t, before, after, []string{"H", "LocY"})
}

func TestSetHeightContract_RoundTripXML(t *testing.T) {
	data := loadFixtureBytes(t, setHeightFixture)
	AssertRoundTripXML(t, data,
		func(v *VisioFile) {
			findShapeByName(t, v, "Rounded Rectangle.5").SetHeight(3.0)
		},
		func(t *testing.T, v *VisioFile) {
			s := findShapeByName(t, v, "Rounded Rectangle.5")
			if got := s.Height(); got != 3.0 {
				t.Errorf("after round-trip Height() = %v, want 3.0", got)
			}
		},
	)
}

func TestSetHeightContract_Idempotent(t *testing.T) {
	data := loadFixtureBytes(t, setHeightFixture)
	AssertIdempotent(t, data,
		func(v *VisioFile) {
			findShapeByName(t, v, "Rounded Rectangle.5").SetHeight(2.5)
		},
		func(v *VisioFile) string {
			return snapshotShape(findShapeByName(t, v, "Rounded Rectangle.5")).XMLHash
		},
	)
}

func TestSetHeightContract_OrderWithSetY(t *testing.T) {
	data := loadFixtureBytes(t, setHeightFixture)
	mutators := []func(*VisioFile){
		func(v *VisioFile) { findShapeByName(t, v, "Rounded Rectangle.5").SetHeight(2.5) },
		func(v *VisioFile) { findShapeByName(t, v, "Rounded Rectangle.5").SetY(5.0) },
	}
	AssertOrderIndependent(t, data, mutators, func(v *VisioFile) any {
		s := findShapeByName(t, v, "Rounded Rectangle.5")
		return [4]float64{s.X(), s.Y(), s.Height(), s.LocY()}
	})
}

func TestSetHeightContract_MasterIsolation(t *testing.T) {
	data := loadFixtureBytes(t, setHeightFixture)
	AssertMasterIsolation(t, data, "Can.15", func(s *Shape) {
		s.SetHeight(s.Height() * 2.0)
	})
}

func TestSetHeightContract_CreatesLocalGeometry(t *testing.T) {
	data := loadFixtureBytes(t, setHeightFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Can.15")
	AssertLocalGeometryCreated(t, s, func(s *Shape) {
		s.SetHeight(s.Height() * 2.0)
	})
}

// NEW vs SetWidth: connectors store NEGATIVE Height when their end point is
// above the begin point (Visio convention). SetHeight on such a shape must
// preserve the sign — flipping it would reverse the connector's direction.
// Scaling logic uses absVal() ratios precisely to avoid this; pin it down.
func TestSetHeightContract_NegativeHeightSignPreserved(t *testing.T) {
	data := loadFixtureBytes(t, setHeightFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	// Dynamic connector.22 has Height = -0.487 in the fixture (verified earlier).
	conn := findShape(t, v, func(s *Shape) bool {
		return s.ShapeName == "Dynamic connector" && s.Height() < 0
	})
	originalSign := conn.Height() < 0
	if !originalSign {
		t.Skip("expected a connector with negative Height in the fixture")
	}
	// Double the magnitude. SetHeight should preserve the sign because
	// scaleGeometryAxis treats sign via |v|/|old|.
	conn.SetHeight(conn.Height() * 2.0)
	if conn.Height() >= 0 {
		t.Errorf("SetHeight flipped the sign: Height() = %v, expected to stay negative", conn.Height())
	}
}
