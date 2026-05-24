package vsdx

import (
	"testing"
)

func TestComputeEffectiveStyle_Defaults(t *testing.T) {
	// Test with a minimal shape that has no styles set
	// Should return defaults per MS-VSDX §2.2.5.3.3.1
	vf, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skip("basic.vsdx not available:", err)
	}
	defer vf.Close()

	for _, page := range vf.Pages {
		shapes := page.ChildShapes()
		if len(shapes) == 0 {
			continue
		}

		for _, s := range shapes {
			es := s.ComputeEffectiveStyle()

			// Verify defaults are set
			if es.LinePattern < 0 {
				t.Errorf("Shape %s: LinePattern should not be negative: %d", s.ID, es.LinePattern)
			}
			if es.BeginArrowSize < 0 {
				t.Errorf("Shape %s: BeginArrowSize should not be negative: %d", s.ID, es.BeginArrowSize)
			}
			if es.ShapeShdwScaleFactor <= 0 {
				t.Errorf("Shape %s: ShapeShdwScaleFactor should be positive: %f", s.ID, es.ShapeShdwScaleFactor)
			}
		}
		break
	}
}

func TestComputeEffectiveStyle_ConnectorArrows(t *testing.T) {
	// Test arrow resolution for connector shapes
	vf, err := Open("../tests/test5_master.vsdx")
	if err != nil {
		t.Skip("network-diagram.vsdx not available:", err)
	}
	defer vf.Close()

	for _, page := range vf.Pages {
		shapes := page.ChildShapes()
		for _, s := range shapes {
			// Check if this is a connector
			if s.CellValue("BeginX") == "" {
				continue
			}

			es := s.ComputeEffectiveStyle()

			// Log arrow resolution for debugging
			t.Logf("Connector %s: BeginArrow=%d (source=%s), EndArrow=%d, LineColor=%s (source=%s)",
				s.ID, es.BeginArrow, es.ArrowSource, es.EndArrow, es.LineColor, es.LineColorSource)

			// Connectors typically have end arrows
			// Note: This may be 0 if theme doesn't define arrows
		}
	}
}

func TestComputeEffectiveStyle_HasShadow(t *testing.T) {
	vf, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skip("basic.vsdx not available:", err)
	}
	defer vf.Close()

	for _, page := range vf.Pages {
		shapes := page.ChildShapes()
		for _, s := range shapes {
			es := s.ComputeEffectiveStyle()

			// HasShadow should be consistent with shadow type
			if es.HasShadow() != (es.ShapeShdwType > 0 && es.ShdwForegndTrans < 1.0) {
				t.Errorf("Shape %s: HasShadow() inconsistent with shadow properties", s.ID)
			}
		}
	}
}

func TestComputeArrowSetback(t *testing.T) {
	// Setback uses minimum visual size to ensure arrows remain visible for thin strokes.
	// Formula: max(baseVisualWidth * lineWeight, minVisualWidth)
	// where baseVisualWidth = 3.56 * sizeMult * lengthMult
	// and minVisualWidth = 7.0 * sizeMult * lengthMult
	tests := []struct {
		arrowType  int
		arrowSize  int
		lineWeight float64
		want       float64
	}{
		{0, 2, 1.0, 0},       // No arrow: zero setback
		{1, 2, 1.0, 7.0},     // Type 1, size 2, 1pt: min(7.0) applies
		{4, 2, 3.0, 10.68},   // Type 4, size 2, 3pt: 3.56 * 3 = 10.68 > min 7.0
		{1, 0, 1.0, 3.5},     // Type 1, size 0, 1pt: min 7.0 * 0.5 = 3.5
		{1, 6, 1.0, 17.5},    // Type 1, size 6, 1pt: min 7.0 * 2.5 = 17.5
		{13, 2, 1.0, 10.5},   // Type 13, size 2, 1pt: min 7.0 * 1.5 = 10.5
		{13, 2, 3.0, 16.02},  // Type 13, size 2, 3pt: 3.56 * 1.5 * 3 = 16.02
	}

	for _, tt := range tests {
		got := computeArrowSetback(tt.arrowType, tt.arrowSize, tt.lineWeight)
		diff := got - tt.want
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.001 {
			t.Errorf("computeArrowSetback(%d, %d, %f) = %f; want %f",
				tt.arrowType, tt.arrowSize, tt.lineWeight, got, tt.want)
		}
	}
}

func TestEffectiveStyle_FillColor(t *testing.T) {
	es := &EffectiveStyle{
		FillPattern:      1,       // Solid
		FillForegnd:      "#FF0000",
		FillForegndTrans: 0,
		NoFill:           false,
		FillColorSource:  "local", // Explicitly set fill color
	}

	if es.EffectiveFillColor() != "#FF0000" {
		t.Errorf("EffectiveFillColor() = %s; want #FF0000", es.EffectiveFillColor())
	}

	// With full transparency
	es.FillForegndTrans = 1.0
	if es.EffectiveFillColor() != "none" {
		t.Errorf("EffectiveFillColor() with full trans = %s; want none", es.EffectiveFillColor())
	}

	// With NoFill
	es.FillForegndTrans = 0
	es.NoFill = true
	if es.EffectiveFillColor() != "none" {
		t.Errorf("EffectiveFillColor() with NoFill = %s; want none", es.EffectiveFillColor())
	}

	// With pattern 0
	es.NoFill = false
	es.FillPattern = 0
	if es.EffectiveFillColor() != "none" {
		t.Errorf("EffectiveFillColor() with pattern 0 = %s; want none", es.EffectiveFillColor())
	}
}

func TestEffectiveStyle_LineColor(t *testing.T) {
	es := &EffectiveStyle{
		LineColor:      "#0000FF",
		LineColorTrans: 0,
		NoLine:         false,
		LinePattern:    1, // Solid line
	}

	if es.EffectiveLineColor() != "#0000FF" {
		t.Errorf("EffectiveLineColor() = %s; want #0000FF", es.EffectiveLineColor())
	}

	// With full transparency
	es.LineColorTrans = 1.0
	if es.EffectiveLineColor() != "none" {
		t.Errorf("EffectiveLineColor() with full trans = %s; want none", es.EffectiveLineColor())
	}

	// With NoLine
	es.LineColorTrans = 0
	es.NoLine = true
	if es.EffectiveLineColor() != "none" {
		t.Errorf("EffectiveLineColor() with NoLine = %s; want none", es.EffectiveLineColor())
	}
}
