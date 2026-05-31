package vsdx

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// createTestPNG generates a solid-color 64x64 PNG for testing.
func createTestPNG(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestSaveVsdxBytesRoundTrip(t *testing.T) {
	vis, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	// Save to bytes
	data, err := vis.SaveVsdxBytes()
	if err != nil {
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("SaveVsdxBytes returned empty data")
	}

	// Reopen from bytes
	vis2, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes(round-trip): %v", err)
	}
	defer vis2.Close() //nolint:errcheck

	if len(vis2.Pages) != len(vis.Pages) {
		t.Fatalf("page count mismatch: got %d, want %d", len(vis2.Pages), len(vis.Pages))
	}
}

func TestAddImage(t *testing.T) {
	vis, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	if page == nil {
		t.Fatal("no page 0")
		return
	}

	pngData := createTestPNG(color.RGBA{255, 0, 0, 255})

	relID, err := vis.AddImage(page, pngData)
	if err != nil {
		t.Fatalf("AddImage: %v", err)
	}
	if !strings.HasPrefix(relID, "rId") {
		t.Fatalf("unexpected relID: %q", relID)
	}

	// Verify PNG is in ZipFileContents
	found := false
	for path, data := range vis.ZipFileContents {
		if strings.HasPrefix(path, "visio/media/image") && strings.HasSuffix(path, ".png") {
			if bytes.Equal(data, pngData) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("PNG not found in ZipFileContents")
	}

	// Verify page rels has the relationship
	if page.RelsXML == nil {
		t.Fatal("page.RelsXML is nil after AddImage")
		return
	}
	relFound := false
	for _, rel := range page.RelsXML.Root().SelectElements("Relationship") {
		if rel.SelectAttrValue("Id", "") == relID {
			relType := rel.SelectAttrValue("Type", "")
			if relType != ImageRelType {
				t.Fatalf("wrong rel type: %q", relType)
			}
			relFound = true
			break
		}
	}
	if !relFound {
		t.Fatalf("relationship %s not found in page rels", relID)
	}

	// Add a second image — relID should be different
	relID2, err := vis.AddImage(page, createTestPNG(color.RGBA{0, 255, 0, 255}))
	if err != nil {
		t.Fatalf("AddImage(2): %v", err)
	}
	if relID2 == relID {
		t.Fatal("second relID should differ from first")
	}
}

func TestAddShape(t *testing.T) {
	vis, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	shape := page.AddShape()
	if shape == nil {
		t.Fatal("AddShape returned nil")
		return
	}
	if shape.ID == "" {
		t.Fatal("shape has no ID")
		return
	}

	// Set position
	shape.SetX(5.0)
	shape.SetY(4.0)
	shape.SetWidth(0.6)
	shape.SetHeight(0.6)

	if shape.X() != 5.0 {
		t.Fatalf("X: got %v, want 5.0", shape.X())
	}
	if shape.Width() != 0.6 {
		t.Fatalf("Width: got %v, want 0.6", shape.Width())
	}

	// Verify shape appears in page
	shapes := page.ChildShapes()
	found := false
	for _, s := range shapes {
		if s.ID == shape.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("shape %s not found in page.ChildShapes()", shape.ID)
	}
}

func TestSetForeignData(t *testing.T) {
	vis, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)

	// Add image and shape
	pngData := createTestPNG(color.RGBA{59, 130, 246, 255})
	relID, err := vis.AddImage(page, pngData)
	if err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	shape := page.AddShape()
	shape.SetX(4.0)
	shape.SetY(5.0)
	shape.SetWidth(0.6)
	shape.SetHeight(0.6)

	// Apply ForeignData
	shape.SetForeignData(relID)

	// Verify shape type
	if shape.ShapeType != "Foreign" {
		t.Fatalf("ShapeType: got %q, want %q", shape.ShapeType, "Foreign")
	}

	// Verify ForeignData element exists
	fd := shape.XML().FindElement("ForeignData")
	if fd == nil {
		t.Fatal("ForeignData element not found")
		return
	}
	if fd.SelectAttrValue("ForeignType", "") != "Bitmap" {
		t.Fatal("ForeignType != Bitmap")
		return
	}
	if fd.SelectAttrValue("CompressionType", "") != "PNG" {
		t.Fatal("CompressionType != PNG")
	}

	// Verify Rel child
	relElem := fd.FindElement("Rel")
	if relElem == nil {
		t.Fatal("Rel element not found in ForeignData")
		return
	}

	// Verify Geometry section
	geo := shape.XML().FindElement("Section[@N='Geometry']")
	if geo == nil {
		t.Fatal("Geometry section not found")
		return
	}
	rows := geo.SelectElements("Row")
	if len(rows) != 5 {
		t.Fatalf("Geometry rows: got %d, want 5", len(rows))
	}

	// Verify image cells have formulas
	imgW := shape.CellFormula("ImgWidth")
	if imgW != "Width*1" {
		t.Fatalf("ImgWidth formula: got %q, want %q", imgW, "Width*1")
	}

	// Verify TxtPinY formula (label below icon)
	txtY := shape.CellFormula(CellTxtPinY)
	if txtY != "Height*0" {
		t.Fatalf("TxtPinY formula: got %q, want %q", txtY, "Height*0")
	}

	// Verify ObjType
	if shape.CellValue("ObjType") != "1" {
		t.Fatalf("ObjType: got %q, want %q", shape.CellValue("ObjType"), "1")
	}
}

