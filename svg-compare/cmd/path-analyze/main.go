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

// SVG represents parsed SVG document
type SVG struct {
	XMLName xml.Name `xml:"svg"`
	Width   string   `xml:"width,attr"`
	Height  string   `xml:"height,attr"`
	ViewBox string   `xml:"viewBox,attr"`
	Groups  []Group  `xml:"g"`
	Paths   []Path   `xml:"path"`
	Defs    Defs     `xml:"defs"`
}

type Defs struct {
	Markers []Marker `xml:"marker"`
	Filters []Filter `xml:"filter"`
}

type Marker struct {
	ID      string `xml:"id,attr"`
	ViewBox string `xml:"viewBox,attr"`
}

type Filter struct {
	ID string `xml:"id,attr"`
}

type Group struct {
	ID        string  `xml:"id,attr"`
	Transform string  `xml:"transform,attr"`
	Paths     []Path  `xml:"path"`
	Rects     []Rect  `xml:"rect"`
	Ellipses  []Ellipse `xml:"ellipse"`
	Texts     []Text  `xml:"text"`
	Groups    []Group `xml:"g"`
}

type Path struct {
	ID        string `xml:"id,attr"`
	D         string `xml:"d,attr"`
	Fill      string `xml:"fill,attr"`
	Stroke    string `xml:"stroke,attr"`
	Transform string `xml:"transform,attr"`
}

type Rect struct {
	X      string `xml:"x,attr"`
	Y      string `xml:"y,attr"`
	Width  string `xml:"width,attr"`
	Height string `xml:"height,attr"`
}

type Ellipse struct {
	CX string `xml:"cx,attr"`
	CY string `xml:"cy,attr"`
	RX string `xml:"rx,attr"`
	RY string `xml:"ry,attr"`
}

type Text struct {
	X       string `xml:"x,attr"`
	Y       string `xml:"y,attr"`
	Content string `xml:",chardata"`
}

// PathCommand represents a parsed SVG path command
type PathCommand struct {
	Type   string    // M, L, C, A, Z, etc.
	Rel    bool      // lowercase = relative
	Coords []float64 // coordinates
}

// Point represents a 2D point
type Point struct {
	X, Y float64
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: path-analyze <svg1> [svg2]")
		fmt.Println("  Single file: dumps path analysis")
		fmt.Println("  Two files: compares paths numerically")
		os.Exit(1)
	}

	svg1, err := parseSVG(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}

	if len(os.Args) == 2 {
		analyzeSVG(os.Args[1], svg1)
		return
	}

	svg2, err := parseSVG(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", os.Args[2], err)
		os.Exit(1)
	}

	compareSVGs(os.Args[1], svg1, os.Args[2], svg2)
}

func parseSVG(path string) (*SVG, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var svg SVG
	if err := xml.Unmarshal(data, &svg); err != nil {
		return nil, err
	}
	return &svg, nil
}

func analyzeSVG(name string, svg *SVG) {
	fmt.Printf("=== SVG Analysis: %s ===\n", name)
	fmt.Printf("ViewBox: %s\n", svg.ViewBox)
	fmt.Printf("Width: %s, Height: %s\n", svg.Width, svg.Height)

	paths := collectPaths(svg)
	fmt.Printf("Total paths: %d\n\n", len(paths))

	for i, p := range paths {
		cmds := parsePath(p.D)
		bbox := computeBBox(cmds)

		fmt.Printf("Path %d:\n", i+1)
		fmt.Printf("  Transform: %s\n", p.Transform)
		fmt.Printf("  Fill: %s, Stroke: %s\n", p.Fill, p.Stroke)
		fmt.Printf("  BBox: (%.3f, %.3f) - (%.3f, %.3f)\n", bbox.MinX, bbox.MinY, bbox.MaxX, bbox.MaxY)
		fmt.Printf("  Commands: %d\n", len(cmds))

		// Summarize command types
		cmdTypes := make(map[string]int)
		for _, c := range cmds {
			cmdTypes[strings.ToUpper(c.Type)]++
		}
		fmt.Printf("  Command types: ")
		for t, n := range cmdTypes {
			fmt.Printf("%s:%d ", t, n)
		}
		fmt.Println()

		// Show first few commands
		for j, c := range cmds {
			if j >= 5 {
				fmt.Printf("    ... and %d more commands\n", len(cmds)-5)
				break
			}
			fmt.Printf("    %s %v\n", c.Type, c.Coords)
		}
		fmt.Println()
	}
}

