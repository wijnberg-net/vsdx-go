# vsdx-go

A Go library for reading, editing, and writing Microsoft Visio (.vsdx) files.

## Installation

```bash
go get github.com/michelwijnberg/vsdx-go/vsdx
```

Requires Go 1.21 or later.

## Quick Start

```go
package main

import (
    "fmt"
    "log"

    "github.com/michelwijnberg/vsdx-go/vsdx"
)

func main() {
    vis, err := vsdx.Open("my_file.vsdx")
    if err != nil {
        log.Fatal(err)
    }
    defer vis.Close()

    page := vis.GetPage(0)
    for _, shape := range page.AllShapes() {
        fmt.Printf("Shape ID=%s Text=%q\n", shape.ID, shape.Text())
    }

    shape := page.FindShapeByText("Hello")
    if shape != nil {
        shape.SetText("Updated")
        shape.SetX(3.0)
        shape.SetFillColor("#ff0000")
    }

    if err := vis.SaveVsdx("output.vsdx"); err != nil {
        log.Fatal(err)
    }
}
```

## Codebase Overview

All library code lives in the `vsdx/` package:

- **Core** — `vsdxfile.go` (open/save, pages, document properties), `page.go`, `shape.go`, `cell.go`, `connect.go`, `data_property.go`
- **Geometry** — `geometry.go`, `geometry_resolve.go` (NURBS to Bezier, arc conversion, arrow setbacks)
- **SVG rendering** — `svg.go`, `svg_emit.go`, `render_tree.go`, `transform.go`, `effective_style.go`, `gradient.go`, `fillpattern.go`, `shadow.go`
- **Features** — `foreign.go`, `template.go`, `diff.go`, `formula.go`, `routing.go`, `export.go`, `validate.go`
- **Stencils & masters** — `master.go`, `stencil.go`, `theme.go`, `styles.go`
- **Comments & data** — `comments.go`, `linegradient.go`, `datalink.go`
- **Support** — `cellname.go`, `compat.go`, `errors.go`, `types.go`, `namespace.go`, `media.go`

Test fixtures are `.vsdx` files in `tests/`; golden SVGs for render tests live in `testdata/golden/`.

## API Reference

### Opening and saving

```go
vis, err := vsdx.Open("file.vsdx")       // open from file (also .vsdm)
vis, err := vsdx.OpenBytes(data)          // open from []byte
err := vis.Close()                        // close and free resources (implements io.Closer)
err := vis.SaveVsdx("output.vsdx")        // save to file
```

### Pages

```go
page := vis.GetPage(0)                    // by index
page := vis.GetPageByName("Page-1")       // by name
names := vis.GetPageNames()               // list all page names

// Page management (return *Page, error)
page, err := vis.AddPage("New Page")
page, err := vis.AddPageAt(0, "First")
page, err := vis.CopyPage(src, int(vsdx.PageAfter), "Copy")
vis.RemovePageByIndex(2)
vis.RemovePageByName("Old Page")

// Master pages
vis.MasterPages                           // []*Page
vis.GetMasterPageByID("2")
```

### Shapes - finding

```go
page.ChildShapes()                        // top-level shapes
page.AllShapes()                          // all shapes recursively
page.FindShapeByID("5")
page.FindShapeByText("hello")
page.FindShapesByText("hello")            // all matches
page.FindShapesByRegex(`\d+`)             // regex search
page.FindShapeByPropertyLabel("Status")
page.FindShapesByPropertyLabelValue("Status", "Active")
page.FindShapesWithSameMaster(shape)
page.GetConnectorsBetween("1", "", "2", "")
```

### Shapes - reading

```go
shape.ID                                  // shape ID string
shape.Text()                              // text (with master fallback)
shape.X() / shape.Y()                     // position (PinX/PinY)
shape.Width() / shape.Height()            // size
shape.BeginX() / shape.EndX()             // connector endpoints
shape.Angle()                             // rotation
shape.LineColor() / shape.FillColor()     // style
shape.CellValue("PinX")                   // any cell value (with master fallback)
shape.CellFormula("LocPinX")              // cell formula

// Structured results
shape.Center()                            // Point{X, Y}
shape.BoundsRect()                        // Rect{BeginX, BeginY, EndX, EndY}
shape.CenterXY()                          // (float64, float64)
shape.Bounds()                            // (beginX, beginY, endX, endY)

// Hierarchy
shape.ChildShapes()                       // direct children
shape.AllShapes()                         // recursive
shape.MasterShape()                       // master shape
shape.MasterPage()                        // master page
shape.ParentShape()                       // parent shape (nil if parent is a page)
shape.ConnectedShapes()                   // shapes connected via connectors
shape.DataProperties()                    // map[string]*DataProperty
```

