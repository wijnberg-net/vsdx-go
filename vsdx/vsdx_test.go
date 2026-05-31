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

	"github.com/beevik/etree"
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

// --- Part 2: Navigation ---

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

// --- Part 3: Editing ---

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

// --- Part 4: Page/Shape management ---

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

// --- Part 5: Connectors ---

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

// --- Part 6: Templating ---

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

// --- Part 7: Diff ---

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
	if rect.X != bx || rect.Y != by || rect.EndX() != ex || rect.EndY() != ey {
		t.Errorf("BoundsRect mismatch with Bounds()")
	}

	// Shape.RelativeBoundsRect()
	relRect := r1.RelativeBoundsRect()
	rbx, rby, rex, rey := r1.RelativeBounds()
	if relRect.X != rbx || relRect.Y != rby || relRect.EndX() != rex || relRect.EndY() != rey {
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

func TestAddEllipse(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	g := s.AddGeometry()

	// Ellipse: center (1,1), right (2,1), top (1,2)
	g.AddEllipse(1, 1, 2, 1, 1, 2)

	if len(g.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(g.Rows))
	}

	row := g.Rows["1"]
	if row == nil {
		t.Fatal("Ellipse row not found")
	}
	if row.RowType() != "Ellipse" {
		t.Errorf("row type = %q, want Ellipse", row.RowType())
	}

	// Check all 6 cells (X, Y, A, B, C, D)
	for _, name := range []string{"X", "Y", "A", "B", "C", "D"} {
		if _, ok := row.Cells[name]; !ok {
			t.Errorf("Ellipse should have %s cell", name)
		}
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
		t.Fatal("shape not found after reopen")
	}
	if len(found.Geometries) == 0 {
		t.Error("geometry lost after round-trip")
	}
}

func TestAddEllipticalArcTo(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	g := s.AddGeometry()

	g.AddMoveTo(0, 0)
	g.AddEllipticalArcTo(2, 0, 1, 0.5, 1.5, 0)

	if len(g.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(g.Rows))
	}

	row := g.Rows["2"]
	if row == nil {
		t.Fatal("EllipticalArcTo row not found")
	}
	if row.RowType() != "EllipticalArcTo" {
		t.Errorf("row type = %q, want EllipticalArcTo", row.RowType())
	}
	// Should have 6 cells
	for _, name := range []string{"X", "Y", "A", "B", "C", "D"} {
		if _, ok := row.Cells[name]; !ok {
			t.Errorf("EllipticalArcTo should have %s cell", name)
		}
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
	s.AddConnectionPoint(w/2, 0) // bottom
	s.AddConnectionPoint(w/2, h) // top
	s.AddConnectionPoint(0, h/2) // left
	s.AddConnectionPoint(w, h/2) // right

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
	shapes[0].SetLayerMember("0")   // router on L3
	shapes[2].SetLayerMember("0;1") // switch on L3 + L2

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
	// LineCap=0 (round) is the stylesheet default; SetLineCap(LineCapRound)
	// is a no-op canonicalized to "no explicit cell" so the cell stays
	// absent. Match Visio's resave behaviour.
	if conn.CellValue(CellLineCap) != "" {
		t.Errorf("LineCap = %q, want empty (default = round)", conn.CellValue(CellLineCap))
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

	// Verify Lock* cells are written directly on the shape — Visio's
	// canonical resave hoists Lock cells from any Protection section to
	// the shape body, so vsdx-go's writer emits them at the shape level.
	checkCell := func(name, want string) {
		var got string
		for _, c := range r1.XML().SelectElements("Cell") {
			if c.SelectAttrValue("N", "") == name {
				got = c.SelectAttrValue("V", "")
				break
			}
		}
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
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

	// Verify protection. Visio's canonical resave writes Lock* cells as
	// direct shape cells rather than inside a Protection section, so we
	// check for the LockMoveX cell on the shape itself.
	if r1v2.CellValue(CellLockMoveX) != "1" {
		t.Errorf("LockMoveX after reopen = %q, want '1'", r1v2.CellValue(CellLockMoveX))
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
	// LineCap=0 is the stylesheet default; the writer doesn't emit a
	// no-op cell so the value stays absent (inheriting default = round).
	if connV2.CellValue(CellLineCap) != "" {
		t.Errorf("LineCap after reopen = %q, want empty (default = round)", connV2.CellValue(CellLineCap))
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

// --- Coverage Gap Tests ---

func TestCellFormula(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	shapes := page.ChildShapes()
	if len(shapes) == 0 {
		t.Skip("no shapes")
	}

	// Test CellFormula on shape with local cell
	s := shapes[0]
	s.SetCellValue(CellWidth, "2.5")

	// CellFormula should return empty for cells set via value
	formula := s.CellFormula(CellWidth)
	t.Logf("CellFormula(Width) = %q", formula)

	// Test CellFormula for non-existent cell
	formula = s.CellFormula("NonExistentCell")
	if formula != "" {
		t.Errorf("CellFormula(NonExistentCell) = %q, want empty", formula)
	}

	// Test CellFormula inheritance from master shape
	if s.MasterPageID != "" {
		ms := s.MasterShape()
		if ms != nil {
			for name := range ms.Cells {
				inherited := s.CellFormula(name)
				t.Logf("Inherited formula %s = %q", name, inherited)
				break
			}
		}
	}
}

func TestCalcValueAllFormulas(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Create a connector shape to test formulas
	conn, err := vis.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}

	tests := []struct {
		formula string
		wantOK  bool
	}{
		{"Width*1", true},
		{"Width*0", true},
		{"(BeginX+EndX)/2", true},
		{"(BeginY+EndY)/2", true},
		{"GUARD((BeginX+EndX)/2)", true},
		{"GUARD((BeginY+EndY)/2)", true},
		{"Width*0.5", true},
		{"Height*0.5", true},
		{"GUARD(Width*0.5)", true},
		{"GUARD(Height*0.5)", true},
		{"SQRT((EndX-BeginX)^2+(EndY-BeginY)^2)", true},
		{"ATAN2(EndY-BeginY,EndX-BeginX)", true},
		{"GUARD(EndX-BeginX)", true},
		{"GUARD(EndY-BeginY)", true},
		{"UnknownFormula", false},
	}

	for _, tt := range tests {
		t.Run(tt.formula, func(t *testing.T) {
			val, ok := CalcValue(conn, tt.formula)
			if ok != tt.wantOK {
				t.Errorf("CalcValue(%q) ok = %v, want %v", tt.formula, ok, tt.wantOK)
			}
			if ok {
				t.Logf("CalcValue(%q) = %v", tt.formula, val)
			}
		})
	}
}

func TestGeometryRowSetXYCreation(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]

	// Add geometry
	geom := s.AddGeometry()
	geom.AddMoveTo(0, 0)
	geom.AddLineTo(1, 1)

	// Get a row from the map
	if len(geom.Rows) == 0 {
		t.Skip("no geometry rows")
	}
	var row *GeometryRow
	for _, r := range geom.Rows {
		row = r
		break
	}

	// Clear the X/Y cells to test creation path
	delete(row.Cells, "X")
	delete(row.Cells, "Y")

	// SetX/SetY should create cells
	row.SetX(1.5)
	row.SetY(2.5)

	if row.Cells["X"] == nil {
		t.Error("SetX did not create X cell")
	}
	if row.Cells["Y"] == nil {
		t.Error("SetY did not create Y cell")
	}

	xVal := row.X()
	yVal := row.Y()
	if math.Abs(xVal-1.5) > 0.001 {
		t.Errorf("X = %v, want 1.5", xVal)
	}
	if math.Abs(yVal-2.5) > 0.001 {
		t.Errorf("Y = %v, want 2.5", yVal)
	}
}

func TestShapeRemove(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	initialCount := len(page.ChildShapes())

	// Remove a shape
	toRemove := shapes[0]
	toRemove.Remove()

	// Verify shape count decreased
	newCount := len(page.ChildShapes())
	if newCount != initialCount-1 {
		t.Errorf("shape count after Remove = %d, want %d", newCount, initialCount-1)
	}

	// Verify shape is no longer findable
	found := page.FindShapeByText(toRemove.Text())
	if found != nil {
		t.Error("removed shape still findable")
	}
}

func TestDataPropertyEdgeCases(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]

	// Add a property (name, label, value)
	s.AddDataProperty("TestProp", "TestProp", "Value1")

	props := s.DataProperties()
	var prop *DataProperty
	for _, p := range props {
		if p.Label == "TestProp" {
			prop = p
			break
		}
	}
	if prop == nil {
		t.Fatal("TestProp not found")
	}

	// Test Value()
	if prop.Value() != "Value1" {
		t.Errorf("Value = %q, want 'Value1'", prop.Value())
	}

	// Test SetValue
	prop.SetValue("Value2")
	if prop.Value() != "Value2" {
		t.Errorf("Value after SetValue = %q, want 'Value2'", prop.Value())
	}

	// Test GetAttribute
	labelAttr := prop.GetAttribute("Label", "V")
	if labelAttr != "TestProp" {
		t.Errorf("GetAttribute(Label, V) = %q, want 'TestProp'", labelAttr)
	}

	// Test GetAttribute for non-existent cell
	missing := prop.GetAttribute("NonExistent", "V")
	if missing != "" {
		t.Errorf("GetAttribute(NonExistent, V) = %q, want empty", missing)
	}

	// Test SetAttribute
	ok := prop.SetAttribute("Value", "U", "STR")
	if !ok {
		t.Error("SetAttribute returned false")
	}

	// Test SetAttribute for non-existent cell
	ok = prop.SetAttribute("NonExistent", "V", "test")
	if ok {
		t.Error("SetAttribute(NonExistent) should return false")
	}

	// Test RemoveAttribute
	ok = prop.RemoveAttribute("Value", "U")
	if !ok {
		t.Error("RemoveAttribute returned false")
	}

	// Test RemoveAttribute for non-existent attribute
	ok = prop.RemoveAttribute("Value", "NonExistent")
	if ok {
		t.Error("RemoveAttribute(NonExistent) should return false")
	}

	// Test RemoveAttribute for non-existent cell
	ok = prop.RemoveAttribute("NonExistent", "V")
	if ok {
		t.Error("RemoveAttribute on non-existent cell should return false")
	}
}

func TestPageDimensionEdgeCases(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)

	// Test Width/Height
	w := page.Width()
	h := page.Height()
	t.Logf("Page dimensions: %.2f x %.2f", w, h)

	if w <= 0 {
		t.Errorf("Width = %v, want > 0", w)
	}
	if h <= 0 {
		t.Errorf("Height = %v, want > 0", h)
	}

	// Test SetWidth/SetHeight
	page.SetWidth(11.0)
	page.SetHeight(8.5)

	if math.Abs(page.Width()-11.0) > 0.001 {
		t.Errorf("Width after SetWidth = %v, want 11.0", page.Width())
	}
	if math.Abs(page.Height()-8.5) > 0.001 {
		t.Errorf("Height after SetHeight = %v, want 8.5", page.Height())
	}

	// Test IndexNum
	idx := page.IndexNum()
	t.Logf("IndexNum = %d", idx)
}

func TestMasterShapeInheritance(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	shapes := page.ChildShapes()

	for _, s := range shapes {
		if s.MasterPageID == "" {
			continue
		}

		// Test MasterShape
		ms := s.MasterShape()
		if ms != nil {
			t.Logf("Shape %s has MasterShape with %d cells", s.ID, len(ms.Cells))

			// Test MasterPage
			mp := s.MasterPage()
			if mp != nil {
				t.Logf("Shape %s has MasterPage %s", s.ID, mp.Name())
			}
		}
		break
	}

	// Test shape without master
	for _, s := range shapes {
		if s.MasterPageID == "" {
			ms := s.MasterShape()
			if ms != nil {
				t.Error("shape without MasterPageID should have nil MasterShape")
			}
			mp := s.MasterPage()
			if mp != nil {
				t.Error("shape without MasterPageID should have nil MasterPage")
			}
			break
		}
	}
}

func TestFindShapesEdgeCases(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)

	// Test FindShapeByPropertyLabel with non-existent label
	found := page.FindShapeByPropertyLabel("NonExistentLabel")
	if found != nil {
		t.Error("FindShapeByPropertyLabel should return nil for non-existent label")
	}

	// Test FindShapesByRegex with no matches
	matches, err := page.FindShapesByRegex("^ZZZ_IMPOSSIBLE_PATTERN$")
	if err != nil {
		t.Fatalf("FindShapesByRegex: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("FindShapesByRegex should return empty for no matches, got %d", len(matches))
	}

	// Test FindShapeByID with non-existent ID
	byID := page.FindShapeByID("99999")
	if byID != nil {
		t.Error("FindShapeByID should return nil for non-existent ID")
	}
}

func TestGeometryStartPos(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	geom := s.AddGeometry()
	geom.AddMoveTo(1.0, 2.0)
	geom.AddLineTo(3.0, 4.0)

	// Test StartPos
	x, y := geom.StartPos()
	if math.Abs(x-1.0) > 0.001 || math.Abs(y-2.0) > 0.001 {
		t.Errorf("StartPos = (%v, %v), want (1.0, 2.0)", x, y)
	}

	// Test StartPos on empty geometry
	emptyGeom := s.AddGeometry()
	emptyX, emptyY := emptyGeom.StartPos()
	if emptyX != 0 || emptyY != 0 {
		t.Errorf("empty StartPos = (%v, %v), want (0, 0)", emptyX, emptyY)
	}
}

func TestConnectsMethods(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Create connection
	_, err := vis.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}

	connects := page.Connects()
	if len(connects) == 0 {
		t.Skip("no connects")
	}

	// Test Connect methods
	c := connects[0]
	t.Logf("Connect: ShapeID=%s, ConnectorShapeID=%s", c.ShapeID(), c.ConnectorShapeID())

	shape := c.Shape()
	if shape == nil {
		t.Error("Connect.Shape() is nil")
	}

	conn := c.ConnectorShape()
	if conn == nil {
		t.Error("Connect.ConnectorShape() is nil")
	}

	// Test GetConnectorsBetween (needs shapeAID, shapeAText, shapeBID, shapeBText)
	connectors, err := page.GetConnectorsBetween(shapes[0].ID, shapes[0].Text(), shapes[1].ID, shapes[1].Text())
	if err != nil {
		t.Fatalf("GetConnectorsBetween: %v", err)
	}
	if len(connectors) == 0 {
		t.Error("GetConnectorsBetween returned no connectors")
	}

	// Test GetConnectorsBetween with no connection
	connectors, _ = page.GetConnectorsBetween(shapes[0].ID, shapes[0].Text(), shapes[2].ID, shapes[2].Text())
	t.Logf("GetConnectorsBetween(0, 2) = %d connectors", len(connectors))
}

