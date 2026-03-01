package vsdx

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var testDir string

func init() {
	// Find the tests directory relative to this test file
	testDir = filepath.Join("..", "tests")
}

func testFile(name string) string {
	return filepath.Join(testDir, name)
}

// --- Open / Close ---

func TestOpenInvalidFileType(t *testing.T) {
	_, err := Open("test.txt")
	if err == nil {
		t.Fatal("expected error for invalid file type")
	}
}

func TestOpenRelPath(t *testing.T) {
	for _, filename := range []string{"test1.vsdx", "diagram_with_macro.vsdm"} {
		t.Run(filename, func(t *testing.T) {
			vis, err := Open(testFile(filename))
			if err != nil {
				t.Fatalf("Open(%s): %v", filename, err)
			}
			defer vis.Close()
			for _, page := range vis.Pages {
				t.Logf("Page: %s", page.Name())
			}
		})
	}
}

func TestLoadZipFileContents(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	if len(vis.ZipFileContents) == 0 {
		t.Fatal("expected non-empty zip file contents")
	}
	for path, content := range vis.ZipFileContents {
		if len(content) == 0 {
			t.Errorf("empty content for %s", path)
		}
	}
}

// --- Pages ---

func TestGetPage(t *testing.T) {
	tests := []struct {
		filename string
		pageName string
	}{
		{"test1.vsdx", "Page-1"},
		{"test2.vsdx", "Page-1"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.GetPage(0)
			if page == nil {
				t.Fatal("expected page 0")
			}
			if page.Name() != tt.pageName {
				t.Errorf("page name = %q, want %q", page.Name(), tt.pageName)
			}
		})
	}
}

func TestGetPageName(t *testing.T) {
	tests := []struct {
		filename  string
		pageIndex int
		pageName  string
	}{
		{"test1.vsdx", 0, "Page-1"},
		{"test2.vsdx", 0, "Page-1"},
		{"test1.vsdx", 1, "Page-2"},
		{"test2.vsdx", 1, "Page-2"},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_page%d", tt.filename, tt.pageIndex)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.Pages[tt.pageIndex]
			if page.Name() != tt.pageName {
				t.Errorf("page name = %q, want %q", page.Name(), tt.pageName)
			}
		})
	}
}

func TestGetPageSize(t *testing.T) {
	tests := []struct {
		filename  string
		pageIndex int
		width     float64
		height    float64
	}{
		{"test1.vsdx", 0, 8.26771653543307, 11.69291338582677},
		{"test2.vsdx", 0, 8.26771653543307, 11.69291338582677},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_page%d", tt.filename, tt.pageIndex)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.Pages[tt.pageIndex]
			if page.Width() != tt.width {
				t.Errorf("width = %v, want %v", page.Width(), tt.width)
			}
			if page.Height() != tt.height {
				t.Errorf("height = %v, want %v", page.Height(), tt.height)
			}
		})
	}
}

// --- Shapes ---

func TestGetPageChildShapes(t *testing.T) {
	tests := []struct {
		filename   string
		childCount int
	}{
		{"test1.vsdx", 4},
		{"test2.vsdx", 6},
		{"test10_nested_shapes.vsdx", 2},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.GetPage(0)
			shapes := page.ChildShapes()
			if len(shapes) != tt.childCount {
				t.Errorf("child shapes = %d, want %d", len(shapes), tt.childCount)
			}
		})
	}
}

func TestGetPageAllShapes(t *testing.T) {
	tests := []struct {
		filename string
		allCount int
	}{
		{"test1.vsdx", 4},
		{"test2.vsdx", 14},
		{"test10_nested_shapes.vsdx", 8},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.GetPage(0)
			all := page.AllShapes()
			if len(all) != tt.allCount {
				t.Errorf("all shapes = %d, want %d", len(all), tt.allCount)
			}
		})
	}
}

