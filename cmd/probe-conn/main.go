package main

import (
	"fmt"
	"os"

	"wijnberg.net/vsdx-go/vsdx"
)

func main() {
	path := "/home/michel/VisiGo/backend/data/diagrams/diagrams/de1202f2-db9d-4726-bdd5-715447657709.vsdx"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("read:", err)
		os.Exit(1)
	}
	v, err := vsdx.OpenBytes(data)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer v.Close()

	page := v.Pages[0]
	for _, sh := range page.AllShapes() {
		if sh.ID == "31" {
			fmt.Printf("Shape 31: W=%.4f H=%.4f Angle=%.4f\n", sh.Width(), sh.Height(), sh.Angle())
			fmt.Printf("  BeginX=%.4f BeginY=%.4f EndX=%.4f EndY=%.4f\n", sh.BeginX(), sh.BeginY(), sh.EndX(), sh.EndY())
			fmt.Printf("  Geometries: %d\n", len(sh.Geometries))
			for gi, g := range sh.Geometries {
				fmt.Printf("  Geometry[%d]: %d rows\n", gi, len(g.Rows))
				for ix, row := range g.Rows {
					fmt.Printf("    row IX=%s type=%s\n", ix, row.RowType())
					for k, c := range row.Cells {
						fmt.Printf("      %s=%s (F=%s)\n", k, c.Value(), c.Formula())
					}
				}
			}
			result, err := vsdx.ShapeToSVG(sh, vsdx.WithSize(105.05, 12.94), vsdx.WithPrecision(2))
			if err != nil {
				fmt.Println("ShapeToSVG error:", err)
				return
			}
			fmt.Println("\n--- SVG output ---")
			fmt.Println(string(result.SVG))
		}
	}
}
