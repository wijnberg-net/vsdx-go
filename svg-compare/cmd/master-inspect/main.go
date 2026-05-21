package main

import (
	"fmt"
	"os"

	"wijnberg.net/vsdx-go/vsdx"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: master-inspect <file.vsdx>")
		os.Exit(1)
	}

	vf, err := vsdx.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer vf.Close()

	fmt.Println("=== Master Pages ===")
	for _, mp := range vf.MasterPages {
		fmt.Printf("Master Page: %s\n", mp.Name())

		shapes := mp.AllShapes()
		for _, s := range shapes {
			fmt.Printf("  Shape ID=%s\n", s.ID)

			// Arrow settings
			fmt.Printf("    BeginArrow: %s, EndArrow: %s\n",
				s.CellValue("BeginArrow"), s.CellValue("EndArrow"))
			fmt.Printf("    BeginArrowSize: %s, EndArrowSize: %s\n",
				s.CellValue("BeginArrowSize"), s.CellValue("EndArrowSize"))

			// Line style
			fmt.Printf("    LineColor: %s, LineWeight: %s\n",
				s.CellValue("LineColor"), s.CellValue("LineWeight"))

			// QuickStyle
			fmt.Printf("    QuickStyleLineMatrix: %s\n", s.CellValue("QuickStyleLineMatrix"))
			fmt.Printf("    QuickStyleEffectsMatrix: %s\n", s.CellValue("QuickStyleEffectsMatrix"))
		}
	}
}
