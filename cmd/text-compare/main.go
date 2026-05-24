package main

import (
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// SVGText represents a text element from SVG
type SVGText struct {
	X              float64
	Y              float64
	TextAnchor     string
	DominantBaseline string
	FontSize       float64
	FontWeight     string
	FontStyle      string
	Fill           string
	Transform      string
	Content        string
}

// TextDiff represents a difference between two text elements
type TextDiff struct {
	Index     int
	Field     string
	Generated string
	Golden    string
	Delta     float64 // For numeric differences
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: text-compare <generated.svg> <golden.svg>")
		os.Exit(1)
	}

	genTexts := extractTexts(os.Args[1])
	goldTexts := extractTexts(os.Args[2])

	fmt.Printf("Generated texts: %d\n", len(genTexts))
	fmt.Printf("Golden texts: %d\n", len(goldTexts))
	fmt.Println()

	// Compare texts by matching content
	matched := 0
	unmatched := 0

	for i, gen := range genTexts {
		fmt.Printf("=== Text %d: %q ===\n", i, truncate(gen.Content, 30))

		// Find matching golden text by content
		var gold *SVGText
		for j := range goldTexts {
			if normalizeText(goldTexts[j].Content) == normalizeText(gen.Content) {
				gold = &goldTexts[j]
				break
			}
		}

		if gold == nil {
			fmt.Printf("  NO MATCH in golden SVG\n")
			unmatched++
			continue
		}

		matched++
		diffs := compareTexts(gen, *gold)
		if len(diffs) == 0 {
			fmt.Printf("  EXACT MATCH\n")
		} else {
			for _, d := range diffs {
				if d.Delta != 0 {
					fmt.Printf("  %s: gen=%s gold=%s delta=%.4f\n", d.Field, d.Generated, d.Golden, d.Delta)
				} else {
					fmt.Printf("  %s: gen=%s gold=%s\n", d.Field, d.Generated, d.Golden)
				}
			}
		}
	}

	fmt.Printf("\nSummary: %d matched, %d unmatched\n", matched, unmatched)
}

func extractTexts(path string) []SVGText {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
		return nil
	}

	var texts []SVGText

	// Simple regex-based extraction (handles both inline and class-based styles)
	textRe := regexp.MustCompile(`<text[^>]*>([^<]*)</text>`)
	attrRe := regexp.MustCompile(`(\w+(?:-\w+)?)=["']([^"']*)["']`)

	content := string(data)

	// Find all text elements
	matches := textRe.FindAllStringSubmatchIndex(content, -1)
	for _, m := range matches {
		fullMatch := content[m[0]:m[1]]
		textContent := content[m[2]:m[3]]

		t := SVGText{Content: strings.TrimSpace(textContent)}

		// Extract attributes
		attrs := attrRe.FindAllStringSubmatch(fullMatch, -1)
		for _, attr := range attrs {
			name, value := attr[1], attr[2]
			switch name {
			case "x":
				t.X, _ = strconv.ParseFloat(value, 64)
			case "y":
				t.Y, _ = strconv.ParseFloat(value, 64)
			case "text-anchor":
				t.TextAnchor = value
			case "dominant-baseline":
				t.DominantBaseline = value
			case "font-size":
				t.FontSize = parseSize(value)
			case "font-weight":
				t.FontWeight = value
			case "font-style":
				t.FontStyle = value
			case "fill":
				t.Fill = value
			case "transform":
				t.Transform = value
			}
		}

		texts = append(texts, t)
	}

	return texts
}

func parseSize(s string) float64 {
	s = strings.TrimSuffix(s, "px")
	s = strings.TrimSuffix(s, "pt")
	s = strings.TrimSuffix(s, "em")
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func compareTexts(gen, gold SVGText) []TextDiff {
	var diffs []TextDiff

	// X coordinate
	if math.Abs(gen.X-gold.X) > 0.01 {
		diffs = append(diffs, TextDiff{
			Field:     "x",
			Generated: fmt.Sprintf("%.2f", gen.X),
			Golden:    fmt.Sprintf("%.2f", gold.X),
			Delta:     gen.X - gold.X,
		})
	}

	// Y coordinate
	if math.Abs(gen.Y-gold.Y) > 0.01 {
		diffs = append(diffs, TextDiff{
			Field:     "y",
			Generated: fmt.Sprintf("%.2f", gen.Y),
			Golden:    fmt.Sprintf("%.2f", gold.Y),
			Delta:     gen.Y - gold.Y,
		})
	}

	// Font size
	if math.Abs(gen.FontSize-gold.FontSize) > 0.01 {
		diffs = append(diffs, TextDiff{
			Field:     "font-size",
			Generated: fmt.Sprintf("%.2f", gen.FontSize),
			Golden:    fmt.Sprintf("%.2f", gold.FontSize),
			Delta:     gen.FontSize - gold.FontSize,
		})
	}

	// Text anchor
	if gen.TextAnchor != gold.TextAnchor && gold.TextAnchor != "" {
		diffs = append(diffs, TextDiff{
			Field:     "text-anchor",
			Generated: gen.TextAnchor,
			Golden:    gold.TextAnchor,
		})
	}

	// Dominant baseline
	if gen.DominantBaseline != gold.DominantBaseline && gold.DominantBaseline != "" {
		diffs = append(diffs, TextDiff{
			Field:     "dominant-baseline",
			Generated: gen.DominantBaseline,
			Golden:    gold.DominantBaseline,
		})
	}

	// Font weight
	genWeight := normalizeWeight(gen.FontWeight)
	goldWeight := normalizeWeight(gold.FontWeight)
	if genWeight != goldWeight && gold.FontWeight != "" {
		diffs = append(diffs, TextDiff{
			Field:     "font-weight",
			Generated: gen.FontWeight,
			Golden:    gold.FontWeight,
		})
	}

	// Fill color
	genFill := strings.ToLower(gen.Fill)
	goldFill := strings.ToLower(gold.Fill)
	if genFill != goldFill && gold.Fill != "" {
		diffs = append(diffs, TextDiff{
			Field:     "fill",
			Generated: gen.Fill,
			Golden:    gold.Fill,
		})
	}

	return diffs
}

func normalizeText(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

func normalizeWeight(w string) string {
	switch strings.ToLower(w) {
	case "normal", "400", "":
		return "normal"
	case "bold", "700":
		return "bold"
	default:
		return w
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

var _ = xml.Name{} // silence unused import