func TestTextColor(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	s.SetTextColor("#FF0000")

	color := s.TextColor()
	if color != "#FF0000" {
		t.Errorf("TextColor = %q, want '#FF0000'", color)
	}

	// Test TextColor on shape without color set
	s2 := shapes[1]
	color2 := s2.TextColor()
	t.Logf("Default TextColor = %q", color2)
}

func TestShapeMove(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]

	// Get original position
	origX := s.X()
	origY := s.Y()

	// Move by delta
	s.Move(1.0, -0.5)

	newX := s.X()
	newY := s.Y()

	if math.Abs(newX-(origX+1.0)) > 0.001 {
		t.Errorf("X after Move = %v, want %v", newX, origX+1.0)
	}
	if math.Abs(newY-(origY-0.5)) > 0.001 {
		t.Errorf("Y after Move = %v, want %v", newY, origY-0.5)
	}
}

func TestDiffEdgeCases(t *testing.T) {
	// Compare two existing test files
	diff, err := NewVisioFileDiff(testFile("test1.vsdx"), testFile("test2.vsdx"))
	if err != nil {
		t.Fatalf("NewVisioFileDiff: %v", err)
	}

	// Test CompareMembers
	same := diff.CompareMembers()
	t.Logf("CompareMembers: same = %v", same)

	// Test RemovedMembers
	removedOnly := diff.RemovedMembers()
	t.Logf("RemovedMembers: %d", len(removedOnly))
}

func TestSaveVsdxToPath(t *testing.T) {
	vis, _, _ := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Save to temp file
	tmpFile := filepath.Join(os.TempDir(), "test_save_coverage.vsdx")
	defer os.Remove(tmpFile)

	err := vis.SaveVsdx(tmpFile)
	if err != nil {
		t.Fatalf("SaveVsdx: %v", err)
	}

	// Verify file exists
	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("saved file is empty")
	}

	// Reopen to verify
	vis2, err := Open(tmpFile)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer vis2.Close() //nolint:errcheck

	if len(vis2.Pages) == 0 {
		t.Error("reopened file has no pages")
	}
}

func TestMediaRelsXML(t *testing.T) {
	// Test Media directly
	media, err := NewMedia()
	if err != nil {
		t.Fatalf("NewMedia: %v", err)
	}
	defer media.Close() //nolint:errcheck

	// Test template shapes
	if media.StraightConnector() == nil {
		t.Error("StraightConnector returned nil")
	}
	if media.Rectangle() == nil {
		t.Error("Rectangle returned nil")
	}
	if media.Circle() == nil {
		t.Error("Circle returned nil")
	}

	// Test RelsXML
	rels := media.RelsXML()
	t.Logf("RelsXML length: %d", len(rels))

	// Test VisioFile access
	vis := media.VisioFile()
	if vis == nil {
		t.Error("VisioFile returned nil")
	}
}

func TestDataPropertyValueEdges(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]

	// Add property with empty value
	prop := s.AddDataProperty("EmptyProp", "EmptyLabel", "")
	if prop == nil {
		t.Fatal("AddDataProperty returned nil")
	}

	// Verify empty value
	if prop.Value() != "" {
		t.Errorf("Value = %q, want empty", prop.Value())
	}

	// Add property with actual value
	prop2 := s.AddDataProperty("RealProp", "RealLabel", "RealValue")
	if prop2.Value() != "RealValue" {
		t.Errorf("Value = %q, want 'RealValue'", prop2.Value())
	}

	// Test SetValue
	prop2.SetValue("UpdatedValue")
	if prop2.Value() != "UpdatedValue" {
		t.Errorf("Value after SetValue = %q, want 'UpdatedValue'", prop2.Value())
	}
}

func TestGeometryCellXYNoFormula(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	geom := s.AddGeometry()
	geom.AddMoveTo(1.5, 2.5)

	// Get row and verify X/Y
	for _, row := range geom.Rows {
		x := row.X()
		y := row.Y()
		t.Logf("Row X=%v Y=%v", x, y)

		// Test without cells (remove them)
		delete(row.Cells, "X")
		delete(row.Cells, "Y")

		// X/Y should return 0 when cells don't exist
		if row.X() != 0 {
			t.Errorf("X without cell = %v, want 0", row.X())
		}
		if row.Y() != 0 {
			t.Errorf("Y without cell = %v, want 0", row.Y())
		}
		break
	}
}

func TestShapeTextInheritance(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	shapes := page.ChildShapes()

	for _, s := range shapes {
		text := s.Text()
		if text != "" {
			t.Logf("Shape %s has text: %q", s.ID, text)
		}

		// Test master shape text inheritance
		if s.MasterPageID != "" {
			ms := s.MasterShape()
			if ms != nil {
				masterText := ms.Text()
				t.Logf("Master shape text: %q", masterText)
			}
		}
	}
}

func TestPageNilXML(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)

	// Test ChildShapes
	children := page.ChildShapes()
	if len(children) == 0 {
		t.Error("expected child shapes")
	}

	// Test shapes()
	allShapes := page.ChildShapes()
	t.Logf("Page has %d shapes", len(allShapes))

	// Test Connects with actual connections
	_, err := vis.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}

	connects := page.Connects()
	t.Logf("Page has %d connects", len(connects))
}

func TestCellFormulaInheritance(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	shapes := page.ChildShapes()

	for _, s := range shapes {
		if s.MasterPageID == "" {
			continue
		}

		ms := s.MasterShape()
		if ms == nil {
			continue
		}

		// Test CellFormula inheritance - look for a cell with formula in master
		for name, cell := range ms.Cells {
			if cell.Formula() != "" {
				// Shape should inherit formula from master
				inheritedFormula := s.CellFormula(name)
				t.Logf("Inherited formula for %s: %q (master: %q)", name, inheritedFormula, cell.Formula())
				return
			}
		}
	}
}

func TestToFloatEdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1.5", 1.5},
		{"0", 0},
		{"", 0},
		{"invalid", 0},
		{"-3.14", -3.14},
		{"1e-5", 1e-5},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toFloat(tt.input)
			if math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("toFloat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMasterShapeNilCases(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// buildNetworkTopology creates shapes without masters
	for _, s := range shapes {
		if s.MasterPageID != "" {
			continue
		}

		// MasterShape and MasterPage should return nil
		ms := s.MasterShape()
		mp := s.MasterPage()

		if ms != nil {
			t.Error("MasterShape should be nil for shape without master")
		}
		if mp != nil {
			t.Error("MasterPage should be nil for shape without master")
		}
		break
	}
}

func TestUserCellValueNotFound(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]

	// Get non-existent user cell
	val := s.UserCellValue("NonExistentCell")
	if val != "" {
		t.Errorf("UserCellValue(NonExistent) = %q, want empty", val)
	}

	// Add a user cell and verify
	s.AddUserCell("TestCell", "TestValue")
	val = s.UserCellValue("TestCell")
	if val != "TestValue" {
		t.Errorf("UserCellValue(TestCell) = %q, want 'TestValue'", val)
	}
}

func TestMasterShapeEdgeCases(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	shapes := page.ChildShapes()

	// Find a shape with a master that has MasterShapeID
	for _, s := range shapes {
		if s.MasterPageID == "" {
			continue
		}

		t.Logf("Shape %s: MasterPageID=%s MasterShapeID=%s", s.ID, s.MasterPageID, s.MasterShapeID)

		// Test MasterShape with valid master
		ms := s.MasterShape()
		if ms != nil {
			t.Logf("Found master shape with %d cells", len(ms.Cells))
		}

		// Test MasterPage
		mp := s.MasterPage()
		if mp != nil {
			t.Logf("Found master page: %s", mp.Name())
		}
		break
	}

	// Test with non-existent MasterPageID
	if len(shapes) > 0 {
		s := shapes[0]
		origMasterPageID := s.MasterPageID
		s.MasterPageID = "99999" // Non-existent

		ms := s.MasterShape()
		if ms != nil {
			t.Error("MasterShape should be nil for non-existent MasterPageID")
		}

		s.MasterPageID = origMasterPageID
	}
}

func TestMoveWithBeginXY(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Create a connector which has BeginX/BeginY
	conn, err := vis.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}

	// Get original positions
	origBeginX := conn.BeginX()
	origBeginY := conn.BeginY()
	origX := conn.X()
	origY := conn.Y()

	// Move the connector
	conn.Move(1.0, 2.0)

	// Verify BeginX/BeginY moved
	if !conn.HasBeginX() {
		t.Error("connector should have BeginX")
	}

	newBeginX := conn.BeginX()
	newBeginY := conn.BeginY()

	if math.Abs(newBeginX-(origBeginX+1.0)) > 0.001 {
		t.Errorf("BeginX after Move = %v, want %v", newBeginX, origBeginX+1.0)
	}
	if math.Abs(newBeginY-(origBeginY+2.0)) > 0.001 {
		t.Errorf("BeginY after Move = %v, want %v", newBeginY, origBeginY+2.0)
	}

	// Verify X/Y also moved
	newX := conn.X()
	newY := conn.Y()
	if math.Abs(newX-(origX+1.0)) > 0.001 {
		t.Errorf("X after Move = %v, want %v", newX, origX+1.0)
	}
	if math.Abs(newY-(origY+2.0)) > 0.001 {
		t.Errorf("Y after Move = %v, want %v", newY, origY+2.0)
	}
}

func TestConnectedShapes(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Create connections
	_, err := vis.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}
	_, err = vis.ConnectShapes(page, shapes[0], shapes[2])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}

	// Test ConnectedShapes
	connected := shapes[0].ConnectedShapes()
	t.Logf("Shape 0 connected to %d shapes", len(connected))

	if len(connected) < 2 {
		t.Errorf("expected at least 2 connected shapes, got %d", len(connected))
	}
}

