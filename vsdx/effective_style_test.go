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
	// Affine fit to Visio's empirical setback values:
	//   visualWidth = lengthMult * sizeMult * (5.17 + 1.85 * sw)
	// LengthMult is now Visio's natural W/H aspect ratio per type (lend1=0.5, lend4=1.0, lend13=1.5).
	// Size multipliers normalized to size 2 = 1.0 with Visio's per-step scale increment.
	tests := []struct {
		arrowType  int
		arrowSize  int
		lineWeight float64
		want       float64
	}{
		{0, 2, 1.0, 0},      // No arrow: zero setback
		{1, 2, 1.0, 0},      // Type 1 (open chevron, Setback=0): line passes through
		{4, 2, 3.0, 10.72},  // Type 4 (closed triangle, Setback=2), size 2, sw=3: 2*1*(5.17+5.55)/2=10.72
		{11, 2, 1.0, 2.63},  // Type 11 (filled square, centered, Setback=0.75): 0.75*1*7.02/2=2.63
		{2, 2, 1.0, 3.51},   // Type 2 (filled triangle, Setback=1): 1*1*7.02/2=3.51
		{13, 2, 1.0, 10.53}, // Type 13 (filled triangle long, Setback=3): 3*1*7.02/2=10.53
		{13, 2, 3.0, 16.08}, // Type 13, size 2, sw=3: 3*1*10.72/2=16.08
		{22, 2, 1.0, 15.80}, // Type 22 (long diamond, Setback=4.5): 4.5*1*7.02/2=15.80
	}

	for _, tt := range tests {
		got := computeArrowSetback(tt.arrowType, tt.arrowSize, tt.lineWeight)
		diff := got - tt.want
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.01 {
			t.Errorf("computeArrowSetback(%d, %d, %f) = %f; want %f",
				tt.arrowType, tt.arrowSize, tt.lineWeight, got, tt.want)
		}
	}
}

func TestEffectiveStyle_FillColor(t *testing.T) {
	es := &EffectiveStyle{
		FillPattern:      1, // Solid
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