func TestAddDataProperty(t *testing.T) {
	vis, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	shape := page.AddShape()

	// Add first property
	dp := shape.AddDataProperty("Hostname", "Hostname", "test-router")
	if dp == nil {
		t.Fatal("AddDataProperty returned nil")
		return
	}
	if dp.Value() != "test-router" {
		t.Fatalf("Value: got %q, want %q", dp.Value(), "test-router")
		return
	}
	if dp.Label != "Hostname" {
		t.Fatalf("Label: got %q, want %q", dp.Label, "Hostname")
	}

	// Verify all 11 cells exist
	row := shape.XML().FindElement("Section[@N='Property']/Row")
	if row == nil {
		t.Fatal("Property row not found")
		return
	}
	expectedCells := []string{"Value", "Prompt", "Label", "Format", "SortKey", "Type", "Invisible", "Verify", "DataLinked", "LangID", "Calendar"}
	for _, cellName := range expectedCells {
		cell := row.FindElement("Cell[@N='" + cellName + "']")
		if cell == nil {
			t.Fatalf("missing cell: %s", cellName)
			return
		}
	}

	// Add second property — should get different row name
	dp2 := shape.AddDataProperty("RouterID", "Router ID", "10.0.0.1")
	if dp2.Value() != "10.0.0.1" {
		t.Fatalf("dp2 Value: got %q, want %q", dp2.Value(), "10.0.0.1")
	}

	// Verify both properties accessible via DataProperties()
	props := shape.DataProperties()
	if len(props) != 2 {
		t.Fatalf("DataProperties: got %d, want 2", len(props))
	}
}

func TestForeignDataRoundTrip(t *testing.T) {
	// Full integration: create Foreign shape, save, reopen, verify PNG intact
	vis, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	pngData := createTestPNG(color.RGBA{34, 197, 94, 255})

	relID, err := vis.AddImage(page, pngData)
	if err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	shape := page.AddShape()
	shape.SetX(4.0)
	shape.SetY(5.0)
	shape.SetWidth(0.6)
	shape.SetHeight(0.6)
	shape.SetForeignData(relID)
	shape.SetText("test-device")
	shape.AddDataProperty("Hostname", "Hostname", "test-device")

	// Save to bytes
	data, err := vis.SaveVsdxBytes()
	if err != nil {
		t.Fatalf("SaveVsdxBytes: %v", err)
	}

	// Verify ZIP structure
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	hasMedia := false
	hasPageRels := false
	hasContentType := false
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "visio/media/image") && strings.HasSuffix(f.Name, ".png") {
			hasMedia = true
		}
		if strings.Contains(f.Name, "page1.xml.rels") {
			hasPageRels = true
		}
		if f.Name == "[Content_Types].xml" {
			rc, _ := f.Open()
			ct := new(bytes.Buffer)
			_, _ = ct.ReadFrom(rc)
			_ = rc.Close()
			if strings.Contains(ct.String(), "image/png") {
				hasContentType = true
			}
		}
	}
	if !hasMedia {
		t.Fatal("no PNG in visio/media/")
	}
	if !hasPageRels {
		t.Fatal("no page1.xml.rels")
	}
	if !hasContentType {
		t.Fatal("PNG content type not registered in [Content_Types].xml")
	}

	// Reopen and verify
	vis2, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes(round-trip): %v", err)
	}
	defer vis2.Close() //nolint:errcheck

	page2 := vis2.GetPage(0)
	shapes := page2.ChildShapes()
	if len(shapes) != 1 {
		t.Fatalf("shapes: got %d, want 1", len(shapes))
	}

	s := shapes[0]
	if s.Text() != "test-device" {
		t.Fatalf("text: got %q, want %q", s.Text(), "test-device")
	}
	if s.ShapeType != "Foreign" {
		t.Fatalf("type: got %q, want %q", s.ShapeType, "Foreign")
	}

	// Verify data property
	props := s.DataProperties()
	if len(props) != 1 {
		t.Fatalf("properties: got %d, want 1", len(props))
	}
	hostProp := props["Hostname"]
	if hostProp == nil {
		t.Fatal("Hostname property not found")
		return
	}
	if hostProp.Value() != "test-device" {
		t.Fatalf("hostname: got %q, want %q", hostProp.Value(), "test-device")
		return
	}

	// Verify PNG data intact
	pngFound := false
	for path, d := range vis2.ZipFileContents {
		if strings.HasPrefix(path, "visio/media/image") && strings.HasSuffix(path, ".png") {
			if bytes.Equal(d, pngData) {
				pngFound = true
				break
			}
		}
	}
	if !pngFound {
		t.Fatal("PNG data not intact after round-trip")
	}
}

