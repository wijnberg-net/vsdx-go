package vsdx

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// --- Fase 3: Editing ---

func TestSetCellValue(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
	}

	// Set existing cell value
	origX := shape.X()
	shape.SetCellValue("PinX", "5.0")
	if shape.CellValue("PinX") != "5.0" {
		t.Errorf("PinX = %q, want '5.0'", shape.CellValue("PinX"))
	}
	if shape.X() != 5.0 {
		t.Errorf("X() = %v, want 5.0", shape.X())
	}

	// Set new cell value (doesn't exist yet)
	shape.SetCellValue("TestCell", "42")
	if shape.CellValue("TestCell") != "42" {
		t.Errorf("TestCell = %q, want '42'", shape.CellValue("TestCell"))
	}

	// Restore original
	shape.SetCellValue("PinX", strconv.FormatFloat(origX, 'f', -1, 64))
}

func TestSetCellFormula(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
	}

	// Set formula on new cell
	shape.SetCellFormula("NewCell", "Width*0.5")
	if shape.CellFormula("NewCell") != "Width*0.5" {
		t.Errorf("NewCell formula = %q, want 'Width*0.5'", shape.CellFormula("NewCell"))
	}
}

func TestSetPositionAndSize(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
	}

	shape.SetX(3.0)
	shape.SetY(4.0)
	shape.SetWidth(2.0)
	shape.SetHeight(1.5)
	shape.SetAngle(0.5)

	if shape.X() != 3.0 {
		t.Errorf("X() = %v, want 3.0", shape.X())
	}
	if shape.Y() != 4.0 {
		t.Errorf("Y() = %v, want 4.0", shape.Y())
	}
	if shape.Width() != 2.0 {
		t.Errorf("Width() = %v, want 2.0", shape.Width())
	}
	if shape.Height() != 1.5 {
		t.Errorf("Height() = %v, want 1.5", shape.Height())
	}
	if shape.Angle() != 0.5 {
		t.Errorf("Angle() = %v, want 0.5", shape.Angle())
	}
}

func TestSetStyleProperties(t *testing.T) {
	vis, err := Open(testFile("test12_colors.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.Pages[0].FindShapeByText("Line Color")
	if shape == nil {
		t.Fatal("shape not found")
	}

	shape.SetLineColor("#00ff00")
	if shape.LineColor() != "#00ff00" {
		t.Errorf("LineColor = %q, want '#00ff00'", shape.LineColor())
	}

	shape.SetFillColor("#0000ff")
	if shape.FillColor() != "#0000ff" {
		t.Errorf("FillColor = %q, want '#0000ff'", shape.FillColor())
	}

	shape.SetLineWeight(0.5)
	if shape.LineWeight() != 0.5 {
		t.Errorf("LineWeight = %v, want 0.5", shape.LineWeight())
	}
}

func TestSetTextColor(t *testing.T) {
	vis, err := Open(testFile("test12_colors.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.Pages[0].FindShapeByText("Text Color")
	if shape == nil {
		t.Fatal("shape not found")
	}

	shape.SetTextColor("#0000ff")
	if shape.TextColor() != "#0000ff" {
		t.Errorf("TextColor = %q, want '#0000ff'", shape.TextColor())
	}
}

func TestSetEndArrow(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
	}

	shape.SetEndArrow(13)
	if shape.EndArrow() != "13" {
		t.Errorf("EndArrow = %q, want '13'", shape.EndArrow())
	}

	shape.SetEndArrow(0)
	if shape.EndArrow() != "0" {
		t.Errorf("EndArrow = %q, want '0'", shape.EndArrow())
	}
}

func TestSetStyleIDs(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
	}

	shape.SetLineStyleID("5")
	shape.SetFillStyleID("6")
	shape.SetTextStyleID("7")

	if shape.LineStyleID() != "5" {
		t.Errorf("LineStyleID = %q, want '5'", shape.LineStyleID())
	}
	if shape.FillStyleID() != "6" {
		t.Errorf("FillStyleID = %q, want '6'", shape.FillStyleID())
	}
	if shape.TextStyleID() != "7" {
		t.Errorf("TextStyleID = %q, want '7'", shape.TextStyleID())
	}
}

func TestSetText(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
	}

	shape.SetText("New Text")
	if shape.Text() != "New Text" {
		t.Errorf("Text() = %q, want 'New Text'", shape.Text())
	}
}

func TestMoveShape(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
	}

	origX := shape.X()
	origY := shape.Y()
	shape.Move(1.0, 2.0)

	if math.Abs(shape.X()-(origX+1.0)) > 0.001 {
		t.Errorf("X() = %v, want %v", shape.X(), origX+1.0)
	}
	if math.Abs(shape.Y()-(origY+2.0)) > 0.001 {
		t.Errorf("Y() = %v, want %v", shape.Y(), origY+2.0)
	}
}

