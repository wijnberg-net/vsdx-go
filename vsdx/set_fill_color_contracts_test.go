package vsdx

import (
	"strings"
	"testing"
)

// SetFillColor is the canonical case for the audit's "F-attribute policy"
// finding: ensureCell copies all master cell attributes EXCEPT the one being
// written. So calling SetFillColor on a themed cell (F="THEMEGUARD(RGB(...))")
// produces a cell with the NEW V and the OLD F. Per MS-VSDX §2.3.4.2.5, V is
// a fallback "until formula evaluation is triggered" — meaning Visio's next
// open will re-evaluate F (the original theme color) and overwrite the
// user's edit. vsdx-go's own reader uses V so the inconsistency only shows
// up after a Visio round-trip, but it's a real spec violation worth pinning.

const fillFixture = setWidthFixture // Rounded Rectangle.5 has a themed fill.

func TestSetFillColorContract_SetAndRead(t *testing.T) {
	data := loadFixtureBytes(t, fillFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	s.SetFillColor("#ff0000")
	if got := s.FillColor(); got != "#ff0000" {
		t.Errorf("FillColor() = %q, want %q", got, "#ff0000")
	}
}

func TestSetFillColorContract_RoundTripXML(t *testing.T) {
	data := loadFixtureBytes(t, fillFixture)
	AssertRoundTripXML(t, data,
		func(v *VisioFile) {
			findShapeByName(t, v, "Rounded Rectangle.5").SetFillColor("#ff0000")
		},
		func(t *testing.T, v *VisioFile) {
			s := findShapeByName(t, v, "Rounded Rectangle.5")
			if got := s.FillColor(); got != "#ff0000" {
				t.Errorf("after round-trip FillColor() = %q, want %q", got, "#ff0000")
			}
		},
	)
}

func TestSetFillColorContract_Idempotent(t *testing.T) {
	data := loadFixtureBytes(t, fillFixture)
	AssertIdempotent(t, data,
		func(v *VisioFile) {
			findShapeByName(t, v, "Rounded Rectangle.5").SetFillColor("#ff0000")
		},
		func(v *VisioFile) string {
			return findShapeByName(t, v, "Rounded Rectangle.5").FillColor()
		},
	)
}

// The Phase-1 master-isolation guarantee should apply just as much to color
// setters. A themed master cell shared across instances would otherwise be
// vulnerable to the same kind of leak as a shared Geometry section.
func TestSetFillColorContract_MasterIsolation(t *testing.T) {
	data := loadFixtureBytes(t, fillFixture)
	AssertMasterIsolation(t, data, "Can.15", func(s *Shape) {
		s.SetFillColor("#ff0000")
	})
}

// THE FINDING TEST. After SetFillColor on a themed cell, the cell's F
// attribute must NOT still reference the master's original theme color —
// otherwise on Visio's next open the formula re-evaluates and discards the
// user's edit.
//
// We accept either policy: F empty (cleared) or F="THEMEGUARD(RGB(255,0,0))"
// (rewritten to track the new color). What we reject is F being the stale
// THEMEGUARD that points back to the original theme.
func TestSetFillColorContract_FAttributePolicy(t *testing.T) {
	data := loadFixtureBytes(t, fillFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")

	// Sanity: the fill cell — either local or inherited — has a THEMEGUARD
	// formula tied to the original theme color. Without this precondition
	// the test has nothing to verify.
	beforeF := s.CellFormula(CellFillForegnd)
	if !strings.Contains(beforeF, "THEMEGUARD") {
		t.Skipf("fixture's FillForegnd is not theme-guarded (F=%q); test does not apply", beforeF)
	}

	s.SetFillColor("#ff0000")

	// Find the LOCAL FillForegnd cell on the instance and inspect F.
	instCell := s.XML().FindElement("Cell[@N='FillForegnd']")
	if instCell == nil {
		t.Fatal("SetFillColor did not create a local FillForegnd cell")
	}
	gotV := instCell.SelectAttrValue("V", "")
	gotF := instCell.SelectAttrValue("F", "")
	if gotV != "#ff0000" {
		t.Errorf("local V = %q, want %q", gotV, "#ff0000")
	}
	// Reject: F still pointing at the master's theme color.
	if strings.Contains(gotF, "0,112,192") {
		t.Errorf("F=%q still references the master's theme color — on Visio re-open the formula would overwrite V and discard the user's edit",
			gotF)
	}
}
