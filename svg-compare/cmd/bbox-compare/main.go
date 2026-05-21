package main

import (
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SVG element types
type SVG struct {
	XMLName xml.Name `xml:"svg"`
	ViewBox string   `xml:"viewBox,attr"`
	Groups  []Group  `xml:"g"`
}

type Group struct {
	ID        string    `xml:"id,attr"`
	Transform string    `xml:"transform,attr"`
	Paths     []Path    `xml:"path"`
	Rects     []Rect    `xml:"rect"`
	Ellipses  []Ellipse `xml:"ellipse"`
	Texts     []Text    `xml:"text"`
	Groups    []Group   `xml:"g"`
}

type Rect struct {
	X         string `xml:"x,attr"`
	Y         string `xml:"y,attr"`
	Width     string `xml:"width,attr"`
	Height    string `xml:"height,attr"`
	Transform string `xml:"transform,attr"`
}

type Ellipse struct {
	CX        string `xml:"cx,attr"`
	CY        string `xml:"cy,attr"`
	RX        string `xml:"rx,attr"`
	RY        string `xml:"ry,attr"`
	Transform string `xml:"transform,attr"`
}

type Path struct {
	D         string `xml:"d,attr"`
	Transform string `xml:"transform,attr"`
}

type Text struct {
	X       string `xml:"x,attr"`
	Y       string `xml:"y,attr"`
	Content string `xml:",chardata"`
}

// Transform matrix
type Matrix struct {
	A, B, C, D, E, F float64
}

func Identity() Matrix {
	return Matrix{1, 0, 0, 1, 0, 0}
}

func (m Matrix) Multiply(n Matrix) Matrix {
	return Matrix{
		A: m.A*n.A + m.C*n.B,
		B: m.B*n.A + m.D*n.B,
		C: m.A*n.C + m.C*n.D,
		D: m.B*n.C + m.D*n.D,
		E: m.A*n.E + m.C*n.F + m.E,
		F: m.B*n.E + m.D*n.F + m.F,
	}
}

func (m Matrix) Apply(x, y float64) (float64, float64) {
	return m.A*x + m.C*y + m.E, m.B*x + m.D*y + m.F
}

func ParseTransform(s string) Matrix {
	m := Identity()
	if s == "" {
		return m
	}

	// Parse translate(x,y)
	translateRe := regexp.MustCompile(`translate\(([^,)]+),?([^)]*)\)`)
	if match := translateRe.FindStringSubmatch(s); match != nil {
		tx := parseNum(match[1])
		ty := 0.0
		if match[2] != "" {
			ty = parseNum(match[2])
		}
		m = m.Multiply(Matrix{1, 0, 0, 1, tx, ty})
	}

	// Parse rotate(angle) or rotate(angle, cx, cy)
	rotateRe := regexp.MustCompile(`rotate\(([^,)]+),?([^,)]*),?([^)]*)\)`)
	if match := rotateRe.FindStringSubmatch(s); match != nil {
		angle := parseNum(match[1]) * math.Pi / 180
		cos := math.Cos(angle)
		sin := math.Sin(angle)
		if match[2] != "" && match[3] != "" {
			cx := parseNum(match[2])
			cy := parseNum(match[3])
			// Rotate around point (cx, cy)
			m = m.Multiply(Matrix{1, 0, 0, 1, cx, cy})
			m = m.Multiply(Matrix{cos, sin, -sin, cos, 0, 0})
			m = m.Multiply(Matrix{1, 0, 0, 1, -cx, -cy})
		} else {
			m = m.Multiply(Matrix{cos, sin, -sin, cos, 0, 0})
		}
	}

	return m
}

func parseNum(s string) float64 {
	s = strings.TrimSpace(s)
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// Shape with world-space bounding box
type ShapeBBox struct {
	ID      string
	ShapeID string // numeric VSDX shape ID (extracted from ID)
	MinX    float64
	MinY    float64
	MaxX    float64
	MaxY    float64
	CX      float64 // center
	CY      float64
	Paths   int
}

var shapeIDRe = regexp.MustCompile(`^(?:shape|group)(\d+)`)

func extractShapeID(id string) string {
	if m := shapeIDRe.FindStringSubmatch(id); m != nil {
		return m[1]
	}
	return ""
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: bbox-compare <svg1> <svg2>")
		fmt.Println("Compares bounding boxes of shapes in world coordinates")
		os.Exit(1)
	}

	svg1 := parseSVGFile(os.Args[1])
	svg2 := parseSVGFile(os.Args[2])

	boxes1 := extractBBoxes(svg1)
	boxes2 := extractBBoxes(svg2)

	fmt.Printf("SVG1 (%s): %d shapes\n", os.Args[1], len(boxes1))
	fmt.Printf("SVG2 (%s): %d shapes\n", os.Args[2], len(boxes2))
	fmt.Println()

	// Match shapes by VSDX shape ID
	type Match struct {
		b1, b2     ShapeBBox
		centerDist float64
		sizeDiff   float64
	}
	var matches []Match

	// Build map of SVG2 shapes by VSDX shape ID
	svg2ByID := make(map[string][]ShapeBBox)
	for _, b2 := range boxes2 {
		if b2.ShapeID != "" {
			svg2ByID[b2.ShapeID] = append(svg2ByID[b2.ShapeID], b2)
		}
	}

	matchedCount := 0
	for _, b1 := range boxes1 {
		if b1.ShapeID == "" {
			continue
		}
		candidates := svg2ByID[b1.ShapeID]
		if len(candidates) == 0 {
			continue
		}
		// Find best match among candidates by center distance
		bestIdx := 0
		bestDist := math.MaxFloat64
		for i, b2 := range candidates {
			dx := b1.CX - b2.CX
			dy := b1.CY - b2.CY
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}
		b2 := candidates[bestIdx]
		w1 := b1.MaxX - b1.MinX
		h1 := b1.MaxY - b1.MinY
		w2 := b2.MaxX - b2.MinX
		h2 := b2.MaxY - b2.MinY
		sizeDiff := math.Abs(w1-w2) + math.Abs(h1-h2)
		matches = append(matches, Match{b1, b2, bestDist, sizeDiff})
		matchedCount++
	}

	// Sort by center distance (worst first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].centerDist > matches[j].centerDist
	})

	fmt.Printf("Matched by shape ID: %d shapes\n", matchedCount)
	fmt.Printf("Shapes in SVG1: %d\n", len(boxes1))
	fmt.Printf("Shapes in SVG2: %d\n", len(boxes2))

	// Report unmatched shapes from SVG1
	var unmatched1 []string
	for _, b1 := range boxes1 {
		if b1.ShapeID == "" || len(svg2ByID[b1.ShapeID]) == 0 {
			unmatched1 = append(unmatched1, b1.ID)
		}
	}
	if len(unmatched1) > 0 {
		fmt.Printf("Unmatched in SVG1: %v\n", unmatched1)
	}
	fmt.Println()

	// Show worst matches
	fmt.Println("=== Largest Position Deviations ===")
	for i, m := range matches {
		if i >= 15 || m.centerDist < 1.0 {
			break
		}
		fmt.Printf("%d. %s <-> %s\n", i+1, m.b1.ID, m.b2.ID)
		fmt.Printf("   Center1: (%.1f, %.1f), Center2: (%.1f, %.1f)\n",
			m.b1.CX, m.b1.CY, m.b2.CX, m.b2.CY)
		fmt.Printf("   Distance: %.1f pts, Size diff: %.1f pts\n", m.centerDist, m.sizeDiff)
	}

	// Summary stats
	var totalCenterDist, totalSizeDiff float64
	var maxCenterDist, maxSizeDiff float64
	for _, m := range matches {
		totalCenterDist += m.centerDist
		totalSizeDiff += m.sizeDiff
		if m.centerDist > maxCenterDist {
			maxCenterDist = m.centerDist
		}
		if m.sizeDiff > maxSizeDiff {
			maxSizeDiff = m.sizeDiff
		}
	}

	fmt.Println("\n=== Summary ===")
	fmt.Printf("Avg center distance: %.2f pts\n", totalCenterDist/float64(len(matches)))
	fmt.Printf("Max center distance: %.2f pts\n", maxCenterDist)
	fmt.Printf("Avg size diff: %.2f pts\n", totalSizeDiff/float64(len(matches)))
	fmt.Printf("Max size diff: %.2f pts\n", maxSizeDiff)
}