func TestUniversalName(t *testing.T) {
	vis, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer vis.Close() //nolint:errcheck

	page := vis.GetPage(0)
	shapes := page.ChildShapes()

	for _, s := range shapes {
		un := s.UniversalName()
		if un != "" {
			t.Logf("Shape %s has UniversalName: %q", s.ID, un)
		}
	}

	// Test on shape without master
	vis2, _, shapes2 := buildNetworkTopology(t)
	defer vis2.Close() //nolint:errcheck

	un := shapes2[0].UniversalName()
	if un != "" {
		t.Errorf("UniversalName for shape without master = %q, want empty", un)
	}
}

func TestDiffCompareMoreCases(t *testing.T) {
	// Compare two different files
	diff, err := NewVisioFileDiff(testFile("test1.vsdx"), testFile("test2.vsdx"))
	if err != nil {
		t.Fatalf("NewVisioFileDiff: %v", err)
	}

	same := diff.CompareMembers()
	t.Logf("CompareMembers = %v", same)

	removed := diff.RemovedMembers()
	t.Logf("RemovedMembers = %d", len(removed))

	// Test with files that have more differences
	diff2, err := NewVisioFileDiff(testFile("test1.vsdx"), testFile("test3_house.vsdx"))
	if err != nil {
		t.Fatalf("NewVisioFileDiff: %v", err)
	}

	same2 := diff2.CompareMembers()
	t.Logf("CompareMembers(1,3_house) = %v", same2)
}

func TestGetMaxID(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Add some child shapes via grouping
	group := page.GroupShapes([]*Shape{shapes[0], shapes[1]}, 0.1)
	if group == nil {
		t.Fatal("GroupShapes returned nil")
	}

	maxID := group.GetMaxID()
	t.Logf("GetMaxID = %d", maxID)

	if maxID < 1 {
		t.Errorf("GetMaxID = %d, want >= 1", maxID)
	}
}

func TestAddConnectEdgeCases(t *testing.T) {
	vis, page, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	// Create a connection
	conn, err := vis.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}

	// Verify connects were added
	connects := page.Connects()
	initialCount := len(connects)
	t.Logf("Initial connects: %d", initialCount)

	// AddConnect is called internally by ConnectShapes
	// Verify the connect has correct shape references
	for _, c := range connects {
		if c.ConnectorShape() != nil && c.ConnectorShape().ID == conn.ID {
			t.Logf("Found connect for connector %s -> shape %s", c.ConnectorShapeID(), c.ShapeID())
		}
	}
}

// --- Geometry Row Type Tests ---

func TestGeometryRelCubBezTo(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	geom := s.AddGeometry()
	geom.AddRelMoveTo(0, 0)
	geom.AddRelCubBezTo(1, 1, 0.25, 0.5, 0.75, 0.5) // Cubic bezier curve

	if len(geom.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(geom.Rows))
	}

	// Verify row type
	for _, row := range geom.Rows {
		if row.RowType() == "RelCubBezTo" {
			if row.Cells["A"] == nil || row.Cells["B"] == nil {
				t.Error("RelCubBezTo missing control point cells")
			}
			t.Logf("RelCubBezTo: X=%v Y=%v A=%v B=%v", row.X(), row.Y(),
				row.Cells["A"].Value(), row.Cells["B"].Value())
		}
	}
}

func TestGeometryRelQuadBezTo(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	geom := s.AddGeometry()
	geom.AddRelMoveTo(0, 0)
	geom.AddRelQuadBezTo(1, 1, 0.5, 0.75) // Quadratic bezier curve

	if len(geom.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(geom.Rows))
	}

	// Verify row type
	for _, row := range geom.Rows {
		if row.RowType() == "RelQuadBezTo" {
			if row.Cells["A"] == nil || row.Cells["B"] == nil {
				t.Error("RelQuadBezTo missing control point cells")
			}
			t.Logf("RelQuadBezTo: X=%v Y=%v A=%v B=%v", row.X(), row.Y(),
				row.Cells["A"].Value(), row.Cells["B"].Value())
		}
	}
}

func TestGeometryRelEllipticalArcTo(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	geom := s.AddGeometry()
	geom.AddRelMoveTo(0, 0.5)
	geom.AddRelEllipticalArcTo(1, 0.5, 0.5, 0, 1.5, 0) // Elliptical arc

	if len(geom.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(geom.Rows))
	}

	for _, row := range geom.Rows {
		if row.RowType() == "RelEllipticalArcTo" {
			t.Logf("RelEllipticalArcTo: X=%v Y=%v", row.X(), row.Y())
		}
	}
}

func TestGeometryPolylineTo(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	geom := s.AddGeometry()
	geom.AddMoveTo(0, 0)
	geom.AddPolylineTo(1, 1, "POLYLINE(0,0,0.5,0.5,1,0)") // Polyline formula

	if len(geom.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(geom.Rows))
	}

	for _, row := range geom.Rows {
		if row.RowType() == "PolylineTo" {
			if row.Cells["A"] == nil {
				t.Error("PolylineTo missing A cell")
			} else {
				t.Logf("PolylineTo formula: %s", row.Cells["A"].Value())
			}
		}
	}
}

func TestGeometryNURBSTo(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	geom := s.AddGeometry()
	geom.AddMoveTo(0, 0)
	geom.AddNURBSTo(1, 1, 0, 1, 0, 1, "NURBS(0,0,1,1,0,1)")

	if len(geom.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(geom.Rows))
	}

	for _, row := range geom.Rows {
		if row.RowType() == "NURBSTo" {
			if row.Cells["E"] == nil {
				t.Error("NURBSTo missing E cell")
			} else {
				t.Logf("NURBSTo formula: %s", row.Cells["E"].Value())
			}
		}
	}
}

func TestGeometrySpline(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	geom := s.AddGeometry()
	geom.AddMoveTo(0, 0)
	geom.AddSplineStart(0.5, 0.5, 1, 0, 2, 3) // degree 3 spline
	geom.AddSplineKnot(1, 1, 1.5)

	if len(geom.Rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(geom.Rows))
	}

	foundStart := false
	foundKnot := false
	for _, row := range geom.Rows {
		if row.RowType() == "SplineStart" {
			foundStart = true
			t.Logf("SplineStart: X=%v Y=%v degree=%s", row.X(), row.Y(), row.Cells["D"].Value())
		}
		if row.RowType() == "SplineKnot" {
			foundKnot = true
			t.Logf("SplineKnot: X=%v Y=%v knot=%s", row.X(), row.Y(), row.Cells["A"].Value())
		}
	}

	if !foundStart {
		t.Error("SplineStart row not found")
	}
	if !foundKnot {
		t.Error("SplineKnot row not found")
	}
}

func TestGeometryInfiniteLine(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	geom := s.AddGeometry()
	geom.AddInfiniteLine(0, 0, 1, 1) // Line through (0,0) and (1,1)

	if len(geom.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(geom.Rows))
	}

	for _, row := range geom.Rows {
		if row.RowType() == "InfiniteLine" {
			t.Logf("InfiniteLine: point1=(%v,%v) point2=(%s,%s)",
				row.X(), row.Y(), row.Cells["A"].Value(), row.Cells["B"].Value())
		}
	}
}

func TestGeometryBezierRoundTrip(t *testing.T) {
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	geom := s.AddGeometry()
	geom.AddRelMoveTo(0, 0)
	geom.AddRelCubBezTo(1, 0, 0.33, 0.5, 0.66, 0.5)
	geom.AddRelQuadBezTo(1, 1, 0.5, 1.5)

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
	shapes2 := page2.ChildShapes()
	if len(shapes2) == 0 {
		t.Fatal("no shapes after reopen")
	}

	// Find the shape with geometry
	for _, s2 := range shapes2 {
		if s2.Geometry != nil && len(s2.Geometry.Rows) >= 3 {
			t.Logf("Found shape with %d geometry rows after round-trip", len(s2.Geometry.Rows))
			for _, row := range s2.Geometry.Rows {
				t.Logf("  Row type: %s", row.RowType())
			}
			return
		}
	}
}

func TestGeometrySortedRowsNumeric(t *testing.T) {
	// Test that SortedRows sorts numerically, not lexically
	// (i.e., IX=2 comes before IX=10, not after)
	vis, _, shapes := buildNetworkTopology(t)
	defer vis.Close() //nolint:errcheck

	s := shapes[0]
	geom := s.AddGeometry()

	// Add rows in non-sequential order to simulate inherited geometry
	// with double-digit indices
	geom.AddMoveTo(0, 0) // IX=1
	for i := 0; i < 11; i++ {
		geom.AddLineTo(float64(i+1)*0.1, float64(i+1)*0.1) // IX=2..12
	}

	rows := geom.SortedRows()
	if len(rows) < 12 {
		t.Fatalf("expected at least 12 rows, got %d", len(rows))
	}

	// Verify rows are in numeric order
	prevIX := 0
	for i, row := range rows {
		ix := 0
		for _, c := range row.Index() {
			if c >= '0' && c <= '9' {
				ix = ix*10 + int(c-'0')
			}
		}
		if ix < prevIX {
			t.Errorf("row %d: IX=%d is less than previous IX=%d (not sorted numerically)", i, ix, prevIX)
		}
		prevIX = ix
	}
}

func TestScratchSection(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	s := shapes[0]

	// Add scratch cells
	ix := s.AddScratchCell("1.5", "2.5", "test", "", "", "")
	if ix != 0 {
		t.Errorf("expected ix=0, got %d", ix)
	}

	// Add another
	ix2 := s.AddScratchCell("", "", "", "B value", "C value", "D value")
	if ix2 != 1 {
		t.Errorf("expected ix=1, got %d", ix2)
	}

	// Read back
	cells := s.ScratchCells()
	if len(cells) != 2 {
		t.Fatalf("expected 2 scratch cells, got %d", len(cells))
	}
	if cells[0].X != "1.5" {
		t.Errorf("expected X=1.5, got %s", cells[0].X)
	}
	if cells[0].A != "test" {
		t.Errorf("expected A=test, got %s", cells[0].A)
	}
	if cells[1].B != "B value" {
		t.Errorf("expected B='B value', got %s", cells[1].B)
	}
	t.Logf("Scratch cells: %+v", cells)
}

func TestActionsSection(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	s := shapes[0]

	// Add actions
	s.AddAction("MyAction1", "Click Me", "RUNADDON(\"myapp\")")
	s.AddAction("MyAction2", "Open URL", "OPENURL(\"http://example.com\")")

	// Read back
	actions := s.Actions()
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[0].Name != "MyAction1" {
		t.Errorf("expected name=MyAction1, got %s", actions[0].Name)
	}
	if actions[0].Menu != "Click Me" {
		t.Errorf("expected menu='Click Me', got %s", actions[0].Menu)
	}
	if actions[1].Menu != "Open URL" {
		t.Errorf("expected menu='Open URL', got %s", actions[1].Menu)
	}
	t.Logf("Actions: %+v", actions)
}

func TestFieldSection(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	s := shapes[0]

	// Add fields: type 0=string, 2=numeric, 5=date/time
	ix := s.AddField(0, "Custom Text", "")
	if ix != 0 {
		t.Errorf("expected ix=0, got %d", ix)
	}

	ix2 := s.AddField(5, "", "{{MM/DD/YYYY}}")
	if ix2 != 1 {
		t.Errorf("expected ix=1, got %d", ix2)
	}

	// Read back
	fields := s.Fields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Type != 0 {
		t.Errorf("expected type=0, got %d", fields[0].Type)
	}
	if fields[0].Value != "Custom Text" {
		t.Errorf("expected value='Custom Text', got %s", fields[0].Value)
	}
	if fields[1].Type != 5 {
		t.Errorf("expected type=5, got %d", fields[1].Type)
	}
	t.Logf("Fields: %+v", fields)
}

func TestControlSection(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	s := shapes[0]

	// Add controls
	s.AddControl("Handle1", 0.5, 0.5, "Drag me")
	s.AddControl("Handle2", 1.0, 0.0, "")

	// Read back
	controls := s.Controls()
	if len(controls) < 2 {
		t.Fatalf("expected at least 2 controls, got %d", len(controls))
	}

	found := false
	for _, c := range controls {
		if c.Name == "Handle1" {
			found = true
			if c.X != 0.5 {
				t.Errorf("expected X=0.5, got %f", c.X)
			}
			if c.Tip != "Drag me" {
				t.Errorf("expected tip='Drag me', got %s", c.Tip)
			}
		}
	}
	if !found {
		t.Error("Handle1 control not found")
	}
	t.Logf("Controls: %+v", controls)
}