func compareSVGs(name1 string, svg1 *SVG, name2 string, svg2 *SVG) {
	fmt.Printf("=== Comparing SVGs ===\n")
	fmt.Printf("File 1: %s\n", name1)
	fmt.Printf("File 2: %s\n\n", name2)

	paths1 := collectPaths(svg1)
	paths2 := collectPaths(svg2)

	fmt.Printf("Path count: %d vs %d\n\n", len(paths1), len(paths2))

	// Extract all path data and compute stats
	allCmds1 := make([][]PathCommand, len(paths1))
	allCmds2 := make([][]PathCommand, len(paths2))

	for i, p := range paths1 {
		allCmds1[i] = parsePath(p.D)
	}
	for i, p := range paths2 {
		allCmds2[i] = parsePath(p.D)
	}

	// Compute bounding boxes for all paths
	bboxes1 := make([]BBox, len(paths1))
	bboxes2 := make([]BBox, len(paths2))

	for i, cmds := range allCmds1 {
		bboxes1[i] = computeBBox(cmds)
	}
	for i, cmds := range allCmds2 {
		bboxes2[i] = computeBBox(cmds)
	}

	// Match paths by bounding box similarity
	fmt.Println("=== Path Matching by BBox ===")
	matched := make(map[int]bool)

	type PathMatch struct {
		idx1, idx2  int
		bboxDist    float64
		cmdDiff     int
		coordDiff   float64
	}
	var matches []PathMatch

	for i, bbox1 := range bboxes1 {
		bestJ := -1
		bestDist := math.MaxFloat64

		for j, bbox2 := range bboxes2 {
			if matched[j] {
				continue
			}
			dist := bboxDistance(bbox1, bbox2)
			if dist < bestDist {
				bestDist = dist
				bestJ = j
			}
		}

		if bestJ >= 0 && bestDist < 50 { // threshold for match
			matched[bestJ] = true
			cmdDiff := abs(len(allCmds1[i]) - len(allCmds2[bestJ]))
			coordDiff := compareCommands(allCmds1[i], allCmds2[bestJ])
			matches = append(matches, PathMatch{i, bestJ, bestDist, cmdDiff, coordDiff})
		}
	}

	// Sort by coordinate difference (largest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].coordDiff > matches[j].coordDiff
	})

	fmt.Printf("\nMatched paths: %d\n", len(matches))
	fmt.Printf("Unmatched in file1: %d\n", len(paths1)-len(matches))
	fmt.Printf("Unmatched in file2: %d\n\n", len(paths2)-len(matches))

	// Show worst matches first
	fmt.Println("=== Largest Geometric Deviations ===")
	for i, m := range matches {
		if i >= 20 {
			break
		}
		fmt.Printf("Match %d: path1[%d] <-> path2[%d]\n", i+1, m.idx1, m.idx2)
		fmt.Printf("  BBox dist: %.3f\n", m.bboxDist)
		fmt.Printf("  Cmd count diff: %d\n", m.cmdDiff)
		fmt.Printf("  Coord diff (sum): %.3f\n", m.coordDiff)

		// Show bbox details
		fmt.Printf("  BBox1: (%.1f,%.1f)-(%.1f,%.1f)\n",
			bboxes1[m.idx1].MinX, bboxes1[m.idx1].MinY,
			bboxes1[m.idx1].MaxX, bboxes1[m.idx1].MaxY)
		fmt.Printf("  BBox2: (%.1f,%.1f)-(%.1f,%.1f)\n",
			bboxes2[m.idx2].MinX, bboxes2[m.idx2].MinY,
			bboxes2[m.idx2].MaxX, bboxes2[m.idx2].MaxY)

		// Show command type comparison
		types1 := cmdTypeSummary(allCmds1[m.idx1])
		types2 := cmdTypeSummary(allCmds2[m.idx2])
		fmt.Printf("  Cmds1: %s\n", types1)
		fmt.Printf("  Cmds2: %s\n", types2)
		fmt.Println()
	}

	// Summary statistics
	var totalCoordDiff float64
	var maxCoordDiff float64
	for _, m := range matches {
		totalCoordDiff += m.coordDiff
		if m.coordDiff > maxCoordDiff {
			maxCoordDiff = m.coordDiff
		}
	}

	fmt.Println("=== Summary ===")
	fmt.Printf("Total coordinate deviation: %.3f\n", totalCoordDiff)
	fmt.Printf("Max single path deviation: %.3f\n", maxCoordDiff)
	fmt.Printf("Avg deviation per path: %.3f\n", totalCoordDiff/float64(len(matches)))
}

