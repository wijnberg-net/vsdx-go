# vsdx-go

[![Go Reference](https://pkg.go.dev/badge/github.com/wijnberg-net/vsdx-go/vsdx.svg)](https://pkg.go.dev/github.com/wijnberg-net/vsdx-go/vsdx)
[![Go Report Card](https://goreportcard.com/badge/github.com/wijnberg-net/vsdx-go)](https://goreportcard.com/report/github.com/wijnberg-net/vsdx-go)
[![CI](https://github.com/wijnberg-net/vsdx-go/actions/workflows/ci.yml/badge.svg)](https://github.com/wijnberg-net/vsdx-go/actions/workflows/ci.yml)
[![License: BSD-3-Clause](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)

A Go library for reading, editing, writing, and rendering Microsoft Visio (`.vsdx`) files — no Visio or Office installation required.

## Features

- **Read & write** `.vsdx` / `.vsdm` files: pages, shapes, text, connectors, masters.
- **Edit** position, size, rotation, text, character/paragraph formatting, line and fill styles, hyperlinks, connection points, layers, user-defined cells, and protection locks.
- **Shape model** with master-inheritance resolution for cells, text, geometry, and data properties.
- **Per-shape SVG rendering**: geometry (incl. NURBS → Bézier), arrows, line patterns, gradients, drop shadows, soft edges, transforms (rotation/flip), and nested groups.
- **Authoring**: create/duplicate masters, read `.vssx` stencils, document themes/variants and stylesheets.
- **Extras**: a 175+ function formula evaluator, A\* connector auto-routing, Jinja-style text templating, schema validation, and file diffing.

## Installation

```bash
go get github.com/wijnberg-net/vsdx-go/vsdx
```

Requires Go 1.21 or later. The only dependency is [github.com/beevik/etree](https://github.com/beevik/etree).

## Quick start

```go
package main

import (
	"fmt"
	"log"

	"github.com/wijnberg-net/vsdx-go/vsdx"
)

func main() {
	vis, err := vsdx.Open("diagram.vsdx")
	if err != nil {
		log.Fatal(err)
	}
	defer vis.Close()

	// Inspect: print every shape on the first page.
	page := vis.GetPage(0)
	for _, shape := range page.AllShapes() {
		fmt.Printf("shape %s: %q\n", shape.ID, shape.Text())
	}

	// Edit: find a shape, change its text, position, and fill.
	if s := page.FindShapeByText("Hello"); s != nil {
		s.SetText("Updated")
		s.SetX(3.0)
		s.SetFillColor("#ff0000")
	}

	// Write the result to a new file.
	if err := vis.SaveVsdx("output.vsdx"); err != nil {
		log.Fatal(err)
	}
}
```

## Rendering to SVG

Rendering is done per shape with `ShapeToSVG`, which returns a `*SVGResult`
whose `SVG` field (a `[]byte`) holds the markup. Options are functional:
`WithSize`, `WithPrecision`, and `WithBrandColor`.

```go
shapes := vis.GetPage(0).AllShapes()

res, err := vsdx.ShapeToSVG(shapes[0],
	vsdx.WithSize(200, 150),
	vsdx.WithPrecision(2),
)
if err != nil {
	log.Fatal(err)
}
fmt.Println(string(res.SVG)) // the SVG markup
```

A whole master can be rendered with `MasterToSVG`. To assemble a full page,
render its shapes and place each result.

## Templating

Shape and page text may contain Jinja-style directives that `RenderTemplate`
fills in from a context map:

```go
vis.RenderTemplate(map[string]any{
	"name":      "Production",
	"count":     42,
	"show_info": true,
	"items":     []any{"Server A", "Server B"},
})
```

| Directive | Description |
|-----------|-------------|
| `{{key}}` | Replace with a context value (supports arithmetic, e.g. `{{x*y}}`) |
| `{% for item in list %}` | Duplicate the shape for each item |
| `{% showif condition %}` | Show or hide a shape or page (`not`, `>`, `<`, `==`) |
| `{% set self.x = expr %}` | Set a shape property from an expression |

## API reference

The complete, always-current API — every type, method, and runnable example —
is on **[pkg.go.dev](https://pkg.go.dev/github.com/wijnberg-net/vsdx-go/vsdx)**.

A few entry points to get oriented:

- **Open/save:** `Open`, `OpenBytes`, `(*VisioFile).Close`, `(*VisioFile).SaveVsdx`
- **Pages:** `(*VisioFile).GetPage`, `GetPageByName`, `GetPageNames`, `AddPage`, `CopyPage`
- **Find shapes:** `(*Page).AllShapes`, `ChildShapes`, `FindShapeByID`, `FindShapeByText`, `FindShapesByText`
- **Read shapes:** `(*Shape).Text`, `X`/`Y`, `Width`/`Height`, `Angle`, `FillColor`, `LineColor`, `CellValue`
- **Edit shapes:** `(*Shape).SetText`, `SetX`/`SetY`, `SetWidth`/`SetHeight`, `SetFillColor`, `SetLineColor`, `Move`, `FindReplace`
- **Geometry:** `(*Shape).AddGeometry`, `AddGeometryRect`, and `(*Geometry).AddMoveTo`/`AddLineTo`/`AddArcTo`
- **Render:** `ShapeToSVG`, `MasterToSVG`
- **Templating:** `(*VisioFile).RenderTemplate`

Runnable examples live in [`examples/`](examples) and as
[Go examples](https://pkg.go.dev/github.com/wijnberg-net/vsdx-go/vsdx#pkg-examples)
on pkg.go.dev.

## How it works

```
.vsdx (ZIP) → in-memory map[string][]byte → etree XML parse
  → VisioFile.Pages   (from visio/pages/…)
  → Page → Shapes      (from <Shapes> elements)
  → Shape → Cells, Geometry, DataProperties (from child elements)
```

Shapes can inherit from masters; a property lookup falls back to the master
shape when a value is not set locally. Saving serialises the modified XML
back into a new ZIP archive.

## VSDX file format

A `.vsdx` file is a ZIP archive of XML parts:

```
[Content_Types].xml        content-type mappings
docProps/app.xml           document properties
visio/document.xml         styles and stylesheets
visio/pages/pages.xml      page definitions
visio/pages/page1.xml      page content (shapes, connects)
visio/masters/…            master shape definitions
```

XML namespace: `http://schemas.microsoft.com/office/visio/2012/main`.

## Running tests

```bash
go test ./vsdx/...
go test ./vsdx/... -cover    # with coverage
```

Test fixtures are `.vsdx` files in `tests/`; golden SVGs for render tests live
in `testdata/golden/`.

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

BSD 3-Clause — see [LICENSE](LICENSE).

## Acknowledgements

vsdx-go was originally derived from the BSD-licensed
[vsdx](https://github.com/dave-howard/vsdx) Python library by Dave Howard, and
has since been substantially rewritten and extended for Go.

## Trademarks

Microsoft and Visio are trademarks of Microsoft Corporation. vsdx-go is an
independent project and is not affiliated with, sponsored by, or endorsed by
Microsoft.
