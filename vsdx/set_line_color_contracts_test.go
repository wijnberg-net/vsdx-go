package vsdx

import (
	"strings"
	"testing"
)

// SetLineColor mirrors SetFillColor's contract on the line-color cell. Same
// THEMEGUARD policy: an explicit literal must clear F so the user's color
// survives Visio's next re-evaluation pass.

const lineFixture = setWidthFixture

func TestSetLineColorContract_SetAndRead(t *testing.T) {
	data := loadFixtureBytes(t, lineFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	s.SetLineColor("#ff0000")
	if got := s.LineColor(); got != "#ff0000" {
		t.Errorf("LineColor() = %q, want %q", got, "#ff0000")
	}
}

func TestSetLineColorContract_RoundTripXML(t *testing.T) {
	data := loadFixtureBytes(t, lineFixture)
	AssertRoundTripXML(t, data,
		func(v *VisioFile) {
			findShapeByName(t, v, "Rounded Rectangle.5").SetLineColor("#ff0000")
		},
		func(t *testing.T, v *VisioFile) {
			if got := findShapeByName(t, v, "Rounded Rectangle.5").LineColor(); got != "#ff0000" {
				t.Errorf("after round-trip LineColor() = %q, want %q", got, "#ff0000")
			}
		},
	)
}

func TestSetLineColorContract_MasterIsolation(t *testing.T) {
	data := loadFixtureBytes(t, lineFixture)
	AssertMasterIsolation(t, data, "Can.15", func(s *Shape) {
		s.SetLineColor("#ff0000")
	})
}

func TestSetLineColorContract_FAttributePolicy(t *testing.T) {
	data := loadFixtureBytes(t, lineFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	// Find a shape whose LineColor cell carries a THEMEGUARD formula. The
	// Dynamic connectors in this fixture have F="THEMEGUARD(RGB(0,0,0))".
	s := findShape(t, v, func(s *Shape) bool {
		return strings.Contains(s.CellFormula(CellLineColor), "THEMEGUARD")
	})
	s.SetLineColor("#ff00ff")
	instCell := s.XML().FindElement("Cell[@N='LineColor']")
	if instCell == nil {
		t.Fatal("SetLineColor did not create a local LineColor cell")
	}
	if gotV := instCell.SelectAttrValue("V", ""); gotV != "#ff00ff" {
		t.Errorf("local V = %q, want %q", gotV, "#ff00ff")
	}
	if gotF := instCell.SelectAttrValue("F", ""); strings.Contains(gotF, "THEMEGUARD") {
		t.Errorf("F=%q still THEMEGUARD-bound after explicit color override", gotF)
	}
}
