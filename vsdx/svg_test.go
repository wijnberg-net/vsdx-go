package vsdx

import (
	"os"
	"strings"
	"testing"
)

// --- ShapeToSVG ---

func TestShapeToSVG_SimpleRect(t *testing.T) {
	// test_master.vsdx has two simple rectangular master shapes
	vis, err := Open(testFile("test_master.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	if len(vis.MasterPages) < 2 {
		t.Fatalf("expected >=2 masters, got %d", len(vis.MasterPages))
	}

	// "Test Master" — unfilled rectangle with RelMoveTo/RelLineTo
	master1 := vis.MasterPages[0]
	shapes1 := master1.ChildShapes()
	if len(shapes1) == 0 {
		t.Fatal("no child shapes in master 1")
	}

	result, err := ShapeToSVG(shapes1[0], WithSize(100, 100))
	if err != nil {
		t.Fatal(err)
	}

	svg := string(result.SVG)
	t.Logf("Test Master SVG:\n%s", svg)

	// Verify SVG structure
	if !strings.Contains(svg, `xmlns="http://www.w3.org/2000/svg"`) {
		t.Error("missing xmlns")
	}
	if !strings.Contains(svg, `viewBox="`) {
		t.Error("wrong viewBox")
	}
	if !strings.Contains(svg, "<path") {
		t.Error("missing <path>")
	}
	// Rectangle path with uniform scaling (aspect ratio preserved).
	// Width fills to 100, height scales proportionally.
	if !strings.Contains(svg, "M0 ") {
		t.Errorf("expected MoveTo at bottom-left, got: %s", svg)
	}
	if !strings.Contains(svg, "L100 ") {
		t.Errorf("expected LineTo at right edge, got: %s", svg)
	}
	// Unfilled
	if !strings.Contains(svg, `fill="none"`) {
		t.Error("expected unfilled rectangle")
	}
}

func TestShapeToSVG_FilledRect(t *testing.T) {
	vis, err := Open(testFile("test_master.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	// "Test Master 2" — yellow filled rectangle
	master2 := vis.MasterPages[1]
	shapes2 := master2.ChildShapes()
	if len(shapes2) == 0 {
		t.Fatal("no child shapes in master 2")
	}

	result, err := ShapeToSVG(shapes2[0], WithSize(200, 200))
	if err != nil {
		t.Fatal(err)
	}

	svg := string(result.SVG)
	t.Logf("Test Master 2 SVG:\n%s", svg)

	if !strings.Contains(svg, `fill="#FFFF00"`) {
		t.Error("expected yellow fill")
	}
	if result.BrandColor != "#FFFF00" {
		t.Errorf("brand color = %q, want #FFFF00", result.BrandColor)
	}
	if !strings.Contains(svg, `viewBox="`) || !strings.Contains(svg, "200") {
		t.Error("wrong viewBox for custom size")
	}
}

func TestShapeToSVG_GroupShape(t *testing.T) {
	// test3_house.vsdx has a "House" group master with 3 child shapes
	vis, err := Open(testFile("test3_house.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	if len(vis.MasterPages) == 0 {
		t.Fatal("no master pages")
	}

	master := vis.MasterPages[0]
	shapes := master.ChildShapes()
	if len(shapes) == 0 {
		t.Fatal("no child shapes")
	}

	shape := shapes[0]
	if shape.ShapeType != "Group" {
		t.Fatalf("expected Group, got %q", shape.ShapeType)
	}

	result, err := ShapeToSVG(shape, WithSize(100, 100))
	if err != nil {
		t.Fatal(err)
	}

	svg := string(result.SVG)
	t.Logf("House SVG:\n%s", svg)

	// Group should produce multiple <path> elements (one per child)
	pathCount := strings.Count(svg, "<path")
	if pathCount < 2 {
		t.Errorf("expected >=2 paths for group, got %d", pathCount)
	}

	// Should have valid SVG wrapper
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("should start with <svg")
	}
	if !strings.HasSuffix(strings.TrimSpace(svg), "</svg>") {
		t.Error("should end with </svg>")
	}
}

func TestShapeToSVG_AbsoluteGeometry(t *testing.T) {
	// test9_rect_and_line.vsdx has shapes with absolute MoveTo/LineTo
	vis, err := Open(testFile("test9_rect_and_line.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	shapes := vis.Pages[0].AllShapes()
	var geomShapes []*Shape
	for _, s := range shapes {
		if s.Geometry != nil && len(s.Geometry.Rows) > 0 {
			geomShapes = append(geomShapes, s)
		}
	}

	if len(geomShapes) < 2 {
		t.Fatalf("expected >=2 shapes with geometry, got %d", len(geomShapes))
	}

	for _, s := range geomShapes {
		result, err := ShapeToSVG(s, WithSize(100, 100))
		if err != nil {
			t.Errorf("shape ID=%s: %v", s.ID, err)
			continue
		}
		svg := string(result.SVG)
		t.Logf("Shape ID=%s SVG:\n%s", s.ID, svg)

		if !strings.Contains(svg, "<path") {
			t.Errorf("shape ID=%s: missing path", s.ID)
		}
	}
}

func TestShapeToSVG_NoGeometry(t *testing.T) {
	// Create a shape with no geometry — should return error
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	// test1.vsdx has no masters, but has page shapes — find one without geometry
	for _, s := range vis.Pages[0].AllShapes() {
		if s.Geometry == nil || len(s.Geometry.Rows) == 0 {
			_, err := ShapeToSVG(s)
			if err == nil {
				t.Error("expected error for shape without geometry")
			}
			return
		}
	}
	t.Skip("no shape without geometry found")
}

func TestShapeToSVG_Precision(t *testing.T) {
	vis, err := Open(testFile("test_master.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	shape := vis.MasterPages[1].ChildShapes()[0]

	// Precision 0
	result, err := ShapeToSVG(shape, WithSize(100, 100), WithPrecision(0))
	if err != nil {
		t.Fatal(err)
	}
	svg := string(result.SVG)
	t.Logf("Precision 0:\n%s", svg)

	// With precision 0, coordinates should be integers.
	// Uniform scaling preserves aspect ratio, so Y may be < 100 for wide shapes.
	if !strings.Contains(svg, "M0 81") {
		t.Errorf("expected integer coords with uniform scale, got: %s", svg)
	}

	// Precision 4
	result, err = ShapeToSVG(shape, WithSize(100, 100), WithPrecision(4))
	if err != nil {
		t.Fatal(err)
	}
	svg = string(result.SVG)
	t.Logf("Precision 4:\n%s", svg)
}

func TestShapeToSVG_BrandColorOverride(t *testing.T) {
	vis, err := Open(testFile("test_master.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	shape := vis.MasterPages[1].ChildShapes()[0]
	result, err := ShapeToSVG(shape, WithBrandColor("#FF0000"))
	if err != nil {
		t.Fatal(err)
	}

	if result.BrandColor != "#FF0000" {
		t.Errorf("brand color = %q, want #FF0000", result.BrandColor)
	}
}

func TestShapeToSVG_RoundedGroup(t *testing.T) {
	// test5_master.vsdx has a Lucidchart process block (rounded rectangle group)
	vis, err := Open(testFile("test5_master.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	if len(vis.MasterPages) == 0 {
		t.Fatal("no master pages")
	}

	master := vis.MasterPages[0]
	shapes := master.ChildShapes()
	if len(shapes) == 0 {
		t.Fatal("no child shapes")
	}

	result, err := ShapeToSVG(shapes[0], WithSize(100, 75))
	if err != nil {
		t.Fatal(err)
	}

	svg := string(result.SVG)
	t.Logf("Process Block SVG:\n%s", svg)

	if !strings.Contains(svg, "<path") {
		t.Error("missing path")
	}
	if !strings.Contains(svg, `fill="#FFFFFF"`) {
		t.Errorf("expected white fill, got: %s", svg)
	}
}

func TestShapeToSVG_WriteSVGFile(t *testing.T) {
	if os.Getenv("WRITE_SVG") == "" {
		t.Skip("set WRITE_SVG=1 to write SVG files for visual inspection")
	}

	vis, err := Open(testFile("test3_house.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	for _, m := range vis.MasterPages {
		for _, s := range m.ChildShapes() {
			result, err := ShapeToSVG(s, WithSize(200, 200))
			if err != nil {
				continue
			}
			fname := "/tmp/vsdx_test_" + m.Name() + ".svg"
			if err := os.WriteFile(fname, result.SVG, 0644); err != nil {
				t.Errorf("writing %s: %v", fname, err)
				continue
			}
			t.Logf("Wrote %s", fname)
		}
	}
}

// --- SVG helpers ---

func TestResolveColor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"#ff0000", "#FF0000"},
		{"#FFFFFF", "#FFFFFF"},
		{"#000000", "#000000"},
		{"RGB(255,0,0)", "#FF0000"},
		{"RGB(0, 128, 255)", "#0080FF"},
		{"", ""},
		{"Themed", ""},
		{"42", ""},      // theme index
		{"accent1", ""}, // theme name
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := resolveColor(tt.input)
			if got != tt.want {
				t.Errorf("resolveColor(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDetectBrandColor(t *testing.T) {
	freq := map[string]int{
		"#FFFFFF": 10, // white — ignored
		"#000000": 5,  // black — ignored
		"#0E6F9C": 3,  // Cisco blue — should win
		"#FF0000": 1,  // red
	}
	got := detectBrandColor(freq)
	if got != "#0E6F9C" {
		t.Errorf("detectBrandColor = %q, want #0E6F9C", got)
	}
}

func TestDetectBrandColor_NoColors(t *testing.T) {
	freq := map[string]int{
		"#FFFFFF": 5,
		"#000000": 3,
	}
	got := detectBrandColor(freq)
	if got != "" {
		t.Errorf("detectBrandColor with only B&W = %q, want empty", got)
	}
}

func TestFmtPrec(t *testing.T) {
	tests := []struct {
		val  float64
		prec int
		want string
	}{
		{100.0, 2, "100"},
		{3.14159, 2, "3.14"},
		{3.10, 2, "3.1"},
		{0.0, 2, "0"},
		{99.999, 0, "100"},
	}
	for _, tt := range tests {
		got := fmtPrec(tt.val, tt.prec)
		if got != tt.want {
			t.Errorf("fmtPrec(%v, %d) = %q, want %q", tt.val, tt.prec, got, tt.want)
		}
	}
}

func TestArcToSVG_ZeroBow(t *testing.T) {
	// Zero bow should produce a straight line
	got := arcToSVG(0, 0, 100, 100, 0, 2)
	if !strings.HasPrefix(got, "L") {
		t.Errorf("zero bow should produce LineTo, got: %s", got)
	}
}

func TestArcToSVG_PositiveBow(t *testing.T) {
	got := arcToSVG(0, 0, 100, 0, 20, 2)
	if !strings.HasPrefix(got, "A") {
		t.Errorf("nonzero bow should produce Arc, got: %s", got)
	}
	// Should contain radius and endpoint
	if !strings.Contains(got, "100 0") {
		t.Errorf("should end at (100,0), got: %s", got)
	}
}

func TestSortGeometryRows(t *testing.T) {
	vis, err := Open(testFile("test3_house.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	// Get a shape with multiple geometry rows
	master := vis.MasterPages[0]
	for _, s := range master.ChildShapes() {
		if s.ShapeType == "Group" {
			for _, child := range s.ChildShapes() {
				if child.Geometry != nil && len(child.Geometry.Rows) > 2 {
					sorted := sortGeometryRows(child.Geometry.Rows)
					for i := 1; i < len(sorted); i++ {
						prevIX, _ := parseIX(sorted[i-1].Index())
						currIX, _ := parseIX(sorted[i].Index())
						if prevIX > currIX {
							t.Errorf("rows not sorted: IX %d > IX %d", prevIX, currIX)
						}
					}
					// First row should be MoveTo or RelMoveTo
					firstType := strings.ToLower(sorted[0].RowType())
					if firstType != "moveto" && firstType != "relmoveto" {
						t.Errorf("first sorted row should be MoveTo/RelMoveTo, got %q", sorted[0].RowType())
					}
					return
				}
			}
		}
	}
	t.Skip("no suitable shape found")
}

// --- Stencil support ---

func TestOpenStencilExtension(t *testing.T) {
	// Test that .vssx extension is accepted (even though we don't have a test file)
	_, err := Open("nonexistent.vssx")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
		return
	}
	// Error should NOT be "invalid file type"
	if strings.Contains(err.Error(), "invalid file type") {
		t.Errorf("should accept .vssx extension, got: %v", err)
	}
}

func TestOpenStencilExtensionVSSM(t *testing.T) {
	_, err := Open("nonexistent.vssm")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
		return
	}
	if strings.Contains(err.Error(), "invalid file type") {
		t.Errorf("should accept .vssm extension, got: %v", err)
	}
}

func TestOpenRejectsInvalidExtension(t *testing.T) {
	_, err := Open("test.xlsx")
	if err == nil {
		t.Fatal("expected error")
		return
	}
	if !strings.Contains(err.Error(), "invalid file type") {
		t.Errorf("expected 'invalid file type' error, got: %v", err)
	}
}

func TestIsStencil_RegularFile(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck
	if vis.IsStencil() {
		t.Error("regular .vsdx should not be a stencil")
	}
}

// --- Foreign image extraction ---

func TestForeignImageData_NoForeign(t *testing.T) {
	vis, err := Open(testFile("test_master.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	shape := vis.MasterPages[0].ChildShapes()[0]
	img := shape.ForeignImageData()
	if img != nil {
		t.Error("expected nil for non-foreign shape")
	}
}

func TestResolveMediaPath(t *testing.T) {
	tests := []struct {
		pagePath string
		target   string
		want     string
	}{
		{"visio/masters/master1.xml", "../media/image1.emf", "visio/media/image1.emf"},
		{"visio/pages/page1.xml", "../media/image3.png", "visio/media/image3.png"},
		{"visio/masters/master42.xml", "../media/image42.emf", "visio/media/image42.emf"},
		{"visio/pages/page1.xml", "image1.png", "visio/pages/image1.png"},
	}
	for _, tt := range tests {
		got := resolveMediaPath(tt.pagePath, tt.target)
		if got != tt.want {
			t.Errorf("resolveMediaPath(%q, %q) = %q, want %q", tt.pagePath, tt.target, got, tt.want)
		}
	}
}

func TestExtensionFromForeignType(t *testing.T) {
	tests := []struct {
		foreignType string
		compression string
		want        string
	}{
		{"EnhMetaFile", "", "emf"},
		{"MetaFile", "", "wmf"},
		{"Bitmap", "PNG", "png"},
		{"Bitmap", "JPEG", "jpeg"},
		{"Bitmap", "", "bmp"},
	}
	for _, tt := range tests {
		got := extensionFromForeignType(tt.foreignType, tt.compression)
		if got != tt.want {
			t.Errorf("extensionFromForeignType(%q, %q) = %q, want %q", tt.foreignType, tt.compression, got, tt.want)
		}
	}
}

func TestDetectSVGBrandColor(t *testing.T) {
	svg := `<svg><rect fill="#0078AA"/><rect fill="#0078AA"/><rect fill="#FFFFFF"/><rect stroke="#000000"/></svg>`
	got := detectSVGBrandColor(svg)
	if got != "#0078AA" {
		t.Errorf("detectSVGBrandColor = %q, want #0078AA", got)
	}
}

func TestSVGRenderNewGeometryRowTypes(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	// Use the first shape on the page
	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes on page")
	}
	shape := shapes[0]

	// Add various geometry row types to test SVG rendering
	geom := shape.AddGeometry()
	geom.AddRelMoveTo(0, 0)
	geom.AddRelCubBezTo(1, 1, 0.25, 0.5, 0.75, 0.5)
	geom.AddRelQuadBezTo(0.5, 0, 0.25, 0.5)
	geom.AddRelEllipticalArcTo(0, 0.5, 0.5, 0.25, 1.5, 0)
	geom.AddPolylineTo(1, 1, "POLYLINE(0,0,0.5,0.5,1,0)")
	geom.AddInfiniteLine(0, 0, 1, 1)

	// Render to SVG
	result, err := ShapeToSVG(shape, WithSize(100, 100))
	if err != nil {
		t.Fatalf("ShapeToSVG: %v", err)
	}

	svg := string(result.SVG)
	// Check that some path commands were generated
	if !strings.Contains(svg, "<path") {
		t.Error("expected SVG to contain path elements")
	}
	t.Logf("SVG rendered successfully, length=%d bytes", len(result.SVG))
}

func TestParsePolylinePoints(t *testing.T) {
	// Test proportional coordinates (xType=0, yType=0)
	pts := parsePolylinePoints("POLYLINE(0,0,0.25,0.25,0.75,0.75)", 2.0, 2.0, 1.0, 1.0)
	if len(pts) != 2 {
		t.Fatalf("expected 2 points, got %d", len(pts))
	}
	// 0.25 * 2.0 + 1.0 = 1.5
	if pts[0].x != 1.5 || pts[0].y != 1.5 {
		t.Errorf("pt[0] = (%v, %v), want (1.5, 1.5)", pts[0].x, pts[0].y)
	}
	// 0.75 * 2.0 + 1.0 = 2.5
	if pts[1].x != 2.5 || pts[1].y != 2.5 {
		t.Errorf("pt[1] = (%v, %v), want (2.5, 2.5)", pts[1].x, pts[1].y)
	}

	// Test absolute coordinates (xType=1, yType=1)
	pts2 := parsePolylinePoints("POLYLINE(1, 1, 3.0, 4.0)", 2.0, 2.0, 0.5, 0.5)
	if len(pts2) != 1 {
		t.Fatalf("expected 1 point, got %d", len(pts2))
	}
	// 3.0 + 0.5 = 3.5
	if pts2[0].x != 3.5 || pts2[0].y != 4.5 {
		t.Errorf("pt2[0] = (%v, %v), want (3.5, 4.5)", pts2[0].x, pts2[0].y)
	}

	// Test invalid formulas
	if parsePolylinePoints("", 1, 1, 0, 0) != nil {
		t.Error("expected nil for empty formula")
	}
	if parsePolylinePoints("POLYLINE(0,0)", 1, 1, 0, 0) != nil {
		t.Error("expected nil for formula with only type params")
	}
}

func TestLinePatternToSVG(t *testing.T) {
	tests := []struct {
		pattern int
		weight  float64
		want    string
	}{
		{0, 1, ""},         // None
		{1, 1, ""},         // Solid
		{2, 1, "4.00 2.00"}, // Dash
		{3, 1, "1.00 2.00"}, // Dot
		{4, 1, "4.00 2.00 1.00 2.00"}, // Dash-Dot
		{10, 1, "1.00 4.00"}, // Sparse Dot
		{11, 1, "1.00 1.00"}, // Dense Dot
	}

	for _, tt := range tests {
		got := linePatternToSVG(tt.pattern, tt.weight)
		if got != tt.want {
			t.Errorf("linePatternToSVG(%d, %v) = %q, want %q", tt.pattern, tt.weight, got, tt.want)
		}
	}
}

func TestArrowMarkerKey(t *testing.T) {
	key := arrowMarkerKey(1, 2, "#FF0000", true)
	if !strings.Contains(key, "arrow") {
		t.Error("key should contain 'arrow'")
	}
	if !strings.Contains(key, "end") {
		t.Error("key should contain 'end' for end arrow")
	}

	keyStart := arrowMarkerKey(1, 2, "#FF0000", false)
	if !strings.Contains(keyStart, "start") {
		t.Error("key should contain 'start' for start arrow")
	}
}

func TestEscapeXML(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"<test>", "&lt;test&gt;"},
		{"&", "&amp;"},
		{`"quoted"`, "&quot;quoted&quot;"},
		{"'single'", "&apos;single&apos;"},
		{"a < b & c > d", "a &lt; b &amp; c &gt; d"},
	}

	for _, tt := range tests {
		got := escapeXML(tt.input)
		if got != tt.want {
			t.Errorf("escapeXML(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