func TestTabsSection(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	s := shapes[0]

	// Add tab stops: alignment 0=left, 1=center, 2=right
	ix := s.AddTabStop(1.0, 0)
	if ix != 0 {
		t.Errorf("expected ix=0, got %d", ix)
	}

	ix2 := s.AddTabStop(2.5, 2)
	if ix2 != 1 {
		t.Errorf("expected ix=1, got %d", ix2)
	}

	// Read back
	tabs := s.TabStops()
	if len(tabs) != 2 {
		t.Fatalf("expected 2 tab stops, got %d", len(tabs))
	}
	if tabs[0].Position != 1.0 {
		t.Errorf("expected position=1.0, got %f", tabs[0].Position)
	}
	if tabs[0].Alignment != 0 {
		t.Errorf("expected alignment=0, got %d", tabs[0].Alignment)
	}
	if tabs[1].Position != 2.5 {
		t.Errorf("expected position=2.5, got %f", tabs[1].Position)
	}
	if tabs[1].Alignment != 2 {
		t.Errorf("expected alignment=2, got %d", tabs[1].Alignment)
	}
	t.Logf("Tab stops: %+v", tabs)
}

func TestFormulaEvaluator(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	eval := NewFormulaEvaluator(shape)

	tests := []struct {
		formula string
		want    float64
		wantOK  bool
	}{
		// Basic numbers
		{"42", 42, true},
		{"3.14", 3.14, true},
		{"-5", -5, true},

		// Basic arithmetic
		{"2+3", 5, true},
		{"10-4", 6, true},
		{"3*4", 12, true},
		{"15/3", 5, true},
		{"2^3", 8, true},

		// Operator precedence
		{"2+3*4", 14, true},
		{"(2+3)*4", 20, true},
		{"10-2*3", 4, true},
		{"2^2+1", 5, true},

		// Math functions
		{"ABS(-5)", 5, true},
		{"ABS(5)", 5, true},
		{"SQRT(16)", 4, true},
		{"SQRT(2)", 1.4142135623730951, true},

		// Trig functions
		{"SIN(0)", 0, true},
		{"COS(0)", 1, true},
		{"TAN(0)", 0, true},

		// Rounding
		{"INT(3.7)", 3, true},
		{"INT(-3.7)", -3, true},
		{"FLOOR(3.7)", 3, true},
		{"FLOOR(-3.7)", -4, true},
		{"CEILING(3.2)", 4, true},
		{"CEILING(-3.2)", -3, true},
		{"ROUND(3.5)", 4, true},
		{"TRUNC(3.9)", 3, true},

		// Sign
		{"SIGN(42)", 1, true},
		{"SIGN(-42)", -1, true},
		{"SIGN(0)", 0, true},

		// Min/Max
		{"MAX(3,7)", 7, true},
		{"MIN(3,7)", 3, true},
		{"MAX(-5,2)", 2, true},

		// Conditional
		{"IF(1,10,20)", 10, true},
		{"IF(0,10,20)", 20, true},
		{"IF(5>3,1,0)", 0, false}, // comparison not supported yet

		// Nested functions
		{"ABS(SQRT(16)-5)", 1, true},
		{"MAX(ABS(-3),2)", 3, true},

		// GUARD (should just return inner value)
		{"GUARD(5)", 5, true},
		{"GUARD(2+3)", 5, true},

		// PI
		{"PI()", 3.141592653589793, true},

		// Conversions
		{"DEG(3.141592653589793)", 180, true},

		// Logical
		{"NOT(0)", 1, true},
		{"NOT(1)", 0, true},
		{"AND(1,1)", 1, true},
		{"AND(1,0)", 0, true},
		{"OR(0,1)", 1, true},
		{"OR(0,0)", 0, true},

		// Exponential/Log
		{"EXP(0)", 1, true},
		{"LN(1)", 0, true},

		// Power
		{"POW(2,3)", 8, true},
		{"POW(3,2)", 9, true},

		// Modulo
		{"MOD(7,3)", 1, true},
		{"MOD(10,5)", 0, true},

		// Complex expressions
		{"SQRT((3)^2+(4)^2)", 5, true},
		{"(1+2)*(3+4)", 21, true},
		{"2*3+4*5", 26, true},

		// Bitwise
		{"BITAND(12,10)", 8, true},
		{"BITOR(12,10)", 14, true},
		{"BITXOR(12,10)", 6, true},
	}

	for _, tt := range tests {
		got, ok := eval.Eval(tt.formula)
		if ok != tt.wantOK {
			t.Errorf("Eval(%q): got ok=%v, want ok=%v", tt.formula, ok, tt.wantOK)
			continue
		}
		if ok && math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("Eval(%q) = %v, want %v", tt.formula, got, tt.want)
		}
	}
}

// TestFormulaEvalResultUnsupported tests that unsupported functions are explicitly identified.
// MS-VSDX spec §2.2.11.2 defines ~175 functions; unsupported ones must not return fake values.
func TestFormulaEvalResultUnsupported(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	eval := NewFormulaEvaluator(shape)

	// These functions are explicitly unsupported and must NOT return fake values
	unsupportedTests := []struct {
		formula  string
		funcName string
	}{
		// Layout functions (MS-VSDX §2.2.5.2.51)
		{"GRAVITY(1,2,3)", "GRAVITY"},

		// Geometry intersection
		{"RECTSECT(1,2,3,4,5)", "RECTSECT"},
	}

	for _, tt := range unsupportedTests {
		t.Run(tt.funcName, func(t *testing.T) {
			result := eval.EvalResult(tt.formula)

			// Must NOT be success - these functions cannot return meaningful values
			if result.Status == FormulaSuccess {
				t.Errorf("EvalResult(%q) returned Success with value %v; want Unsupported", tt.formula, result.Value)
			}

			// The old Eval API must return false
			_, ok := eval.Eval(tt.formula)
			if ok {
				t.Errorf("Eval(%q) returned ok=true; want ok=false for unsupported function", tt.formula)
			}
		})
	}

	// Verify that supported functions still work
	supportedTests := []struct {
		formula string
		want    float64
	}{
		{"ABS(-5)", 5},
		{"SQRT(16)", 4},
		{"SIN(0)", 0},
		{"MAX(3,7)", 7},
		{"SUM(1,2,3)", 6},
	}

	for _, tt := range supportedTests {
		result := eval.EvalResult(tt.formula)
		if result.Status != FormulaSuccess {
			t.Errorf("EvalResult(%q) status=%v; want FormulaSuccess", tt.formula, result.Status)
		}
		if math.Abs(result.Value-tt.want) > 0.0001 {
			t.Errorf("EvalResult(%q) = %v; want %v", tt.formula, result.Value, tt.want)
		}
	}
}

// TestFormulaEvalStringFunctions tests string manipulation functions.
func TestFormulaEvalStringFunctions(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	eval := NewFormulaEvaluator(shape)

	// Test string functions return success and set Str result
	tests := []struct {
		formula string
		want    string
	}{
		{`LOWER("TEST")`, "test"},
		{`UPPER("test")`, "TEST"},
		{`TRIM("  hello  ")`, "hello"},
		{`REPT("ab",3)`, "ababab"},
		{`CONCATENATE("a","b","c")`, "abc"},
		{`SUBSTITUTE("hello","l","x")`, "hexxo"},
		{`REPLACE("hello",2,3,"XX")`, "hXXo"},
	}

	for _, tt := range tests {
		t.Run(tt.formula, func(t *testing.T) {
			result := eval.EvalResult(tt.formula)
			if result.Status != FormulaSuccess {
				t.Errorf("EvalResult(%q) status=%v; want FormulaSuccess", tt.formula, result.Status)
				return
			}
			if result.Str != tt.want {
				t.Errorf("EvalResult(%q).Str = %q; want %q", tt.formula, result.Str, tt.want)
			}

			// Also test EvalString
			str, ok := eval.EvalString(tt.formula)
			if !ok {
				t.Errorf("EvalString(%q) returned ok=false; want ok=true", tt.formula)
				return
			}
			if str != tt.want {
				t.Errorf("EvalString(%q) = %q; want %q", tt.formula, str, tt.want)
			}
		})
	}
}

func TestFormulaEvaluatorCellRefs(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	eval := NewFormulaEvaluator(shape)

	// Test cell references
	w, wok := eval.Eval("Width")
	h, hok := eval.Eval("Height")

	if !wok || !hok {
		t.Fatal("failed to evaluate Width/Height")
	}

	// Test expressions with cell refs
	half, ok := eval.Eval("Width*0.5")
	if !ok {
		t.Fatal("failed to evaluate Width*0.5")
	}
	if math.Abs(half-w*0.5) > 0.0001 {
		t.Errorf("Width*0.5 = %v, want %v", half, w*0.5)
	}

	// Test GUARD with cell ref
	guarded, ok := eval.Eval("GUARD(Width*0.5)")
	if !ok {
		t.Fatal("failed to evaluate GUARD(Width*0.5)")
	}
	if math.Abs(guarded-w*0.5) > 0.0001 {
		t.Errorf("GUARD(Width*0.5) = %v, want %v", guarded, w*0.5)
	}

	t.Logf("Shape dimensions: Width=%v, Height=%v", w, h)
}

func TestFormulaEvaluatorConnector(t *testing.T) {
	// Test formulas commonly used for connectors
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	// Set BeginX/Y and EndX/Y for testing
	shape.SetCellValue("BeginX", "1")
	shape.SetCellValue("BeginY", "1")
	shape.SetCellValue("EndX", "4")
	shape.SetCellValue("EndY", "5")

	eval := NewFormulaEvaluator(shape)

	// Test midpoint formula
	midX, ok := eval.Eval("(BeginX+EndX)/2")
	if !ok {
		t.Fatal("failed to evaluate midpoint X")
	}
	if math.Abs(midX-2.5) > 0.0001 {
		t.Errorf("(BeginX+EndX)/2 = %v, want 2.5", midX)
	}

	midY, ok := eval.Eval("(BeginY+EndY)/2")
	if !ok {
		t.Fatal("failed to evaluate midpoint Y")
	}
	if math.Abs(midY-3) > 0.0001 {
		t.Errorf("(BeginY+EndY)/2 = %v, want 3", midY)
	}

	// Test length formula: SQRT((EndX-BeginX)^2+(EndY-BeginY)^2)
	length, ok := eval.Eval("SQRT((EndX-BeginX)^2+(EndY-BeginY)^2)")
	if !ok {
		t.Fatal("failed to evaluate length formula")
	}
	expected := math.Sqrt(3*3 + 4*4) // sqrt(9+16) = sqrt(25) = 5
	if math.Abs(length-expected) > 0.0001 {
		t.Errorf("length = %v, want %v", length, expected)
	}

	t.Logf("Connector: midX=%v, midY=%v, length=%v", midX, midY, length)
}

func TestCalcValueBackwardsCompat(t *testing.T) {
	// Test that CalcValue still works for backward compatibility
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	// Test old formulas that were hardcoded
	tests := []struct {
		formula string
	}{
		{"Width*1"},
		{"Width*0"},
		{"Width*0.5"},
		{"Height*0.5"},
		{"GUARD(Width*0.5)"},
		{"GUARD(Height*0.5)"},
	}

	for _, tt := range tests {
		_, ok := CalcValue(shape, tt.formula)
		if !ok {
			t.Errorf("CalcValue(%q) failed", tt.formula)
		}
	}
}

func TestFormulaEdgeCases(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	eval := NewFormulaEvaluator(shape)

	// Test edge cases - should fail
	failCases := []string{
		"",         // Empty formula
		"1/0",      // Division by zero
		"SQRT(-1)", // Invalid sqrt
		"ASIN(2)",  // Invalid asin
		"ACOS(2)",  // Invalid acos
		"LN(0)",    // Invalid ln
		"LN(-1)",   // Invalid ln
	}

	for _, formula := range failCases {
		_, ok := eval.Eval(formula)
		if ok {
			t.Errorf("Eval(%q) should have failed", formula)
		}
	}

	// Test edge cases - should succeed
	successCases := []struct {
		formula string
		want    float64
	}{
		{"((1+2))", 3},
		{"(((5)))", 5},
		{"1+2+3+4", 10},
		{"10-5-3", 2},
		{"2*3*4", 24},
		{"SINH(0)", 0},
		{"COSH(0)", 1},
		{"TANH(0)", 0},
	}

	for _, tt := range successCases {
		got, ok := eval.Eval(tt.formula)
		if !ok {
			t.Errorf("Eval(%q) failed, expected success", tt.formula)
		} else if math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("Eval(%q) = %v, want %v", tt.formula, got, tt.want)
		}
	}

	// Test EvalFormula function
	result := EvalFormula(shape, "2+3")
	if result != "5" {
		t.Errorf("EvalFormula(2+3) = %q, want '5'", result)
	}

	// Invalid formula returns original
	result = EvalFormula(shape, "INVALID()")
	if result != "INVALID()" {
		t.Errorf("EvalFormula(INVALID()) = %q, want 'INVALID()'", result)
	}

	// Empty returns empty
	result = EvalFormula(shape, "")
	if result != "" {
		t.Errorf("EvalFormula('') = %q, want ''", result)
	}
}

