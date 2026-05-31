package vsdx

import (
	"strings"
	"testing"
)

// SetText is the only Laag-C mutator that does NOT have a localize-style
// inheritance fix, because Text inheritance works via the master-fallback in
// Text() rather than via cell aliasing.
//
// a regression case contract (closed 2026-05-29): SetText fully replaces the Text
// element body. Any <cp>/<pp>/<fld> format-marker children are REMOVED
// because their indices reference text positions that no longer exist
// after the rewrite. The Character/Paragraph/Field SECTIONS on the shape
// are not touched — only the in-text markers. Callers who want format-rich
// text must re-insert markers themselves.

func TestSetTextContract_SetAndRead(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	s.SetText("Hello")
	if got := s.Text(); !strings.HasPrefix(got, "Hello") {
		t.Errorf("Text() = %q, want prefix %q", got, "Hello")
	}
}

func TestSetTextContract_RoundTripXML(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	AssertRoundTripXML(t, data,
		func(v *VisioFile) {
			findShapeByName(t, v, "Rounded Rectangle.5").SetText("ROUNDTRIPPED")
		},
		func(t *testing.T, v *VisioFile) {
			s := findShapeByName(t, v, "Rounded Rectangle.5")
			if !strings.HasPrefix(s.Text(), "ROUNDTRIPPED") {
				t.Errorf("after round-trip Text() = %q, want prefix %q", s.Text(), "ROUNDTRIPPED")
			}
		},
	)
}

func TestSetTextContract_EmptyClearsText(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	v := openFromBytes(t, data)
	defer v.Close()
	s := findShapeByName(t, v, "Rounded Rectangle.5")
	s.SetText("")
	if got := s.Text(); got != "" {
		t.Errorf("Text() after SetText(\"\") = %q, want empty", got)
	}
}

// Idempotence: SetText("X") then SetText("X") again must yield the same
// observable state. Without this, repeated assignments could compound (e.g.
// duplicate cp children).
func TestSetTextContract_Idempotent(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	AssertIdempotent(t, data,
		func(v *VisioFile) {
			findShapeByName(t, v, "Rounded Rectangle.5").SetText("REPEATED")
		},
		func(v *VisioFile) string {
			return snapshotShape(findShapeByName(t, v, "Rounded Rectangle.5")).XMLHash
		},
	)
}

// Master isolation: SetText on an instance must not change any other shape's
// text. Particular concern: shapes whose Text() falls back to master via the
// "no local Text element" path. We test with the Can instances which all
// share a master.
func TestSetTextContract_MasterIsolation(t *testing.T) {
	data := loadFixtureBytes(t, setWidthFixture)
	AssertMasterIsolation(t, data, "Can.15", func(s *Shape) {
		s.SetText("ISOLATED")
	})
}

// a regression case (closed): SetText strips all in-text format markers (<cp>, <pp>,
// <fld>). The shape's Character/Paragraph/Field SECTIONS remain untouched,
// so a caller that wants the old formatting can re-insert markers.
func TestSetTextContract_EC007_StripsFormatMarkerElements(t *testing.T) {
	v, err := Open(testFile("test12_colors.vsdx"))
	if err != nil {
		t.Skipf("test12_colors.vsdx not available: %v", err)
	}
	defer v.Close()

	// Find a shape with at least one <cp> child in its Text element.
	target := findShape(t, v, func(s *Shape) bool {
		textEl := s.XML().FindElement("Text")
		if textEl == nil {
			return false
		}
		for _, c := range textEl.ChildElements() {
			if c.Tag == "cp" {
				return true
			}
		}
		return false
	})

	textEl := target.XML().FindElement("Text")
	cpCountBefore := 0
	for _, c := range textEl.ChildElements() {
		if c.Tag == "cp" {
			cpCountBefore++
		}
	}
	if cpCountBefore == 0 {
		t.Skip("no <cp> children found in fixture")
	}

	target.SetText("OVERWRITTEN")

	textElAfter := target.XML().FindElement("Text")
	if textElAfter == nil {
		t.Fatal("Text element disappeared after SetText")
	}
	if n := len(textElAfter.ChildElements()); n != 0 {
		t.Errorf("Text element has %d child elements after SetText, want 0 (all format markers stripped)", n)
	}
	if got := target.Text(); !strings.Contains(got, "OVERWRITTEN") {
		t.Errorf("Text() = %q, expected to contain %q", got, "OVERWRITTEN")
	}
}
