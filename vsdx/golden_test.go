package vsdx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// GoldenFixture represents a test fixture for golden file testing.
type GoldenFixture struct {
	Name        string   `json:"name"`
	VSDXPath    string   `json:"vsdx_path"`
	ShapeID     string   `json:"shape_id"`
	Width       float64  `json:"width"`
	Height      float64  `json:"height"`
	Precision   int      `json:"precision"`
	GoldenSVG   string   `json:"golden_svg"`
	Categories  []string `json:"categories"` // geometry, text, markers, transforms, etc.
	Description string   `json:"description"`
}

// GoldenManifest contains all golden fixtures.
type GoldenManifest struct {
	Version   string           `json:"version"`
	Generated string           `json:"generated"`
	Fixtures  []*GoldenFixture `json:"fixtures"`
}

// GoldenTestResult represents the result of comparing a render to golden.
type GoldenTestResult struct {
	Fixture    *GoldenFixture
	Passed     bool
	ActualSVG  string
	GoldenSVG  string
	Diffs      []string
	GoldenHash string
	ActualHash string
}

const goldenDir = "../testdata/golden"

// TestGenerateGoldenFixtures creates golden fixtures from test files.
// Run with: go test -run TestGenerateGoldenFixtures -v
func TestGenerateGoldenFixtures(t *testing.T) {
	if os.Getenv("GENERATE_GOLDEN") != "1" {
		t.Skip("Set GENERATE_GOLDEN=1 to generate golden fixtures")
	}
	fixtures := []*GoldenFixture{
		{
			Name:        "simple_rect",
			VSDXPath:    "../tests/test_master.vsdx",
			ShapeID:     "5",
			Width:       100,
			Height:      100,
			Precision:   2,
			Categories:  []string{"geometry", "text"},
			Description: "Simple rectangle with text",
		},
		{
			Name:        "filled_rect",
			VSDXPath:    "../tests/test_master.vsdx",
			ShapeID:     "6",
			Width:       100,
			Height:      100,
			Precision:   2,
			Categories:  []string{"geometry", "fill", "text"},
			Description: "Filled rectangle with text",
		},
		{
			Name:        "house_group",
			VSDXPath:    "../tests/test3_house.vsdx",
			ShapeID:     "7",
			Width:       100,
			Height:      100,
			Precision:   2,
			Categories:  []string{"geometry", "group", "transform"},
			Description: "House shape with nested geometries",
		},
		{
			Name:        "connector_arrow",
			VSDXPath:    "../tests/test5_master.vsdx",
			ShapeID:     "5",
			Width:       100,
			Height:      100,
			Precision:   2,
			Categories:  []string{"geometry", "markers"},
			Description: "Connector with arrow marker",
		},
	}

	manifest := &GoldenManifest{
		Version:   "1.0",
		Generated: "2024-01-01",
		Fixtures:  fixtures,
	}

	for _, fix := range fixtures {
		svg, err := generateGoldenSVG(fix)
		if err != nil {
			t.Logf("Skipping %s: %v", fix.Name, err)
			continue
		}
		fix.GoldenSVG = fix.Name + ".svg"

		// Write SVG file
		svgPath := filepath.Join(goldenDir, fix.GoldenSVG)
		if err := os.WriteFile(svgPath, svg, 0644); err != nil {
			t.Errorf("Failed to write %s: %v", svgPath, err)
			continue
		}
		t.Logf("Generated: %s", svgPath)
	}

	// Write manifest
	manifestPath := filepath.Join(goldenDir, "manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}
	t.Logf("Generated manifest: %s", manifestPath)
}

func generateGoldenSVG(fix *GoldenFixture) ([]byte, error) {
	vf, err := Open(fix.VSDXPath)
	if err != nil {
		return nil, err
	}
	defer vf.Close()

	// Find shape
	var shape *Shape
	for _, page := range vf.Pages {
		for _, s := range page.ChildShapes() {
			if s.ID == fix.ShapeID {
				shape = s
				break
			}
		}
		if shape != nil {
			break
		}
	}
	for _, page := range vf.MasterPages {
		for _, s := range page.ChildShapes() {
			if s.ID == fix.ShapeID {
				shape = s
				break
			}
		}
		if shape != nil {
			break
		}
	}

	if shape == nil {
		return nil, os.ErrNotExist
	}

	svg, err := EmitRenderTree(shape, fix.Width, fix.Height, fix.Precision)
	if err != nil {
		return nil, err
	}

	return svg, nil
}

// TestGoldenFixtures runs all golden file tests.
func TestGoldenFixtures(t *testing.T) {
	manifestPath := filepath.Join(goldenDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Skip("Golden manifest not found, run GenerateGoldenFixtures first")
		return
	}

	var manifest GoldenManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	for _, fix := range manifest.Fixtures {
		t.Run(fix.Name, func(t *testing.T) {
			result := runGoldenTest(fix)
			if !result.Passed {
				t.Errorf("Golden test failed for %s:", fix.Name)
				for _, diff := range result.Diffs {
					t.Errorf("  %s", diff)
				}
				if result.GoldenHash != result.ActualHash {
					t.Errorf("Hash mismatch: golden=%s actual=%s",
						result.GoldenHash[:16], result.ActualHash[:16])
				}
			}
		})
	}
}

func runGoldenTest(fix *GoldenFixture) *GoldenTestResult {
	result := &GoldenTestResult{
		Fixture: fix,
		Passed:  true,
	}

	// Load golden SVG
	goldenPath := filepath.Join(goldenDir, fix.GoldenSVG)
	goldenData, err := os.ReadFile(goldenPath)
	if err != nil {
		result.Passed = false
		result.Diffs = append(result.Diffs, "Failed to load golden: "+err.Error())
		return result
	}
	result.GoldenSVG = string(goldenData)
	result.GoldenHash = hashSVG(goldenData)

	// Generate actual SVG
	actualData, err := generateGoldenSVG(fix)
	if err != nil {
		result.Passed = false
		result.Diffs = append(result.Diffs, "Failed to generate: "+err.Error())
		return result
	}
	result.ActualSVG = string(actualData)
	result.ActualHash = hashSVG(actualData)

	// Compare
	if !bytes.Equal(normalizedSVG(goldenData), normalizedSVG(actualData)) {
		result.Passed = false
		result.Diffs = compareSVGDiffs(result.GoldenSVG, result.ActualSVG)
	}

	return result
}

func hashSVG(data []byte) string {
	h := sha256.Sum256(normalizedSVG(data))
	return hex.EncodeToString(h[:])
}

func normalizedSVG(data []byte) []byte {
	// Normalize whitespace for comparison
	s := string(data)
	s = strings.TrimSpace(s)
	// Normalize newlines
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// Normalize multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return []byte(s)
}

func compareSVGDiffs(golden, actual string) []string {
	var diffs []string

	goldenPaths := parseSVGPaths(golden)
	actualPaths := parseSVGPaths(actual)

	if len(goldenPaths) != len(actualPaths) {
		diffs = append(diffs, "Path count mismatch: golden=%d actual=%d")
	}

	goldenTexts := parseSVGTexts(golden)
	actualTexts := parseSVGTexts(actual)

	if len(goldenTexts) != len(actualTexts) {
		diffs = append(diffs, "Text count mismatch")
	}

	// Compare path data
	for i := 0; i < len(goldenPaths) && i < len(actualPaths); i++ {
		if goldenPaths[i].D != actualPaths[i].D {
			diffs = append(diffs, "Path["+string(rune('0'+i))+"] d attribute differs")
		}
		if goldenPaths[i].Fill != actualPaths[i].Fill {
			diffs = append(diffs, "Path["+string(rune('0'+i))+"] fill differs")
		}
	}

	return diffs
}

// TestDeterministicOutput verifies that multiple renders produce identical output.
func TestDeterministicOutput(t *testing.T) {
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

	// Render 10 times
	var results []string
	for i := 0; i < 10; i++ {
		svg, err := EmitRenderTree(shape, 100, 100, 2)
		if err != nil {
			t.Fatalf("Render %d failed: %v", i, err)
		}
		results = append(results, string(svg))
	}

	// All should be identical
	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Errorf("Render %d differs from render 0", i)
			t.Logf("Render 0:\n%s", results[0])
			t.Logf("Render %d:\n%s", i, results[i])
		}
	}
}

