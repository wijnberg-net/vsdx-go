package main

import (
	"fmt"
	"os"
	"strings"

	"wijnberg.net/vsdx-go/vsdx"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: shape-inspect <file.vsdx> [shape-id]")
		os.Exit(1)
	}

	vf, err := vsdx.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer vf.Close()

	targetID := ""
	if len(os.Args) > 2 {
		targetID = os.Args[2]
	}

	for _, page := range vf.Pages {
		fmt.Printf("=== Page: %s ===\n", page.Name())
		fmt.Printf("Dimensions: %.4f x %.4f inches (%.3f x %.3f pts)\n",
			page.Width(), page.Height(), page.Width()*72, page.Height()*72)
		fmt.Println()

		shapes := page.ChildShapes()
		for _, s := range shapes {
			if targetID != "" && s.ID != targetID {
				continue
			}
			dumpShape(s, 0, page.Height()*72)
		}
	}
}

func dumpShape(s *vsdx.Shape, depth int, pageHPt float64) {
	indent := strings.Repeat("  ", depth)

	pinX := s.X()
	pinY := s.Y()
	locPinX := s.LocX()
	locPinY := s.LocY()
	width := s.Width()
	height := s.Height()
	angle := s.Angle()

	// Compute SVG transform like render_page does
	absWidth := width
	if absWidth < 0 {
		absWidth = -absWidth
	}
	if absWidth == 0 {
		absWidth = 0.001
	}
	absHeight := height
	if absHeight < 0 {
		absHeight = -absHeight
	}
	if absHeight == 0 {
		absHeight = 0.001
	}

	scale := 72.0
	tlX := pinX - locPinX
	tlY := pinY - locPinY
	svgX := tlX * scale
	svgY := pageHPt - (tlY+absHeight)*scale

	text := s.Text()
	if len(text) > 40 {
		text = text[:40] + "..."
	}
	text = strings.ReplaceAll(text, "\n", " ")

	fmt.Printf("%sShape ID=%s [%s]\n", indent, s.ID, text)
	fmt.Printf("%s  Visio: PinX=%.4f PinY=%.4f LocPinX=%.4f LocPinY=%.4f\n",
		indent, pinX, pinY, locPinX, locPinY)
	fmt.Printf("%s  Visio: W=%.4f H=%.4f Angle=%.4f rad (%.2f deg)\n",
		indent, width, height, angle, angle*180/3.14159)
	fmt.Printf("%s  TopLeft: (%.4f, %.4f) inches\n", indent, tlX, tlY)
	fmt.Printf("%s  SVG:    translate(%.3f, %.3f)\n", indent, svgX, svgY)

	// Children
	for _, child := range s.ChildShapes() {
		dumpShape(child, depth+1, absHeight*scale)
	}
}
