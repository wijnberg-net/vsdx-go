package vsdx

import (
	"math"
	"testing"
)

// a regression case: minimal viable dependency walker. RecalculateDependents reads the
// V of cells whose F references the named cell as a token, and writes the
// CalcValue result. One level deep, single shape.

func TestEC012_RecalculateDependents_PropagatesOneLevel(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	s := v.GetPage(0).AddShape()

	// Set up: PinX = 5 (literal), Width = PinX + 1 (formula).
	s.SetX(5.0)
	s.SetCellFormula(CellWidth, "PinX+1")

	// Change PinX → Width's V is now stale.
	s.SetX(10.0)
	stale := s.Width()

	updated := s.RecalculateDependents(CellPinX)
	if updated == 0 {
		t.Fatal("RecalculateDependents reported 0 updated cells; expected Width to be re-evaluated")
	}
	got := s.Width()
	if math.Abs(got-11.0) > 1e-9 {
		t.Errorf("Width after recalc = %v, want 11 (PinX=10 + 1); stale was %v", got, stale)
	}
}

// Cells whose formula does NOT reference the named cell must be left alone.
func TestEC012_RecalculateDependents_LeavesUnrelatedAlone(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	s := v.GetPage(0).AddShape()
	s.SetX(5.0)
	s.SetCellFormula(CellWidth, "3.14")
	originalW := s.Width()

	updated := s.RecalculateDependents(CellPinX)
	if updated != 0 {
		t.Errorf("RecalculateDependents updated %d cells; expected 0 (no formula references PinX)", updated)
	}
	if math.Abs(s.Width()-originalW) > 1e-9 {
		t.Errorf("Width was modified: %v → %v", originalW, s.Width())
	}
}

// Word-boundary discrimination: a formula containing "PinXSubstring" must
// NOT match "PinX" as a dependency.
func TestEC012_RecalculateDependents_WordBoundary(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	s := v.GetPage(0).AddShape()
	s.SetX(5.0)
	// Synthetic formula that contains PinX as a substring (PinXY) but
	// not as a token. The cell's name happens to be Width so we can
	// observe whether it gets re-evaluated.
	s.SetCellFormula(CellWidth, "PinXY * 2")
	beforeWidth := s.Width()

	updated := s.RecalculateDependents(CellPinX)
	if updated != 0 {
		t.Errorf("RecalculateDependents updated %d cells; expected 0 (PinX token not present)", updated)
	}
	// Width's V must be unchanged regardless of CalcValue's success on the
	// nonsense formula.
	if math.Abs(s.Width()-beforeWidth) > 1e-9 {
		t.Errorf("Width changed despite no PinX token in formula")
	}
}
