package vsdx

import (
	"testing"
)

// a regression case: ensureSectionCell used to create a partial local Row when an
// instance overrode one cell in a master section (Character, Paragraph,
// etc.). Visio interprets a present-but-partial local Row as fully local,
// silently dropping every cell the master had on the same row.
//
// The fix: when creating a NEW local Row that shadows a master Row, copy
// the full set of master cells into the local Row first, then update the
// one we wanted to write. Subsequent calls into the same Row only touch
// the requested cell.
//
// The fixture is test4_connectors.vsdx, whose master2.xml carries a
// Character section with both Color (theme-bound) and Size.

func TestEC006_CharacterRowPreservesSiblingCells(t *testing.T) {
	v, err := Open(testFile("test4_connectors.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	// Shape 6 (Dynamic connector) uses Master=2, which has Character/Color
	// + Size at Row IX=0.
	target := findShape(t, v, func(s *Shape) bool {
		if s.MasterPageID == "" {
			return false
		}
		ms := s.MasterShape()
		if ms == nil {
			return false
		}
		// Master must have a Character row with at least 2 cells.
		msRow := ms.XML().FindElement("Section[@N='Character']/Row")
		if msRow == nil {
			return false
		}
		return len(msRow.SelectElements("Cell")) >= 2
	})

	// Snapshot what the master Row carries.
	msRow := target.MasterShape().XML().FindElement("Section[@N='Character']/Row")
	expectedCells := map[string]bool{}
	for _, c := range msRow.SelectElements("Cell") {
		if name := c.SelectAttrValue("N", ""); name != "" {
			expectedCells[name] = true
		}
	}
	if len(expectedCells) < 2 {
		t.Skipf("master Character row has only %d cells; a regression case needs >=2 to demonstrate", len(expectedCells))
	}

	// Mutate ONE cell on the instance. Without the fix, this drops the others.
	target.SetTextSize(0.05)

	// Inspect the instance's resulting local Character row.
	localRow := target.XML().FindElement("Section[@N='Character']/Row")
	if localRow == nil {
		t.Fatal("instance has no local Character row after SetTextSize")
	}
	gotCells := map[string]bool{}
	for _, c := range localRow.SelectElements("Cell") {
		if name := c.SelectAttrValue("N", ""); name != "" {
			gotCells[name] = true
		}
	}
	for name := range expectedCells {
		if !gotCells[name] {
			t.Errorf("local Character row missing cell %q — a regression case regression (Visio would treat partial row as fully local and drop inherited %q)",
				name, name)
		}
	}
}

// Idempotence with the fix: calling SetTextSize twice should not duplicate
// the inherited sibling cells (e.g. two <Cell N="Color"> entries).
func TestEC006_CharacterRowIdempotentAcrossMultipleSets(t *testing.T) {
	v, err := Open(testFile("test4_connectors.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	target := findShape(t, v, func(s *Shape) bool {
		if s.MasterShape() == nil {
			return false
		}
		row := s.MasterShape().XML().FindElement("Section[@N='Character']/Row")
		return row != nil && len(row.SelectElements("Cell")) >= 2
	})

	target.SetTextSize(0.05)
	target.SetTextSize(0.08)

	localRow := target.XML().FindElement("Section[@N='Character']/Row")
	if localRow == nil {
		t.Fatal("no local Character row")
	}
	seen := map[string]int{}
	for _, c := range localRow.SelectElements("Cell") {
		name := c.SelectAttrValue("N", "")
		seen[name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("cell %q duplicated %d times in local Character row", name, count)
		}
	}
}