func TestXMLFindAllShapes(t *testing.T) {
	tests := []struct {
		filename      string
		shapeElements int
	}{
		{"test1.vsdx", 4},
		{"test2.vsdx", 14},
		{"test3_house.vsdx", 10},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.GetPage(0)
			elements := page.XML().Root().FindElements(".//Shape")
			if len(elements) != tt.shapeElements {
				t.Errorf("Shape elements = %d, want %d", len(elements), tt.shapeElements)
			}
		})
	}
}

func TestXMLFindAllGroupShapes(t *testing.T) {
	tests := []struct {
		filename           string
		groupShapeElements int
	}{
		{"test1.vsdx", 0},
		{"test2.vsdx", 3},
		{"test3_house.vsdx", 2},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.GetPage(0)
			var count int
			for _, elem := range page.XML().Root().FindElements(".//Shape") {
				if elem.SelectAttrValue("Type", "") == "Group" {
					count++
				}
			}
			if count != tt.groupShapeElements {
				t.Errorf("Group Shape elements = %d, want %d", count, tt.groupShapeElements)
			}
		})
	}
}

// --- Shape text ---

func TestGetShapeText(t *testing.T) {
	tests := []struct {
		filename     string
		shapeID      string
		expectedText string
	}{
		{"test3_house.vsdx", "1", "Shape Text\n"},
		{"test3_house.vsdx", "11", "Shape to remove\n"},
		{"test2.vsdx", "9", "Group shape text\n"},
		{"test10_nested_shapes.vsdx", "5", "Shape 1.2.1\n"},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_shape%s", tt.filename, tt.shapeID)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.GetPage(0)
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
			}
			if shape.Text() != tt.expectedText {
				t.Errorf("text = %q, want %q", shape.Text(), tt.expectedText)
			}
		})
	}
}

func TestGetShapeWithText(t *testing.T) {
	tests := []struct {
		filename string
		shapeID  string
	}{
		{"test1.vsdx", "6"},
		{"test2.vsdx", "6"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.GetPage(0)
			shape := page.FindShapeByText("{{date}}")
			if shape == nil {
				t.Fatal("shape not found")
			}
			if shape.ID != tt.shapeID {
				t.Errorf("shape ID = %q, want %q", shape.ID, tt.shapeID)
			}
		})
	}
}

// --- Shape child/group shapes ---

func TestGetShapeChildShapes(t *testing.T) {
	tests := []struct {
		filename   string
		shapeID    string
		childCount int
	}{
		{"test3_house.vsdx", "7", 3},
		{"test3_house.vsdx", "11", 3},
		{"test2.vsdx", "9", 3},
		{"test10_nested_shapes.vsdx", "7", 2},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_shape%s", tt.filename, tt.shapeID)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.GetPage(0)
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
			}
			if len(shape.ChildShapes()) != tt.childCount {
				t.Errorf("child shapes = %d, want %d", len(shape.ChildShapes()), tt.childCount)
			}
		})
	}
}

func TestGetShapeAllShapes(t *testing.T) {
	tests := []struct {
		filename string
		shapeID  string
		allCount int
	}{
		{"test3_house.vsdx", "7", 3},
		{"test3_house.vsdx", "11", 3},
		{"test2.vsdx", "9", 3},
		{"test10_nested_shapes.vsdx", "7", 6},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_shape%s", tt.filename, tt.shapeID)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.GetPage(0)
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
			}
			if len(shape.AllShapes()) != tt.allCount {
				t.Errorf("all shapes = %d, want %d", len(shape.AllShapes()), tt.allCount)
			}
		})
	}
}

// --- Shape positions ---

func TestShapeLocations(t *testing.T) {
	tests := []struct {
		filename          string
		expectedLocations string
	}{
		{"test1.vsdx", "1.33,10.66 4.13,10.66 6.94,10.66 2.33,9.02 "},
		{"test2.vsdx", "2.33,8.72 1.33,10.66 4.13,10.66 5.91,8.72 1.61,8.58 3.25,8.65 "},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.GetPage(0)
			shapes := page.ChildShapes()
			locations := ""
			for _, shape := range shapes {
				locations += fmt.Sprintf("%.2f,%.2f ", shape.X(), shape.Y())
			}
			if locations != tt.expectedLocations {
				t.Errorf("locations = %q, want %q", locations, tt.expectedLocations)
			}
		})
	}
}

