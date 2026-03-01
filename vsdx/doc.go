// Package vsdx provides reading, editing, and writing of Microsoft Visio (.vsdx) files.
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
// # Templating
//
//	context := map[string]any{
//	    "title":    "Network Diagram",
//	    "servers":  []any{"web-01", "web-02", "db-01"},
//	    "show_dmz": true,
//	}
//	vis.RenderTemplate(context)
package vsdx