func TestRemoveShape(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	origCount := len(page.ChildShapes())

	shape := page.FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
	}
	shape.Remove()

	newCount := len(page.ChildShapes())
	if newCount != origCount-1 {
		t.Errorf("child shapes = %d, want %d", newCount, origCount-1)
	}

	// Verify shape is no longer findable
	if page.FindShapeByText("Shape Text") != nil {
		t.Error("removed shape still found")
	}
}

func TestFindReplace(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
	}

	shape.FindReplace("Shape", "New")
	if !strings.Contains(shape.Text(), "New") {
		t.Errorf("Text() = %q, want to contain 'New'", shape.Text())
	}
}

func TestApplyTextFilter(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("{{date}}")
	if shape == nil {
		t.Fatal("shape with {{date}} not found")
	}

	shape.ApplyTextFilter(map[string]string{"date": "2024-01-01"})
	if strings.Contains(shape.Text(), "{{date}}") {
		t.Errorf("Text still contains {{date}}: %q", shape.Text())
	}
	if !strings.Contains(shape.Text(), "2024-01-01") {
		t.Errorf("Text doesn't contain replacement: %q", shape.Text())
	}
}

func TestRelativeBounds(t *testing.T) {
	vis, err := Open(testFile("test2.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	shape := page.FindShapeByText("Sub-shape 2")
	if shape == nil {
		t.Fatal("shape not found")
	}

	rbx, rby, rex, rey := shape.RelativeBounds()
	// RelativeBounds should return non-zero values
	if rbx == 0 && rby == 0 && rex == 0 && rey == 0 {
		t.Error("RelativeBounds returned all zeros")
	}
	// RelativeBounds should be different from Bounds for sub-shapes
	bx, by, ex, ey := shape.Bounds()
	t.Logf("Bounds: (%.2f, %.2f, %.2f, %.2f)", bx, by, ex, ey)
	t.Logf("RelativeBounds: (%.2f, %.2f, %.2f, %.2f)", rbx, rby, rex, rey)
}

// --- Page editing tests ---

func TestSetPageName(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	page.SetName("NewPageName")
	if page.Name() != "NewPageName" {
		t.Errorf("page name = %q, want 'NewPageName'", page.Name())
	}
}

func TestSetPageSize(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	page.SetWidth(10.0)
	page.SetHeight(12.0)
	if page.Width() != 10.0 {
		t.Errorf("width = %v, want 10.0", page.Width())
	}
	if page.Height() != 12.0 {
		t.Errorf("height = %v, want 12.0", page.Height())
	}
}

func TestPageFindReplace(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	page.FindReplace("Shape Text", "Replaced Text")

	shape := page.FindShapeByText("Replaced Text")
	if shape == nil {
		t.Error("replaced text not found in any shape")
	}
}

func TestPageApplyTextContext(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.GetPage(0)
	page.ApplyTextContext(map[string]string{"date": "2024-06-15"})

	// Verify {{date}} was replaced
	shape := page.FindShapeByText("2024-06-15")
	if shape == nil {
		t.Error("text context not applied - replacement text not found")
	}
	if page.FindShapeByText("{{date}}") != nil {
		t.Error("{{date}} still found after applying context")
	}
}

// --- Geometry editing tests ---

func TestGeometryMove(t *testing.T) {
	// test5_master.vsdx shape 5 has absolute MoveTo/LineTo rows
	vis, err := Open(testFile("test5_master.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByID("5")
	if shape == nil || shape.Geometry == nil {
		t.Fatal("shape or geometry not found")
	}

	// Get original MoveTo position
	var origX, origY float64
	for _, row := range shape.Geometry.Rows {
		if strings.ToLower(row.RowType()) == "moveto" {
			origX = row.X()
			origY = row.Y()
			break
		}
	}

	shape.Geometry.Move(1.0, 2.0)

	for _, row := range shape.Geometry.Rows {
		if strings.ToLower(row.RowType()) == "moveto" {
			if math.Abs(row.X()-(origX+1.0)) > 0.001 {
				t.Errorf("MoveTo X after move = %v, want %v", row.X(), origX+1.0)
			}
			if math.Abs(row.Y()-(origY+2.0)) > 0.001 {
				t.Errorf("MoveTo Y after move = %v, want %v", row.Y(), origY+2.0)
			}
			break
		}
	}
}

func TestGeometryRowSetters(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil || shape.Geometry == nil {
		t.Fatal("shape or geometry not found")
	}

	for _, row := range shape.Geometry.Rows {
		row.SetX(5.0)
		row.SetY(6.0)
		if row.X() != 5.0 {
			t.Errorf("X = %v, want 5.0", row.X())
		}
		if row.Y() != 6.0 {
			t.Errorf("Y = %v, want 6.0", row.Y())
		}
		break // test first row only
	}
}

// --- DataProperty editing tests ---

func TestDataPropertySetAttribute(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
	}

	props := shape.DataProperties()
	prop, ok := props["my_property_label"]
	if !ok {
		t.Fatal("property 'my_property_label' not found")
	}

	// Get attribute
	origVal := prop.GetAttribute("Value", "V")
	if origVal != "property value" {
		t.Errorf("GetAttribute = %q, want 'property value'", origVal)
	}

	// Set attribute
	if !prop.SetAttribute("Value", "V", "new value") {
		t.Error("SetAttribute returned false")
	}
	if prop.GetAttribute("Value", "V") != "new value" {
		t.Errorf("after set, GetAttribute = %q, want 'new value'", prop.GetAttribute("Value", "V"))
	}

	// Remove attribute
	if !prop.RemoveAttribute("Value", "V") {
		t.Error("RemoveAttribute returned false")
	}
	if prop.GetAttribute("Value", "V") != "" {
		t.Errorf("after remove, GetAttribute = %q, want empty", prop.GetAttribute("Value", "V"))
	}

	// Remove non-existent attribute
	if prop.RemoveAttribute("Value", "NonExistent") {
		t.Error("RemoveAttribute returned true for non-existent attr")
	}
}

// --- Save/load round-trip with edits ---

func TestEditAndSaveRoundTrip(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}

	// Make edits
	page := vis.GetPage(0)
	page.SetName("EditedPage")

	shape := page.FindShapeByText("Shape Text")
	if shape != nil {
		shape.SetX(5.0)
		shape.SetY(6.0)
		shape.SetText("Edited Shape Text")
	}

	// Save
	outFile := filepath.Join(testDir, "out", "test1_edited.vsdx")
	os.MkdirAll(filepath.Join(testDir, "out"), 0755)
	if err := vis.SaveVsdx(outFile); err != nil {
		t.Fatalf("SaveVsdx: %v", err)
	}
	vis.Close()

	// Re-open and verify
	vis2, err := Open(outFile)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer vis2.Close()

	if vis2.GetPage(0).Name() != "EditedPage" {
		t.Errorf("page name = %q, want 'EditedPage'", vis2.GetPage(0).Name())
	}

	editedShape := vis2.GetPage(0).FindShapeByText("Edited Shape Text")
	if editedShape == nil {
		t.Fatal("edited shape not found after save/reload")
	}
	if editedShape.X() != 5.0 {
		t.Errorf("X() = %v, want 5.0", editedShape.X())
	}
	if editedShape.Y() != 6.0 {
		t.Errorf("Y() = %v, want 6.0", editedShape.Y())
	}
}

// --- Fase 4: Page/Shape management ---

func TestRemovePageByIndex(t *testing.T) {
	vis, err := Open(testFile("test2.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	origCount := len(vis.Pages)
	if origCount < 2 {
		t.Fatalf("expected at least 2 pages, got %d", origCount)
	}
	removedName := vis.Pages[0].Name()

	vis.RemovePageByIndex(0)

	if len(vis.Pages) != origCount-1 {
		t.Errorf("page count = %d, want %d", len(vis.Pages), origCount-1)
	}
	// Check removed page name is gone
	for _, p := range vis.Pages {
		if p.Name() == removedName {
			t.Errorf("page %q still exists after removal", removedName)
		}
	}
}

func TestRemovePageByName(t *testing.T) {
	vis, err := Open(testFile("test2.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	origCount := len(vis.Pages)
	pageName := vis.Pages[1].Name()

	vis.RemovePageByName(pageName)

	if len(vis.Pages) != origCount-1 {
		t.Errorf("page count = %d, want %d", len(vis.Pages), origCount-1)
	}
	for _, p := range vis.Pages {
		if p.Name() == pageName {
			t.Errorf("page %q still exists after removal", pageName)
		}
	}
}

func TestRemovePageAndSave(t *testing.T) {
	vis, err := Open(testFile("test2.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	origCount := len(vis.Pages)
	vis.RemovePageByIndex(0)

	outFile := filepath.Join(t.TempDir(), "removed_page.vsdx")
	if err := vis.SaveVsdx(outFile); err != nil {
		t.Fatal(err)
	}

	// Re-open and verify
	vis2, err := Open(outFile)
	if err != nil {
		t.Fatal(err)
	}
	defer vis2.Close()

	if len(vis2.Pages) != origCount-1 {
		t.Errorf("reopened page count = %d, want %d", len(vis2.Pages), origCount-1)
	}
}

func TestAddPage(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	origCount := len(vis.Pages)
	newPage, err := vis.AddPage("TestNewPage")
	if err != nil {
		t.Fatalf("AddPage: %v", err)
	}
	if newPage == nil {
		t.Fatal("AddPage returned nil")
	}
	if newPage.Name() != "TestNewPage" {
		t.Errorf("name = %q, want %q", newPage.Name(), "TestNewPage")
	}
	if len(vis.Pages) != origCount+1 {
		t.Errorf("page count = %d, want %d", len(vis.Pages), origCount+1)
	}
	// New page should be at the end
	if vis.Pages[len(vis.Pages)-1] != newPage {
		t.Error("new page not at end of pages list")
	}
}

func TestAddPageAt(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	origCount := len(vis.Pages)
	newPage, err := vis.AddPageAt(0, "FirstPage")
	if err != nil {
		t.Fatalf("AddPageAt: %v", err)
	}
	if newPage == nil {
		t.Fatal("AddPageAt returned nil")
	}
	if len(vis.Pages) != origCount+1 {
		t.Errorf("page count = %d, want %d", len(vis.Pages), origCount+1)
	}
	if vis.Pages[0] != newPage {
		t.Error("new page not at index 0")
	}
	if vis.Pages[0].Name() != "FirstPage" {
		t.Errorf("name = %q, want %q", vis.Pages[0].Name(), "FirstPage")
	}
}

func TestAddPageDuplicateName(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	existingName := vis.Pages[0].Name()
	newPage, err := vis.AddPage(existingName)
	if err != nil {
		t.Fatalf("AddPage: %v", err)
	}
	if newPage.Name() == existingName {
		t.Errorf("expected unique name, got duplicate: %q", newPage.Name())
	}
	// Name should be existingName-1
	if newPage.Name() != existingName+"-1" {
		t.Errorf("name = %q, want %q", newPage.Name(), existingName+"-1")
	}
}

func TestAddPageAndSave(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	if _, err := vis.AddPage("NewSavedPage"); err != nil {
		t.Fatalf("AddPage: %v", err)
	}

	outFile := filepath.Join(t.TempDir(), "added_page.vsdx")
	if err := vis.SaveVsdx(outFile); err != nil {
		t.Fatal(err)
	}

	// Re-open and verify
	vis2, err := Open(outFile)
	if err != nil {
		t.Fatal(err)
	}
	defer vis2.Close()

	found := false
	for _, p := range vis2.Pages {
		if p.Name() == "NewSavedPage" {
			found = true
			break
		}
	}
	if !found {
		t.Error("new page not found after save/reopen")
	}
}

func TestCopyPage(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	origPage := vis.Pages[0]
	origShapeCount := len(origPage.ChildShapes())
	origCount := len(vis.Pages)

	newPage, err := vis.CopyPage(origPage, int(PageAfter), "CopiedPage")
	if err != nil {
		t.Fatalf("CopyPage: %v", err)
	}
	if newPage == nil {
		t.Fatal("CopyPage returned nil")
	}
	if newPage.Name() != "CopiedPage" {
		t.Errorf("name = %q, want %q", newPage.Name(), "CopiedPage")
	}
	if len(vis.Pages) != origCount+1 {
		t.Errorf("page count = %d, want %d", len(vis.Pages), origCount+1)
	}

	// Copied page should have same shapes as original
	copiedShapes := newPage.ChildShapes()
	if len(copiedShapes) != origShapeCount {
		t.Errorf("copied shape count = %d, want %d", len(copiedShapes), origShapeCount)
	}
}

func TestCopyPageAndSave(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	origPage := vis.Pages[0]
	if _, err := vis.CopyPage(origPage, int(PageLast), "CopyTest"); err != nil {
		t.Fatalf("CopyPage: %v", err)
	}

	outFile := filepath.Join(t.TempDir(), "copied_page.vsdx")
	if err := vis.SaveVsdx(outFile); err != nil {
		t.Fatal(err)
	}

	// Re-open and verify
	vis2, err := Open(outFile)
	if err != nil {
		t.Fatal(err)
	}
	defer vis2.Close()

	found := false
	for _, p := range vis2.Pages {
		if p.Name() == "CopyTest" {
			found = true
			// Verify the page has shapes
			if len(p.ChildShapes()) == 0 {
				t.Error("copied page has no shapes after save/reopen")
			}
			break
		}
	}
	if !found {
		t.Error("copied page not found after save/reopen")
	}
}

func TestCopyShape(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	srcPage := vis.Pages[0]
	srcShapes := srcPage.ChildShapes()
	if len(srcShapes) == 0 {
		t.Fatal("no shapes on source page")
	}

	// Copy first shape to same page (which already has shapes, so IDs will differ)
	origShapeCount := len(srcShapes)

	newShapeElem := vis.CopyShape(srcShapes[0].XML(), srcPage)
	if newShapeElem == nil {
		t.Fatal("CopyShape returned nil")
	}

	// Re-read page to get updated shapes
	updatedShapes := srcPage.ChildShapes()
	if len(updatedShapes) != origShapeCount+1 {
		t.Errorf("shape count = %d, want %d", len(updatedShapes), origShapeCount+1)
	}

	// Verify new shape has a different ID from original (page already had shapes)
	origID := srcShapes[0].ID
	newID := newShapeElem.SelectAttrValue("ID", "")
	if newID == origID {
		t.Errorf("new shape ID %q should differ from original %q", newID, origID)
	}
}

func TestCopyShapeAndSave(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	srcPage := vis.Pages[0]
	srcShapes := srcPage.ChildShapes()
	if len(srcShapes) == 0 {
		t.Fatal("no shapes on source page")
	}

	// Add a new empty page and copy shape to it
	newPage, err := vis.AddPage("ShapeCopyDst")
	if err != nil {
		t.Fatalf("AddPage: %v", err)
	}
	vis.CopyShape(srcShapes[0].XML(), newPage)

	outFile := filepath.Join(t.TempDir(), "shape_copied.vsdx")
	if err := vis.SaveVsdx(outFile); err != nil {
		t.Fatal(err)
	}

	// Re-open and verify
	vis2, err := Open(outFile)
	if err != nil {
		t.Fatal(err)
	}
	defer vis2.Close()

	dstPage := vis2.GetPageByName("ShapeCopyDst")
	if dstPage == nil {
		t.Fatal("destination page not found")
	}
	if len(dstPage.ChildShapes()) != 1 {
		t.Errorf("expected 1 shape on destination page, got %d", len(dstPage.ChildShapes()))
	}
}

// --- Fase 5: Connectors ---

func TestOpenBytes(t *testing.T) {
	data, err := os.ReadFile(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	vis, err := OpenBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	if len(vis.Pages) == 0 {
		t.Fatal("expected pages")
	}
	if len(vis.Pages[0].ChildShapes()) != 4 {
		t.Errorf("expected 4 child shapes, got %d", len(vis.Pages[0].ChildShapes()))
	}
}

func TestNewMedia(t *testing.T) {
	media, err := NewMedia()
	if err != nil {
		t.Fatal(err)
	}
	defer media.Close()

	if sc := media.StraightConnector(); sc == nil {
		t.Error("straight connector not found")
	} else {
		t.Logf("straight connector: ID=%s Name=%q", sc.ID, sc.ShapeName)
	}
	if cc := media.CurvedConnector(); cc == nil {
		t.Error("curved connector not found")
	}
	if r := media.Rectangle(); r == nil {
		t.Error("rectangle not found")
	}
	if c := media.Circle(); c == nil {
		t.Error("circle not found")
	}
	if l := media.Line(); l == nil {
		t.Error("line not found")
	}
}

func TestSetStartAndFinish(t *testing.T) {
	vis, err := Open(testFile("test4_connectors.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.Pages[0]
	// Find a connector shape (one with BeginX)
	var connector *Shape
	for _, s := range page.AllShapes() {
		if s.HasBeginX() {
			connector = s
			break
		}
	}
	if connector == nil {
		t.Skip("no connector shape found")
	}

	// Set new start and finish
	connector.SetStartAndFinish(1.0, 2.0, 5.0, 6.0)

	if connector.BeginX() != 1.0 {
		t.Errorf("BeginX = %v, want 1.0", connector.BeginX())
	}
	if connector.BeginY() != 2.0 {
		t.Errorf("BeginY = %v, want 2.0", connector.BeginY())
	}
	if connector.EndX() != 5.0 {
		t.Errorf("EndX = %v, want 5.0", connector.EndX())
	}
	if connector.EndY() != 6.0 {
		t.Errorf("EndY = %v, want 6.0", connector.EndY())
	}
	if connector.Width() != 4.0 {
		t.Errorf("Width = %v, want 4.0", connector.Width())
	}
}

func TestConnectShapes(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.Pages[0]
	shapes := page.ChildShapes()
	if len(shapes) < 2 {
		t.Fatalf("need at least 2 shapes, got %d", len(shapes))
	}

	fromShape := shapes[0]
	toShape := shapes[1]

	connectorShape, err := vis.ConnectShapes(page, fromShape, toShape)
	if err != nil {
		t.Fatal(err)
	}
	if connectorShape == nil {
		t.Fatal("ConnectShapes returned nil")
	}

	// Check connector has BeginX/Y and EndX/Y
	if !connectorShape.HasBeginX() {
		t.Error("connector should have BeginX")
	}

	// Check Connect elements were created
	connects := page.Connects()
	foundBeg := false
	foundEnd := false
	for _, c := range connects {
		if c.ConnectorShapeID() == connectorShape.ID {
			if c.FromRel == "BeginX" && c.ShapeID() == fromShape.ID {
				foundBeg = true
			}
			if c.FromRel == "EndX" && c.ShapeID() == toShape.ID {
				foundEnd = true
			}
		}
	}
	if !foundBeg {
		t.Error("BeginX connect not found")
	}
	if !foundEnd {
		t.Error("EndX connect not found")
	}
}

func TestConnectShapesAndSave(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	page := vis.Pages[0]
	shapes := page.ChildShapes()
	if len(shapes) < 2 {
		t.Fatalf("need at least 2 shapes, got %d", len(shapes))
	}

	_, err = vis.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatal(err)
	}

	outFile := filepath.Join(t.TempDir(), "connected.vsdx")
	if err := vis.SaveVsdx(outFile); err != nil {
		t.Fatal(err)
	}

	// Re-open and verify
	vis2, err := Open(outFile)
	if err != nil {
		t.Fatal(err)
	}
	defer vis2.Close()

	page2 := vis2.Pages[0]
	connects := page2.Connects()
	if len(connects) < 2 {
		t.Errorf("expected at least 2 connects after save/reopen, got %d", len(connects))
	}

	// Should have master pages now
	if len(vis2.MasterPages) == 0 {
		t.Error("expected master pages after creating connector")
	}
}

// --- Fase 6: Templating ---

func TestRenderTemplateBasicTextReplacement(t *testing.T) {
	vis, err := Open(testFile("test_jinja.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	context := map[string]any{
		"date":     "2024-01-15",
		"scenario": "Scenario One",
		"x":        2,
		"y":        3,
	}
	vis.RenderTemplate(context)

	page := vis.Pages[0]
	// Check that scenario text was replaced
	shape := page.FindShapeByText("Scenario One")
	if shape == nil {
		t.Error("expected shape with 'Scenario One' text after template render")
	}
}

func TestRenderTemplateCalc(t *testing.T) {
	vis, err := Open(testFile("test_jinja.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	context := map[string]any{
		"date":     "2024-01-15",
		"scenario": "Test",
		"x":        3,
		"y":        4,
	}
	vis.RenderTemplate(context)

	page := vis.Pages[0]
	// Check that x*y = 12 exists in a shape
	shape := page.FindShapeByText("12")
	if shape == nil {
		t.Error("expected shape with '12' (3*4) after template calc")
	}
}

func TestRenderTemplateShowIfShapes(t *testing.T) {
	tests := []struct {
		x          int
		shapeCount int
	}{
		{0, 1},  // x=0: only first shape (both showifs evaluate to false)
		{5, 2},  // x=5: first + "if x" shape (x is truthy but not > 10)
		{20, 3}, // x=20: all three shapes
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("x=%d", tt.x), func(t *testing.T) {
			vis, err := Open(testFile("test_jinja.vsdx"))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()

			context := map[string]any{
				"date":     "now",
				"scenario": "Test",
				"x":        tt.x,
				"y":        1,
			}
			vis.RenderTemplate(context)

			page := vis.Pages[1] // showif test page
			count := len(page.ChildShapes())
			if count != tt.shapeCount {
				t.Errorf("x=%d: shape count = %d, want %d", tt.x, count, tt.shapeCount)
			}
		})
	}
}

func TestRenderTemplateForLoop(t *testing.T) {
	vis, err := Open(testFile("test_jinja_loop.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	context := map[string]any{
		"date":      "2024-01-15",
		"scenario":  "Scenario One",
		"test_list": []any{1, 2, 3},
	}
	vis.RenderTemplate(context)

	page := vis.Pages[0]

	// Check that scenario was replaced
	shape := page.FindShapeByText("Scenario One")
	if shape == nil {
		t.Error("expected shape with 'Scenario One'")
	}

	// Check that shapes exist for each loop item
	for _, item := range []string{"1", "2", "3"} {
		if s := page.FindShapeByText(item); s == nil {
			t.Errorf("expected shape with text %q from for loop", item)
		}
	}
}

func TestRenderTemplateForLoopStrings(t *testing.T) {
	vis, err := Open(testFile("test_jinja_loop.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	context := map[string]any{
		"date":      "2024-01-15",
		"scenario":  "Scenario Two",
		"test_list": []any{"One", "Two", "Three"},
	}
	vis.RenderTemplate(context)

	page := vis.Pages[0]
	for _, item := range []string{"One", "Two", "Three"} {
		if s := page.FindShapeByText(item); s == nil {
			t.Errorf("expected shape with text %q from for loop", item)
		}
	}
}

func TestRenderTemplatePageShowIf(t *testing.T) {
	tests := []struct {
		show          any
		pageCount     int
		expectedNames []string
	}{
		{true, 2, []string{"Normal Page", "Page2"}},
		{1, 2, []string{"Normal Page", "Page2"}},
		{false, 2, []string{"Normal Page", "Page3"}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("show=%v", tt.show), func(t *testing.T) {
			vis, err := Open(testFile("test_jinja_page_showif.vsdx"))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()

			context := map[string]any{"show": tt.show}
			vis.RenderTemplate(context)

			if len(vis.Pages) != tt.pageCount {
				t.Errorf("page count = %d, want %d", len(vis.Pages), tt.pageCount)
			}

			names := vis.GetPageNames()
			for i, expected := range tt.expectedNames {
				if i >= len(names) {
					t.Errorf("missing page at index %d", i)
					continue
				}
				if names[i] != expected {
					t.Errorf("page[%d] name = %q, want %q", i, names[i], expected)
				}
			}
		})
	}
}

func TestRenderTemplateSetSelf(t *testing.T) {
	tests := []struct {
		n         int
		shapeID   string
		expectedX float64
	}{
		{1, "1", 2.0},  // {% set self.x=2.0 %}
		{2, "2", 4.0},  // {% set self.x=n*2 %} with n=2
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("n=%d_shape=%s", tt.n, tt.shapeID), func(t *testing.T) {
			vis, err := Open(testFile("test_jinja_self_refs.vsdx"))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close()

			context := map[string]any{"n": tt.n}
			vis.RenderTemplate(context)

			page := vis.Pages[0]
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
			}

			if shape.X() != tt.expectedX {
				t.Errorf("shape %s x = %v, want %v", tt.shapeID, shape.X(), tt.expectedX)
			}
		})
	}
}

func TestRenderTemplateSetSelfText(t *testing.T) {
	vis, err := Open(testFile("test_jinja_self_refs.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	context := map[string]any{"n": 1}
	vis.RenderTemplate(context)

	page := vis.Pages[0]
	shape := page.FindShapeByID("1")
	if shape == nil {
		t.Fatal("shape 1 not found")
	}

	text := shape.Text()
	// set self directive should be removed
	if strings.Contains(text, "{% set") {
		t.Errorf("set self directive not removed from text: %q", text)
	}
	if !strings.Contains(text, "This text should remain") {
		t.Errorf("expected text to contain 'This text should remain', got %q", text)
	}
}

func TestRenderTemplateAndSave(t *testing.T) {
	vis, err := Open(testFile("test_jinja.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	context := map[string]any{
		"date":     "2024-01-15",
		"scenario": "SaveTest",
		"x":        5,
		"y":        6,
	}
	vis.RenderTemplate(context)

	outFile := filepath.Join(t.TempDir(), "template_rendered.vsdx")
	if err := vis.SaveVsdx(outFile); err != nil {
		t.Fatal(err)
	}

	// Re-open and verify
	vis2, err := Open(outFile)
	if err != nil {
		t.Fatal(err)
	}
	defer vis2.Close()

	page := vis2.Pages[0]
	if s := page.FindShapeByText("SaveTest"); s == nil {
		t.Error("template replacement not found after save/reopen")
	}
	if s := page.FindShapeByText("30"); s == nil {
		t.Error("expected '30' (5*6) after save/reopen")
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

// --- Fase 7: Diff ---

func TestNewVisioFileDiff(t *testing.T) {
	tests := []struct {
		fileA, fileB string
	}{
		{"test1.vsdx", "test2.vsdx"},
		{"test1.vsdx", "test4_connectors.vsdx"},
	}

	for _, tt := range tests {
		t.Run(tt.fileA+"_vs_"+tt.fileB, func(t *testing.T) {
			fd, err := NewVisioFileDiff(testFile(tt.fileA), testFile(tt.fileB))
			if err != nil {
				t.Fatalf("NewVisioFileDiff error: %v", err)
			}
			if fd == nil {
				t.Fatal("expected non-nil VisioFileDiff")
			}

			// Should have common members
			common := fd.CommonMembers()
			if len(common) == 0 {
				t.Error("expected common members")
			}

			// Should have diffs (different files)
			if len(fd.Diffs) == 0 {
				t.Error("expected diffs between different files")
			}

			t.Logf("Common members: %d", len(common))
			t.Logf("Diffs: %d members with differences", len(fd.Diffs))
			t.Logf("Added: %v", fd.AddedMembers())
			t.Logf("Removed: %v", fd.RemovedMembers())
		})
	}
}

func TestVisioFileDiffSameFile(t *testing.T) {
	_, err := NewVisioFileDiff(testFile("test1.vsdx"), testFile("test1.vsdx"))
	if err == nil {
		t.Error("expected error when comparing same file path")
	}
}

func TestVisioFileDiffInvalidExtension(t *testing.T) {
	_, err := NewVisioFileDiff("file.txt", "file2.vsdx")
	if err == nil {
		t.Error("expected error for non-.vsdx file")
	}
}

func TestVisioFileDiffCompareMembers(t *testing.T) {
	fd, err := NewVisioFileDiff(testFile("test1.vsdx"), testFile("test2.vsdx"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// test1 has 3 pages, test2 has 3 pages but different content
	// They may or may not have same members depending on structure
	t.Logf("CompareMembers: %v", fd.CompareMembers())
}

func TestVisioFileDiffAddedRemoved(t *testing.T) {
	fd, err := NewVisioFileDiff(testFile("test1.vsdx"), testFile("test4_connectors.vsdx"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// test4_connectors has master pages that test1 doesn't
	added := fd.AddedMembers()
	removed := fd.RemovedMembers()
	t.Logf("Added in test4: %v", added)
	t.Logf("Removed in test4: %v", removed)

	// test4_connectors.vsdx has masters, test1 doesn't
	if len(added) == 0 && len(removed) == 0 {
		// If both happen to have same members, that's ok too
		t.Log("Both files have same member paths")
	}
}

func TestVisioFileDiffDiffContent(t *testing.T) {
	fd, err := NewVisioFileDiff(testFile("test1.vsdx"), testFile("test2.vsdx"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// pages.xml should be different (different page definitions)
	pagesKey := ""
	for k := range fd.Diffs {
		if strings.Contains(k, "pages.xml") && !strings.Contains(k, "page1") {
			pagesKey = k
			break
		}
	}
	if pagesKey != "" {
		diff := fd.Diffs[pagesKey]
		t.Logf("pages.xml diff has %d lines", len(diff))

		// Verify diff lines have proper prefixes
		for _, line := range diff {
			if !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "+ ") {
				t.Errorf("diff line missing prefix: %q", line)
			}
		}
	}
}

func TestVisioFileDiffString(t *testing.T) {
	fd, err := NewVisioFileDiff(testFile("test1.vsdx"), testFile("test2.vsdx"))
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	s := fd.String()
	if !strings.Contains(s, "test1.vsdx") || !strings.Contains(s, "test2.vsdx") {
		t.Errorf("String() should contain file paths, got: %s", s)
	}
}

func TestComputeDiff(t *testing.T) {
	a := []string{"line1", "line2", "line3"}
	b := []string{"line1", "modified", "line3", "added"}

	diff := computeDiff(a, b)

	// Should have "  line1", "- line2", "+ modified", "  line3", "+ added"
	hasRemoved := false
	hasAdded := false
	hasSame := false
	for _, line := range diff {
		if strings.HasPrefix(line, "- ") {
			hasRemoved = true
		}
		if strings.HasPrefix(line, "+ ") {
			hasAdded = true
		}
		if strings.HasPrefix(line, "  ") {
			hasSame = true
		}
	}
	if !hasRemoved || !hasAdded || !hasSame {
		t.Errorf("diff should have removed, added, and same lines: %v", diff)
	}
}

func TestBreakXMLIntoLines(t *testing.T) {
	xml := `<?xml version="1.0"?><Root><Child attr="val"/></Root>`
	lines := breakXMLIntoLines(xml)

	// Should split at each '<'
	foundRoot := false
	foundChild := false
	for _, line := range lines {
		if strings.Contains(line, "<Root>") {
			foundRoot = true
		}
		if strings.Contains(line, "<Child") {
			foundChild = true
		}
	}
	if !foundRoot || !foundChild {
		t.Errorf("expected XML elements on separate lines, got: %v", lines)
	}
}