func TestShapeCenter(t *testing.T) {
	tests := []struct {
		filename string
		shapeID  string
		centerX  float64
		centerY  float64
	}{
		{"test1.vsdx", "1", 1.332677148526936, 10.65551182326173},
		{"test2.vsdx", "2", 1.082677148526936, 0.7874015625650443},   // group shape
		{"test2.vsdx", "16", 1.6903102768832179, 8.188976116607332}, // line
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_shape%s", tt.filename, tt.shapeID)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.GetPage(0)
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
			}
			cx, cy := shape.CenterXY()
			if cx != tt.centerX || cy != tt.centerY {
				t.Errorf("center = (%v, %v), want (%v, %v)", cx, cy, tt.centerX, tt.centerY)
			}
		})
	}
}

// --- Page IDs ---

func TestPageID(t *testing.T) {
	tests := []struct {
		filename       string
		pageIndex      int
		expectedPageID string
	}{
		{"test1.vsdx", 0, "0"},
		{"test1.vsdx", 1, "4"},
		{"test1.vsdx", 2, "7"},
		{"test2.vsdx", 0, "0"},
		{"test2.vsdx", 1, "4"},
		{"test2.vsdx", 2, "5"},
		{"test4_connectors.vsdx", 0, "0"},
		{"test4_connectors.vsdx", 1, "4"},
		{"test4_connectors.vsdx", 2, "7"},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_page%d", tt.filename, tt.pageIndex)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.Pages[tt.pageIndex]
			if page.PageID() != tt.expectedPageID {
				t.Errorf("page ID = %q, want %q", page.PageID(), tt.expectedPageID)
			}
		})
	}
}

func TestMasterPageID(t *testing.T) {
	tests := []struct {
		filename       string
		masterIndex    int
		expectedPageID string
	}{
		{"test3_house.vsdx", 0, "2"},
		{"test5_master.vsdx", 0, "1"},
		{"test4_connectors.vsdx", 0, "2"},
		{"test4_connectors.vsdx", 1, "6"},
		{"test4_connectors.vsdx", 2, "7"},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_master%d", tt.filename, tt.masterIndex)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			if tt.masterIndex >= len(vis.MasterPages) {
				t.Fatalf("master index %d out of range (have %d)", tt.masterIndex, len(vis.MasterPages))
			}
			page := vis.MasterPages[tt.masterIndex]
			if page.PageID() != tt.expectedPageID {
				t.Errorf("master page ID = %q, want %q", page.PageID(), tt.expectedPageID)
			}
		})
	}
}