func parseSVGFile(path string) *SVG {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
		os.Exit(1)
	}
	var svg SVG
	if err := xml.Unmarshal(data, &svg); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", path, err)
		os.Exit(1)
	}
	return &svg
}

func extractBBoxes(svg *SVG) []ShapeBBox {
	var boxes []ShapeBBox
	for _, g := range svg.Groups {
		boxes = append(boxes, extractGroupBBoxes(g, Identity())...)
	}
	return boxes
}

func extractGroupBBoxes(g Group, parentTx Matrix) []ShapeBBox {
	var boxes []ShapeBBox

	tx := parentTx.Multiply(ParseTransform(g.Transform))

	// If this group has geometric elements, compute its bbox
	if len(g.Paths) > 0 || len(g.Rects) > 0 || len(g.Ellipses) > 0 {
		bbox := computeGroupBBox(g, tx)
		if bbox.MaxX > bbox.MinX && bbox.MaxY > bbox.MinY {
			boxes = append(boxes, bbox)
		}
	}

	// Recurse into child groups
	for _, child := range g.Groups {
		boxes = append(boxes, extractGroupBBoxes(child, tx)...)
	}

	return boxes
}

func computeGroupBBox(g Group, tx Matrix) ShapeBBox {
	bbox := ShapeBBox{
		ID:      g.ID,
		ShapeID: extractShapeID(g.ID),
		MinX:    math.MaxFloat64, MinY: math.MaxFloat64,
		MaxX:    -math.MaxFloat64, MaxY: -math.MaxFloat64,
	}

	updateBBox := func(wx, wy float64) {
		if wx < bbox.MinX {
			bbox.MinX = wx
		}
		if wx > bbox.MaxX {
			bbox.MaxX = wx
		}
		if wy < bbox.MinY {
			bbox.MinY = wy
		}
		if wy > bbox.MaxY {
			bbox.MaxY = wy
		}
	}

	for _, p := range g.Paths {
		pathTx := tx.Multiply(ParseTransform(p.Transform))
		points := extractPathPoints(p.D)
		for _, pt := range points {
			wx, wy := pathTx.Apply(pt[0], pt[1])
			updateBBox(wx, wy)
		}
		bbox.Paths++
	}

	for _, r := range g.Rects {
		rectTx := tx.Multiply(ParseTransform(r.Transform))
		x := parseNum(r.X)
		y := parseNum(r.Y)
		w := parseNum(r.Width)
		h := parseNum(r.Height)
		// Transform all four corners
		for _, pt := range [][2]float64{{x, y}, {x + w, y}, {x, y + h}, {x + w, y + h}} {
			wx, wy := rectTx.Apply(pt[0], pt[1])
			updateBBox(wx, wy)
		}
		bbox.Paths++
	}

	for _, e := range g.Ellipses {
		ellTx := tx.Multiply(ParseTransform(e.Transform))
		cx := parseNum(e.CX)
		cy := parseNum(e.CY)
		rx := parseNum(e.RX)
		ry := parseNum(e.RY)
		// Use bounding box of ellipse
		for _, pt := range [][2]float64{
			{cx - rx, cy - ry}, {cx + rx, cy - ry},
			{cx - rx, cy + ry}, {cx + rx, cy + ry},
		} {
			wx, wy := ellTx.Apply(pt[0], pt[1])
			updateBBox(wx, wy)
		}
		bbox.Paths++
	}

	bbox.CX = (bbox.MinX + bbox.MaxX) / 2
	bbox.CY = (bbox.MinY + bbox.MaxY) / 2
	return bbox
}