func collectPaths(svg *SVG) []Path {
	var paths []Path
	paths = append(paths, svg.Paths...)
	for _, g := range svg.Groups {
		paths = append(paths, collectPathsFromGroup(g)...)
	}
	return paths
}

func collectPathsFromGroup(g Group) []Path {
	var paths []Path
	paths = append(paths, g.Paths...)
	for _, sub := range g.Groups {
		paths = append(paths, collectPathsFromGroup(sub)...)
	}
	return paths
}

type BBox struct {
	MinX, MinY, MaxX, MaxY float64
}

func computeBBox(cmds []PathCommand) BBox {
	if len(cmds) == 0 {
		return BBox{}
	}

	bbox := BBox{
		MinX: math.MaxFloat64, MinY: math.MaxFloat64,
		MaxX: -math.MaxFloat64, MaxY: -math.MaxFloat64,
	}

	var x, y float64
	for _, cmd := range cmds {
		switch strings.ToUpper(cmd.Type) {
		case "M", "L":
			if len(cmd.Coords) >= 2 {
				if cmd.Rel {
					x += cmd.Coords[0]
					y += cmd.Coords[1]
				} else {
					x = cmd.Coords[0]
					y = cmd.Coords[1]
				}
				updateBBox(&bbox, x, y)
			}
		case "C":
			if len(cmd.Coords) >= 6 {
				// Cubic bezier: include all control points and endpoint
				for i := 0; i < len(cmd.Coords); i += 2 {
					px, py := cmd.Coords[i], cmd.Coords[i+1]
					if cmd.Rel {
						px += x
						py += y
					}
					updateBBox(&bbox, px, py)
				}
				if cmd.Rel {
					x += cmd.Coords[4]
					y += cmd.Coords[5]
				} else {
					x = cmd.Coords[4]
					y = cmd.Coords[5]
				}
			}
		case "A":
			if len(cmd.Coords) >= 7 {
				// Arc: just use endpoint for bbox
				if cmd.Rel {
					x += cmd.Coords[5]
					y += cmd.Coords[6]
				} else {
					x = cmd.Coords[5]
					y = cmd.Coords[6]
				}
				updateBBox(&bbox, x, y)
			}
		case "H":
			if len(cmd.Coords) >= 1 {
				if cmd.Rel {
					x += cmd.Coords[0]
				} else {
					x = cmd.Coords[0]
				}
				updateBBox(&bbox, x, y)
			}
		case "V":
			if len(cmd.Coords) >= 1 {
				if cmd.Rel {
					y += cmd.Coords[0]
				} else {
					y = cmd.Coords[0]
				}
				updateBBox(&bbox, x, y)
			}
		}
	}

	return bbox
}

func updateBBox(bbox *BBox, x, y float64) {
	if x < bbox.MinX {
		bbox.MinX = x
	}
	if x > bbox.MaxX {
		bbox.MaxX = x
	}
	if y < bbox.MinY {
		bbox.MinY = y
	}
	if y > bbox.MaxY {
		bbox.MaxY = y
	}
}

func bboxDistance(a, b BBox) float64 {
	dx := (a.MinX+a.MaxX)/2 - (b.MinX+b.MaxX)/2
	dy := (a.MinY+a.MaxY)/2 - (b.MinY+b.MaxY)/2
	dw := (a.MaxX-a.MinX) - (b.MaxX-b.MinX)
	dh := (a.MaxY-a.MinY) - (b.MaxY-b.MinY)
	return math.Sqrt(dx*dx + dy*dy + dw*dw + dh*dh)
}

