package vsdx

import (
	"strings"
	"testing"
)

// a regression case: extend the F-attribute clear policy (already applied to
// SetFillColor / SetLineColor) to every Laag-B setter that takes an
// explicit literal user value. Without the clear, a master cell with
// F="THEMEGUARD(...)" or F="Inh" survives on the local cell and Visio's
// next open re-evaluates F over the user's V.
//
// The setters covered here:
//
//   - SetLineWeight, SetLinePattern, SetLineCap, SetBeginArrow, SetEndArrow
//   - SetRounding
//   - SetFillPattern, SetFillTransparency, SetFillBkgndColor,
//     SetFillBkgndTransparency
//
// Each test follows the same pattern: find a shape whose cell has a
// non-empty F attribute, mutate, then assert the local cell's F is empty.

const ec011Fixture = setWidthFixture

// helper: find a shape whose named cell has a non-empty F attribute.
func findShapeWithCellFormula(t *testing.T, v *VisioFile, cellName string) *Shape {
	t.Helper()
	for _, p := range v.Pages {
		for _, s := range p.AllShapes() {
			if s.CellFormula(cellName) != "" {
				return s
			}
		}
	}
	t.Skipf("no shape with non-empty F on cell %q in fixture", cellName)
	return nil
}

// helper: assert the local cell on a shape has no F attribute. Works whether
// or not the original master had a formula.
func assertCellFormulaCleared(t *testing.T, s *Shape, cellName string) {
	t.Helper()
	local := s.XML().FindElement("Cell[@N='" + cellName + "']")
	if local == nil {
		t.Errorf("no local cell %q on shape %s after mutation", cellName, s.ID)
		return
	}
	if got := local.SelectAttrValue("F", ""); got != "" {
		t.Errorf("local cell %q on shape %s still has F=%q after literal override", cellName, s.ID, got)
	}
}

func TestEC011_SetLineWeight_ClearsFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeWithCellFormula(t, v, CellLineWeight)
	s.SetLineWeight(0.5)
	assertCellFormulaCleared(t, s, CellLineWeight)
}

func TestEC011_SetLinePattern_ClearsFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeWithCellFormula(t, v, CellLinePattern)
	s.SetLinePattern(LinePatternDash)
	assertCellFormulaCleared(t, s, CellLinePattern)
}

func TestEC011_SetLineCap_ClearsFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeWithCellFormula(t, v, CellLineCap)
	s.SetLineCap(LineCapSquare)
	assertCellFormulaCleared(t, s, CellLineCap)
}

func TestEC011_SetBeginArrow_ClearsFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeWithCellFormula(t, v, CellBeginArrow)
	s.SetBeginArrow(13)
	assertCellFormulaCleared(t, s, CellBeginArrow)
}

func TestEC011_SetEndArrow_ClearsFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeWithCellFormula(t, v, CellEndArrow)
	s.SetEndArrow(13)
	assertCellFormulaCleared(t, s, CellEndArrow)
}

func TestEC011_SetRounding_ClearsFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeWithCellFormula(t, v, CellRounding)
	s.SetRounding(0.1)
	assertCellFormulaCleared(t, s, CellRounding)
}

func TestEC011_SetFillPattern_ClearsFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeWithCellFormula(t, v, CellFillPattern)
	s.SetFillPattern(1)
	assertCellFormulaCleared(t, s, CellFillPattern)
}

func TestEC011_SetFillTransparency_ClearsFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeWithCellFormula(t, v, CellFillForegndTrans)
	s.SetFillTransparency(0.5)
	assertCellFormulaCleared(t, s, CellFillForegndTrans)
}

func TestEC011_SetFillBkgndColor_ClearsFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeWithCellFormula(t, v, CellFillBkgnd)
	s.SetFillBkgndColor("#ff0000")
	assertCellFormulaCleared(t, s, CellFillBkgnd)
}

func TestEC011_SetFillBkgndTransparency_ClearsFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeWithCellFormula(t, v, CellFillBkgndTrans)
	s.SetFillBkgndTransparency(0.3)
	assertCellFormulaCleared(t, s, CellFillBkgndTrans)
}

// Synthetic coverage: programmatically install a formula on each target
// cell, then call the setter, then assert F is cleared. This covers the
// setters whose corresponding cell happened to have no formula in the
// real-world fixture above.
func TestEC011_SyntheticAllSetters_ClearFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)

	type sub struct {
		name     string
		cellName string
		mutate   func(*Shape)
	}
	subtests := []sub{
		{"SetLineWeight", CellLineWeight, func(s *Shape) { s.SetLineWeight(0.5) }},
		{"SetLinePattern", CellLinePattern, func(s *Shape) { s.SetLinePattern(LinePatternDash) }},
		{"SetLineCap", CellLineCap, func(s *Shape) { s.SetLineCap(LineCapSquare) }},
		{"SetBeginArrow", CellBeginArrow, func(s *Shape) { s.SetBeginArrow(13) }},
		{"SetEndArrow", CellEndArrow, func(s *Shape) { s.SetEndArrow(13) }},
		{"SetRounding", CellRounding, func(s *Shape) { s.SetRounding(0.1) }},
		{"SetFillPattern", CellFillPattern, func(s *Shape) { s.SetFillPattern(1) }},
		{"SetFillTransparency", CellFillForegndTrans, func(s *Shape) { s.SetFillTransparency(0.5) }},
		{"SetFillBkgndColor", CellFillBkgnd, func(s *Shape) { s.SetFillBkgndColor("#ff0000") }},
		{"SetFillBkgndTransparency", CellFillBkgndTrans, func(s *Shape) { s.SetFillBkgndTransparency(0.3) }},
	}

	for _, st := range subtests {
		t.Run(st.name, func(t *testing.T) {
			v := openFromBytes(t, data)
			defer v.Close()
			s := findShapeByName(t, v, "Rounded Rectangle.5")
			// Install a synthetic formula on the target cell.
			s.SetCellFormula(st.cellName, "THEMEGUARD(0)")
			if got := s.CellFormula(st.cellName); got == "" {
				t.Skipf("could not install synthetic F on cell %q; SetCellFormula did nothing", st.cellName)
			}
			// Apply the user-literal mutation.
			st.mutate(s)
			// Verify F is cleared.
			assertCellFormulaCleared(t, s, st.cellName)
		})
	}
}

// Cross-cutting: assert the original FillColor / LineColor F-policy still
// works (regression for prior fixes that should not be re-broken).
func TestEC011_FillColorAndLineColor_StillClearFormula(t *testing.T) {
	data := loadFixtureBytes(t, ec011Fixture)
	v := openFromBytes(t, data)
	defer v.Close()

	for _, tc := range []struct {
		name   string
		cell   string
		mutate func(*Shape)
	}{
		{"SetFillColor", CellFillForegnd, func(s *Shape) { s.SetFillColor("#abcdef") }},
		{"SetLineColor", CellLineColor, func(s *Shape) { s.SetLineColor("#abcdef") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := findShape(t, v, func(s *Shape) bool {
				return strings.Contains(s.CellFormula(tc.cell), "THEMEGUARD")
			})
			tc.mutate(s)
			assertCellFormulaCleared(t, s, tc.cell)
		})
	}
}