var pathCmdRe = regexp.MustCompile(`([MLHVCSQTAZmlhvcsqtaz])([^MLHVCSQTAZmlhvcsqtaz]*)`)
var numRe = regexp.MustCompile(`[-+]?[0-9]*\.?[0-9]+(?:[eE][-+]?[0-9]+)?`)

func extractPathPoints(d string) [][2]float64 {
	var points [][2]float64
	var x, y float64

	matches := pathCmdRe.FindAllStringSubmatch(d, -1)
	for _, m := range matches {
		cmdType := m[1]
		coordStr := m[2]
		coords := parseNumbers(coordStr)
		rel := cmdType >= "a" && cmdType <= "z"

		switch strings.ToUpper(cmdType) {
		case "M", "L":
			for i := 0; i+1 < len(coords); i += 2 {
				if rel {
					x += coords[i]
					y += coords[i+1]
				} else {
					x = coords[i]
					y = coords[i+1]
				}
				points = append(points, [2]float64{x, y})
			}
		case "C":
			for i := 0; i+5 < len(coords); i += 6 {
				// Include all control points
				for j := 0; j < 3; j++ {
					px, py := coords[i+j*2], coords[i+j*2+1]
					if rel {
						points = append(points, [2]float64{x + px, y + py})
					} else {
						points = append(points, [2]float64{px, py})
					}
				}
				if rel {
					x += coords[i+4]
					y += coords[i+5]
				} else {
					x = coords[i+4]
					y = coords[i+5]
				}
			}
		case "A":
			for i := 0; i+6 < len(coords); i += 7 {
				if rel {
					x += coords[i+5]
					y += coords[i+6]
				} else {
					x = coords[i+5]
					y = coords[i+6]
				}
				points = append(points, [2]float64{x, y})
			}
		case "H":
			for _, v := range coords {
				if rel {
					x += v
				} else {
					x = v
				}
				points = append(points, [2]float64{x, y})
			}
		case "V":
			for _, v := range coords {
				if rel {
					y += v
				} else {
					y = v
				}
				points = append(points, [2]float64{x, y})
			}
		}
	}

	return points
}

func parseNumbers(s string) []float64 {
	matches := numRe.FindAllString(s, -1)
	var nums []float64
	for _, m := range matches {
		f, _ := strconv.ParseFloat(m, 64)
		nums = append(nums, f)
	}
	return nums
}