func compareCommands(cmds1, cmds2 []PathCommand) float64 {
	// Convert both to absolute coordinates and compare
	pts1 := commandsToPoints(cmds1)
	pts2 := commandsToPoints(cmds2)

	var totalDiff float64
	maxLen := len(pts1)
	if len(pts2) > maxLen {
		maxLen = len(pts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 Point
		if i < len(pts1) {
			p1 = pts1[i]
		}
		if i < len(pts2) {
			p2 = pts2[i]
		}
		dx := p1.X - p2.X
		dy := p1.Y - p2.Y
		totalDiff += math.Sqrt(dx*dx + dy*dy)
	}

	return totalDiff
}

func commandsToPoints(cmds []PathCommand) []Point {
	var points []Point
	var x, y float64

	for _, cmd := range cmds {
		switch strings.ToUpper(cmd.Type) {
		case "M", "L":
			if len(cmd.Coords) >= 2 {
				if cmd.Rel {
					x += cmd.Coords[0]
					y += cmd.Coords[1]
				} else {
					x = cmd.Coords[0]
					y = cmd.Coords[1]
				}
				points = append(points, Point{x, y})
			}
		case "C":
			// Include control points
			for i := 0; i+1 < len(cmd.Coords); i += 2 {
				px, py := cmd.Coords[i], cmd.Coords[i+1]
				if cmd.Rel {
					points = append(points, Point{x + px, y + py})
				} else {
					points = append(points, Point{px, py})
				}
			}
			if len(cmd.Coords) >= 6 {
				if cmd.Rel {
					x += cmd.Coords[4]
					y += cmd.Coords[5]
				} else {
					x = cmd.Coords[4]
					y = cmd.Coords[5]
				}
			}
		case "A":
			if len(cmd.Coords) >= 7 {
				if cmd.Rel {
					x += cmd.Coords[5]
					y += cmd.Coords[6]
				} else {
					x = cmd.Coords[5]
					y = cmd.Coords[6]
				}
				points = append(points, Point{x, y})
			}
		case "H":
			if len(cmd.Coords) >= 1 {
				if cmd.Rel {
					x += cmd.Coords[0]
				} else {
					x = cmd.Coords[0]
				}
				points = append(points, Point{x, y})
			}
		case "V":
			if len(cmd.Coords) >= 1 {
				if cmd.Rel {
					y += cmd.Coords[0]
				} else {
					y = cmd.Coords[0]
				}
				points = append(points, Point{x, y})
			}
		}
	}

	return points
}

func cmdTypeSummary(cmds []PathCommand) string {
	counts := make(map[string]int)
	for _, c := range cmds {
		counts[strings.ToUpper(c.Type)]++
	}
	var parts []string
	for t, n := range counts {
		parts = append(parts, fmt.Sprintf("%s:%d", t, n))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

var pathCmdRe = regexp.MustCompile(`([MLHVCSQTAZmlhvcsqtaz])([^MLHVCSQTAZmlhvcsqtaz]*)`)
var numRe = regexp.MustCompile(`[-+]?[0-9]*\.?[0-9]+(?:[eE][-+]?[0-9]+)?`)

func parsePath(d string) []PathCommand {
	var cmds []PathCommand

	matches := pathCmdRe.FindAllStringSubmatch(d, -1)
	for _, m := range matches {
		cmdType := m[1]
		coordStr := m[2]

		coords := parseNumbers(coordStr)

		cmd := PathCommand{
			Type:   cmdType,
			Rel:    cmdType >= "a" && cmdType <= "z",
			Coords: coords,
		}
		cmds = append(cmds, cmd)
	}

	return cmds
}

func parseNumbers(s string) []float64 {
	matches := numRe.FindAllString(s, -1)
	var nums []float64
	for _, m := range matches {
		f, err := strconv.ParseFloat(m, 64)
		if err == nil {
			nums = append(nums, f)
		}
	}
	return nums
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