// TestNormalization verifies SVG normalization for comparison.
func TestNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "<svg>  <path/>  </svg>",
			expected: "<svg> <path/> </svg>",
		},
		{
			input:    "<svg>\n\n<path/>\n\n</svg>\n",
			expected: "<svg>\n\n<path/>\n\n</svg>",
		},
	}

	for _, tt := range tests {
		result := string(normalizedSVG([]byte(tt.input)))
		if result != tt.expected {
			t.Errorf("normalizedSVG(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// TestMapOrdering verifies that map iteration order doesn't affect output.
func TestMapOrdering(t *testing.T) {
	vf, err := Open("../tests/test5_master.vsdx")
	if err != nil {
		t.Skip("test5_master.vsdx not available:", err)
	}
	defer vf.Close()

	for _, page := range vf.Pages {
		shapes := page.ChildShapes()
		for _, shape := range shapes {
			// Build render tree multiple times
			var hashes []string
			for i := 0; i < 5; i++ {
				builder := NewRenderTreeBuilder(shape)
				tree := builder.BuildWithScale(72, 72, 2)
				if tree == nil {
					continue
				}

				// Check marker map ordering
				var markerIDs []string
				for id := range tree.Markers {
					markerIDs = append(markerIDs, id)
				}
				sort.Strings(markerIDs)
				hash := strings.Join(markerIDs, ",")
				hashes = append(hashes, hash)
			}

			// All hashes should be identical
			for i := 1; i < len(hashes); i++ {
				if hashes[i] != hashes[0] {
					t.Errorf("Shape %s: marker ordering differs between builds", shape.ID)
				}
			}
		}
		break
	}
}
