package vsdx

import (
	"strings"
	"testing"
)

// a regression case: introduce explicit "demote a local cell to master inheritance"
// API. Spec §2.2.5.4.6 supports both directions (local override + inherited)
// via the presence/absence of the local <Cell>. vsdx-go could add but not
// remove — RemoveCell / RevertToInherited closes that gap.

func TestEC010_RemoveCell_FallsBackToMaster(t *testing.T) {
	v, err := Open(testFile("test_master.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	// Find any instance with a master; override its FillForegnd locally,
	// then revert.
	target := findShape(t, v, func(s *Shape) bool { return s.MasterPageID != "" })
	target.SetFillColor("#abc123")
	if got := target.FillColor(); got != "#abc123" {
		t.Fatalf("after SetFillColor: got %q, want %q", got, "#abc123")
	}
	// Confirm a local cell is present.
	if target.XML().FindElement("Cell[@N='FillForegnd']") == nil {
		t.Fatal("expected local FillForegnd cell after SetFillColor")
	}

	if !target.RemoveCell(CellFillForegnd) {
		t.Error("RemoveCell returned false; expected true (a local cell existed)")
	}
	// Local cell must be gone.
	if target.XML().FindElement("Cell[@N='FillForegnd']") != nil {
		t.Error("local FillForegnd cell still present after RemoveCell")
	}
	// Reading the color now must NOT return the overridden value.
	if got := target.FillColor(); got == "#abc123" {
		t.Errorf("FillColor after RemoveCell still returns the overridden value %q — inheritance not restored", got)
	}
}

func TestEC010_RemoveCell_ReturnsFalseWhenAbsent(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	s := v.GetPage(0).AddShape()
	// Brand-new shape: no local LineWeight, no master link.
	if s.RemoveCell(CellLineWeight) {
		t.Error("RemoveCell on absent cell returned true; want false")
	}
}

func TestEC010_RevertToInheritedIsAlias(t *testing.T) {
	v, err := Open(testFile("test_master.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	target := findShape(t, v, func(s *Shape) bool { return s.MasterPageID != "" })
	target.SetLineColor("#fedcba")

	if !target.RevertToInherited(CellLineColor) {
		t.Error("RevertToInherited returned false")
	}
	// XML should no longer have a LineColor cell.
	if elem := target.XML().FindElement("Cell[@N='LineColor']"); elem != nil {
		t.Errorf("LineColor still present after RevertToInherited: %s",
			strings.TrimSpace(elem.SelectAttrValue("V", "")))
	}
}