### Shapes - editing

```go
// Position and size
shape.SetX(3.0) / shape.SetY(5.0)
shape.SetWidth(2.0) / shape.SetHeight(1.5)
shape.SetAngle(0.5)
shape.Move(1.0, 2.0)                      // move by delta

// Text
shape.SetText("new text")
shape.FindReplace("old", "new")

// Character formatting
shape.SetCharBold(true)
shape.SetCharItalic(true)
shape.SetCharSize(12)                      // points
shape.SetCharFont("Arial")
shape.SetTextColor("#0000ff")
shape.SetParagraphAlign(vsdx.AlignCenter)  // AlignLeft/Center/Right/Justify

// Line style
shape.SetLineColor("#ff0000")
shape.SetLineWeight(0.02)
shape.SetLinePattern(vsdx.LinePatternDash) // Solid/Dash/Dot/DashDot/DashDotDot
shape.SetLineCap(vsdx.LineCapRound)        // Round/Square/Extended
shape.SetBeginArrow(13)                    // bidirectional arrows
shape.SetEndArrow(13)
shape.SetRounding(0.1)                     // rounded corners (inches)

// Fill style
shape.SetFillColor("#00ff00")
shape.SetFillPattern(1)                    // 0=transparent, 1=solid, 2-24=hatches
shape.SetFillTransparency(0.5)             // 0.0=opaque, 1.0=transparent
shape.SetFillBkgndColor("#ffffff")
shape.SetFillBkgndTransparency(0.8)

// Text block positioning (for connector labels)
shape.SetTxtPinX(1.0) / shape.SetTxtPinY(0.2)
shape.SetTxtWidth(2.0) / shape.SetTxtHeight(0.25)

// Generic cell access
shape.SetCellValue("PinX", "5.0")
shape.SetCellFormula("LocPinX", "Width*0.5")

// Hyperlinks
shape.AddHyperlink("https://example.com", "Click here")

// Connection points
shape.AddConnectionPoint(0.5, 0)           // bottom center
shape.AddConnectionPoint(0.5, 1.0)         // top center

// Protection
shape.SetLockMove(true)
shape.SetLockDelete(true)
shape.SetLockSize(true)

// User-defined cells (metadata without Shape Data pane)
shape.AddUserCell("device_id", "12345")
val := shape.UserCellValue("device_id")

// Tooltip
shape.SetComment("Hover text")

// Layers
idx := page.AddLayer("L3 Links")
shape.SetLayerMember("0")                  // or "0;1" for multiple

// Page auto-size
page.AutoSize(0.5)                         // margin in inches

// Shape removal and connectors
shape.Remove()
conn, err := vis.ConnectShapes(page, shapeA, shapeB)
vis.CopyShape(shape.XML(), destPage)
```

### Geometry builders

```go
// Add rectangular geometry (fills shape bounds)
shape.AddGeometryRect()

// Custom geometry paths
g := shape.AddGeometry()
g.AddMoveTo(0, 0)
g.AddLineTo(2, 0)
g.AddLineTo(2, 1)
g.AddArcTo(0, 1, 0.5)                     // curved segment

// Relative coordinates (0-1 range)
g.AddRelMoveTo(0, 0)
g.AddRelLineTo(1, 0)
```

### Templating

Jinja2-style directives in shape text:

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
| `{{key}}` | Replace with context value (supports arithmetic: `{{x*y}}`) |
| `{% for item in list %}` | Duplicate shape for each item |
| `{% showif condition %}` | Show/hide shape or page (`not`, `>`, `<`, `==`) |
| `{% set self.x = expr %}` | Set shape property from expression |

### Comparing files

```go
diff, err := vsdx.NewVisioFileDiff("v1.vsdx", "v2.vsdx")
diff.CompareMembers()                     // common ZIP members
diff.AddedMembers()                       // only in v2
diff.RemovedMembers()                     // only in v1
diff.Diffs                                // map[string][]string with line-level diffs
```

### Error handling

```go
vis, err := vsdx.Open("bad.txt")
// err is *vsdx.FileError wrapping vsdx.ErrInvalidFileType

var fe *vsdx.FileError
if errors.As(err, &fe) {
    fmt.Println("path:", fe.Path)
}
if errors.Is(err, vsdx.ErrInvalidFileType) {
    fmt.Println("wrong file type")
}
```