func TestGroupShapes(t *testing.T) {
	vis, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)

	// Create 3 shapes at known positions.
	s1 := page.AddShape()
	s1.SetX(2.0)
	s1.SetY(3.0)
	s1.SetWidth(0.6)
	s1.SetHeight(0.6)
	s1.SetLocX(0.3)
	s1.SetLocY(0.3)
	s1.SetText("shape-1")

	s2 := page.AddShape()
	s2.SetX(3.0)
	s2.SetY(3.5)
	s2.SetWidth(1.0)
	s2.SetHeight(0.2)
	s2.SetLocX(0.5)
	s2.SetLocY(0.1)
	s2.SetText("label-1")

	s3 := page.AddShape()
	s3.SetX(1.5)
	s3.SetY(2.5)
	s3.SetWidth(0.8)
	s3.SetHeight(0.2)
	s3.SetLocX(0.4)
	s3.SetLocY(0.1)
	s3.SetText("label-2")

	s1ID, s2ID, s3ID := s1.ID, s2.ID, s3.ID

	// Group all 3.
	group := page.GroupShapes([]*Shape{s1, s2, s3}, 0.05)
	if group == nil {
		t.Fatal("GroupShapes returned nil")
		return
	}
	if group.ShapeType != "Group" {
		t.Fatalf("ShapeType: got %q, want %q", group.ShapeType, "Group")
		return
	}

	// Group should have 3 children.
	children := group.ChildShapes()
	if len(children) != 3 {
		t.Fatalf("children: got %d, want 3", len(children))
	}

	// Original shape IDs must be preserved.
	childIDs := make(map[string]bool)
	for _, c := range children {
		childIDs[c.ID] = true
	}
	for _, id := range []string{s1ID, s2ID, s3ID} {
		if !childIDs[id] {
			t.Fatalf("child ID %s not found in group", id)
		}
	}

	// Page-level shapes should no longer include the grouped shapes.
	pageShapes := page.ChildShapes()
	for _, ps := range pageShapes {
		if ps.ID == s1ID || ps.ID == s2ID || ps.ID == s3ID {
			t.Fatalf("shape %s still at page level after grouping", ps.ID)
		}
	}

	// Group should appear at page level.
	found := false
	for _, ps := range pageShapes {
		if ps.ID == group.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("group not found at page level")
	}

	// ObjType should be 8 (group).
	if group.CellValue("ObjType") != "8" {
		t.Fatalf("ObjType: got %q, want %q", group.CellValue("ObjType"), "8")
	}

	// Save, reopen, verify structure survives round-trip.
	data, err := vis.SaveVsdxBytes()
	if err != nil {
		t.Fatalf("SaveVsdxBytes: %v", err)
	}

	vis2, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer vis2.Close() //nolint:errcheck

	page2 := vis2.GetPage(0)
	allShapes := page2.AllShapes()

	// Should find the group and its 3 children (4 total).
	groupFound := false
	for _, s := range page2.ChildShapes() {
		if s.ShapeType == "Group" {
			groupFound = true
			if ch := s.ChildShapes(); len(ch) != 3 {
				t.Fatalf("round-trip: group children: got %d, want 3", len(ch))
			}
			break
		}
	}
	if !groupFound {
		t.Fatalf("round-trip: group not found; all shapes: %d", len(allShapes))
	}
}

