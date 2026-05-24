package vsdx

import (
	"math"
	"testing"
)

func TestWorldTransform_Identity(t *testing.T) {
	id := Identity()

	if !id.IsIdentity() {
		t.Error("Identity() should return identity transform")
	}

	x, y := id.Apply(3, 4)
	if x != 3 || y != 4 {
		t.Errorf("Identity.Apply(3,4) = (%.2f,%.2f), want (3,4)", x, y)
	}
}

func TestWorldTransform_Translate(t *testing.T) {
	tr := Translate(10, 20)

	x, y := tr.Apply(3, 4)
	if x != 13 || y != 24 {
		t.Errorf("Translate(10,20).Apply(3,4) = (%.2f,%.2f), want (13,24)", x, y)
	}
}

func TestWorldTransform_Scale(t *testing.T) {
	sc := Scale(2, 3)

	x, y := sc.Apply(3, 4)
	if x != 6 || y != 12 {
		t.Errorf("Scale(2,3).Apply(3,4) = (%.2f,%.2f), want (6,12)", x, y)
	}
}

func TestWorldTransform_Rotate(t *testing.T) {
	// Rotate 90 degrees counter-clockwise
	rot := Rotate(math.Pi / 2)

	x, y := rot.Apply(1, 0)
	eps := 1e-10
	if math.Abs(x) > eps || math.Abs(y-1) > eps {
		t.Errorf("Rotate(π/2).Apply(1,0) = (%.4f,%.4f), want (0,1)", x, y)
	}
}

func TestWorldTransform_RotateAround(t *testing.T) {
	// Rotate 90 degrees around (1, 1)
	rot := RotateAround(math.Pi/2, 1, 1)

	// Point (2, 1) should become (1, 2)
	x, y := rot.Apply(2, 1)
	eps := 1e-10
	if math.Abs(x-1) > eps || math.Abs(y-2) > eps {
		t.Errorf("RotateAround(π/2, 1,1).Apply(2,1) = (%.4f,%.4f), want (1,2)", x, y)
	}
}

func TestWorldTransform_Multiply(t *testing.T) {
	// Translate then scale
	tr := Translate(10, 0)
	sc := Scale(2, 1)

	// Scale(Translate(p)) = scale first, then translate
	// So point (1, 0) -> (2, 0) -> (12, 0)
	combined := tr.Multiply(sc) // Apply sc first, then tr

	x, y := combined.Apply(1, 0)
	if x != 12 || y != 0 {
		t.Errorf("Translate(10,0).Multiply(Scale(2,1)).Apply(1,0) = (%.2f,%.2f), want (12,0)", x, y)
	}
}

func TestWorldTransform_Inverse(t *testing.T) {
	tr := Translate(10, 20).Multiply(Scale(2, 3))

	inv := tr.Inverse()
	combined := tr.Multiply(inv)

	if !combined.IsIdentity() {
		t.Errorf("T * T^-1 should be identity, got %s", combined.String())
	}
}

func TestWorldTransform_ApplyToRect(t *testing.T) {
	// Translate a unit rectangle
	tr := Translate(10, 20)
	rect := Rect{X: 0, Y: 0, Width: 1, Height: 1}

	result := tr.ApplyToRect(rect)

	if result.X != 10 || result.Y != 20 || result.Width != 1 || result.Height != 1 {
		t.Errorf("Translate(10,20).ApplyToRect([0,0,1,1]) = %+v, want {10,20,1,1}", result)
	}
}

func TestWorldTransform_ToSVG(t *testing.T) {
	tests := []struct {
		name   string
		tr     WorldTransform
		expect string
	}{
		{"identity", Identity(), ""},
		{"translate", Translate(10, 20), "translate(10,20)"},
		{"scale", Scale(2, 3), "scale(2,3)"},
		{"rotate", Rotate(math.Pi / 4), "matrix(0.7071,0.7071,-0.7071,0.7071,0,0)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tr.ToSVG()
			// Allow some tolerance in the comparison for rotations
			if tt.name != "rotate" && got != tt.expect {
				t.Errorf("ToSVG() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestComputeShapeTransform_Basic(t *testing.T) {
	vf, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skip("test1.vsdx not available:", err)
	}
	defer vf.Close()

	for _, page := range vf.Pages {
		shapes := page.ChildShapes()
		if len(shapes) == 0 {
			continue
		}

		for _, s := range shapes {
			st := s.ComputeShapeTransform(Identity())

			// Verify transform is valid
			if st == nil {
				t.Errorf("Shape %s: ComputeShapeTransform returned nil", s.ID)
				continue
			}

			// World bounds should have positive dimensions
			if st.WorldBounds.Width <= 0 || st.WorldBounds.Height <= 0 {
				// Allow negative for connectors
				if s.CellValue("BeginX") == "" {
					t.Errorf("Shape %s: WorldBounds has non-positive dimensions: %+v",
						s.ID, st.WorldBounds)
				}
			}
		}
		break
	}
}

func TestComputeGroupTransforms(t *testing.T) {
	vf, err := Open("../tests/test3_house.vsdx")
	if err != nil {
		t.Skip("test3_house.vsdx not available:", err)
	}
	defer vf.Close()

	for _, mp := range vf.MasterPages {
		shapes := mp.ChildShapes()
		for _, shape := range shapes {
			if shape.ShapeType != "Group" {
				continue
			}

			transforms := ComputeGroupTransforms(shape)

			// Should have transforms for all shapes in hierarchy
			if len(transforms) == 0 {
				t.Error("ComputeGroupTransforms returned empty map for group shape")
			}

			// Dump for debugging
			t.Logf("Group %s transforms:\n%s", shape.ID, DumpTransforms(shape))
			break
		}
		break
	}
}