Sentinel errors: `ErrInvalidFileType`, `ErrInvalidFormat`, `ErrShapeNotFound`

### Constants

Cell name constants avoid magic strings:

```go
shape.CellValue(vsdx.CellPinX)           // instead of "PinX"
shape.SetCellValue(vsdx.CellWidth, "2.0") // instead of "Width"
```

Position: `CellPinX`, `CellPinY`, `CellLocPinX`, `CellLocPinY`, `CellBeginX`, `CellBeginY`, `CellEndX`, `CellEndY`
Size: `CellWidth`, `CellHeight`, `CellAngle`
Line: `CellLineWeight`, `CellLineColor`, `CellLinePattern`, `CellLineCap`, `CellBeginArrow`, `CellEndArrow`, `CellRounding`
Fill: `CellFillForegnd`, `CellFillBkgnd`, `CellFillPattern`, `CellFillForegndTrans`, `CellFillBkgndTrans`
Text: `CellTxtPinX`, `CellTxtPinY`, `CellTxtLocPinX`, `CellTxtLocPinY`, `CellTxtWidth`, `CellTxtHeight`, `CellTxtAngle`
Protection: `CellLockWidth`, `CellLockHeight`, `CellLockMoveX`, `CellLockMoveY`, `CellLockDelete`, `CellLockRotate`, `CellLockAspect`
Other: `CellLayerMember`, `CellBegTrigger`, `CellEndTrigger`, `CellPageWidth`, `CellPageHeight`

## VSDX File Format

A `.vsdx` file is a ZIP archive containing XML files:

```
[Content_Types].xml           Content type mappings
docProps/app.xml              Document properties
visio/document.xml            Styles and stylesheets
visio/pages/pages.xml         Page definitions (names, IDs)
visio/pages/page1.xml         Page content (shapes, connects)
visio/masters/masters.xml     Master shape definitions
visio/masters/master1.xml     Individual master shapes
```

XML namespace: `http://schemas.microsoft.com/office/visio/2012/main`

XML parsing uses [github.com/beevik/etree](https://github.com/beevik/etree) for XPath-like navigation.

## Running Tests

```bash
go test ./vsdx/... -v
go test ./vsdx/... -cover -count=1   # coverage report
```

An extensive unit, contract, and golden-render test suite covers the package. Test fixtures are `.vsdx` files in `tests/` and golden
SVGs in `testdata/golden/`. ## SVG Rendering

The library can render Visio shapes to SVG with high fidelity:

```go
page := vis.GetPage(0)
svg := page.ToSVG(&vsdx.SVGOptions{
    Precision: 2,
    Scale:     96.0,  // DPI
})
```

Rendering features:
- **Geometry**: rectangles, ellipses, arcs, NURBS curves (converted to Bezier)
- **Connectors**: proper B-spline to Bezier conversion, arrow setbacks in points
- **Arrows**: 45+ marker types with correct sizing (markerUnits="strokeWidth")
- **Line styles**: 24 dash patterns, line caps, rounded corners
- **Fill styles**: solid, gradients (linear + radial), transparency, 8×8 bitmap hatches (patterns 2-9 + 25-26)
- **Effects**: drop shadow (feDropShadow), soft edges (feGaussianBlur)
- **Text**: positioned text blocks with character formatting, hyphen-aware wrap, vertical text, FlipX/Y with text upright
- **Transforms**: rotation, scaling, FlipX/FlipY, hierarchical transform propagation
- **Groups**: nested shape groups with correct coordinate transforms

See `vsdx/UNSUPPORTED_FEATURES.md` for the Model / API / Render support
matrix.

## Writer Canonical Form

`SaveVsdxBytes` applies several canonical-form normalisations on save so
the output matches what Visio's "Save As" would produce: vt: namespace
on extended-properties typed elements, HLinks vector auto-generation,
document color palette + FaceName auto-refresh, per-page RecalcColor
triggers, windows.xml children strip, cp/pp text format markers, and
default-value stripping for cells that Visio's resave would strip.
## License

BSD License - see [LICENSE](LICENSE) for details.

## Acknowledgements

vsdx-go was originally derived from the BSD-licensed [vsdx](https://github.com/dave-howard/vsdx) Python library by Dave Howard, and has since been substantially rewritten and extended for Go.

## Trademarks

Microsoft and Visio are trademarks of Microsoft Corporation. vsdx-go is an independent project and is not affiliated with, sponsored by, or endorsed by Microsoft.