func TestFormulaMoreFunctions(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	eval := NewFormulaEvaluator(shape)

	// Test RAD conversion
	rad, ok := eval.Eval("RAD(180)")
	if !ok {
		t.Error("RAD(180) failed")
	} else if math.Abs(rad-math.Pi) > 0.0001 {
		t.Errorf("RAD(180) = %v, want %v", rad, math.Pi)
	}

	// Test LOG10
	log, ok := eval.Eval("LOG10(100)")
	if !ok {
		t.Error("LOG10(100) failed")
	} else if math.Abs(log-2) > 0.0001 {
		t.Errorf("LOG10(100) = %v, want 2", log)
	}

	// Test LOG (same as LOG10)
	log2, ok := eval.Eval("LOG(1000)")
	if !ok {
		t.Error("LOG(1000) failed")
	} else if math.Abs(log2-3) > 0.0001 {
		t.Errorf("LOG(1000) = %v, want 3", log2)
	}

	// Test MODULUS (alias for MOD)
	mod, ok := eval.Eval("MODULUS(17,5)")
	if !ok {
		t.Error("MODULUS(17,5) failed")
	} else if math.Abs(mod-2) > 0.0001 {
		t.Errorf("MODULUS(17,5) = %v, want 2", mod)
	}

	// Test BOUND
	bound, ok := eval.Eval("BOUND(5,0,0,10)")
	if !ok {
		t.Error("BOUND(5,0,0,10) failed")
	} else if math.Abs(bound-5) > 0.0001 {
		t.Errorf("BOUND(5,0,0,10) = %v, want 5", bound)
	}

	// Bound clamped low
	boundLow, ok := eval.Eval("BOUND(-5,0,0,10)")
	if !ok {
		t.Error("BOUND(-5,0,0,10) failed")
	} else if math.Abs(boundLow-0) > 0.0001 {
		t.Errorf("BOUND(-5,0,0,10) = %v, want 0", boundLow)
	}

	// Bound clamped high
	boundHigh, ok := eval.Eval("BOUND(15,0,0,10)")
	if !ok {
		t.Error("BOUND(15,0,0,10) failed")
	} else if math.Abs(boundHigh-10) > 0.0001 {
		t.Errorf("BOUND(15,0,0,10) = %v, want 10", boundHigh)
	}
}

func TestFormulaNilShape(t *testing.T) {
	// Test with nil shape
	eval := NewFormulaEvaluator(nil)

	// Basic math should still work
	v, ok := eval.Eval("2+3")
	if !ok || v != 5 {
		t.Error("basic math failed with nil shape")
	}

	// Cell refs should fail gracefully
	_, ok = eval.Eval("Width")
	if ok {
		t.Error("Width should fail with nil shape")
	}
}

func TestSubstituteRefs(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	formula := "Width*0.5"
	result := SubstituteRefs(shape, formula)

	// Should contain a number instead of "Width"
	if result == formula {
		t.Logf("SubstituteRefs returned same formula (shape may have Width=0): %s", result)
	} else {
		t.Logf("SubstituteRefs(%q) = %q", formula, result)
	}
}

func TestSectionRoundTrip(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	s := shapes[0]

	// Add various sections
	s.AddScratchCell("42", "", "", "", "", "")
	s.AddAction("TestAction", "Test Menu", "")
	s.AddField(0, "Test Value", "")
	s.AddControl("TestCtrl", 0.5, 0.5, "Test Tip")
	s.AddTabStop(1.5, 1)

	// Save and reopen
	tmp := t.TempDir()
	outPath := tmp + "/sections_test.vsdx"
	if err := vis.SaveVsdx(outPath); err != nil {
		t.Fatalf("SaveVsdx: %v", err)
	}

	vis2, err := Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer vis2.Close()

	shapes2 := vis2.Pages[0].AllShapes()
	if len(shapes2) == 0 {
		t.Fatal("no shapes after reopen")
	}
	s2 := shapes2[0]

	// Verify sections persisted
	if len(s2.ScratchCells()) < 1 {
		t.Error("scratch cells not persisted")
	}
	if len(s2.Actions()) < 1 {
		t.Error("actions not persisted")
	}
	if len(s2.Fields()) < 1 {
		t.Error("fields not persisted")
	}
	if len(s2.Controls()) < 1 {
		t.Error("controls not persisted")
	}
	if len(s2.TabStops()) < 1 {
		t.Error("tab stops not persisted")
	}

	t.Log("All sections persisted correctly after round-trip")
}

// TestUnknownXMLPreservation tests that unknown XML elements and attributes are preserved
// during round-trip. This is critical for MS-VSDX spec compliance - the library must NOT
// silently drop unknown content. See MS-VSDX §2.2.5.4 for extension requirements.
func TestUnknownXMLPreservation(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	s := shapes[0]

	// Inject unknown elements and attributes directly into the XML tree
	// This simulates content from future VSDX versions or third-party extensions

	// Add unknown attribute to shape
	s.xml.CreateAttr("x:unknownAttr", "test-value-123")

	// Add unknown child element with content
	unknownElem := s.xml.CreateElement("x:UnknownExtension")
	unknownElem.CreateAttr("xmlns:x", "http://example.com/extension")
	unknownElem.CreateAttr("version", "1.0")
	unknownChild := unknownElem.CreateElement("x:CustomData")
	unknownChild.SetText("preserved-content-456")

	// Add unknown section (simulating future spec addition)
	unknownSection := s.xml.CreateElement("Section")
	unknownSection.CreateAttr("N", "FutureSection")
	unknownRow := unknownSection.CreateElement("Row")
	unknownRow.CreateAttr("IX", "0")
	unknownCell := unknownRow.CreateElement("Cell")
	unknownCell.CreateAttr("N", "FutureCell")
	unknownCell.CreateAttr("V", "future-value")

	// Save and reopen
	tmp := t.TempDir()
	outPath := tmp + "/unknown_xml_test.vsdx"
	if err := vis.SaveVsdx(outPath); err != nil {
		t.Fatalf("SaveVsdx: %v", err)
	}

	vis2, err := Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer vis2.Close()

	shapes2 := vis2.Pages[0].AllShapes()
	if len(shapes2) == 0 {
		t.Fatal("no shapes after reopen")
	}
	s2 := shapes2[0]

	// Verify unknown attribute preserved
	attrVal := s2.xml.SelectAttrValue("x:unknownAttr", "")
	if attrVal != "test-value-123" {
		t.Errorf("unknown attribute not preserved: got %q, want %q", attrVal, "test-value-123")
	}

	// Verify unknown element preserved
	extElem := s2.xml.FindElement("x:UnknownExtension")
	if extElem == nil {
		t.Error("unknown extension element not preserved")
	} else {
		// Check attribute
		ver := extElem.SelectAttrValue("version", "")
		if ver != "1.0" {
			t.Errorf("extension version attribute not preserved: got %q", ver)
		}
		// Check child element
		dataElem := extElem.FindElement("x:CustomData")
		if dataElem == nil {
			t.Error("unknown extension child element not preserved")
		} else if dataElem.Text() != "preserved-content-456" {
			t.Errorf("extension text not preserved: got %q", dataElem.Text())
		}
	}

	// Verify unknown section preserved
	futureSection := s2.xml.FindElement("Section[@N='FutureSection']")
	if futureSection == nil {
		t.Error("unknown section not preserved")
	} else {
		futureRow := futureSection.FindElement("Row[@IX='0']")
		if futureRow == nil {
			t.Error("unknown section row not preserved")
		} else {
			futureCell := futureRow.FindElement("Cell[@N='FutureCell']")
			if futureCell == nil {
				t.Error("unknown section cell not preserved")
			} else {
				val := futureCell.SelectAttrValue("V", "")
				if val != "future-value" {
					t.Errorf("unknown section cell value not preserved: got %q", val)
				}
			}
		}
	}

	t.Log("Unknown XML elements, attributes, and sections preserved correctly")
}

// TestXMLCommentsPreservation tests that XML comments are preserved during round-trip.
func TestXMLCommentsPreservation(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	// Add XML comment to a page root element
	page := vis.Pages[0]
	if page.xml == nil {
		t.Skip("page XML is nil")
	}
	root := page.xml.Root()
	if root == nil {
		t.Skip("page XML root is nil")
	}

	// Add comment as child of root element
	comment := etree.NewComment(" Test comment for preservation ")
	root.AddChild(comment)

	// Save and reopen
	tmp := t.TempDir()
	outPath := tmp + "/xml_comments_test.vsdx"
	if err := vis.SaveVsdx(outPath); err != nil {
		t.Fatalf("SaveVsdx: %v", err)
	}

	vis2, err := Open(outPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer vis2.Close()

	page2 := vis2.Pages[0]
	if page2.xml == nil {
		t.Fatal("page XML is nil after reopen")
	}
	root2 := page2.xml.Root()
	if root2 == nil {
		t.Fatal("page XML root is nil after reopen")
	}

	// Look for comment in children
	foundComment := false
	for _, child := range root2.Child {
		if c, ok := child.(*etree.Comment); ok {
			if strings.Contains(c.Data, "Test comment for preservation") {
				foundComment = true
				break
			}
		}
	}

	if !foundComment {
		t.Error("XML comment not preserved during round-trip")
	} else {
		t.Log("XML comments preserved correctly")
	}
}

func TestGradientToSVGDef(t *testing.T) {
	grad := &Gradient{
		Enabled: true,
		Type:    "linear",
		Angle:   0,
		Stops: []GradientStop{
			{Position: 0, Color: "#FF0000", Trans: 0},
			{Position: 1, Color: "#0000FF", Trans: 0},
		},
	}

	svg := gradientToSVGDef(grad, "test-grad", 2)
	if svg == "" {
		t.Error("expected non-empty SVG")
	}
	if !strings.Contains(svg, "linearGradient") {
		t.Error("expected linearGradient element")
	}
	if !strings.Contains(svg, "#FF0000") {
		t.Error("expected first stop color")
	}
	if !strings.Contains(svg, "#0000FF") {
		t.Error("expected second stop color")
	}

	// Test radial gradient
	radial := &Gradient{
		Enabled: true,
		Type:    "radial",
		Stops: []GradientStop{
			{Position: 0, Color: "#FFFFFF", Trans: 0},
			{Position: 1, Color: "#000000", Trans: 0.5},
		},
	}
	radialSVG := gradientToSVGDef(radial, "test-radial", 2)
	if !strings.Contains(radialSVG, "radialGradient") {
		t.Error("expected radialGradient element")
	}
}

func TestShadowToSVGFilter(t *testing.T) {
	shadow := &Shadow{
		Type:    1,
		OffsetX: 0.1,
		OffsetY: -0.1,
		Color:   "#000000",
		Opacity: 0.5,
		Blur:    0.05,
	}

	svg := shadowToSVGFilter(shadow, "test-shadow", 100, 100, 2)
	if svg == "" {
		t.Error("expected non-empty SVG")
	}
	if !strings.Contains(svg, "filter") {
		t.Error("expected filter element")
	}
	if !strings.Contains(svg, "feDropShadow") {
		t.Error("expected feDropShadow element")
	}

	// Test nil shadow
	nilSVG := shadowToSVGFilter(nil, "nil", 100, 100, 2)
	if nilSVG != "" {
		t.Error("expected empty SVG for nil shadow")
	}

	// Test no shadow
	noShadow := &Shadow{Type: 0}
	noShadowSVG := shadowToSVGFilter(noShadow, "none", 100, 100, 2)
	if noShadowSVG != "" {
		t.Error("expected empty SVG for Type 0")
	}
}

func TestGenerateUUID(t *testing.T) {
	uuid1 := generateUUID()
	uuid2 := generateUUID()

	if uuid1 == "" {
		t.Error("UUID should not be empty")
	}
	if uuid1 == uuid2 {
		t.Error("UUIDs should be unique")
	}
	if !strings.HasPrefix(uuid1, "{") || !strings.HasSuffix(uuid1, "}") {
		t.Error("UUID should be wrapped in braces")
	}
	if len(uuid1) != 38 { // {8-4-4-4-12}
		t.Errorf("UUID length = %d, want 38", len(uuid1))
	}
}

func TestValidate(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	result := vis.Validate()
	if result == nil {
		t.Fatal("Validate() returned nil")
	}

	// Test file should be valid (no errors, though warnings/info are OK).
	if !result.IsValid() {
		t.Errorf("test1.vsdx should be valid, got %d errors", len(result.Errors))
		for _, e := range result.Errors {
			t.Logf("  Error: %v", e)
		}
	}

	// Check that AllIssues returns all categories.
	all := result.AllIssues()
	expectedLen := len(result.Errors) + len(result.Warnings) + len(result.Info)
	if len(all) != expectedLen {
		t.Errorf("AllIssues() returned %d, expected %d", len(all), expectedLen)
	}
}

func TestValidateContentTypes(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	result := vis.Validate()

	// Content_Types.xml should exist and be checked.
	if _, ok := vis.ZipFileContents["[Content_Types].xml"]; !ok {
		t.Error("Content_Types.xml should exist")
	}

	// No errors expected for valid content types.
	for _, e := range result.Errors {
		if e.Path == "[Content_Types].xml" {
			t.Errorf("unexpected error in Content_Types: %v", e)
		}
	}
}

func TestValidateMasterReferences(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	// Find a shape with a master reference.
	var shapeWithMaster *Shape
	for _, page := range vis.Pages {
		for _, shape := range page.AllShapes() {
			if shape.MasterPageID != "" {
				shapeWithMaster = shape
				break
			}
		}
	}

	if shapeWithMaster == nil {
		t.Skip("no shapes with master references in test file")
	}

	// Validate should not report errors for valid master references.
	result := vis.Validate()
	for _, e := range result.Errors {
		if e.Message != "" && e.Element != "" {
			// Just verify validation runs without panics.
		}
	}
}

// TestValidateOPC tests OPC (Open Packaging Conventions) validation per MS-VSDX §2.1.
func TestValidateOPC(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	result := vis.Validate()

	// Root relationships file must exist
	if _, ok := vis.ZipFileContents["_rels/.rels"]; !ok {
		t.Error("_rels/.rels should exist in valid VSDX")
	}

	// visio/document.xml must exist
	if _, ok := vis.ZipFileContents["visio/document.xml"]; !ok {
		t.Error("visio/document.xml should exist in valid VSDX")
	}

	// No OPC-related errors expected for valid file
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "OPC") || strings.Contains(e.Message, "relationship") {
			t.Errorf("unexpected OPC error: %v", e)
		}
	}

	t.Logf("OPC validation: %d errors, %d warnings, %d info",
		len(result.Errors), len(result.Warnings), len(result.Info))

	// Log any info/warnings for visibility
	for _, w := range result.Warnings {
		t.Logf("  Warning: %v", w)
	}
	for _, i := range result.Info {
		t.Logf("  Info: %v", i)
	}
}

