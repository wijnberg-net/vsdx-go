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
│   ├── vsdxfile.go             # VisioFile: Open/Close/Save, page management (1255 lines)
│   ├── page.go                 # Page: shapes, search, connects, dimensions, layers, backgrounds (565 lines)
│   ├── shape.go                # Shape: positie, tekst, stijl, cellen, hiërarchie, protection (1596 lines)
│   ├── cell.go                 # Cell: name/value/formula triple (43 lines)
│   ├── connect.go              # Connect: from/to shape relaties (52 lines)
│   ├── data_property.go        # DataProperty: custom shape properties met master inheritance (123 lines)
│   │
│   │── # Geometry
│   ├── geometry.go             # Geometry, GeometryRow, GeometryCell: shape paden + builders (543 lines)
│   │
│   │── # SVG Rendering
│   ├── svg.go                  # ShapeToSVG: SVG rendering met arrows, text, line patterns (1522 lines)
│   ├── gradient.go             # Gradient: fill gradients voor shapes (175 lines)
│   ├── shadow.go               # Shadow: drop shadow effecten (116 lines)
│   │
│   │── # Features
│   ├── foreign.go              # AddImage, AddShape, GroupShapes, SetForeignData (425 lines)
│   ├── template.go             # RenderTemplate: Jinja2-achtige directives (490 lines)
│   ├── diff.go                 # VisioFileDiff: twee .vsdx bestanden vergelijken (241 lines)
│   ├── media.go                # Media: embedded template shapes voor connectors (67 lines)
│   ├── formula.go              # FormulaEvaluator: volledige formule-evaluatie (600 lines)
│   ├── routing.go              # Router: A* pathfinding voor auto-routing connectors (414 lines)
│   ├── export.go               # ExportPNG, ExportPDF: raster/vector export via externe tools (284 lines)
│   ├── validate.go             # Validate: schema validation en error recovery (232 lines)
│   │
│   │── # Stencils & Masters
│   ├── master.go               # CreateMaster, DeleteMaster, DuplicateMaster (305 lines)
│   ├── stencil.go              # Stencil: .vssx stencil bestanden (357 lines)
│   ├── theme.go                # Theme: document themes en QuickStyle kleuren (331 lines)
│   │
│   │── # Comments & Data Links
│   ├── comments.go             # Comments: document/shape comments + authors (300 lines)
│   ├── linegradient.go         # LineGradient: stroke gradients + Reviewer/Annotation (180 lines)
│   ├── datalink.go             # DataLink: DataConnections, DataRecordSets (275 lines)
│   │
│   │── # Support
│   ├── cellname.go             # CellName constants: 40+ cel definities (83 lines)
│   ├── errors.go               # Sentinel errors: ErrInvalidFileType, FileError (27 lines)
│   ├── types.go                # Result structs: Point, Rect (27 lines)
│   ├── namespace.go            # XML namespace constants (14 lines)
│   ├── util.go                 # writeFile helper (15 lines)
│   │
│   ├── vsdx_test.go            # 320+ test cases (6400 lines)
│   ├── foreign_test.go         # 10 test cases (726 lines)
│   └── svg_test.go             # 30+ test cases (671 lines)
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
| `Page` | `page.go` | Pagina of master-pagina: shapes, connects, afmetingen, layers, backgrounds |
| `Shape` | `shape.go` | Shape of groep: tekst, positie, stijl, cellen, hiërarchie, protection |
| `ShapeParent` | `shape.go` | Interface voor Shape.Parent (`*Page` of `*Shape`) |
| `Cell` | `cell.go` | Naam/waarde/formule paar uit XML Cell element |
| `DataProperty` | `data_property.go` | Custom properties met master inheritance |
| `Connect` | `connect.go` | Verbinding tussen twee shapes |
| `Geometry` | `geometry.go` | Shape pad-definitie + builders (MoveTo, LineTo, ArcTo, etc.) |
| `Gradient` | `gradient.go` | Fill gradient met stops en angle |
| `Shadow` | `shadow.go` | Drop shadow met offset, blur, kleur |
| `Theme` | `theme.go` | Document theme met kleuren en fonts |
| `Stencil` | `stencil.go` | .vssx stencil bestand met masters |
| `Router` | `routing.go` | A* pathfinding voor connector routing |
| `ValidationResult` | `validate.go` | Schema validation resultaten |
| `Comment`, `Author` | `comments.go` | Document/shape comments met authors |
| `LineGradient` | `linegradient.go` | Stroke gradient met stops |
| `Reviewer`, `Annotation` | `linegradient.go` | Review markup |
| `DataConnection` | `datalink.go` | External data source connection |
| `DataRecordSet` | `datalink.go` | Data records gelinkt aan shapes |
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
| **FillGradient** | ✓ | ✓ | `FillGradient()`, `SetFillGradient(angle, stops)` |
| **LineGradient** | ✓ | ✓ | `LineGradient()`, `SetLineGradient(angle, stops)` |
| **Reviewer** | ✓ | - | `Reviewers()`, `GetReviewer(id)` |
| **Annotation** | ✓ | - | `Annotations()` |

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

- 29 Go source bestanden, ~12,000+ lines code + ~7,800 lines tests = ~19,800 total
- 370 test cases (alle passing), ~90% code coverage
- ~98% MS-VSDX spec coverage (19 secties + 75+ formule functies)
- Alle fasen compleet: lezen, navigatie, bewerken, schrijven, connectors, templating, diff
- **Rendering features**: SVG met line patterns (24 types), arrow markers (45+ types), 
  gradient fills (fill + line), drop shadows, text positioning, ellipse geometry
- **Authoring features**: master shapes aanmaken/verwijderen, stencils (.vssx), themes
- **Advanced features**: auto-routing connectors (A* pathfinding), PNG/PDF export,
  background pages, schema validation, error recovery
- **Data features**: comments/annotations, data links/recordsets, reviewers
- Netwerk-diagram features: character/paragraph formatting, fill transparency, line patterns,
  geometry builders, layers, hyperlinks, connection points, protection, user-defined cells
- Idiomatisch Go: cell constants, sentinel errors, typed interfaces, result structs
