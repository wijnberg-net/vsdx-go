// stencil-diag: diagnostic tool to inspect a .vssx stencil's master shapes.
// Usage: go run ./cmd/stencil-diag/main.go <file.vssx>
package main

import (
	"fmt"
	"os"
	"strings"

	"wijnberg.net/vsdx-go/vsdx"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <file.vssx>\n", os.Args[0])
		os.Exit(1)
	}

	v, err := vsdx.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer v.Close()

	fmt.Printf("File: %s\n", os.Args[1])
	fmt.Printf("Stencil: %v\n", v.IsStencil())
	fmt.Printf("Masters: %d\n\n", len(v.MasterPages))

	// Inspect first 20 masters.
	limit := 20
	if len(v.MasterPages) < limit {
		limit = len(v.MasterPages)
	}

	for i := 0; i < limit; i++ {
		master := v.MasterPages[i]
		shapes := master.ChildShapes()
		fmt.Printf("Master[%d]: %q (%d shapes)\n", i, master.Name(), len(shapes))

		for j, shape := range shapes {
			if j > 2 {
				fmt.Printf("  ... +%d more shapes\n", len(shapes)-3)
				break
			}
			fmt.Printf("  Shape[%d]: type=%q  W=%.3f  H=%.3f\n",
				j, shape.ShapeType, shape.Width(), shape.Height())

			// Check for Foreign data.
			img := shape.ForeignImageData()
			if img != nil {
				fmt.Printf("    Foreign: type=%q compression=%q ext=%q  size=%d bytes\n",
					img.ForeignType, img.Compression, img.Extension, len(img.Data))
			}

			// Check geometry.
			if shape.Geometry != nil && len(shape.Geometry.Rows) > 0 {
				rowTypes := map[string]int{}
				for _, r := range shape.Geometry.Rows {
					rowTypes[r.RowType()]++
				}
				parts := []string{}
				for rt, count := range rowTypes {
					parts = append(parts, fmt.Sprintf("%s:%d", rt, count))
				}
				fmt.Printf("    Geometry: %d rows [%s]\n",
					len(shape.Geometry.Rows), strings.Join(parts, " "))
			}

			// Check for child shapes (group).
			children := shape.ChildShapes()
			if len(children) > 0 {
				fmt.Printf("    Group: %d children\n", len(children))
				for k, child := range children {
					if k > 4 {
						fmt.Printf("      ... +%d more children\n", len(children)-5)
						break
					}
					childImg := child.ForeignImageData()
					fmt.Printf("      Child[%d]: type=%q  W=%.3f  H=%.3f  geomRows=%d",
						k, child.ShapeType, child.Width(), child.Height(),
						func() int {
							if child.Geometry != nil {
								return len(child.Geometry.Rows)
							}
							return 0
						}())
					if childImg != nil {
						fmt.Printf("  foreign=%q/%q", childImg.ForeignType, childImg.Extension)
					}
					fmt.Println()
				}
			}

			// Try converting to SVG.
			result, err := vsdx.MasterToSVG(master, vsdx.WithSize(200, 200))
			if err != nil {
				fmt.Printf("    SVG ERROR: %v\n", err)
			} else {
				svgStr := string(result.SVG)
				fmt.Printf("    SVG OK: %d bytes  brand=%q  paths=%d\n",
					len(svgStr), result.BrandColor, strings.Count(svgStr, "<path"))
			}
		}
		fmt.Println()
	}
}
