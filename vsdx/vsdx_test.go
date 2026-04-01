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
		return
	}
}

func TestOpenRelPath(t *testing.T) {
	for _, filename := range []string{"test1.vsdx", "diagram_with_macro.vsdm"} {
		t.Run(filename, func(t *testing.T) {
			vis, err := Open(testFile(filename))
			if err != nil {
				t.Fatalf("Open(%s): %v", filename, err)
			}
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
	defer vis.Close() //nolint:errcheck

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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
			page := vis.GetPage(0)
			if page == nil {
				t.Fatal("expected page 0")
				return
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
			page := vis.GetPage(0)
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
				return
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
			page := vis.GetPage(0)
			shape := page.FindShapeByText("{{date}}")
			if shape == nil {
				t.Fatal("shape not found")
				return
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
			page := vis.GetPage(0)
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
				return
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
			page := vis.GetPage(0)
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
				return
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
		{"test2.vsdx", "2", 1.082677148526936, 0.7874015625650443},  // group shape
		{"test2.vsdx", "16", 1.6903102768832179, 8.188976116607332}, // line
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%s_shape%s", tt.filename, tt.shapeID)
		t.Run(name, func(t *testing.T) {
			vis, err := Open(testFile(tt.filename))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close() //nolint:errcheck //nolint:errcheck
			page := vis.GetPage(0)
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
				return
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	for _, tt := range tests {
		t.Run("shape_"+tt.shapeID, func(t *testing.T) {
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
				return
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
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	for _, tt := range tests {
		t.Run("shape_"+tt.shapeID, func(t *testing.T) {
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
				return
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck

			shape := vis.Pages[tt.pageIndex].FindShapeByText(tt.shapeName)
			if shape == nil {
				t.Fatalf("shape %q not found", tt.shapeName)
				return
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck

			shape := vis.Pages[tt.pageIndex].FindShapeByPropertyLabel(tt.propertyLabel)
			if shape == nil {
				t.Fatalf("shape with property %q not found", tt.propertyLabel)
				return
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck

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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
			page := vis.Pages[tt.pageIndex]
			shape := page.FindShapeByText(tt.shapeText)
			if shape == nil {
				t.Fatalf("shape %q not found", tt.shapeText)
				return
			}
			if shape.Geometry == nil {
				t.Fatal("shape has no geometry")
				return
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
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	for _, tt := range tests {
		t.Run("shape_"+tt.shapeID, func(t *testing.T) {
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
				return
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
			defer vis.Close() //nolint:errcheck //nolint:errcheck
			page := vis.Pages[tt.pageIndex]
			shape := page.FindShapeByText(tt.shapeText)
			if shape == nil {
				t.Fatalf("shape %q not found", tt.shapeText)
				return
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
	defer vis.Close() //nolint:errcheck

	for _, tt := range tests {
		name := fmt.Sprintf("page%d_%s_%s", tt.pageIndex, tt.shapeText, tt.colorParam)
		t.Run(name, func(t *testing.T) {
			shape := vis.Pages[tt.pageIndex].FindShapeByText(tt.shapeText)
			if shape == nil {
				t.Fatalf("shape %q not found", tt.shapeText)
				return
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
	defer vis.Close() //nolint:errcheck

	childShape := vis.GetPage(0).ChildShapes()[0]
	masterPage := childShape.MasterPage()
	if masterPage == nil {
		t.Fatal("expected master page")
		return
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
	_ = os.MkdirAll(filepath.Join(testDir, "out"), 0755)

	if err := vis.SaveVsdx(outFile); err != nil {
		t.Fatalf("SaveVsdx: %v", err)
	}
	_ = vis.Close()

	// Re-open
	vis2, err := Open(outFile)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer vis2.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.Pages[0].FindShapeByText("Line Color")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.Pages[0].FindShapeByText("Text Color")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	origCount := len(page.ChildShapes())

	shape := page.FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("{{date}}")
	if shape == nil {
		t.Fatal("shape with {{date}} not found")
		return
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
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	shape := page.FindShapeByText("Sub-shape 2")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByID("5")
	if shape == nil || shape.Geometry == nil {
		t.Fatal("shape or geometry not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil || shape.Geometry == nil {
		t.Fatal("shape or geometry not found")
		return
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
	defer vis.Close() //nolint:errcheck

	shape := vis.GetPage(0).FindShapeByText("Shape Text")
	if shape == nil {
		t.Fatal("shape not found")
		return
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
	_ = os.MkdirAll(filepath.Join(testDir, "out"), 0755)
	if err := vis.SaveVsdx(outFile); err != nil {
		t.Fatalf("SaveVsdx: %v", err)
	}
	_ = vis.Close()

	// Re-open and verify
	vis2, err := Open(outFile)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer vis2.Close() //nolint:errcheck

	if vis2.GetPage(0).Name() != "EditedPage" {
		t.Errorf("page name = %q, want 'EditedPage'", vis2.GetPage(0).Name())
	}

	editedShape := vis2.GetPage(0).FindShapeByText("Edited Shape Text")
	if editedShape == nil {
		t.Fatal("edited shape not found after save/reload")
		return
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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis2.Close() //nolint:errcheck

	if len(vis2.Pages) != origCount-1 {
		t.Errorf("reopened page count = %d, want %d", len(vis2.Pages), origCount-1)
	}
}

func TestAddPage(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	origCount := len(vis.Pages)
	newPage, err := vis.AddPage("TestNewPage")
	if err != nil {
		t.Fatalf("AddPage: %v", err)
	}
	if newPage == nil {
		t.Fatal("AddPage returned nil")
		return
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
	defer vis.Close() //nolint:errcheck

	origCount := len(vis.Pages)
	newPage, err := vis.AddPageAt(0, "FirstPage")
	if err != nil {
		t.Fatalf("AddPageAt: %v", err)
	}
	if newPage == nil {
		t.Fatal("AddPageAt returned nil")
		return
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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis2.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

	origPage := vis.Pages[0]
	origShapeCount := len(origPage.ChildShapes())
	origCount := len(vis.Pages)

	newPage, err := vis.CopyPage(origPage, int(PageAfter), "CopiedPage")
	if err != nil {
		t.Fatalf("CopyPage: %v", err)
	}
	if newPage == nil {
		t.Fatal("CopyPage returned nil")
		return
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
	defer vis.Close() //nolint:errcheck

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
	defer vis2.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
		return
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
	defer vis.Close() //nolint:errcheck

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
	defer vis2.Close() //nolint:errcheck

	dstPage := vis2.GetPageByName("ShapeCopyDst")
	if dstPage == nil {
		t.Fatal("destination page not found")
		return
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
	defer vis.Close() //nolint:errcheck

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
	defer media.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
		return
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
	defer vis.Close() //nolint:errcheck

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
	defer vis2.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
			defer vis.Close() //nolint:errcheck //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
	defer vis.Close() //nolint:errcheck

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
			defer vis.Close() //nolint:errcheck //nolint:errcheck

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
		{1, "1", 2.0}, // {% set self.x=2.0 %}
		{2, "2", 4.0}, // {% set self.x=n*2 %} with n=2
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("n=%d_shape=%s", tt.n, tt.shapeID), func(t *testing.T) {
			vis, err := Open(testFile("test_jinja_self_refs.vsdx"))
			if err != nil {
				t.Fatal(err)
			}
			defer vis.Close() //nolint:errcheck //nolint:errcheck

			context := map[string]any{"n": tt.n}
			vis.RenderTemplate(context)

			page := vis.Pages[0]
			shape := page.FindShapeByID(tt.shapeID)
			if shape == nil {
				t.Fatalf("shape %s not found", tt.shapeID)
				return
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
	defer vis.Close() //nolint:errcheck

	context := map[string]any{"n": 1}
	vis.RenderTemplate(context)

	page := vis.Pages[0]
	shape := page.FindShapeByID("1")
	if shape == nil {
		t.Fatal("shape 1 not found")
		return
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
	defer vis.Close() //nolint:errcheck

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
	defer vis2.Close() //nolint:errcheck

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
				return
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

// =============================================================================
// Network Diagram Tests
// =============================================================================

// buildNetworkTopology creates a small network: 2 routers + 1 switch + connectors + data properties.
// Returns the VisioFile (caller must Close) and the page.
func buildNetworkTopology(t *testing.T) (*VisioFile, *Page, []*Shape) {
	t.Helper()
	vis, err := Open(testFile("blank.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	page := vis.GetPage(0)

	// Router 1
	r1 := page.AddShape()
	r1.SetX(2.0)
	r1.SetY(8.0)
	r1.SetWidth(1.0)
	r1.SetHeight(1.0)
	r1.SetText("router-1")
	r1.AddDataProperty("Hostname", "Hostname", "router-1")
	r1.AddDataProperty("IP", "IP Address", "10.0.0.1")
	r1.AddDataProperty("Role", "Role", "core")

	// Router 2
	r2 := page.AddShape()
	r2.SetX(6.0)
	r2.SetY(8.0)
	r2.SetWidth(1.0)
	r2.SetHeight(1.0)
	r2.SetText("router-2")
	r2.AddDataProperty("Hostname", "Hostname", "router-2")
	r2.AddDataProperty("IP", "IP Address", "10.0.0.2")
	r2.AddDataProperty("Role", "Role", "core")

	// Switch 1
	sw := page.AddShape()
	sw.SetX(4.0)
	sw.SetY(4.0)
	sw.SetWidth(1.2)
	sw.SetHeight(0.8)
	sw.SetText("switch-1")
	sw.AddDataProperty("Hostname", "Hostname", "switch-1")
	sw.AddDataProperty("IP", "IP Address", "10.0.1.1")
	sw.AddDataProperty("Role", "Role", "access")

	// Connect router-1 to switch-1
	conn1, err := vis.ConnectShapes(page, r1, sw)
	if err != nil {
		t.Fatalf("ConnectShapes r1-sw: %v", err)
	}
	conn1.SetText("ge-0/0/0")

	// Connect router-2 to switch-1
	conn2, err := vis.ConnectShapes(page, r2, sw)
	if err != nil {
		t.Fatalf("ConnectShapes r2-sw: %v", err)
	}
	conn2.SetText("ge-0/0/1")

	return vis, page, []*Shape{r1, r2, sw}
}

func TestNetworkFindShapesByText(t *testing.T) {
	vis, page, _ := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Page.FindShapesByText - find all routers
	routers := page.FindShapesByText("router")
	if len(routers) != 2 {
		t.Fatalf("FindShapesByText('router') = %d shapes, want 2", len(routers))
	}

	// Find single switch
	switches := page.FindShapesByText("switch")
	if len(switches) != 1 {
		t.Fatalf("FindShapesByText('switch') = %d shapes, want 1", len(switches))
	}
	if switches[0].Text() != "switch-1" {
		t.Errorf("switch text = %q, want 'switch-1'", switches[0].Text())
	}

	// No match
	none := page.FindShapesByText("firewall")
	if len(none) != 0 {
		t.Errorf("FindShapesByText('firewall') = %d shapes, want 0", len(none))
	}
}

func TestNetworkFindShapeByAttr(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Page.FindShapeByAttr - find by ID
	found := page.FindShapeByAttr("ID", shapes[0].ID)
	if found == nil {
		t.Fatal("FindShapeByAttr by ID returned nil")
	}
	if found.Text() != "router-1" {
		t.Errorf("found text = %q, want 'router-1'", found.Text())
	}

	// Not found
	notFound := page.FindShapeByAttr("ID", "99999")
	if notFound != nil {
		t.Error("FindShapeByAttr should return nil for nonexistent ID")
	}
}

func TestNetworkFindByPropertyLabel(t *testing.T) {
	vis, page, _ := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Page.FindShapesByPropertyLabel - all shapes with "Role"
	withRole := page.FindShapesByPropertyLabel("Role")
	if len(withRole) != 3 {
		t.Fatalf("FindShapesByPropertyLabel('Role') = %d, want 3", len(withRole))
	}

	// Page.FindShapeByPropertyLabelValue - find the access switch
	accessShape := page.FindShapeByPropertyLabelValue("Role", "access")
	if accessShape == nil {
		t.Fatal("FindShapeByPropertyLabelValue('Role','access') returned nil")
	}
	if accessShape.Text() != "switch-1" {
		t.Errorf("access shape text = %q, want 'switch-1'", accessShape.Text())
	}

	// Find all core devices
	coreDevices := page.FindShapesByPropertyLabelValue("Role", "core")
	if len(coreDevices) != 2 {
		t.Fatalf("FindShapesByPropertyLabelValue('Role','core') = %d, want 2", len(coreDevices))
	}

	// Not found
	nope := page.FindShapeByPropertyLabelValue("Role", "distribution")
	if nope != nil {
		t.Error("expected nil for nonexistent property value")
	}
}

func TestNetworkConnectTraversal(t *testing.T) {
	vis, page, _ := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	connects := page.Connects()
	if len(connects) < 2 {
		t.Fatalf("connects = %d, want >= 2", len(connects))
	}

	// Test Connect.Shape() and Connect.ConnectorShape()
	for _, c := range connects {
		shape := c.Shape()
		connShape := c.ConnectorShape()
		if shape == nil {
			t.Error("Connect.Shape() returned nil")
		}
		if connShape == nil {
			t.Error("Connect.ConnectorShape() returned nil")
		}
	}
}

func TestNetworkPropertyUpdate(t *testing.T) {
	vis, page, _ := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Find router-1 and update its IP
	router := page.FindShapeByPropertyLabelValue("Hostname", "router-1")
	if router == nil {
		t.Fatal("router-1 not found")
	}

	props := router.DataProperties()
	ipProp, ok := props["IP Address"]
	if !ok {
		t.Fatal("'IP Address' property not found")
	}
	if ipProp.Value() != "10.0.0.1" {
		t.Errorf("IP = %q, want '10.0.0.1'", ipProp.Value())
	}

	// SetValue
	ipProp.SetValue("192.168.1.1")
	if ipProp.Value() != "192.168.1.1" {
		t.Errorf("after SetValue, IP = %q, want '192.168.1.1'", ipProp.Value())
	}

	// Round-trip: save and reopen
	data, err := vis.SaveVsdxBytes()
	if err != nil {
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	vis2, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer vis2.Close() //nolint:errcheck

	router2 := vis2.GetPage(0).FindShapeByPropertyLabelValue("Hostname", "router-1")
	if router2 == nil {
		t.Fatal("router-1 not found after reopen")
	}
	props2 := router2.DataProperties()
	if ip2, ok := props2["IP Address"]; !ok {
		t.Fatal("'IP Address' property not found after reopen")
	} else if ip2.Value() != "192.168.1.1" {
		t.Errorf("IP after reopen = %q, want '192.168.1.1'", ip2.Value())
	}
}

func TestNetworkShapeInfo(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	r1 := shapes[0]

	// Shape.String()
	str := r1.String()
	if !strings.Contains(str, "router-1") {
		t.Errorf("String() = %q, should contain 'router-1'", str)
	}
	if !strings.Contains(str, r1.ID) {
		t.Errorf("String() = %q, should contain ID %q", str, r1.ID)
	}

	// Shape.IsMasterShape() - these are regular shapes, not masters
	if r1.IsMasterShape() {
		t.Error("regular shape should not be a master shape")
	}

	// Page.IsMasterPage()
	if page.IsMasterPage() {
		t.Error("regular page should not be a master page")
	}

	// Check actual master pages
	for _, mp := range vis.MasterPages {
		if !mp.IsMasterPage() {
			t.Errorf("master page %q should report IsMasterPage=true", mp.Name())
		}
	}

	// Shape.Center()
	center := r1.Center()
	if center.X != r1.X() || center.Y != r1.Y() {
		t.Errorf("Center() = (%v,%v), want (%v,%v)", center.X, center.Y, r1.X(), r1.Y())
	}

	// Shape.BoundsRect()
	rect := r1.BoundsRect()
	bx, by, ex, ey := r1.Bounds()
	if rect.BeginX != bx || rect.BeginY != by || rect.EndX != ex || rect.EndY != ey {
		t.Errorf("BoundsRect mismatch with Bounds()")
	}

	// Shape.RelativeBoundsRect()
	relRect := r1.RelativeBoundsRect()
	rbx, rby, rex, rey := r1.RelativeBounds()
	if relRect.BeginX != rbx || relRect.BeginY != rby || relRect.EndX != rex || relRect.EndY != rey {
		t.Errorf("RelativeBoundsRect mismatch with RelativeBounds()")
	}

	// Cell.String()
	if cell, ok := r1.Cells[string(CellPinX)]; ok {
		cs := cell.String()
		if !strings.Contains(cs, "Cell:") {
			t.Errorf("Cell.String() = %q, should contain 'Cell:'", cs)
		}
	}

	// Shape.LocXFormula() / LocYFormula()
	lxf := r1.LocXFormula()
	lyf := r1.LocYFormula()
	if lxf == "" || lyf == "" {
		t.Errorf("LocXFormula=%q, LocYFormula=%q, expected non-empty formulas", lxf, lyf)
	}

	// Shape.SetTextSize
	r1.SetTextSize(0.166667) // 12pt
	charSection := r1.XML().FindElement("Section[@N='Character']")
	if charSection == nil {
		t.Fatal("Character section should exist after SetTextSize")
	}
	sizeCell := charSection.FindElement("Row/Cell[@N='Size']")
	if sizeCell == nil {
		t.Fatal("Size cell should exist after SetTextSize")
	}
}

func TestNetworkGeometryStartPoint(t *testing.T) {
	vis, err := Open(testFile("test3_house.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	for _, shape := range page.AllShapes() {
		for _, g := range shape.Geometries {
			pt := g.StartPoint()
			sx, sy := g.StartPos()
			if pt.X != sx || pt.Y != sy {
				t.Errorf("StartPoint() != StartPos() for shape %s", shape.ID)
			}
		}
	}
}

func TestNetworkGeometryRowOperations(t *testing.T) {
	vis, err := Open(testFile("test3_house.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	shape := page.ChildShapes()[0]
	if len(shape.Geometries) == 0 {
		t.Fatal("no geometry")
	}

	geom := shape.Geometries[0]
	// Find a row to test setters
	for _, row := range geom.Rows {
		// SetRowType
		origType := row.RowType()
		row.SetRowType(origType)
		if row.RowType() != origType {
			t.Errorf("SetRowType roundtrip failed")
		}

		// SetIndex
		origIx := row.Index()
		row.SetIndex(origIx)
		if row.Index() != origIx {
			t.Errorf("SetIndex roundtrip failed")
		}

		// SetDelBool
		row.SetDelBool(true)
		if !row.DelBool() {
			t.Error("SetDelBool(true) should make DelBool() true")
		}
		row.SetDelBool(false)
		if row.DelBool() {
			t.Error("SetDelBool(false) should make DelBool() false")
		}

		// GeometryCell operations
		for _, cell := range row.Cells {
			// Formula / Func
			f := cell.Formula()
			f2 := cell.Func()
			if f != f2 {
				t.Errorf("Formula() != Func(): %q vs %q", f, f2)
			}

			// SetFormula
			cell.SetFormula("test")
			if cell.Formula() != "test" {
				t.Error("SetFormula failed")
			}
			cell.SetFormula(f) // restore

			// SetName
			origName := cell.Name()
			cell.SetName(origName)
			if cell.Name() != origName {
				t.Error("SetName roundtrip failed")
			}
		}
		break // one row is enough
	}

	// CreateRowXML
	row := geom.Rows["1"] // use existing row
	if row != nil {
		newRow := row.CreateRowXML("LineTo", "99")
		if newRow == nil {
			t.Error("CreateRowXML should return new element")
		}

		// Duplicate index should return nil
		dup := row.CreateRowXML("LineTo", "99")
		if dup != nil {
			t.Error("CreateRowXML with duplicate index should return nil")
		}

		// Empty params should return nil
		nilRow := row.CreateRowXML("", "")
		if nilRow != nil {
			t.Error("CreateRowXML with empty params should return nil")
		}
	}

	// CreateCellXML
	if row != nil {
		xCell := row.Cells["X"]
		if xCell != nil {
			newCell := xCell.CreateCellXML("TestCell")
			if newCell == nil {
				t.Error("CreateCellXML should return new element")
			}
		}
	}
}

func TestVisioFileAccessors(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close() //nolint:errcheck

	// AppXML
	if vis.AppXML() == nil {
		t.Error("AppXML() should not be nil")
	}

	// PagesXML
	if vis.PagesXML() == nil {
		t.Error("PagesXML() should not be nil")
	}

	// MastersXML - may be nil for files without masters
	// Just call it to exercise the code path
	_ = vis.MastersXML()

	// PrettyPrintElement
	root := vis.PagesXML().Root()
	if root != nil {
		pp := PrettyPrintElement(root)
		if pp == "" {
			t.Error("PrettyPrintElement should return non-empty string")
		}
		if !strings.Contains(pp, "<") {
			t.Error("PrettyPrintElement should contain XML tags")
		}
	}
}

func TestFileErrorUnwrap(t *testing.T) {
	inner := fmt.Errorf("something broke")
	fe := &FileError{Path: "/tmp/test.vsdx", Err: inner}

	if fe.Unwrap() != inner {
		t.Error("Unwrap should return the inner error")
	}
	if !strings.Contains(fe.Error(), "/tmp/test.vsdx") {
		t.Errorf("Error() = %q, should contain path", fe.Error())
	}
}

func TestCharacterFormatting(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]

	// SetCharBold
	s.SetCharBold(true)
	charSection := s.XML().FindElement("Section[@N='Character']")
	if charSection == nil {
		t.Fatal("Character section should exist")
	}
	styleCell := charSection.FindElement("Row/Cell[@N='Style']")
	if styleCell == nil || styleCell.SelectAttrValue("V", "") != "1" {
		t.Error("SetCharBold(true) should set Style=1")
	}

	// Bold + Italic combined
	s.SetCharItalic(true)
	styleCell = charSection.FindElement("Row/Cell[@N='Style']")
	if styleCell.SelectAttrValue("V", "") != "3" {
		t.Errorf("Bold+Italic should be Style=3, got %s", styleCell.SelectAttrValue("V", ""))
	}

	// Clear bold, keep italic
	s.SetCharBold(false)
	styleCell = charSection.FindElement("Row/Cell[@N='Style']")
	if styleCell.SelectAttrValue("V", "") != "2" {
		t.Errorf("Italic only should be Style=2, got %s", styleCell.SelectAttrValue("V", ""))
	}

	// SetCharSize (12pt)
	s.SetCharSize(12)
	sizeCell := charSection.FindElement("Row/Cell[@N='Size']")
	if sizeCell == nil {
		t.Fatal("Size cell should exist")
	}
	// 12/72 = 0.166667
	sizeVal := sizeCell.SelectAttrValue("V", "")
	if !strings.HasPrefix(sizeVal, "0.1666") {
		t.Errorf("SetCharSize(12) = %q, want ~0.166667", sizeVal)
	}

	// SetCharFont
	s.SetCharFont("Consolas")
	fontCell := charSection.FindElement("Row/Cell[@N='Font']")
	if fontCell == nil || fontCell.SelectAttrValue("V", "") != "Consolas" {
		t.Error("SetCharFont should set Font cell")
	}
}

func TestParagraphAlign(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	s.SetParagraphAlign(AlignCenter)

	paraSection := s.XML().FindElement("Section[@N='Paragraph']")
	if paraSection == nil {
		t.Fatal("Paragraph section should exist")
	}
	alignCell := paraSection.FindElement("Row/Cell[@N='HorzAlign']")
	if alignCell == nil || alignCell.SelectAttrValue("V", "") != "1" {
		t.Error("SetParagraphAlign(AlignCenter) should set HorzAlign=1")
	}

	// Change to right
	s.SetParagraphAlign(AlignRight)
	alignCell = paraSection.FindElement("Row/Cell[@N='HorzAlign']")
	if alignCell.SelectAttrValue("V", "") != "2" {
		t.Error("SetParagraphAlign(AlignRight) should set HorzAlign=2")
	}
}

func TestLinePattern(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	s.SetLinePattern(LinePatternDash)

	val := s.CellValue(CellLinePattern)
	if val != "2" {
		t.Errorf("SetLinePattern(Dash) = %q, want '2'", val)
	}

	s.SetLinePattern(LinePatternDot)
	val = s.CellValue(CellLinePattern)
	if val != "3" {
		t.Errorf("SetLinePattern(Dot) = %q, want '3'", val)
	}
}

func TestAddGeometryRect(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	g := s.AddGeometryRect()

	if g == nil {
		t.Fatal("AddGeometryRect returned nil")
	}
	// Should have 5 rows: RelMoveTo + 4x RelLineTo
	if len(g.Rows) != 5 {
		t.Fatalf("geometry rows = %d, want 5", len(g.Rows))
	}

	// Verify first row is RelMoveTo
	if row, ok := g.Rows["1"]; ok {
		if row.RowType() != "RelMoveTo" {
			t.Errorf("row 1 type = %q, want RelMoveTo", row.RowType())
		}
	}

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

	// Verify geometry survived
	page := vis2.GetPage(0)
	found := page.FindShapeByText("router-1")
	if found == nil {
		t.Fatal("shape not found after reopen")
	}
	if len(found.Geometries) == 0 {
		t.Error("geometry lost after round-trip")
	}
}

func TestGeometryBuilders(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	g := s.AddGeometry()

	// Absolute coordinates
	g.AddMoveTo(0, 0)
	g.AddLineTo(2, 0)
	g.AddLineTo(2, 1)
	g.AddArcTo(0, 1, 0.5)

	if len(g.Rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(g.Rows))
	}

	// Check ArcTo has A (bow) cell
	arcRow := g.Rows["4"]
	if arcRow == nil {
		t.Fatal("ArcTo row not found")
	}
	if arcRow.RowType() != "ArcTo" {
		t.Errorf("row type = %q, want ArcTo", arcRow.RowType())
	}
	if aCell, ok := arcRow.Cells["A"]; !ok {
		t.Error("ArcTo should have A cell")
	} else if aCell.Value() != "0.5" {
		t.Errorf("A cell = %q, want '0.5'", aCell.Value())
	}
}

func TestAddHyperlink(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	s.AddHyperlink("https://osprey.example.com/device/router-1", "Open in Osprey")

	section := s.XML().FindElement("Section[@N='Hyperlink']")
	if section == nil {
		t.Fatal("Hyperlink section should exist")
	}

	row := section.FindElement("Row[@N='Row_1']")
	if row == nil {
		t.Fatal("Row_1 should exist")
	}

	addr := row.FindElement("Cell[@N='Address']")
	if addr == nil || addr.SelectAttrValue("V", "") != "https://osprey.example.com/device/router-1" {
		t.Error("Address cell mismatch")
	}

	desc := row.FindElement("Cell[@N='Description']")
	if desc == nil || desc.SelectAttrValue("V", "") != "Open in Osprey" {
		t.Error("Description cell mismatch")
	}

	// Add second hyperlink
	s.AddHyperlink("https://grafana.example.com/router-1", "Grafana")
	row2 := section.FindElement("Row[@N='Row_2']")
	if row2 == nil {
		t.Error("second hyperlink Row_2 should exist")
	}

	// Round-trip
	data, err := vis.SaveVsdxBytes()
	if err != nil {
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	vis2, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer vis2.Close() //nolint:errcheck

	found := vis2.GetPage(0).FindShapeByText("router-1")
	if found == nil {
		t.Fatal("shape not found")
	}
	hlSection := found.XML().FindElement("Section[@N='Hyperlink']")
	if hlSection == nil {
		t.Error("Hyperlink section lost after round-trip")
	}
}

func TestAddConnectionPoint(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	w, h := s.Width(), s.Height()

	// Add 4 connection points at edges
	s.AddConnectionPoint(w/2, 0)     // bottom
	s.AddConnectionPoint(w/2, h)     // top
	s.AddConnectionPoint(0, h/2)     // left
	s.AddConnectionPoint(w, h/2)     // right

	section := s.XML().FindElement("Section[@N='Connection']")
	if section == nil {
		t.Fatal("Connection section should exist")
	}

	rows := section.SelectElements("Row")
	if len(rows) != 4 {
		t.Fatalf("connection points = %d, want 4", len(rows))
	}

	// Verify IX numbering
	for i, row := range rows {
		ix := row.SelectAttrValue("IX", "")
		if ix != strconv.Itoa(i) {
			t.Errorf("row %d IX = %q, want %q", i, ix, strconv.Itoa(i))
		}
	}
}

func TestAddLayerAndSetMember(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Add layers
	l3Idx := page.AddLayer("L3 Links")
	l2Idx := page.AddLayer("L2 Links")
	labelIdx := page.AddLayer("Labels")

	if l3Idx != 0 || l2Idx != 1 || labelIdx != 2 {
		t.Errorf("layer indices = %d,%d,%d, want 0,1,2", l3Idx, l2Idx, labelIdx)
	}

	// Assign shapes to layers
	shapes[0].SetLayerMember("0")     // router on L3
	shapes[2].SetLayerMember("0;1")   // switch on L3 + L2

	if shapes[0].CellValue(CellLayerMember) != "0" {
		t.Error("router layer should be '0'")
	}
	if shapes[2].CellValue(CellLayerMember) != "0;1" {
		t.Error("switch layer should be '0;1'")
	}

	// Round-trip
	data, err := vis.SaveVsdxBytes()
	if err != nil {
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	vis2, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer vis2.Close() //nolint:errcheck

	// Verify layer section in pages.xml
	page2 := vis2.GetPage(0)
	ps := page2.pagesheetXML()
	if ps == nil {
		t.Fatal("PageSheet should exist after reopen")
	}
	layerSection := ps.FindElement("Section[@N='Layer']")
	if layerSection == nil {
		t.Fatal("Layer section should exist after reopen")
	}
	layerRows := layerSection.SelectElements("Row")
	if len(layerRows) != 3 {
		t.Fatalf("layers after reopen = %d, want 3", len(layerRows))
	}
}

func TestAutoSize(t *testing.T) {
	vis, page, _ := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	page.AutoSize(0.5)

	w := page.Width()
	h := page.Height()
	// router-2 is at x=6, width=1, so rightmost edge = 6.5
	// router-1 is at y=8, height=1, so top edge = 8.5
	// With 0.5 margin: width >= 7, height >= 9
	if w < 7.0 {
		t.Errorf("page width = %v, want >= 7.0", w)
	}
	if h < 9.0 {
		t.Errorf("page height = %v, want >= 9.0", h)
	}
}

func TestSetComment(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	shapes[0].SetComment("Cisco ISR 4321 - Core Router")
	if shapes[0].CellValue("Comment") != "Cisco ISR 4321 - Core Router" {
		t.Error("SetComment value mismatch")
	}
}

func TestFillTransparency(t *testing.T) {
	vis, page, _ := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Create an OSPF area background
	area := page.AddShape()
	area.SetX(4.0)
	area.SetY(6.0)
	area.SetWidth(6.0)
	area.SetHeight(4.0)
	area.SetText("OSPF Area 0")
	area.SetFillColor("#CCE5FF")
	area.SetFillPattern(1) // solid
	area.SetFillTransparency(0.5)
	area.SetFillBkgndColor("#FFFFFF")
	area.SetFillBkgndTransparency(0.8)

	if area.CellValue(CellFillForegndTrans) != "0.5" {
		t.Errorf("FillForegndTrans = %q, want '0.5'", area.CellValue(CellFillForegndTrans))
	}
	if area.CellValue(CellFillBkgndTrans) != "0.8" {
		t.Errorf("FillBkgndTrans = %q, want '0.8'", area.CellValue(CellFillBkgndTrans))
	}
	if area.CellValue(CellFillPattern) != "1" {
		t.Errorf("FillPattern = %q, want '1'", area.CellValue(CellFillPattern))
	}
	if area.CellValue(CellFillBkgnd) != "#FFFFFF" {
		t.Errorf("FillBkgnd = %q, want '#FFFFFF'", area.CellValue(CellFillBkgnd))
	}
}

func TestRounding(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	shapes[0].SetRounding(0.125) // 1/8 inch rounded corners
	if shapes[0].CellValue(CellRounding) != "0.125" {
		t.Errorf("Rounding = %q, want '0.125'", shapes[0].CellValue(CellRounding))
	}
}

func TestBeginArrowAndLineCap(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Connect two shapes and style the connector
	conn, err := vis.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}

	// Bidirectional arrow
	conn.SetBeginArrow(13)
	conn.SetEndArrow(13)
	conn.SetLineCap(LineCapRound)

	if conn.CellValue(CellBeginArrow) != "13" {
		t.Errorf("BeginArrow = %q, want '13'", conn.CellValue(CellBeginArrow))
	}
	if conn.CellValue(CellEndArrow) != "13" {
		t.Errorf("EndArrow = %q, want '13'", conn.CellValue(CellEndArrow))
	}
	if conn.CellValue(CellLineCap) != "0" {
		t.Errorf("LineCap = %q, want '0'", conn.CellValue(CellLineCap))
	}
}

func TestTextBlockPositioning(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Connect and position label above the connector
	conn, err := vis.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}
	conn.SetText("ge-0/0/0 — 10Gbps")

	// Position text block above the line
	conn.SetTxtPinX(1.0)
	conn.SetTxtPinY(0.2)
	conn.SetTxtWidth(2.0)
	conn.SetTxtHeight(0.25)
	conn.SetTxtLocPinX(1.0)
	conn.SetTxtLocPinY(0.0)
	conn.SetTxtAngle(0)

	if conn.CellValue(CellTxtWidth) != "2" {
		t.Errorf("TxtWidth = %q, want '2'", conn.CellValue(CellTxtWidth))
	}
	if conn.CellValue(CellTxtHeight) != "0.25" {
		t.Errorf("TxtHeight = %q, want '0.25'", conn.CellValue(CellTxtHeight))
	}
	if conn.CellValue(CellTxtAngle) != "0" {
		t.Errorf("TxtAngle = %q, want '0'", conn.CellValue(CellTxtAngle))
	}
}

func TestProtection(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	r1 := shapes[0]

	// Lock movement and deletion
	r1.SetLockMove(true)
	r1.SetLockDelete(true)
	r1.SetLockSize(true)
	r1.SetLockRotate(true)
	r1.SetLockAspect(true)

	// Verify Protection section exists
	protSection := r1.XML().FindElement("Section[@N='Protection']")
	if protSection == nil {
		t.Fatal("Protection section should exist")
	}

	// Check individual cells
	checkCell := func(name, want string) {
		cell := protSection.FindElement("Row/Cell[@N='" + name + "']")
		if cell == nil {
			t.Errorf("cell %s not found", name)
			return
		}
		if cell.SelectAttrValue("V", "") != want {
			t.Errorf("%s = %q, want %q", name, cell.SelectAttrValue("V", ""), want)
		}
	}
	checkCell("LockMoveX", "1")
	checkCell("LockMoveY", "1")
	checkCell("LockDelete", "1")
	checkCell("LockWidth", "1")
	checkCell("LockHeight", "1")
	checkCell("LockRotate", "1")
	checkCell("LockAspect", "1")

	// Unlock move
	r1.SetLockMove(false)
	checkCell("LockMoveX", "0")
	checkCell("LockMoveY", "0")
}

func TestUserDefinedCells(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	r1 := shapes[0]

	// Add user cells for internal metadata
	r1.AddUserCell("osprey_id", "dev-12345")
	r1.AddUserCell("snmp_community", "public")
	r1.AddUserCell("port_count", "48")

	// Read back
	if r1.UserCellValue("osprey_id") != "dev-12345" {
		t.Errorf("osprey_id = %q, want 'dev-12345'", r1.UserCellValue("osprey_id"))
	}
	if r1.UserCellValue("snmp_community") != "public" {
		t.Errorf("snmp_community = %q, want 'public'", r1.UserCellValue("snmp_community"))
	}
	if r1.UserCellValue("port_count") != "48" {
		t.Errorf("port_count = %q, want '48'", r1.UserCellValue("port_count"))
	}

	// Not found
	if r1.UserCellValue("nonexistent") != "" {
		t.Error("nonexistent user cell should return empty string")
	}

	// Verify User section structure
	section := r1.XML().FindElement("Section[@N='User']")
	if section == nil {
		t.Fatal("User section should exist")
	}
	rows := section.SelectElements("Row")
	if len(rows) != 3 {
		t.Fatalf("user cells = %d, want 3", len(rows))
	}
}

func TestNetworkFeaturesRoundTrip(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	r1, sw := shapes[0], shapes[2]

	// Apply all new features
	r1.SetRounding(0.1)
	r1.SetFillColor("#E8F4FD")
	r1.SetFillPattern(1)
	r1.SetFillTransparency(0.0)
	r1.SetLockMove(true)
	r1.SetLockDelete(true)
	r1.AddUserCell("device_type", "router")

	sw.SetFillColor("#E8F8E8")
	sw.AddUserCell("device_type", "switch")

	// Area background
	area := page.AddShape()
	area.SetX(4.0)
	area.SetY(6.0)
	area.SetWidth(8.0)
	area.SetHeight(6.0)
	area.SetFillColor("#CCE5FF")
	area.SetFillTransparency(0.7)
	area.SetText("Area 0")

	// Connector with arrows and label positioning
	conn, _ := vis.ConnectShapes(page, r1, sw)
	conn.SetBeginArrow(13)
	conn.SetEndArrow(13)
	conn.SetLineCap(LineCapRound)
	conn.SetLinePattern(LinePatternDash)
	conn.SetText("10.0.0.0/30")
	conn.SetTxtWidth(1.5)

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

	// Verify rounding
	r1v2 := page2.FindShapeByText("router-1")
	if r1v2 == nil {
		t.Fatal("router-1 not found")
	}
	if r1v2.CellValue(CellRounding) != "0.1" {
		t.Errorf("Rounding after reopen = %q, want '0.1'", r1v2.CellValue(CellRounding))
	}

	// Verify fill transparency
	if r1v2.CellValue(CellFillForegndTrans) != "0" {
		t.Errorf("FillForegndTrans after reopen = %q, want '0'", r1v2.CellValue(CellFillForegndTrans))
	}

	// Verify protection
	protSection := r1v2.XML().FindElement("Section[@N='Protection']")
	if protSection == nil {
		t.Error("Protection section missing after reopen")
	}

	// Verify user cells
	if r1v2.UserCellValue("device_type") != "router" {
		t.Errorf("user cell after reopen = %q, want 'router'", r1v2.UserCellValue("device_type"))
	}

	// Verify connector arrows
	connShapes := page2.FindShapesByText("10.0.0.0/30")
	if len(connShapes) == 0 {
		t.Fatal("connector not found")
	}
	connV2 := connShapes[0]
	if connV2.CellValue(CellBeginArrow) != "13" {
		t.Errorf("BeginArrow after reopen = %q, want '13'", connV2.CellValue(CellBeginArrow))
	}
	if connV2.CellValue(CellLineCap) != "0" {
		t.Errorf("LineCap after reopen = %q, want '0'", connV2.CellValue(CellLineCap))
	}
}

func TestFormattingRoundTrip(t *testing.T) {
	vis, page, _ := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Apply all formatting to a shape
	s := page.FindShapeByText("router-1")
	s.SetCharBold(true)
	s.SetCharSize(10)
	s.SetCharFont("Arial")
	s.SetParagraphAlign(AlignCenter)
	s.SetLinePattern(LinePatternDash)
	s.SetLineWeight(0.02)
	s.SetLineColor("#0070C0")
	s.AddHyperlink("https://osprey.example.com", "Osprey")
	s.AddConnectionPoint(0.5, 0)
	s.AddConnectionPoint(0.5, 1)
	s.SetComment("Core Router")

	l3 := page.AddLayer("L3")
	s.SetLayerMember(strconv.Itoa(l3))

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

	s2 := vis2.GetPage(0).FindShapeByText("router-1")
	if s2 == nil {
		t.Fatal("shape not found after reopen")
	}

	// Verify formatting survived
	if s2.CellValue(CellLinePattern) != "2" {
		t.Errorf("LinePattern = %q, want '2'", s2.CellValue(CellLinePattern))
	}
	if s2.CellValue(CellLineColor) != "#0070C0" {
		t.Errorf("LineColor = %q, want '#0070C0'", s2.CellValue(CellLineColor))
	}
	if s2.CellValue("Comment") != "Core Router" {
		t.Errorf("Comment = %q, want 'Core Router'", s2.CellValue("Comment"))
	}

	charSection := s2.XML().FindElement("Section[@N='Character']")
	if charSection == nil {
		t.Fatal("Character section missing after reopen")
	}
	style := charSection.FindElement("Row/Cell[@N='Style']")
	if style == nil || style.SelectAttrValue("V", "") != "1" {
		t.Error("Bold style missing after reopen")
	}

	hlSection := s2.XML().FindElement("Section[@N='Hyperlink']")
	if hlSection == nil {
		t.Error("Hyperlink section missing after reopen")
	}

	connSection := s2.XML().FindElement("Section[@N='Connection']")
	if connSection == nil {
		t.Error("Connection section missing after reopen")
	}
}

func TestNetworkTopologyRoundTrip(t *testing.T) {
	vis, _, _ := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

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

	page := vis2.GetPage(0)

	// Verify all shapes survived
	allShapes := page.ChildShapes()
	// 3 devices + 2 connectors = 5
	if len(allShapes) < 5 {
		t.Fatalf("shapes after reopen = %d, want >= 5", len(allShapes))
	}

	// Verify text search works after round-trip
	routers := page.FindShapesByText("router")
	if len(routers) != 2 {
		t.Errorf("routers after reopen = %d, want 2", len(routers))
	}

	// Verify property search works after round-trip
	coreDevices := page.FindShapesByPropertyLabelValue("Role", "core")
	if len(coreDevices) != 2 {
		t.Errorf("core devices after reopen = %d, want 2", len(coreDevices))
	}

	// Verify connects survived
	connects := page.Connects()
	if len(connects) < 2 {
		t.Errorf("connects after reopen = %d, want >= 2", len(connects))
	}

	// Traverse connections
	for _, c := range connects {
		if c.Shape() == nil {
			t.Error("Connect.Shape() nil after reopen")
		}
		if c.ConnectorShape() == nil {
			t.Error("Connect.ConnectorShape() nil after reopen")
		}
	}
}