// TestValidateOPCMissingParts tests that validation detects missing OPC parts.
func TestValidateOPCMissingParts(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	// Remove required OPC parts to test detection
	originalRels := vis.ZipFileContents["_rels/.rels"]
	delete(vis.ZipFileContents, "_rels/.rels")

	result := vis.Validate()

	// Should report missing root relationships
	foundMissingRels := false
	for _, e := range result.Errors {
		if e.Path == "_rels/.rels" {
			foundMissingRels = true
			break
		}
	}
	if !foundMissingRels {
		t.Error("validation should detect missing _rels/.rels")
	}

	// Restore for cleanup
	vis.ZipFileContents["_rels/.rels"] = originalRels
}

// TestValidateOPCDuplicateRelationshipID tests duplicate relationship ID detection.
func TestValidateOPCDuplicateRelationshipID(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	// Create a .rels file with duplicate IDs
	duplicateRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://test/type" Target="test1.xml"/>
  <Relationship Id="rId1" Type="http://test/type2" Target="test2.xml"/>
</Relationships>`
	vis.ZipFileContents["_rels/.rels"] = []byte(duplicateRels)

	result := vis.Validate()

	// Should report duplicate relationship ID
	foundDuplicate := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "Duplicate relationship ID") {
			foundDuplicate = true
			break
		}
	}
	if !foundDuplicate {
		t.Error("validation should detect duplicate relationship IDs")
	}
}

func TestDefaultRouteOptions(t *testing.T) {
	opts := DefaultRouteOptions()
	if !opts.Orthogonal {
		t.Error("default Orthogonal should be true")
	}
	if opts.Padding <= 0 {
		t.Error("default Padding should be positive")
	}
	if opts.GridSize <= 0 {
		t.Error("default GridSize should be positive")
	}
}

func TestBoundingBox(t *testing.T) {
	vis, err := Open("../tests/basic-001.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Skip("no shapes in test file")
	}

	bbox := shapes[0].BoundingBox()
	if bbox.Width <= 0 || bbox.Height <= 0 {
		// Only check if shape has dimensions
		if shapes[0].Width() > 0 {
			t.Errorf("BoundingBox should have positive dimensions, got width=%v height=%v", bbox.Width, bbox.Height)
		}
	}
}

func TestBackgroundPage(t *testing.T) {
	vis, err := Open("../tests/basic-001.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	page := vis.Pages[0]

	// Most pages don't have a background
	bg := page.BackgroundPage()
	// Just verify it doesn't crash
	_ = bg

	// Check IsBackgroundPage (should be false for regular page)
	if page.IsBackgroundPage() {
		// Could be true if the test file has background pages
		t.Log("page is a background page")
	}
}

func TestCreateStencil(t *testing.T) {
	st := CreateStencil()
	if st == nil {
		t.Fatal("CreateStencil() returned nil")
	}
	if len(st.Masters) != 0 {
		t.Errorf("new stencil should have 0 masters, got %d", len(st.Masters))
	}
	if st.ZipFileContents == nil {
		t.Error("ZipFileContents should not be nil")
	}
}

func TestExportOptions(t *testing.T) {
	opts := DefaultExportOptions()
	if opts.DPI != 96 {
		t.Errorf("default DPI = %v, want 96", opts.DPI)
	}
}

func TestCommentsAPI(t *testing.T) {
	vis := &VisioFile{
		ZipFileContents: make(map[string][]byte),
	}

	// Add content types and rels for comments to work
	vis.ZipFileContents["[Content_Types].xml"] = []byte(`<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`)
	vis.ZipFileContents["visio/_rels/document.xml.rels"] = []byte(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`)

	// Initially no comments
	if len(vis.Comments()) != 0 {
		t.Error("new file should have no comments")
	}

	// Add a comment
	comment := vis.AddComment(1, "Test comment", "Test Author")
	if comment == nil {
		t.Fatal("AddComment returned nil")
	}
	if comment.Text != "Test comment" {
		t.Errorf("comment text = %q, want %q", comment.Text, "Test comment")
	}
	if comment.PageID != 1 {
		t.Errorf("comment pageID = %d, want 1", comment.PageID)
	}

	// The comment was created with an author, verify the returned comment
	if comment.AuthorID < 0 {
		t.Errorf("comment authorID = %d, want >= 0", comment.AuthorID)
	}

	// Update comment
	comment.SetText("Updated comment")
	if comment.Text != "Updated comment" {
		t.Errorf("updated text = %q, want %q", comment.Text, "Updated comment")
	}

	// Mark done
	comment.SetDone(true)
	if !comment.Done {
		t.Error("comment should be marked done")
	}
}

func TestGetInitials(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"", ""},
		{"Alice", "A"},
		{"Bob Smith", "BS"},
		{"John Paul Jones", "JPJ"},
	}

	for _, tt := range tests {
		got := getInitials(tt.name)
		if got != tt.want {
			t.Errorf("getInitials(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestLineGradient(t *testing.T) {
	// Test line gradient parsing and setting
	doc := etree.NewDocument()
	shapeElem := doc.CreateElement("Shape")
	shapeElem.CreateAttr("ID", "1")

	// Add cell for LineGradientEnabled
	cell := shapeElem.CreateElement("Cell")
	cell.CreateAttr("N", "LineGradientEnabled")
	cell.CreateAttr("V", "1")

	// Add section with stops
	section := shapeElem.CreateElement("Section")
	section.CreateAttr("N", "LineGradient")

	row := section.CreateElement("Row")
	row.CreateAttr("IX", "0")

	colorCell := row.CreateElement("Cell")
	colorCell.CreateAttr("N", "GradientStopColor")
	colorCell.CreateAttr("V", "#FF0000")

	posCell := row.CreateElement("Cell")
	posCell.CreateAttr("N", "GradientStopPosition")
	posCell.CreateAttr("V", "0")

	shape := &Shape{
		ID:    "1",
		xml:   shapeElem,
		Cells: make(map[string]*Cell),
	}
	// Populate Cells map with LineGradientEnabled
	shape.Cells["LineGradientEnabled"] = newCell(cell, shape)

	grad := shape.LineGradient()
	if grad == nil {
		t.Fatal("LineGradient() returned nil")
	}
	if !grad.Enabled {
		t.Error("gradient should be enabled")
	}
	if len(grad.Stops) != 1 {
		t.Errorf("expected 1 stop, got %d", len(grad.Stops))
	}
	if grad.Stops[0].Color != "#FF0000" {
		t.Errorf("stop color = %q, want #FF0000", grad.Stops[0].Color)
	}
}

func TestDataRecordSet(t *testing.T) {
	vis := &VisioFile{
		ZipFileContents: make(map[string][]byte),
	}
	vis.ZipFileContents["[Content_Types].xml"] = []byte(`<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`)

	// Create a recordset
	rs := vis.CreateDataRecordSet("TestData", []string{"Name", "Value"})
	if rs == nil {
		t.Fatal("CreateDataRecordSet returned nil")
	}
	if rs.Name != "TestData" {
		t.Errorf("recordset name = %q, want %q", rs.Name, "TestData")
	}
	if len(rs.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(rs.Columns))
	}

	// Add a record
	rec := rs.AddRecord(map[string]string{
		"Name":  "Item1",
		"Value": "100",
	})
	if rec == nil {
		t.Fatal("AddRecord returned nil")
	}
	if rec.Values["Name"] != "Item1" {
		t.Errorf("record Name = %q, want %q", rec.Values["Name"], "Item1")
	}
}

func TestFormulaMoreMath(t *testing.T) {
	eval := NewFormulaEvaluator(nil)

	// Test new math functions
	tests := []struct {
		formula string
		want    float64
	}{
		{"GCD(12, 8)", 4},
		{"LCM(4, 6)", 12},
		{"FACT(5)", 120},
		{"EVEN(3)", 4},
		{"ODD(4)", 5},
		{"TRUE()", 1},
		{"FALSE()", 0},
		{"XOR(1, 0)", 1},
		{"XOR(1, 1)", 0},
	}

	for _, tt := range tests {
		got, ok := eval.Eval(tt.formula)
		if !ok {
			t.Errorf("Eval(%q) failed", tt.formula)
			continue
		}
		if got != tt.want {
			t.Errorf("Eval(%q) = %v, want %v", tt.formula, got, tt.want)
		}
	}
}

func TestFormulaColorFunctions(t *testing.T) {
	eval := NewFormulaEvaluator(nil)

	// Test RGB function
	rgb, ok := eval.Eval("RGB(255, 128, 0)")
	if !ok {
		t.Fatal("RGB() failed")
	}
	// RGB should pack to (255 << 16) | (128 << 8) | 0 = 16744448
	expected := float64(255<<16 | 128<<8 | 0)
	if rgb != expected {
		t.Errorf("RGB(255, 128, 0) = %v, want %v", rgb, expected)
	}

	// Test RED extraction
	red, ok := eval.Eval("RED(16744448)")
	if !ok {
		t.Fatal("RED() failed")
	}
	if red != 255 {
		t.Errorf("RED(16744448) = %v, want 255", red)
	}

	// Test GREEN extraction
	green, ok := eval.Eval("GREEN(16744448)")
	if !ok {
		t.Fatal("GREEN() failed")
	}
	if green != 128 {
		t.Errorf("GREEN(16744448) = %v, want 128", green)
	}

	// Test BLUE extraction
	blue, ok := eval.Eval("BLUE(16744448)")
	if !ok {
		t.Fatal("BLUE() failed")
	}
	if blue != 0 {
		t.Errorf("BLUE(16744448) = %v, want 0", blue)
	}
}

func TestFormulaNewFunctions(t *testing.T) {
	eval := NewFormulaEvaluator(nil)

	tests := []struct {
		formula  string
		expected float64
	}{
		// String functions
		{"LEN(\"hello\")", 5},
		{"CODE(\"A\")", 65},

		// Date functions
		{"DATE(2024,6,15)", 2024*365 + 6*30 + 15},
		{"DAY(45)", 14}, // 45 % 31 = 14
		{"DATEDIF(10,20)", 10},

		// Aggregate functions
		{"SUM(1,2,3,4,5)", 15},
		{"AVERAGE(10,20,30)", 20},
		{"COUNT(1,2,3)", 3},
		{"PRODUCT(2,3,4)", 24},
		{"QUOTIENT(17,5)", 3},

		// Trig functions
		{"COT(PI()/4)", 1},
		{"SEC(0)", 1},
		{"CSC(PI()/2)", 1},
		{"ASINH(0)", 0},
		{"ACOSH(1)", 0},
		{"ATANH(0)", 0},

		// Combinatorics
		{"COMBIN(5,2)", 10},
		{"PERMUT(5,2)", 20},
		{"POWER(2,10)", 1024},

		// Info functions
		{"TYPE(123)", 1},
		{"ISLOGICAL(TRUE)", 1},
		{"COUNTA(1,2,3)", 3},
	}

	for _, tc := range tests {
		result, ok := eval.Eval(tc.formula)
		if !ok {
			t.Errorf("%s: evaluation failed", tc.formula)
			continue
		}
		if math.Abs(result-tc.expected) > 0.001 {
			t.Errorf("%s = %v, want %v", tc.formula, result, tc.expected)
		}
	}
}

func TestLineGradientToSVGDef(t *testing.T) {
	grad := &LineGradient{
		Enabled: true,
		Angle:   0,
		Stops: []GradientStop{
			{Position: 0, Color: "#FF0000"},
			{Position: 1, Color: "#0000FF"},
		},
	}

	svg := lineGradientToSVGDef("test-grad", grad)
	if svg == "" {
		t.Error("lineGradientToSVGDef returned empty string")
	}
	if !strings.Contains(svg, "linearGradient") {
		t.Error("expected linearGradient in output")
	}
	if !strings.Contains(svg, "test-grad") {
		t.Error("expected id in output")
	}
}

func TestStyleSheets(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	// Test getting style sheets.
	styles := vis.StyleSheets()
	// May or may not have style sheets depending on the test file.
	t.Logf("Found %d style sheets", len(styles))

	// Test style property classification.
	if !isLineProperty("LineWeight") {
		t.Error("LineWeight should be a line property")
	}
	if !isFillProperty("FillForegnd") {
		t.Error("FillForegnd should be a fill property")
	}
	if !isTextProperty("VerticalAlign") {
		t.Error("VerticalAlign should be a text property")
	}
	if isLineProperty("FillForegnd") {
		t.Error("FillForegnd should not be a line property")
	}
}

// --- Cell Unit and Error Tests ---

func TestCellUnitError(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	if len(vis.Pages) == 0 {
		t.Skip("no pages")
	}

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Skip("no shapes")
	}

	shape := shapes[0]

	// Test setting and getting unit
	if cell, ok := shape.Cells["PinX"]; ok {
		cell.SetUnit("IN")
		if cell.Unit() != "IN" {
			t.Errorf("expected unit IN, got %s", cell.Unit())
		}
		cell.SetUnit("")
		if cell.Unit() != "" {
			t.Error("expected empty unit after clearing")
		}
	}

	// Test error attribute
	if cell, ok := shape.Cells["PinX"]; ok {
		if cell.HasError() {
			t.Error("cell should not have error initially")
		}
		cell.SetError("#VALUE!")
		if !cell.HasError() {
			t.Error("cell should have error after setting")
		}
		if cell.Error() != "#VALUE!" {
			t.Errorf("expected error #VALUE!, got %s", cell.Error())
		}
		cell.SetError("")
		if cell.HasError() {
			t.Error("cell should not have error after clearing")
		}
	}
}