func TestPageHasPageSheetXML(t *testing.T) {
	for _, filename := range []string{"test1.vsdx", "test2.vsdx", "test4_connectors.vsdx"} {
		t.Run(filename, func(t *testing.T) {
			vis, err := Open(testFile(filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			for i, page := range vis.Pages {
				if page.pagesheetXML() == nil {
					t.Errorf("page %d has nil pagesheet XML", i)
				}
			}
		})
	}
}

func TestMasterPageHasSheetXML(t *testing.T) {
	for _, filename := range []string{"test3_house.vsdx", "test5_master.vsdx", "test4_connectors.vsdx"} {
		t.Run(filename, func(t *testing.T) {
			vis, err := Open(testFile(filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			for _, page := range vis.MasterPages {
				if page.pagesheetXML() == nil {
					t.Errorf("master page %s (ID=%s) has nil pagesheet XML", page.Name(), page.PageID())
				}
			}
		})
	}
}

// --- Page all_shapes with IDs ---

func TestPageAllShapesIDs(t *testing.T) {
	tests := []struct {
		filename    string
		pageIndex   int
		expectedIDs []string
	}{
		{"test1.vsdx", 0, []string{"1", "2", "5", "6"}},
		{"test1.vsdx", 1, nil},
		{"test1.vsdx", 2, []string{"1"}},
		{"test2.vsdx", 0, []string{"6", "9", "1", "7", "8", "11", "2", "10", "14", "5", "12", "13", "16", "17"}},
		{"test2.vsdx", 1, nil},
		{"test2.vsdx", 2, []string{"1", "2", "3", "4"}},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_page%d", tt.filename, tt.pageIndex)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.Pages[tt.pageIndex]
			all := page.AllShapes()
			var ids []string
			for _, s := range all {
				ids = append(ids, s.ID)
			}
			if len(ids) != len(tt.expectedIDs) {
				t.Fatalf("all shapes IDs = %v, want %v", ids, tt.expectedIDs)
			}
			for i, id := range ids {
				if id != tt.expectedIDs[i] {
					t.Errorf("shape[%d] ID = %q, want %q", i, id, tt.expectedIDs[i])
				}
			}
		})
	}
}

// --- Connects ---

func TestFindPageConnects(t *testing.T) {
	vis, err := Open(testFile("test4_connectors.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	connects := page.Connects()

	expected := []string{"from 7 to 5", "from 7 to 2", "from 6 to 2", "from 6 to 1"}
	var actual []string
	for _, c := range connects {
		actual = append(actual, fmt.Sprintf("from %s to %s", c.FromID, c.ToID))
	}
	sort.Strings(expected)
	sort.Strings(actual)

	if len(actual) != len(expected) {
		t.Fatalf("connects = %v, want %v", actual, expected)
	}
	for i := range actual {
		if actual[i] != expected[i] {
			t.Errorf("connect[%d] = %q, want %q", i, actual[i], expected[i])
		}
	}
}

func TestFindConnectedShapes(t *testing.T) {
	tests := []struct {
		shapeID          string
		expectedShapeIDs []string
	}{
		{"1", []string{"6"}},
		{"2", []string{"6", "7"}},
	}
	vis, err := Open(testFile("test4_connectors.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	for _, tt := range tests {
		t.Run("shape_"+tt.shapeID, func(t *testing.T) {
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
			}
			var ids []string
			for _, s := range shape.ConnectedShapes() {
				ids = append(ids, s.ID)
			}
			sort.Strings(ids)
			sort.Strings(tt.expectedShapeIDs)
			if len(ids) != len(tt.expectedShapeIDs) {
				t.Fatalf("connected shapes = %v, want %v", ids, tt.expectedShapeIDs)
			}
			for i := range ids {
				if ids[i] != tt.expectedShapeIDs[i] {
					t.Errorf("connected[%d] = %q, want %q", i, ids[i], tt.expectedShapeIDs[i])
				}
			}
		})
	}
}

func TestFindConnectedShapeRelationships(t *testing.T) {
	tests := []struct {
		shapeID         string
		expectedFromIDs []string
		expectedToIDs   []string
		expectedFromRel []string
		expectedToRel   []string
	}{
		{"1", []string{"6"}, []string{"1"}, []string{"BeginX"}, []string{"PinX"}},
		{"2", []string{"6", "7"}, []string{"2", "2"}, []string{"BeginX", "EndX"}, []string{"PinX", "PinX"}},
		{"6", []string{"6", "6"}, []string{"1", "2"}, []string{"BeginX", "EndX"}, []string{"PinX", "PinX"}},
	}
	vis, err := Open(testFile("test4_connectors.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	for _, tt := range tests {
		t.Run("shape_"+tt.shapeID, func(t *testing.T) {
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
			}
			var fromIDs, toIDs, fromRels, toRels []string
			for _, c := range shape.Connects() {
				fromIDs = append(fromIDs, c.FromID)
				toIDs = append(toIDs, c.ToID)
				fromRels = append(fromRels, c.FromRel)
				toRels = append(toRels, c.ToRel)
			}
			sort.Strings(fromIDs)
			sort.Strings(toIDs)
			sort.Strings(fromRels)
			sort.Strings(toRels)
			sort.Strings(tt.expectedFromIDs)
			sort.Strings(tt.expectedToIDs)
			sort.Strings(tt.expectedFromRel)
			sort.Strings(tt.expectedToRel)

			assertStringSlice(t, "fromIDs", fromIDs, tt.expectedFromIDs)
			assertStringSlice(t, "toIDs", toIDs, tt.expectedToIDs)
			assertStringSlice(t, "fromRels", fromRels, tt.expectedFromRel)
			assertStringSlice(t, "toRels", toRels, tt.expectedToRel)
		})
	}
}

// --- Data Properties ---

func TestGetShapeDataProperties(t *testing.T) {
	tests := []struct {
		filename     string
		pageIndex    int
		shapeName    string
		propertyDict map[string]string
	}{
		{"test1.vsdx", 0, "Shape Text", map[string]string{
			"my_property_label":        "property value",
			"my_second_property_label": "another value",
			"Network Name":             "Box01",
		}},
		{"test6_shape_properties.vsdx", 2, "A", map[string]string{
			"master_Prop": "master prop value",
		}},
		{"test6_shape_properties.vsdx", 2, "B", map[string]string{
			"master_Prop": "master prop value",
			"shape_prop":  "shape property value",
		}},
		{"test6_shape_properties.vsdx", 2, "C", map[string]string{
			"master_Prop": "override",
		}},
		{"test6_shape_properties.vsdx", 2, "D", map[string]string{
			"LongProp": `value not in an "attrib"`,
		}},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_page%d_%s", tt.filename, tt.pageIndex, tt.shapeName)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()

			shape := vis.Pages[tt.pageIndex].FindShapeByText(tt.shapeName)
			if shape == nil {
				t.Fatalf("shape %q not found", tt.shapeName)
			}

			props := shape.DataProperties()
			if len(props) != len(tt.propertyDict) {
				t.Errorf("property count = %d, want %d", len(props), len(tt.propertyDict))
				for k, v := range props {
					t.Logf("  property: label=%q value=%q", k, v.Value())
				}
			}

			for label, expectedValue := range tt.propertyDict {
				prop, ok := props[label]
				if !ok {
					t.Errorf("property %q not found", label)
					continue
				}
				if prop.Value() != expectedValue {
					t.Errorf("property %q value = %q, want %q", label, prop.Value(), expectedValue)
				}
			}
		})
	}
}

func TestFindShapeByPropertyLabel(t *testing.T) {
	tests := []struct {
		filename          string
		pageIndex         int
		expectedShapeName string
		propertyLabel     string
	}{
		{"test1.vsdx", 0, "Shape Text", "my_property_label"},
		{"test6_shape_properties.vsdx", 0, "Shape One", "my_property_label"},
		{"test6_shape_properties.vsdx", 2, "A", "master_Prop"},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_page%d_%s", tt.filename, tt.pageIndex, tt.propertyLabel)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()

			shape := vis.Pages[tt.pageIndex].FindShapeByPropertyLabel(tt.propertyLabel)
			if shape == nil {
				t.Fatalf("shape with property %q not found", tt.propertyLabel)
			}

			text := trimNewlines(shape.Text())
			if text != tt.expectedShapeName {
				t.Errorf("shape text = %q, want %q", text, tt.expectedShapeName)
			}
		})
	}
}

