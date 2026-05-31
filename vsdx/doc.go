// Package vsdx provides reading, editing, writing, and SVG rendering of Microsoft Visio (.vsdx) files.
//
// A .vsdx file is a ZIP archive containing XML files that define pages, shapes,
// masters, and their relationships. This package handles the ZIP/XML plumbing
// and exposes a high-level API for working with Visio documents.
//
// # Opening and saving
//
//	vis, err := vsdx.Open("diagram.vsdx")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer vis.Close()
//
//	// Make changes...
//	if err := vis.SaveVsdx("output.vsdx"); err != nil {
//	    log.Fatal(err)
//	}
//
// # Working with pages and shapes
//
//	page := vis.GetPage(0)
//	for _, shape := range page.AllShapes() {
//	    fmt.Printf("Shape %s: %s\n", shape.ID, shape.Text())
//	}
//
//	shape := page.FindShapeByText("Server")
//	shape.SetText("Database")
//	shape.SetX(5.0)
//	shape.SetFillColor("#00ff00")
//
// # SVG Rendering
//
// Render pages or shapes to SVG with high fidelity:
//
//	svg := page.ToSVG(&vsdx.SVGOptions{
//	    Precision: 2,
//	    Scale:     96.0,  // DPI
//	})
//
// Rendering supports geometry (rectangles, ellipses, arcs, NURBS curves converted to Bezier),
// connectors with proper arrow sizing, 24 line patterns, gradients, text positioning,
// and hierarchical transforms for nested shape groups.
//
// # Connectors
//
//	conn, err := vis.ConnectShapes(page, shapeA, shapeB)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Created connector:", conn.ID)
//
// # Templating
//
//	context := map[string]any{
//	    "title":    "Network Diagram",
//	    "servers":  []any{"web-01", "web-02", "db-01"},
//	    "show_dmz": true,
//	}
//	vis.RenderTemplate(context)
//
// # Error handling
//
// Open returns typed errors that can be inspected with errors.Is and errors.As:
//
//	vis, err := vsdx.Open("bad.txt")
//	if errors.Is(err, vsdx.ErrInvalidFileType) {
//	    // wrong file extension
//	}
//	var fe *vsdx.FileError
//	if errors.As(err, &fe) {
//	    fmt.Println("path:", fe.Path)
//	}
package vsdx