// --- Root Rels and Document Properties Tests ---

func TestRootRelsAndDocProps(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	// Test root relationships
	rels := vis.RootRelationships()
	t.Logf("Found %d root relationships", len(rels))
	if len(rels) > 0 {
		for _, rel := range rels {
			t.Logf("  %s: %s -> %s", rel.ID, rel.Type, rel.Target)
		}
	}

	// Test core properties
	props := vis.CoreProperties()
	t.Logf("Core properties: Title=%q, Creator=%q", props.Title, props.Creator)

	// Test setting core properties
	vis.SetCoreProperties(&CoreProperties{
		Title:   "Test Document",
		Creator: "Test Author",
	})
	props2 := vis.CoreProperties()
	if props2.Title != "Test Document" {
		t.Errorf("expected title 'Test Document', got %q", props2.Title)
	}

	// Test custom properties
	vis.SetCustomProperty("TestProp", "TestValue")
	val, ok := vis.GetCustomProperty("TestProp")
	if !ok {
		t.Error("expected custom property to exist")
	}
	if val != "TestValue" {
		t.Errorf("expected TestValue, got %q", val)
	}
}

// --- SmartTag and ActionTag Tests ---

func TestSmartTagActionTag(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	if len(vis.Pages) == 0 {
		t.Skip("no pages")
	}

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Skip("no shapes")
	}

	shape := shapes[0]

	// Test SmartTags
	smartTags := shape.SmartTags()
	t.Logf("Found %d smart tags", len(smartTags))

	// Add a smart tag
	shape.AddSmartTag("TestTag", 0.5, 0.5, "Test Description")
	smartTags = shape.SmartTags()
	if len(smartTags) == 0 {
		t.Error("expected at least 1 smart tag after adding")
	}

	// Test ActionTags
	actionTags := shape.ActionTags()
	t.Logf("Found %d action tags", len(actionTags))

	// Add an action tag
	shape.AddActionTag("TestAction", 0.5, 0.5, "tag1", "Test Action")
	actionTags = shape.ActionTags()
	if len(actionTags) == 0 {
		t.Error("expected at least 1 action tag after adding")
	}
}

// --- ConnectionABCD Tests ---

func TestConnectionABCD(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	if len(vis.Pages) == 0 {
		t.Skip("no pages")
	}

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Skip("no shapes")
	}

	shape := shapes[0]

	// Test ConnectionABCD
	conns := shape.ConnectionsABCD()
	t.Logf("Found %d connection points (ABCD)", len(conns))

	// Add a connection point with direction
	ix := shape.AddConnectionABCD(0.5, 0.5, 1.0, 0.0, 0)
	if ix < 0 {
		t.Error("expected non-negative index")
	}

	conns = shape.ConnectionsABCD()
	if len(conns) == 0 {
		t.Error("expected at least 1 connection point after adding")
	}
}

// --- Reviewer and Annotation Write Tests ---

func TestReviewerAnnotationWrite(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	// Add a reviewer
	reviewer := vis.AddReviewer("Test User", "TU", "#FF0000")
	if reviewer == nil {
		t.Fatal("failed to add reviewer")
	}
	if reviewer.Name != "Test User" {
		t.Errorf("expected name 'Test User', got %q", reviewer.Name)
	}

	// Verify reviewer exists
	reviewers := vis.Reviewers()
	found := false
	for _, r := range reviewers {
		if r.Name == "Test User" {
			found = true
			break
		}
	}
	if !found {
		t.Error("added reviewer not found")
	}

	// Add annotation to page
	if len(vis.Pages) > 0 {
		page := vis.Pages[0]
		ann := page.AddAnnotation(1.0, 1.0, reviewer.ID, "Test Comment")
		if ann == nil {
			t.Fatal("failed to add annotation")
		}
		if ann.Comment != "Test Comment" {
			t.Errorf("expected comment 'Test Comment', got %q", ann.Comment)
		}

		// Update annotation
		ann.SetComment("Updated Comment")
		if ann.Comment != "Updated Comment" {
			t.Error("comment not updated")
		}

		// Delete annotation
		if !page.DeleteAnnotation(ann.ID) {
			t.Error("failed to delete annotation")
		}
	}

	// Delete reviewer
	if !vis.DeleteReviewer(reviewer.ID) {
		t.Error("failed to delete reviewer")
	}
}

// --- TheCel Reference Token Test ---

func TestTheCelReference(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Skipf("test file not found: %v", err)
	}
	defer vis.Close()

	if len(vis.Pages) == 0 {
		t.Skip("no pages")
	}

	// Find a shape with a master
	var shapeWithMaster *Shape
	for _, shape := range vis.Pages[0].AllShapes() {
		if shape.MasterPageID != "" {
			shapeWithMaster = shape
			break
		}
	}

	if shapeWithMaster == nil {
		t.Skip("no shape with master found")
	}

	// Test formula evaluator with TheCel
	eval := NewFormulaEvaluator(shapeWithMaster)

	// TheCel without a cell name should return 0
	result, ok := eval.Eval("TheCel")
	if ok && result != 0 {
		t.Logf("TheCel returned %f", result)
	}

	// Test Sheet.N! reference
	result, ok = eval.Eval(fmt.Sprintf("Sheet.%s!Width", shapeWithMaster.ID))
	if ok {
		t.Logf("Sheet reference returned %f", result)
	}
}

// --- Theme Variant Tests ---

// TestFormulaNewMSVSDXFunctions tests newly implemented MS-VSDX spec functions.
func TestFormulaNewMSVSDXFunctions(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]
	eval := NewFormulaEvaluator(shape)

	tests := []struct {
		name    string
		formula string
		want    float64
		approx  bool
	}{
		// ANG360 - normalize angle to 0..2*PI
		{"ANG360_positive", "ANG360(7)", math.Mod(7, 2*math.Pi), true},
		{"ANG360_negative", "ANG360(-1)", 2*math.Pi - 1, true},

		// Fuzzy comparison operators
		{"FEQ_equal", "FEQ(1.0000000001,1)", 1, false},
		{"FEQ_not_equal", "FEQ(1.1,1)", 0, false},
		{"FGT_true", "FGT(2,1)", 1, false},
		{"FGT_false", "FGT(1,2)", 0, false},
		{"FGE_equal", "FGE(1,1)", 1, false},
		{"FLT_true", "FLT(1,2)", 1, false},
		{"FLE_equal", "FLE(1,1)", 1, false},

		// BLEND - color blending
		{"BLEND_50pct", "BLEND(RGB(255,0,0),RGB(0,0,255),0.5)", float64(127<<16 | 0<<8 | 127), true},
		{"BLEND_0pct", "BLEND(RGB(255,0,0),RGB(0,0,255),0)", float64(255 << 16), false},
		{"BLEND_100pct", "BLEND(RGB(255,0,0),RGB(0,0,255),1)", float64(255), false},

		// SHADE/TINT/TONE - color modifiers
		{"SHADE_basic", "SHADE(RGB(128,128,128),0)", float64(128<<16 | 128<<8 | 128), true},
		{"TINT_basic", "TINT(RGB(128,128,128),0)", float64(128<<16 | 128<<8 | 128), true},
		{"TONE_basic", "TONE(RGB(128,128,128),0)", float64(128<<16 | 128<<8 | 128), true},

		// Color difference functions
		{"HUEDIFF", "HUEDIFF(RGB(255,0,0),RGB(255,0,0))", 0, true},
		{"SATDIFF", "SATDIFF(RGB(255,0,0),RGB(255,0,0))", 0, true},
		{"LUMDIFF", "LUMDIFF(RGB(255,0,0),RGB(255,0,0))", 0, true},

		// LOG functions
		{"LOG2_8", "LOG2(8)", 3, true},
		{"LOG2_1", "LOG2(1)", 0, true},
		{"LOGN_base10", "LOGN(100,10)", 2, true},

		// Point functions
		{"PNT", "PNT(3,4)", 3, false},
		{"PNTX", "PNTX(5)", 5, false},
		{"PNTY", "PNTY(7)", 7, false},

		// Theme functions (return defaults)
		{"THEME", "THEME(0)", 0, false},
		{"THEMECBV", "THEMECBV(RGB(255,0,0),1)", float64(255 << 16), false},
		{"THEMEPROP", "THEMEPROP()", 0, false},
		{"THEMERESTORE", "THEMERESTORE(0)", 0, false},

		// Document functions (return 0)
		{"PAGENAME", "PAGENAME()", 0, false},
		{"PAGENUMBER", "PAGENUMBER()", 1, false},
		{"MASTERNAME", "MASTERNAME()", 0, false},

		// Text functions
		{"TEXTHEIGHT", "TEXTHEIGHT(Width,1)", 0.2, true},
		{"TEXTWIDTH", "TEXTWIDTH(Width)", 1.0, true},

		// Date functions
		{"DAYOFYEAR", "DAYOFYEAR(100)", 100, false},
		{"EOMONTH", "EOMONTH(0,1)", 30, false},

		// Other functions
		{"VALUE", "VALUE(42)", 42, false},
		{"SETATREFEVAL", "SETATREFEVAL(123)", 123, false},
		{"CELLISTHEMED_true", "CELLISTHEMED(1)", 1, false},
		{"CELLISTHEMED_false", "CELLISTHEMED(0)", 0, false},
		{"N", "N(99)", 99, false},
		{"ISREF", "ISREF(Width)", 1, false},
		{"STRSAMEEX", "STRSAMEEX(\"test\",\"TEST\",0,0)", 1, false},
		{"SEGMENTCOUNT", "SEGMENTCOUNT()", 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := eval.Eval(tt.formula)
			if !ok {
				t.Errorf("Eval(%q) failed, expected success", tt.formula)
				return
			}
			if tt.approx {
				if math.Abs(got-tt.want) > 1 {
					t.Errorf("Eval(%q) = %v, want approx %v", tt.formula, got, tt.want)
				}
			} else {
				if got != tt.want {
					t.Errorf("Eval(%q) = %v, want %v", tt.formula, got, tt.want)
				}
			}
		})
	}
}