func TestFindShapesByPropertyLabelValue(t *testing.T) {
	tests := []struct {
		filename           string
		pageIndex          int
		expectedShapeNames []string
		propertyLabel      string
		propertyValue      string
	}{
		{"test1.vsdx", 0, []string{"Shape Text"}, "my_property_label", "property value"},
		{"test6_shape_properties.vsdx", 2, []string{"A", "B"}, "master_Prop", "master prop value"},
		{"test6_shape_properties.vsdx", 2, []string{"C"}, "master_Prop", "override"},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_page%d_%s_%s", tt.filename, tt.pageIndex, tt.propertyLabel, tt.propertyValue)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()

			shapes := vis.Pages[tt.pageIndex].FindShapesByPropertyLabelValue(tt.propertyLabel, tt.propertyValue)
			if len(shapes) != len(tt.expectedShapeNames) {
				t.Fatalf("shapes count = %d, want %d", len(shapes), len(tt.expectedShapeNames))
			}

			var names []string
			for _, s := range shapes {
				names = append(names, trimNewlines(s.Text()))
			}
			sort.Strings(names)
			sort.Strings(tt.expectedShapeNames)
			assertStringSlice(t, "shape names", names, tt.expectedShapeNames)
		})
	}
}

// --- Geometry ---

func TestGetShapeGeometry(t *testing.T) {
	tests := []struct {
		filename       string
		pageIndex      int
		shapeText      string
		expectedCoords string // simplified expected output
	}{
		{"test1.vsdx", 0, "Shape Text", "RelMoveTo:X=0,Y=0|RelLineTo:X=1,Y=0|RelLineTo:X=1,Y=1|RelLineTo:X=0,Y=1|RelLineTo:X=0,Y=0"},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_page%d_%s", tt.filename, tt.pageIndex, tt.shapeText)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.Pages[tt.pageIndex]
			shape := page.FindShapeByText(tt.shapeText)
			if shape == nil {
				t.Fatalf("shape %q not found", tt.shapeText)
			}
			if shape.Geometry == nil {
				t.Fatal("shape has no geometry")
			}

			// Build simplified geometry string
			result := ""
			for _, row := range sortedGeometryRows(shape.Geometry.Rows) {
				if result != "" {
					result += "|"
				}
				cells := ""
				for _, cellName := range []string{"X", "Y", "A", "B", "C", "D"} {
					if cell, ok := row.Cells[cellName]; ok {
						if cells != "" {
							cells += ","
						}
						cells += cellName + "=" + cell.Value()
					}
				}
				result += row.RowType() + ":" + cells
			}

			if result != tt.expectedCoords {
				t.Errorf("geometry = %q, want %q", result, tt.expectedCoords)
			}
		})
	}
}

