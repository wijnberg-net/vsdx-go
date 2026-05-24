package vsdx

import (
	"regexp"
	"strconv"
	"strings"
)

// parsedPath represents a path element from SVG.
type parsedPath struct {
	D           string
	Fill        string
	Stroke      string
	StrokeWidth float64
}

// parsedText represents a text element from SVG.
type parsedText struct {
	Content string
	X, Y    float64
}

// parseSVGPaths extracts path elements from SVG markup.
func parseSVGPaths(svg string) []parsedPath {
	var paths []parsedPath

	// Simple regex to find path elements
	pathRe := regexp.MustCompile(`<path[^>]*>`)
	matches := pathRe.FindAllString(svg, -1)

	for _, match := range matches {
		p := parsedPath{}

		// Extract d attribute
		if dMatch := regexp.MustCompile(`d="([^"]+)"`).FindStringSubmatch(match); len(dMatch) > 1 {
			p.D = dMatch[1]
		}

		// Extract fill
		if fillMatch := regexp.MustCompile(`fill="([^"]+)"`).FindStringSubmatch(match); len(fillMatch) > 1 {
			p.Fill = fillMatch[1]
		}

		// Extract stroke
		if strokeMatch := regexp.MustCompile(`stroke="([^"]+)"`).FindStringSubmatch(match); len(strokeMatch) > 1 {
			p.Stroke = strokeMatch[1]
		}

		// Extract stroke-width
		if swMatch := regexp.MustCompile(`stroke-width="([^"]+)"`).FindStringSubmatch(match); len(swMatch) > 1 {
			p.StrokeWidth, _ = strconv.ParseFloat(swMatch[1], 64)
		}

		paths = append(paths, p)
	}

	return paths
}

// parseSVGTexts extracts text elements from SVG markup.
func parseSVGTexts(svg string) []parsedText {
	var texts []parsedText

	// Simple regex to find text elements
	textRe := regexp.MustCompile(`<text[^>]*>([^<]*(?:<[^/][^<]*</[^>]*>)*[^<]*)</text>`)
	matches := textRe.FindAllStringSubmatch(svg, -1)

	for _, match := range matches {
		t := parsedText{}

		// Extract x and y from the opening tag
		if xMatch := regexp.MustCompile(`x="([^"]+)"`).FindStringSubmatch(match[0]); len(xMatch) > 1 {
			t.X, _ = strconv.ParseFloat(xMatch[1], 64)
		}
		if yMatch := regexp.MustCompile(`y="([^"]+)"`).FindStringSubmatch(match[0]); len(yMatch) > 1 {
			t.Y, _ = strconv.ParseFloat(yMatch[1], 64)
		}

		// Get content (may include tspan elements)
		if len(match) > 1 {
			content := match[1]
			// Strip tspan tags but keep content
			content = regexp.MustCompile(`<tspan[^>]*>`).ReplaceAllString(content, "")
			content = strings.ReplaceAll(content, "</tspan>", " ")
			t.Content = strings.TrimSpace(content)
		}

		texts = append(texts, t)
	}

	return texts
}
