# vsdx-go

Go library voor het lezen, bewerken en schrijven van Microsoft Visio (.vsdx) bestanden.
Port van de Python [vsdx](https://github.com/dave-howard/vsdx) library (v0.6.1).

## Go Package Structuur

```
vsdx-go/
├── go.mod
├── vsdx/                       # Alle library code in één package
│   ├── doc.go                  # Package-level documentatie (61 lines)
│   │
│   │── # Core types
│   ├── vsdxfile.go             # VisioFile: Open/Close/Save, page management (1234 lines)
│   ├── page.go                 # Page: shapes, search, connects, dimensions, layers (476 lines)
│   ├── shape.go                # Shape: positie, tekst, stijl, cellen, hiërarchie (1575 lines)
│   ├── cell.go                 # Cell: name/value/formula triple (43 lines)
│   ├── connect.go              # Connect: from/to shape relaties (52 lines)
│   ├── data_property.go        # DataProperty: custom shape properties met master inheritance (123 lines)
│   │
│   │── # Geometry
│   ├── geometry.go             # Geometry, GeometryRow, GeometryCell: shape paden + builders (543 lines)
│   │
│   │── # Features
│   ├── foreign.go              # AddImage, AddShape, GroupShapes, SetForeignData (421 lines)
│   ├── template.go             # RenderTemplate: Jinja2-achtige directives (490 lines)
│   ├── diff.go                 # VisioFileDiff: twee .vsdx bestanden vergelijken (241 lines)
│   ├── svg.go                  # ShapeToSVG: SVG rendering van shapes (1014 lines)
│   ├── media.go                # Media: embedded template shapes voor connectors (67 lines)
│   ├── formula.go              # FormulaEvaluator: volledige formule-evaluatie (600 lines)
│   │
│   │── # Support
│   ├── cellname.go             # CellName constants: 40+ cel definities (83 lines)
│   ├── errors.go               # Sentinel errors: ErrInvalidFileType, FileError (27 lines)
│   ├── types.go                # Result structs: Point, Rect (11 lines)
│   ├── namespace.go            # XML namespace constants (14 lines)
│   ├── util.go                 # writeFile helper (15 lines)
│   │
│   ├── vsdx_test.go            # 172 test cases (5959 lines)
│   ├── foreign_test.go         # 10 test cases (726 lines)
│   └── svg_test.go             # 25 test cases (612 lines)
│
├── cmd/stencil-diag/main.go    # Diagnostic tool voor stencil bestanden
├── tests/                      # Test fixture .vsdx bestanden (15+ files)
└── docs/MS-VSDX.pdf            # Microsoft VSDX format specificatie (468 pagina's)
```

## Architectuur

### Data Flow

**Openen:**
```
.vsdx (ZIP) → map[string][]byte (in-memory) → etree XML parse
  → VisioFile.Pages []*Page       (vanuit visio/pages/)
  → Page.shapes() []*Shape        (vanuit <Shapes> elementen)
  → Shape.Cells, Geometry, etc.   (vanuit child XML elementen)
  → VisioFile.MasterPages []*Page (vanuit visio/masters/)
```

**Opslaan:**
```
Gewijzigde etree XML → serialize naar []byte
  → update map[string][]byte entries
  → schrijf nieuw ZIP-archief naar disk
```

### Shape Property Resolution

Properties worden opgelost via master-inheritance chain:
```
shape.CellValue("PinX")
  → check eigen <Cell N="PinX">
  → zo niet, check MasterShape().CellValue("PinX")
  → zo niet, return ""
```

Dit geldt voor cells, text, data properties, en geometry.

### Key Types

| Type | Bestand | Verantwoordelijkheid |
|------|---------|---------------------|
| `VisioFile` | `vsdxfile.go` | Hoofd-entrypoint: ZIP openen/opslaan, pagina-beheer |
| `Page` | `page.go` | Pagina of master-pagina: shapes, connects, afmetingen, layers |
| `Shape` | `shape.go` | Shape of groep: tekst, positie, stijl, cellen, hiërarchie, protection |
| `ShapeParent` | `shape.go` | Interface voor Shape.Parent (`*Page` of `*Shape`) |
| `Cell` | `cell.go` | Naam/waarde/formule paar uit XML Cell element |
| `DataProperty` | `data_property.go` | Custom properties met master inheritance |
| `Connect` | `connect.go` | Verbinding tussen twee shapes |
| `Geometry` | `geometry.go` | Shape pad-definitie + builders (MoveTo, LineTo, ArcTo, etc.) |
| `Point`, `Rect` | `types.go` | Gestructureerde return waarden |
| `CellName` | `cellname.go` | Type alias + 40+ constants voor cell namen |
| `FileError` | `errors.go` | Error type met path en wrapping |

### Interfaces

- **`ShapeParent`** - Unexported method interface, geïmplementeerd door `*Page` en `*Shape`. Maakt `Shape.Remove()` type-safe.
- **`GeometryCellParent`** - Marker interface voor `*Geometry` en `*GeometryRow`.

### Shape Secties (XML Section types)

De library leest en schrijft de volgende VSDX shape secties:

| Sectie | Lezen | Schrijven | Methods |
|--------|-------|-----------|---------|
| **Character** | ✓ | ✓ | `SetCharBold`, `SetCharItalic`, `SetCharSize`, `SetCharFont`, `SetTextColor` |
| **Paragraph** | ✓ | ✓ | `SetParagraphAlign` (AlignLeft/Center/Right/Justify) |
| **Geometry** | ✓ | ✓ | `AddGeometry`, `AddGeometryRect`, `AddMoveTo/LineTo/RelMoveTo/RelLineTo/ArcTo` |
| **Property** | ✓ | ✓ | `DataProperties`, `AddDataProperty`, `SetValue`, `GetAttribute` |
| **Hyperlink** | ✓ | ✓ | `AddHyperlink(address, description)` |
| **Connection** | ✓ | ✓ | `AddConnectionPoint(x, y)` |
| **Layer** | ✓ | ✓ | `Page.AddLayer(name)`, `Shape.SetLayerMember("0;1")` |
| **Protection** | ✓ | ✓ | `SetLockMove`, `SetLockSize`, `SetLockDelete`, `SetLockRotate`, `SetLockAspect` |
| **User** | ✓ | ✓ | `AddUserCell(name, value)`, `UserCellValue(name)` |
| **ForeignData** | ✓ | ✓ | `AddImage`, `SetForeignData` |
| **Scratch** | ✓ | ✓ | `ScratchCells()`, `AddScratchCell(x, y, a, b, c, d)` |
| **Actions** | ✓ | ✓ | `Actions()`, `AddAction(name, menu, action)` |
| **Field** | ✓ | ✓ | `Fields()`, `AddField(type, value, format)` |
| **Control** | ✓ | ✓ | `Controls()`, `AddControl(name, x, y, tip)` |
| **Tabs** | ✓ | ✓ | `TabStops()`, `AddTabStop(position, alignment)` |

## VSDX Bestandsformaat

Een `.vsdx` bestand is een ZIP-archief met XML-bestanden:

```
[Content_Types].xml              Content type mappings
docProps/app.xml                 Document properties (pagina-telling)
visio/document.xml               Stijlen/stylesheets
visio/pages/pages.xml            Paginadefinities (namen, IDs)
visio/pages/page1.xml            Pagina-inhoud (shapes, connects)
visio/masters/masters.xml        Master shape definities
visio/masters/master1.xml        Individuele master shapes
```

XML namespace: `http://schemas.microsoft.com/office/visio/2012/main`

### Shape XML structuur

```xml
<Shape ID="1" MasterShape="2" Master="3">
  <Cell N="PinX" V="3.5"/>
  <Cell N="Width" V="2.0"/>
  <Text>Hello World</Text>
  <Section N="Property">
    <Row N="Status"><Cell N="Value" V="Active"/></Row>
  </Section>
  <Section N="Geometry1">
    <Row T="MoveTo" IX="1"><Cell N="X" V="0"/><Cell N="Y" V="0"/></Row>
    <Row T="LineTo" IX="2"><Cell N="X" V="1"/><Cell N="Y" V="1"/></Row>
  </Section>
  <Shapes><!-- child shapes --></Shapes>
</Shape>
```

## Commando's

```bash
# Go tests
cd /home/michel/vsdx-go && go test ./vsdx/... -v

# Enkele test
cd /home/michel/vsdx-go && go test ./vsdx/... -run TestName -v
```

## Afhankelijkheden

- `github.com/beevik/etree` v1.4.1 - XML parsing met XPath-achtige navigatie
- Go 1.21+

## Referentie Documentatie

- `docs/MS-VSDX.pdf` - Officiële Microsoft VSDX format specificatie (468 pagina's)
  - §2.2.5.3.3.1 Cell Default Values
  - §2.2.11.2 Formulas - volledige formule grammatica
  - §2.2.5.4 Inheritance - 5 types (wij ondersteunen master-to-shape)
  - §2.4.2 GeometryRowTypes - 15 types (wij: MoveTo, LineTo, RelMoveTo, RelLineTo, ArcTo, EllipticalArcTo, RelEllipticalArcTo, RelCubBezTo, RelQuadBezTo, NURBSTo, PolylineTo, SplineStart, SplineKnot, InfiniteLine)
  - §2.4.4 Cells - complete catalogus van cel definities

## Huidige Status

- 19 Go source bestanden, ~5919 lines code + ~6300 lines tests = ~12219 total
- 207 test cases (alle passing), 88.6% code coverage
- ~65% MS-VSDX spec coverage (15 van 17 secties + volledige formule-evaluatie)
- Alle fasen compleet: lezen, navigatie, bewerken, schrijven, connectors, templating, diff
- Netwerk-diagram features: character/paragraph formatting, fill transparency, line patterns,
  geometry builders, layers, hyperlinks, connection points, protection, user-defined cells
- Idiomatisch Go: cell constants, sentinel errors, typed interfaces, result structs