// --- Shape angle ---

func TestGetShapeAngle(t *testing.T) {
	tests := []struct {
		shapeID       string
		expectedAngle float64
	}{
		{"1", 0.52},
		{"2", -1.39},
		{"5", 2.53},
		{"6", -0.26},
	}
	vis, err := Open(testFile("test11_rotate.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	for _, tt := range tests {
		t.Run("shape_"+tt.shapeID, func(t *testing.T) {
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
			}
			if math.Abs(shape.Angle()-tt.expectedAngle) >= 0.01 {
				t.Errorf("angle = %v, want ~%v", shape.Angle(), tt.expectedAngle)
			}
		})
	}
}

// --- Shape bounds ---

func TestShapeBounds(t *testing.T) {
	tests := []struct {
		filename       string
		pageIndex      int
		shapeText      string
		expectedBounds []string
	}{
		{"test1.vsdx", 0, "Shape Text", []string{"0.25", "9.87", "2.42", "11.44"}},
		{"test2.vsdx", 0, "Sub-shape 2", []string{"0.54", "0.17", "1.62", "0.51"}},
		{"test2.vsdx", 0, "Scenario:", []string{"0.73", "8.19", "2.49", "8.97"}},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_page%d_%s", tt.filename, tt.pageIndex, tt.shapeText)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()
			page := vis.Pages[tt.pageIndex]
			shape := page.FindShapeByText(tt.shapeText)
			if shape == nil {
				t.Fatalf("shape %q not found", tt.shapeText)
			}
			bx, by, ex, ey := shape.Bounds()
			bounds := []string{
				fmt.Sprintf("%.2f", bx),
				fmt.Sprintf("%.2f", by),
				fmt.Sprintf("%.2f", ex),
				fmt.Sprintf("%.2f", ey),
			}
			assertStringSlice(t, "bounds", bounds, tt.expectedBounds)
		})
	}
}

