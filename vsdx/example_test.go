package vsdx_test

import (
	"fmt"
	"log"

	"github.com/wijnberg-net/vsdx-go/vsdx"
)

// Open a .vsdx file and print every shape on the first page.
func ExampleOpen() {
	vis, err := vsdx.Open("diagram.vsdx")
	if err != nil {
		log.Fatal(err)
	}
	defer vis.Close()

	for _, shape := range vis.GetPage(0).AllShapes() {
		fmt.Printf("%s: %q\n", shape.ID, shape.Text())
	}
}

// Find a shape by its text, restyle it, and write the result to a new file.
func ExampleVisioFile_SaveVsdx() {
	vis, err := vsdx.Open("diagram.vsdx")
	if err != nil {
		log.Fatal(err)
	}
	defer vis.Close()

	if s := vis.GetPage(0).FindShapeByText("Draft"); s != nil {
		s.SetText("Approved")
		s.SetFillColor("#33cc66")
	}

	if err := vis.SaveVsdx("approved.vsdx"); err != nil {
		log.Fatal(err)
	}
}

// Render an individual shape to SVG. ShapeToSVG returns a structured
// result whose SVG field holds the markup.
func ExampleShapeToSVG() {
	vis, err := vsdx.Open("diagram.vsdx")
	if err != nil {
		log.Fatal(err)
	}
	defer vis.Close()

	shapes := vis.GetPage(0).AllShapes()
	if len(shapes) == 0 {
		return
	}

	res, err := vsdx.ShapeToSVG(shapes[0], vsdx.WithSize(200, 150), vsdx.WithPrecision(2))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("rendered %d bytes of SVG\n", len(res.SVG))
}