func TestGroupShapesWithConnectors(t *testing.T) {
	vis, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)

	// Create 2 shapes.
	s1 := page.AddShape()
	s1.SetX(4.0)
	s1.SetY(5.0)
	s1.SetWidth(0.6)
	s1.SetHeight(0.6)
	s1.SetLocX(0.3)
	s1.SetLocY(0.3)

	s2 := page.AddShape()
	s2.SetX(8.0)
	s2.SetY(5.0)
	s2.SetWidth(0.6)
	s2.SetHeight(0.6)
	s2.SetLocX(0.3)
	s2.SetLocY(0.3)

	// Create connector between them.
	conn, err := vis.ConnectShapes(page, s1, s2)
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}
	connID := conn.ID

	// Create a label shape to group with s1.
	label := page.AddShape()
	label.SetX(4.5)
	label.SetY(5.5)
	label.SetWidth(1.0)
	label.SetHeight(0.2)
	label.SetLocX(0.5)
	label.SetLocY(0.1)
	label.SetText("Gi0/0")

	s1ID := s1.ID

	// Group s1 with its label.
	group := page.GroupShapes([]*Shape{s1, label}, 0.05)
	if group == nil {
		t.Fatal("GroupShapes returned nil")
		return
	}

	// Connector should still reference original shape IDs.
	connects := page.Connects()
	foundBegin := false
	foundEnd := false
	for _, c := range connects {
		if c.ConnectorShapeID() == connID {
			if c.ShapeID() == s1ID {
				foundBegin = true
			}
		}
	}
	// Check EndX connect too.
	for _, c := range connects {
		if c.ConnectorShapeID() == connID {
			if c.ShapeID() == s2.ID {
				foundEnd = true
			}
		}
	}
	if !foundBegin {
		t.Fatalf("connector lost reference to shape %s", s1ID)
	}
	if !foundEnd {
		t.Fatalf("connector lost reference to shape %s", s2.ID)
	}

	// Save and reopen to verify XML integrity.
	data, err := vis.SaveVsdxBytes()
	if err != nil {
		t.Fatalf("SaveVsdxBytes: %v", err)
	}

	vis2, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer vis2.Close() //nolint:errcheck

	// Should have at least 2 Connect elements.
	page2 := vis2.GetPage(0)
	if len(page2.Connects()) < 2 {
		t.Fatalf("round-trip: connects: got %d, want at least 2", len(page2.Connects()))
	}
}

func TestGroupShapesEmpty(t *testing.T) {
	vis, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)

	// Nil input should return nil.
	if g := page.GroupShapes(nil, 0.1); g != nil {
		t.Fatal("expected nil for nil input")
	}

	// Empty slice should return nil.
	if g := page.GroupShapes([]*Shape{}, 0.1); g != nil {
		t.Fatal("expected nil for empty input")
	}
}

func TestFullTopologyDiagram(t *testing.T) {
	// Integration test: 2 Foreign shapes + connector + data properties
	vis, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)

	// Shape 1: "router"
	png1 := createTestPNG(color.RGBA{59, 130, 246, 255})
	rel1, _ := vis.AddImage(page, png1)
	s1 := page.AddShape()
	s1.SetX(4.0)
	s1.SetY(5.0)
	s1.SetWidth(0.6)
	s1.SetHeight(0.6)
	s1.SetForeignData(rel1)
	s1.SetText("router-1")
	s1.AddDataProperty("Hostname", "Hostname", "router-1")
	s1.AddDataProperty("RouterID", "Router ID", "10.0.0.1")

	// Shape 2: "switch"
	png2 := createTestPNG(color.RGBA{34, 197, 94, 255})
	rel2, _ := vis.AddImage(page, png2)
	s2 := page.AddShape()
	s2.SetX(8.0)
	s2.SetY(5.0)
	s2.SetWidth(0.6)
	s2.SetHeight(0.6)
	s2.SetForeignData(rel2)
	s2.SetText("switch-1")
	s2.AddDataProperty("Hostname", "Hostname", "switch-1")

	// Connector
	conn, err := vis.ConnectShapes(page, s1, s2)
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}
	conn.SetText("10")

	// Save and reopen
	data, err := vis.SaveVsdxBytes()
	if err != nil {
		t.Fatalf("SaveVsdxBytes: %v", err)
	}

	vis2, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer vis2.Close() //nolint:errcheck

	page2 := vis2.GetPage(0)
	shapes := page2.ChildShapes()

	// Should have 2 Foreign shapes + 1 connector = 3 shapes
	if len(shapes) < 3 {
		t.Fatalf("shapes: got %d, want at least 3", len(shapes))
	}

	// Verify connects
	connects := page2.Connects()
	if len(connects) < 2 {
		t.Fatalf("connects: got %d, want at least 2", len(connects))
	}

	// Save output for manual Visio inspection (optional)
	_ = vis.SaveVsdx(testFile("out/test_foreign_topology.vsdx"))
}
