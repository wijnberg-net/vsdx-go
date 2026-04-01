# vsdx-go

A Go library for reading, editing, and writing Microsoft Visio (.vsdx) files.

This is a Go port of the Python [vsdx](https://github.com/dave-howard/vsdx) library (v0.6.1).

## Installation

```bash
go get github.com/MichelW6667/vsdx-go/vsdx
```

Requires Go 1.21 or later.

## Quick Start

```go
package main

import (
    "fmt"
    "log"

    "github.com/MichelW6667/vsdx-go/vsdx"
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

```
vsdx-go/
├── go.mod
├── vsdx/                       # All library code in one package
│   ├── doc.go                  # Package-level documentation (61 lines)
│   │
│   │── # Core types
│   ├── vsdxfile.go             # VisioFile: Open/Close/Save, page management (1234 lines)
│   ├── page.go                 # Page: shapes, search, connects, dimensions, layers (476 lines)
│   ├── shape.go                # Shape: position, text, style, cells, hierarchy, protection (1193 lines)
│   ├── cell.go                 # Cell: name/value/formula triple (43 lines)
│   ├── connect.go              # Connect: from/to shape relationships (52 lines)
│   ├── data_property.go        # DataProperty: custom shape properties with master inheritance (123 lines)
│   │
│   │── # Geometry
│   ├── geometry.go             # Geometry, GeometryRow, GeometryCell: shape paths + builders (385 lines)
│   │
│   │── # Features
│   ├── foreign.go              # AddImage, AddShape, GroupShapes, SetForeignData (421 lines)
│   ├── template.go             # RenderTemplate: Jinja2-style directives (490 lines)
│   ├── diff.go                 # VisioFileDiff: compare two .vsdx files (241 lines)
│   ├── svg.go                  # ShapeToSVG: SVG rendering of shapes (893 lines)
│   ├── media.go                # Media: embedded template shapes for connectors (67 lines)
│   ├── formula.go              # CalcValue: formula evaluation (35 lines)
│   │
│   │── # Support
│   ├── cellname.go             # CellName constants: 40+ cell definitions (83 lines)
│   ├── errors.go               # Sentinel errors: ErrInvalidFileType, FileError (27 lines)
│   ├── types.go                # Result structs: Point, Rect (11 lines)
│   ├── namespace.go            # XML namespace constants (14 lines)
│   ├── util.go                 # writeFile helper (15 lines)
│   │
│   ├── vsdx_test.go            # 124 test cases (3825 lines)
│   ├── foreign_test.go         # 10 test cases (727 lines)
│   └── svg_test.go             # 24 test cases (541 lines)
│
├── cmd/stencil-diag/main.go    # Diagnostic tool for stencil files
└── tests/                      # Test fixture .vsdx files (15+ files)
```

### Key data flow

**Opening a file:**

```
.vsdx (ZIP) → map[string][]byte (in-memory) → etree XML parse
  → VisioFile.Pages []*Page  (from visio/pages/pages.xml + page1.xml, page2.xml, ...)
  → Page.shapes() []*Shape   (from <Shapes> elements in page XML)
  → Shape.Cells, Shape.Geometry, Shape.DataProperties (from child XML elements)
  → VisioFile.MasterPages []*Page (from visio/masters/)
```

**Saving a file:**

```
Modified etree XML → serialize to []byte
  → update map[string][]byte entries
  → write new ZIP archive to disk
```

### Shape property resolution

Shapes can inherit from master shapes. Property lookup follows this chain:

```
shape.CellValue("PinX")
  → check shape's own <Cell N="PinX"> element
  → if not found, check MasterShape().CellValue("PinX")
  → if not found, return ""
```

This pattern applies to cells, text, data properties, and geometry.

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

XML parsing uses [github.com/beevik/etree](https://github.com/beevik/etree) for XPath-like navigation, matching the Python library's ElementTree approach.

## Running Tests

```bash
go test ./vsdx/... -v
```

158 test cases across 3 files, 85.9% code coverage. Test fixtures are `.vsdx` files in `tests/`.

## Credits

Go port of the Python [vsdx](https://github.com/dave-howard/vsdx) library by Dave Howard.

## License

BSD License - see [LICENSE](LICENSE) for details.
