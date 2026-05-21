package main

import (
	"fmt"
	"os"

	"wijnberg.net/vsdx-go/vsdx"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: connector-inspect <file.vsdx>")
		fmt.Println("Dumps detailed connector geometry and connection info")
		os.Exit(1)
	}

	vf, err := vsdx.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer vf.Close()

	for _, page := range vf.Pages {
		fmt.Printf("=== Page: %s ===\n", page.Name())
		fmt.Printf("Dimensions: %.3f x %.3f inches\n\n", page.Width(), page.Height())

		shapes := page.AllShapes()
		var connectors, regularShapes []*vsdx.Shape

		for _, s := range shapes {
			if isConnector(s) {
				connectors = append(connectors, s)
			} else {
				regularShapes = append(regularShapes, s)
			}
		}

		fmt.Printf("Shapes: %d, Connectors: %d\n\n", len(regularShapes), len(connectors))

		// Analyze each connector
		for _, c := range connectors {
			fmt.Printf("--- Connector ID=%s ---\n", c.ID)

			// Master info
			if m := c.MasterShape(); m != nil {
				fmt.Printf("Master: ID=%s\n", m.ID)
			}

			// Dimensions and position
			fmt.Printf("Width: %s, Height: %s\n", c.CellValue("Width"), c.CellValue("Height"))
			fmt.Printf("PinX: %s, PinY: %s\n", c.CellValue("PinX"), c.CellValue("PinY"))
			fmt.Printf("LocPinX: %s, LocPinY: %s\n", c.CellValue("LocPinX"), c.CellValue("LocPinY"))
			fmt.Printf("Angle: %s\n", c.CellValue("Angle"))

			// Connection cells - these define where connector attaches
			fmt.Printf("BeginX: %s (formula: %s)\n", c.CellValue("BeginX"), c.CellFormula("BeginX"))
			fmt.Printf("BeginY: %s (formula: %s)\n", c.CellValue("BeginY"), c.CellFormula("BeginY"))
			fmt.Printf("EndX: %s (formula: %s)\n", c.CellValue("EndX"), c.CellFormula("EndX"))
			fmt.Printf("EndY: %s (formula: %s)\n", c.CellValue("EndY"), c.CellFormula("EndY"))

			// Control handles for routing
			dumpControlHandles(c)

			// Arrow info
			fmt.Printf("BeginArrow: %s, EndArrow: %s\n",
				c.CellValue("BeginArrow"), c.CellValue("EndArrow"))
			fmt.Printf("BeginArrowSize: %s, EndArrowSize: %s\n",
				c.CellValue("BeginArrowSize"), c.CellValue("EndArrowSize"))

			// Geometry sections
			geoms := c.Geometries
			fmt.Printf("Geometry sections: %d\n", len(geoms))
			for gi, geom := range geoms {
				fmt.Printf("  Geometry[%d]:\n", gi)

				// Print rows
				for ix, row := range geom.Rows {
					fmt.Printf("    [IX=%s] %s:", ix, row.RowType())

					// Print all cells
					for name, cell := range row.Cells {
						v := cell.Value()
						f := cell.Formula()
						if f != "" {
							fmt.Printf(" %s=%s(F:%s)", name, v, f)
						} else if v != "" {
							fmt.Printf(" %s=%s", name, v)
						}
					}
					fmt.Println()
				}
			}

			// Check for glued connections
			dumpConnects(page, c)

			fmt.Println()
		}

		// Show connection points on regular shapes
		fmt.Println("=== Shape Connection Points ===")
		for _, s := range regularShapes {
			conns := getConnectionPoints(s)
			if len(conns) > 0 {
				name := s.Text()
				if len(name) > 30 {
					name = name[:30] + "..."
				}
				if name == "" {
					name = "(no text)"
				}
				fmt.Printf("Shape ID=%s [%s]:\n", s.ID, name)
				for _, cp := range conns {
					fmt.Printf("  Connection %s: X=%s, Y=%s\n", cp.Name, cp.X, cp.Y)
				}
			}
		}
	}
}

func isConnector(s *vsdx.Shape) bool {
	return s.CellValue("BeginX") != "" || s.CellValue("EndX") != ""
}

func dumpControlHandles(s *vsdx.Shape) {
	elem := s.XML()
	if elem == nil {
		return
	}

	for _, section := range elem.SelectElements("Section") {
		if section.SelectAttrValue("N", "") == "Control" {
			fmt.Println("Control handles:")
			for _, row := range section.SelectElements("Row") {
				name := row.SelectAttrValue("N", "")
				fmt.Printf("  %s:", name)
				for _, cell := range row.SelectElements("Cell") {
					n := cell.SelectAttrValue("N", "")
					v := cell.SelectAttrValue("V", "")
					f := cell.SelectAttrValue("F", "")
					if f != "" {
						fmt.Printf(" %s=%s(F:%s)", n, v, f)
					} else if v != "" {
						fmt.Printf(" %s=%s", n, v)
					}
				}
				fmt.Println()
			}
		}
	}
}

func dumpConnects(page *vsdx.Page, connector *vsdx.Shape) {
	connects := page.Connects()
	var found bool
	for _, c := range connects {
		if c.FromID == connector.ID {
			if !found {
				fmt.Println("Glue connections:")
				found = true
			}
			fmt.Printf("  FromRel=%s -> ToID=%s ToRel=%s\n",
				c.FromRel, c.ToID, c.ToRel)
		}
	}
}

type ConnectionPoint struct {
	Name string
	X    string
	Y    string
}

func getConnectionPoints(s *vsdx.Shape) []ConnectionPoint {
	var points []ConnectionPoint
	elem := s.XML()
	if elem == nil {
		return points
	}

	for _, section := range elem.SelectElements("Section") {
		if section.SelectAttrValue("N", "") == "Connection" {
			for _, row := range section.SelectElements("Row") {
				cp := ConnectionPoint{Name: row.SelectAttrValue("N", "")}
				for _, cell := range row.SelectElements("Cell") {
					switch cell.SelectAttrValue("N", "") {
					case "X":
						cp.X = cell.SelectAttrValue("V", cell.SelectAttrValue("F", ""))
					case "Y":
						cp.Y = cell.SelectAttrValue("V", cell.SelectAttrValue("F", ""))
					}
				}
				points = append(points, cp)
			}
		}
	}
	return points
}
