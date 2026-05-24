package main

import (
	"flag"
	"fmt"
	"os"

	"wijnberg.net/vsdx-go/vsdx"
)

func main() {
	transforms := flag.Bool("transforms", false, "Dump world transforms")
	connectors := flag.Bool("connectors", false, "Validate connector endpoints")
	zorder := flag.Bool("zorder", false, "Validate z-order")
	arrows := flag.Bool("arrows", false, "Validate arrow setbacks")
	tree := flag.Bool("tree", false, "Inspect render tree")
	all := flag.Bool("all", false, "Run all validations")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: render-audit [options] <file.vsdx>")
		fmt.Println()
		fmt.Println("Options:")
		fmt.Println("  -transforms  Dump world transforms for all shapes")
		fmt.Println("  -connectors  Validate connector endpoints")
		fmt.Println("  -zorder      Validate z-order")
		fmt.Println("  -arrows      Validate arrow setbacks")
		fmt.Println("  -tree        Inspect render tree")
		fmt.Println("  -all         Run all validations")
		os.Exit(1)
	}

	vf, err := vsdx.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer vf.Close()

	if *all {
		*transforms = true
		*connectors = true
		*zorder = true
		*arrows = true
		*tree = true
	}

	for i, page := range vf.Pages {
		fmt.Printf("=== Page %d: %s ===\n\n", i+1, page.Name())

		if *transforms {
			fmt.Println("--- World Transforms ---")
			for _, shape := range page.ChildShapes() {
				fmt.Print(vsdx.DumpTransforms(shape))
				fmt.Println()
			}
		}

		if *connectors {
			fmt.Println("--- Connector Validation ---")
			result := vsdx.ValidateConnectorEndpoints(page)
			fmt.Print(result.String())
			fmt.Println()
		}

		if *zorder {
			fmt.Println("--- Z-Order Validation ---")
			result := vsdx.ValidateZOrder(page)
			fmt.Print(result.String())
			fmt.Println()
		}

		if *arrows {
			fmt.Println("--- Arrow Setback Validation ---")
			result := vsdx.ValidateArrowSetbacks(page)
			fmt.Print(result.String())
			fmt.Println()
		}

		if *tree {
			fmt.Println("--- Render Tree ---")
			for _, shape := range page.ChildShapes() {
				builder := vsdx.NewRenderTreeBuilder(shape)
				tree := builder.Build()
				fmt.Print(vsdx.InspectRenderTree(tree))
				fmt.Println()
			}
		}
	}

	// Also check master pages
	if len(vf.MasterPages) > 0 {
		fmt.Println("=== Master Pages ===")
		for _, mp := range vf.MasterPages {
			fmt.Printf("--- Master: %s ---\n", mp.Name())
			for _, shape := range mp.ChildShapes() {
				if *transforms {
					fmt.Print(vsdx.DumpTransforms(shape))
				}
				if *tree {
					builder := vsdx.NewRenderTreeBuilder(shape)
					tree := builder.Build()
					fmt.Print(vsdx.InspectRenderTree(tree))
				}
				fmt.Println()
			}
		}
	}
}