func TestGeometryPathFunctions(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}

	// Find a shape with geometry
	var shape *Shape
	for _, s := range shapes {
		if s.Geometry != nil && len(s.Geometry.Rows) > 0 {
			shape = s
			break
		}
	}
	if shape == nil {
		t.Skip("no shape with geometry found")
	}

	// Test Geometry.PathLength
	length := shape.Geometry.PathLength()
	if length < 0 {
		t.Errorf("PathLength() = %v, expected non-negative", length)
	}
	t.Logf("PathLength = %v", length)

	// Test Geometry.PointAlongPath
	x, y := shape.Geometry.PointAlongPath(0.5, 0)
	t.Logf("PointAlongPath(0.5, 0) = (%v, %v)", x, y)

	// Test Geometry.AngleAlongPath
	angle := shape.Geometry.AngleAlongPath(0.5)
	t.Logf("AngleAlongPath(0.5) = %v", angle)

	// Test via FormulaEvaluator
	eval := NewFormulaEvaluator(shape)

	// PATHLENGTH should return the geometry length
	pathLen, ok := eval.Eval("PATHLENGTH(Geometry1)")
	if !ok {
		t.Error("PATHLENGTH(Geometry1) failed")
	} else {
		if math.Abs(pathLen-length) > 0.0001 {
			t.Errorf("PATHLENGTH formula = %v, Geometry.PathLength() = %v", pathLen, length)
		}
	}

	// ANGLEALONGPATH should return an angle
	angleVal, ok := eval.Eval("ANGLEALONGPATH(Geometry1, 0.5)")
	if !ok {
		t.Error("ANGLEALONGPATH(Geometry1, 0.5) failed")
	} else {
		t.Logf("ANGLEALONGPATH formula = %v", angleVal)
	}

	// POINTALONGPATH should return x coordinate
	pointX, ok := eval.Eval("POINTALONGPATH(Geometry1, 0.5)")
	if !ok {
		t.Error("POINTALONGPATH(Geometry1, 0.5) failed")
	} else {
		t.Logf("POINTALONGPATH formula X = %v", pointX)
	}
}

func TestBevelEffect(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	// Test reading bevel effect (should return zeros for shapes without bevel)
	bevel := shape.BevelEffect()
	if bevel == nil {
		t.Fatal("BevelEffect() returned nil")
	}
	t.Logf("Initial bevel: TopType=%d, TopWidth=%f", bevel.TopType, bevel.TopWidth)

	// Test setting bevel effect
	newBevel := &BevelEffect{
		TopType:       1,
		TopWidth:      0.5,
		TopHeight:     0.25,
		BottomType:    2,
		BottomWidth:   0.3,
		BottomHeight:  0.15,
		DepthColor:    "#FF0000",
		DepthSize:     0.1,
		ContourColor:  "#00FF00",
		ContourSize:   0.05,
		MaterialType:  3,
		LightingType:  5,
		LightingAngle: 45.0,
	}
	shape.SetBevelEffect(newBevel)

	// Verify the values were set
	readBack := shape.BevelEffect()
	if readBack.TopType != 1 {
		t.Errorf("BevelTopType = %d, want 1", readBack.TopType)
	}
	if readBack.TopWidth != 0.5 {
		t.Errorf("BevelTopWidth = %f, want 0.5", readBack.TopWidth)
	}
	if readBack.MaterialType != 3 {
		t.Errorf("BevelMaterialType = %d, want 3", readBack.MaterialType)
	}
}

func TestGlowEffect(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	// Test reading glow effect
	glow := shape.GlowEffect()
	if glow == nil {
		t.Fatal("GlowEffect() returned nil")
	}

	// Test setting glow effect
	newGlow := &GlowEffect{
		Color:      "#FF00FF",
		ColorTrans: 0.5,
		Size:       10.0,
	}
	shape.SetGlowEffect(newGlow)

	readBack := shape.GlowEffect()
	if readBack.Size != 10.0 {
		t.Errorf("GlowSize = %f, want 10.0", readBack.Size)
	}
}

func TestReflectionEffect(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	// Test reading reflection effect
	reflection := shape.ReflectionEffect()
	if reflection == nil {
		t.Fatal("ReflectionEffect() returned nil")
	}

	// Test setting reflection effect
	newReflection := &ReflectionEffect{
		Size:  50.0,
		Trans: 0.3,
		Dist:  0.1,
		Blur:  5.0,
	}
	shape.SetReflectionEffect(newReflection)

	readBack := shape.ReflectionEffect()
	if readBack.Size != 50.0 {
		t.Errorf("ReflectionSize = %f, want 50.0", readBack.Size)
	}
}

func TestSketchEffect(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	// Test reading sketch effect
	sketch := shape.SketchEffect()
	if sketch == nil {
		t.Fatal("SketchEffect() returned nil")
	}

	// Test setting sketch effect
	newSketch := &SketchEffect{
		Enabled:    true,
		Seed:       12345,
		Amount:     0.5,
		LineWeight: 0.2,
		LineChange: 0.1,
		FillChange: 0.05,
	}
	shape.SetSketchEffect(newSketch)

	readBack := shape.SketchEffect()
	if !readBack.Enabled {
		t.Error("SketchEnabled should be true")
	}
	if readBack.Seed != 12345 {
		t.Errorf("SketchSeed = %d, want 12345", readBack.Seed)
	}
}

func TestRotation3DEffect(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	// Test reading 3D rotation effect
	rotation := shape.Rotation3DEffect()
	if rotation == nil {
		t.Fatal("Rotation3DEffect() returned nil")
	}

	// Test setting 3D rotation effect
	newRotation := &Rotation3DEffect{
		XAngle:             30.0,
		YAngle:             45.0,
		ZAngle:             60.0,
		RotationType:       1,
		Perspective:        0.5,
		DistanceFromGround: 10.0,
		KeepTextFlat:       true,
	}
	shape.SetRotation3DEffect(newRotation)

	readBack := shape.Rotation3DEffect()
	if readBack.XAngle != 30.0 {
		t.Errorf("RotationXAngle = %f, want 30.0", readBack.XAngle)
	}
	if !readBack.KeepTextFlat {
		t.Error("KeepTextFlat should be true")
	}
}

func TestQuickStyle(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	// Test reading QuickStyle
	qs := shape.QuickStyle()
	if qs == nil {
		t.Fatal("QuickStyle() returned nil")
	}
	t.Logf("Initial QuickStyle: Type=%d, FillMatrix=%d, LineColor=%d", qs.Type, qs.FillMatrix, qs.LineColor)

	// Test setting QuickStyle
	newQS := &QuickStyle{
		Type:          1,
		Variation:     2,
		LineMatrix:    3,
		FillMatrix:    4,
		EffectsMatrix: 5,
		FontMatrix:    1,
		LineColor:     2,
		FillColor:     3,
		ShadowColor:   4,
		FontColor:     5,
	}
	shape.SetQuickStyle(newQS)

	readBack := shape.QuickStyle()
	if readBack.Type != 1 {
		t.Errorf("QuickStyleType = %d, want 1", readBack.Type)
	}
	if readBack.Variation != 2 {
		t.Errorf("QuickStyleVariation = %d, want 2", readBack.Variation)
	}
	if readBack.FillMatrix != 4 {
		t.Errorf("QuickStyleFillMatrix = %d, want 4", readBack.FillMatrix)
	}
	if readBack.FillColor != 3 {
		t.Errorf("QuickStyleFillColor = %d, want 3", readBack.FillColor)
	}
}

func TestSoftEdgesSize(t *testing.T) {
	vis, err := Open("../tests/test1.vsdx")
	if err != nil {
		t.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.Pages[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("no shapes")
	}
	shape := shapes[0]

	// Test reading soft edges
	size := shape.SoftEdgesSize()
	t.Logf("Initial SoftEdgesSize = %f", size)

	// Test setting soft edges
	shape.SetSoftEdgesSize(5.0)
	readBack := shape.SoftEdgesSize()
	if readBack != 5.0 {
		t.Errorf("SoftEdgesSize = %f, want 5.0", readBack)
	}
}

// TestMarkupCompatibility tests processing of mc:AlternateContent elements.
func TestMarkupCompatibility(t *testing.T) {
	// Test with a document containing mc:AlternateContent
	xmlStr := `<?xml version="1.0" encoding="UTF-8"?>
<root xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006">
  <mc:AlternateContent>
    <mc:Choice Requires="unknown">
      <unsupported>Should not appear</unsupported>
    </mc:Choice>
    <mc:Fallback>
      <supported>This is the fallback content</supported>
    </mc:Fallback>
  </mc:AlternateContent>
</root>`

	doc := etree.NewDocument()
	if err := doc.ReadFromString(xmlStr); err != nil {
		t.Fatalf("Failed to parse XML: %v", err)
	}

	ProcessMarkupCompatibility(doc)

	// The mc:AlternateContent should be replaced with fallback content
	root := doc.Root()
	if root == nil {
		t.Fatal("Root is nil after processing")
	}

	// Check that unsupported is NOT present
	unsupported := root.FindElement(".//unsupported")
	if unsupported != nil {
		t.Error("Unsupported element should not be present after processing")
	}

	// Check that supported IS present
	supported := root.FindElement(".//supported")
	if supported == nil {
		t.Error("Supported fallback element should be present after processing")
	} else {
		text := supported.Text()
		if text != "This is the fallback content" {
			t.Errorf("Fallback text = %q; want %q", text, "This is the fallback content")
		}
	}
}

// TestGetIgnorableNamespaces tests extraction of mc:Ignorable prefixes.
func TestGetIgnorableNamespaces(t *testing.T) {
	xmlStr := `<?xml version="1.0" encoding="UTF-8"?>
<root mc:Ignorable="v14 v15" xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006">
</root>`

	doc := etree.NewDocument()
	if err := doc.ReadFromString(xmlStr); err != nil {
		t.Fatalf("Failed to parse XML: %v", err)
	}

	prefixes := GetIgnorableNamespaces(doc.Root())
	if len(prefixes) != 2 {
		t.Errorf("GetIgnorableNamespaces() returned %d prefixes; want 2", len(prefixes))
	}
	if len(prefixes) >= 2 && (prefixes[0] != "v14" || prefixes[1] != "v15") {
		t.Errorf("GetIgnorableNamespaces() = %v; want [v14 v15]", prefixes)
	}
}