// --- Colors ---

func TestGetShapeColors(t *testing.T) {
	tests := []struct {
		pageIndex     int
		shapeText     string
		colorParam    string
		expectedColor string
	}{
		{0, "Line Color", "line", "#ff0000"},
		{0, "Text Color", "text", "#ff0000"},
		{0, "Fill Color", "fill", "#ff0000"},
		{1, "Line Color", "line", "#00ff00"},
		{1, "Text Color", "text", "#00ff00"},
		{1, "Fill Color", "fill", "#00ff00"},
	}
	vis, err := Open(testFile("test12_colors.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	for _, tt := range tests {
		name := fmt.Sprintf("page%d_%s_%s", tt.pageIndex, tt.shapeText, tt.colorParam)
		t.Run(name, func(t *testing.T) {
			shape := vis.Pages[tt.pageIndex].FindShapeByText(tt.shapeText)
			if shape == nil {
				t.Fatalf("shape %q not found", tt.shapeText)
			}

			var color string
			switch tt.colorParam {
			case "line":
				color = shape.LineColor()
			case "text":
				color = shape.TextColor()
			case "fill":
				color = shape.FillColor()
			}

			if color != tt.expectedColor {
				t.Errorf("%s color = %q, want %q", tt.colorParam, color, tt.expectedColor)
			}
		})
	}
}

// --- Master shapes ---

func TestGetShapeMasterPage(t *testing.T) {
	vis, err := Open(testFile("test_master_multiple_child_shapes.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	childShape := vis.GetPage(0).ChildShapes()[0]
	masterPage := childShape.MasterPage()
	if masterPage == nil {
		t.Fatal("expected master page")
	}
	expectedName := "AWS Step Functions workflow "
	if masterPage.Name() != expectedName {
		t.Errorf("master page name = %q, want %q", masterPage.Name(), expectedName)
	}
}

// --- Save/Load round-trip ---

func TestSaveAndReopen(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(testDir, "out", "test1_go_save.vsdx")
	os.MkdirAll(filepath.Join(testDir, "out"), 0755)

	if err := vis.SaveVsdx(outFile); err != nil {
		t.Fatalf("SaveVsdx: %v", err)
	}
	vis.Close()

	// Re-open
	vis2, err := Open(outFile)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer vis2.Close()

	if len(vis2.Pages) != 3 {
		t.Errorf("pages = %d, want 3", len(vis2.Pages))
	}
	if vis2.GetPage(0).Name() != "Page-1" {
		t.Errorf("page name = %q, want 'Page-1'", vis2.GetPage(0).Name())
	}
}

// --- Fase 2: Navigation ---

func TestShapeValue(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
	}
	// ID attribute should be accessible via ShapeValue
	if shape.ShapeValue("ID") != shape.ID {
		t.Errorf("ShapeValue(ID) = %q, want %q", shape.ShapeValue("ID"), shape.ID)
	}
	// Non-existent attribute should return empty string
	if shape.ShapeValue("NonExistent") != "" {
		t.Errorf("ShapeValue(NonExistent) = %q, want empty", shape.ShapeValue("NonExistent"))
	}
}

func TestFindShapesByID(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	// Shape ID "1" should exist
	shapes := page.FindShapesByID("1")
	if len(shapes) != 1 {
		t.Errorf("FindShapesByID('1') count = %d, want 1", len(shapes))
	}
	// Non-existent shape ID
	shapes = page.FindShapesByID("999")
	if len(shapes) != 0 {
		t.Errorf("FindShapesByID('999') count = %d, want 0", len(shapes))
	}
}

func TestFindShapesByMaster(t *testing.T) {
	vis, err := Open(testFile("test5_master.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	// Shapes 1 and 3 both have MasterPageID="1"
	s := page.shapes()
	if len(s) == 0 {
		t.Fatal("no shapes container")
	}
	found := s[0].FindShapesByMaster("1", "")
	if len(found) != 2 {
		t.Fatalf("FindShapesByMaster('1', '') count = %d, want 2", len(found))
	}
	var ids []string
	for _, f := range found {
		ids = append(ids, f.ID)
	}
	sort.Strings(ids)
	assertStringSlice(t, "shape IDs", ids, []string{"1", "3"})
}

func TestFindShapesByRegex(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)

	// Find shapes with text matching "Shape"
	shapes, err := page.FindShapesByRegex("Shape")
	if err != nil {
		t.Fatal(err)
	}
	if len(shapes) == 0 {
		t.Error("expected at least one shape matching 'Shape'")
	}

	// Find shapes with text matching date pattern
	shapes, err = page.FindShapesByRegex(`\{\{date\}\}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(shapes) != 1 {
		t.Errorf("shapes matching date pattern = %d, want 1", len(shapes))
	}

	// Invalid regex should return error
	_, err = page.FindShapesByRegex("[invalid")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestFindShapesWithSameMaster(t *testing.T) {
	vis, err := Open(testFile("test3_house.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	// Get first shape that has a master
	var shapeWithMaster *Shape
	for _, s := range page.AllShapes() {
		if s.MasterPageID != "" {
			shapeWithMaster = s
			break
		}
	}
	if shapeWithMaster == nil {
		t.Skip("no shape with master found")
	}

	found := page.FindShapesWithSameMaster(shapeWithMaster)
	if len(found) == 0 {
		t.Error("expected at least one shape with same master")
	}
	// The original shape should be in the results
	var foundOriginal bool
	for _, s := range found {
		if s.ID == shapeWithMaster.ID {
			foundOriginal = true
			break
		}
	}
	if !foundOriginal {
		t.Error("original shape not found in results")
	}
}

func TestGetConnectorsBetween(t *testing.T) {
	vis, err := Open(testFile("test4_connectors.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)

	// Shapes 1 and 2 are connected via connector shape 6
	connectors, err := page.GetConnectorsBetween("1", "", "2", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(connectors) != 1 {
		t.Fatalf("connectors between 1 and 2 = %d, want 1", len(connectors))
	}
	if connectors[0].ID != "6" {
		t.Errorf("connector ID = %q, want '6'", connectors[0].ID)
	}

	// Non-existent shape should return error
	_, err = page.GetConnectorsBetween("999", "", "1", "")
	if err == nil {
		t.Error("expected error for non-existent shape")
	}
}

func TestConnectString(t *testing.T) {
	vis, err := Open(testFile("test4_connectors.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	connects := page.Connects()
	if len(connects) == 0 {
		t.Fatal("expected connects")
	}
	str := connects[0].String()
	if str == "" {
		t.Error("expected non-empty string representation")
	}
	// Should contain "Connect:" prefix
	if !strings.Contains(str, "Connect:") {
		t.Errorf("String() = %q, want to contain 'Connect:'", str)
	}
}

// --- Helpers ---

func assertStringSlice(t *testing.T, name string, actual, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("%s: len = %d, want %d: %v vs %v", name, len(actual), len(expected), actual, expected)
	}
	for i := range actual {
		if actual[i] != expected[i] {
			t.Errorf("%s[%d] = %q, want %q", name, i, actual[i], expected[i])
		}
	}
}

func trimNewlines(s string) string {
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return s
}

// sortedGeometryRows returns geometry rows sorted by their IX index.
func sortedGeometryRows(rows map[string]*GeometryRow) []*GeometryRow {
	var keys []string
	for k := range rows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var result []*GeometryRow
	for _, k := range keys {
		result = append(result, rows[k])
	}
	return result
}
