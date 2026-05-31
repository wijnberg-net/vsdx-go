package vsdx

import (
	"strings"
	"testing"
)

func TestEmitRenderTree_Basic(t *testing.T) {
	vf, err := Open("../tests/test_master.vsdx")
	if err != nil {
		t.Skip("test_master.vsdx not available:", err)
	}
	defer vf.Close()

	if len(vf.MasterPages) < 2 {
		t.Fatal("expected at least 2 master pages")
	}

	// Test first master shape
	master1 := vf.MasterPages[0]
	shapes := master1.ChildShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes in master")
	}

	svg, err := EmitRenderTree(shapes[0], 100, 100, 2)
	if err != nil {
		t.Fatalf("EmitRenderTree error: %v", err)
	}

	svgStr := string(svg)
	t.Logf("Emitted SVG:\n%s", svgStr)

	// Verify basic SVG structure
	if !strings.Contains(svgStr, `xmlns="http://www.w3.org/2000/svg"`) {
		t.Error("missing xmlns")
	}
	if !strings.Contains(svgStr, `viewBox="`) {
		t.Error("missing viewBox")
	}
	if !strings.Contains(svgStr, "</svg>") {
		t.Error("missing closing svg tag")
	}
}

func TestEmitRenderTree_Deterministic(t *testing.T) {
	vf, err := Open("../tests/test_master.vsdx")
	if err != nil {
		t.Skip("test_master.vsdx not available:", err)
	}
	defer vf.Close()

	if len(vf.MasterPages) == 0 {
		t.Fatal("no master pages")
	}

	shapes := vf.MasterPages[0].ChildShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}

	// Render multiple times and verify identical output
	svg1, err := EmitRenderTree(shapes[0], 100, 100, 2)
	if err != nil {
		t.Fatalf("first render error: %v", err)
	}

	svg2, err := EmitRenderTree(shapes[0], 100, 100, 2)
	if err != nil {
		t.Fatalf("second render error: %v", err)
	}

	svg3, err := EmitRenderTree(shapes[0], 100, 100, 2)
	if err != nil {
		t.Fatalf("third render error: %v", err)
	}

	if string(svg1) != string(svg2) {
		t.Error("non-deterministic output: render 1 != render 2")
	}
	if string(svg2) != string(svg3) {
		t.Error("non-deterministic output: render 2 != render 3")
	}
}

func TestGeometryResolver_Basic(t *testing.T) {
	vf, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skip("test1.vsdx not available:", err)
	}
	defer vf.Close()

	for _, page := range vf.Pages {
		shapes := page.ChildShapes()
		for _, shape := range shapes {
			if len(shape.Geometries) == 0 {
				continue
			}

			style := shape.ComputeEffectiveStyle()
			shapeW := shape.Width()
			shapeH := shape.Height()
			if shapeW < 0 {
				shapeW = -shapeW
			}
			if shapeH < 0 {
				shapeH = -shapeH
			}
			if shapeW == 0 {
				shapeW = 1
			}
			if shapeH == 0 {
				shapeH = 1
			}

			totalGeoms := len(shape.Geometries)
			for i, geom := range shape.Geometries {
				result := ResolveGeometry(
					shape, geom, style,
					shapeW, shapeH,
					0, 0,
					72, 72, // 72 points per inch
					shapeH,
					false,
					2,
					i, totalGeoms,
				)

				if result != nil && result.PathData != "" {
					t.Logf("Shape %s geometry: %s", shape.ID, result.PathData[:min(len(result.PathData), 100)])
				}
			}
		}
		break
	}
}

func TestRenderTreeBuilder_Scale(t *testing.T) {
	vf, err := Open("../tests/test_master.vsdx")
	if err != nil {
		t.Skip("test_master.vsdx not available:", err)
	}
	defer vf.Close()

	if len(vf.MasterPages) == 0 {
		t.Fatal("no master pages")
	}

	shapes := vf.MasterPages[0].ChildShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}

	builder := NewRenderTreeBuilder(shapes[0])
	tree := builder.BuildWithScale(72, 72, 2) // 72 pts/inch

	if tree == nil {
		t.Fatal("BuildWithScale returned nil")
	}

	// Check that geometry was resolved
	totalPaths := countPaths(tree)
	t.Logf("Total paths in tree: %d", totalPaths)

	if totalPaths == 0 {
		// It's okay if the simple master has no paths (text only)
		t.Log("Note: no paths found (may be text-only shape)")
	}
}

func countPaths(node *RenderNode) int {
	count := len(node.Geometry)
	for _, child := range node.Children {
		count += countPaths(child)
	}
	return count
}

func TestRenderTreeBuilder_Markers(t *testing.T) {
	vf, err := Open("../tests/test5_master.vsdx")
	if err != nil {
		t.Skip("test5_master.vsdx not available:", err)
	}
	defer vf.Close()

	for _, page := range vf.Pages {
		shapes := page.ChildShapes()
		for _, shape := range shapes {
			// Look for connectors with arrows
			if shape.CellValue("BeginX") == "" {
				continue
			}

			builder := NewRenderTreeBuilder(shape)
			tree := builder.BuildWithScale(72, 72, 2)

			if len(tree.Markers) > 0 {
				t.Logf("Shape %s has %d markers", shape.ID, len(tree.Markers))
				for id, m := range tree.Markers {
					t.Logf("  Marker %s: type=%d size=%d filled=%v",
						id, m.ArrowType, m.Size, m.Filled)
				}
			}
		}
		break
	}
}

func TestSVGEmitter_NoMutation(t *testing.T) {
	// This test verifies that the emitter doesn't mutate the render tree
	vf, err := Open("../tests/test_master.vsdx")
	if err != nil {
		t.Skip("test_master.vsdx not available:", err)
	}
	defer vf.Close()

	if len(vf.MasterPages) == 0 {
		t.Fatal("no master pages")
	}

	shapes := vf.MasterPages[0].ChildShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}

	builder := NewRenderTreeBuilder(shapes[0])
	tree := builder.BuildWithScale(72, 72, 2)

	// Capture state before emit
	pathDataBefore := make(map[string]string)
	capturePathData(tree, pathDataBefore)

	// Emit
	emitter := NewSVGEmitter(tree, 100, 100, 2)
	_ = emitter.Emit()

	// Verify state after emit is unchanged
	pathDataAfter := make(map[string]string)
	capturePathData(tree, pathDataAfter)

	for id, before := range pathDataBefore {
		after := pathDataAfter[id]
		if before != after {
			t.Errorf("Path data mutated during emit: %s", id)
		}
	}
}

func capturePathData(node *RenderNode, result map[string]string) {
	for i, path := range node.Geometry {
		key := node.Shape.ID + "_" + string(rune('0'+i))
		result[key] = path.D
	}
	for _, child := range node.Children {
		capturePathData(child, result)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestEmitRenderTreeWithResult_BrandColor(t *testing.T) {
	vf, err := Open("../tests/test5_master.vsdx")
	if err != nil {
		t.Skip("test5_master.vsdx not available:", err)
	}
	defer vf.Close()

	if len(vf.Pages) == 0 {
		t.Fatal("no pages")
	}

	shapes := vf.Pages[0].ChildShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}

	// Test auto-detection
	result, err := EmitRenderTreeWithResult(shapes[0], 100, 100, 2, "")
	if err != nil {
		t.Fatalf("EmitRenderTreeWithResult error: %v", err)
	}

	t.Logf("Auto-detected brand color: %s", result.BrandColor)
	t.Logf("Output dimensions: %.2f x %.2f", result.Width, result.Height)

	// Test override
	result2, err := EmitRenderTreeWithResult(shapes[0], 100, 100, 2, "#FF0000")
	if err != nil {
		t.Fatalf("EmitRenderTreeWithResult with override error: %v", err)
	}

	if result2.BrandColor != "#FF0000" {
		t.Errorf("expected brand color override #FF0000, got %s", result2.BrandColor)
	}
}

func TestEmitRenderTree_CompareWithShapeToSVG(t *testing.T) {
	vf, err := Open("../tests/test_master.vsdx")
	if err != nil {
		t.Skip("test_master.vsdx not available:", err)
	}
	defer vf.Close()

	if len(vf.MasterPages) == 0 {
		t.Fatal("no master pages")
	}

	shapes := vf.MasterPages[0].ChildShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}

	shape := shapes[0]

	// Get output from both renderers
	oldResult, err := ShapeToSVG(shape, WithSize(100, 100), WithPrecision(2))
	if err != nil {
		t.Fatalf("ShapeToSVG error: %v", err)
	}

	newResult, err := EmitRenderTreeWithResult(shape, 100, 100, 2, "")
	if err != nil {
		t.Fatalf("EmitRenderTreeWithResult error: %v", err)
	}

	t.Logf("Old SVG:\n%s", string(oldResult.SVG))
	t.Logf("New SVG:\n%s", string(newResult.SVG))

	// Check both produce valid SVG
	oldStr := string(oldResult.SVG)
	newStr := string(newResult.SVG)

	if !strings.Contains(oldStr, `xmlns="http://www.w3.org/2000/svg"`) {
		t.Error("old SVG missing xmlns")
	}
	if !strings.Contains(newStr, `xmlns="http://www.w3.org/2000/svg"`) {
		t.Error("new SVG missing xmlns")
	}

	// Both should have path elements
	if !strings.Contains(oldStr, "<path") {
		t.Error("old SVG missing path element")
	}
	if !strings.Contains(newStr, "<path") {
		t.Error("new SVG missing path element")
	}

	// Log dimension differences
	t.Logf("Old dimensions: %.2f x %.2f", oldResult.Width, oldResult.Height)
	t.Logf("New dimensions: %.2f x %.2f", newResult.Width, newResult.Height)
}
