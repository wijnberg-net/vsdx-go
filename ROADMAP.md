# vsdx-go Roadmap: Professional Diagram Tool

Dit document beschrijft alle uitbreidingen nodig om een professionele diagram-applicatie te bouwen die volledig compatible is met Microsoft Visio.

## Huidige Status

**Versie**: Post-coverage expansion (commit bb96235)

| Metric | Waarde |
|--------|--------|
| Code coverage | 88.6% |
| Test cases | 207 |
| MS-VSDX sections | 15/17 |
| Geometry row types | 14/15 |
| Formula functies | 40+ |

### Wat werkt goed
- Volledige .vsdx read/write cycle
- Shape CRUD (create, read, update, delete)
- Connectors met connection points
- Text content en basis formatting
- Kleuren (solid fills, line colors)
- Layers, hyperlinks, data properties
- SVG export (basis)
- Formula evaluatie

---

## Fase 1: Rendering Completeness (P0)

**Doel**: SVG output die er professioneel uitziet

### 1.1 Line Patterns in SVG

**Status**: We lezen/schrijven LinePattern, maar renderen niet in SVG

**Locatie**: `vsdx/svg.go`

**Implementatie**:
```go
// In renderSubShape of buildPathStyle functie:
func linePatternToSVG(pattern int, weight float64) string {
    switch pattern {
    case 0: // None
        return ""
    case 1: // Solid
        return ""
    case 2: // Dash
        return fmt.Sprintf("stroke-dasharray=\"%.2f %.2f\"", weight*4, weight*2)
    case 3: // Dot
        return fmt.Sprintf("stroke-dasharray=\"%.2f %.2f\"", weight, weight*2)
    case 4: // Dash-Dot
        return fmt.Sprintf("stroke-dasharray=\"%.2f %.2f %.2f %.2f\"", weight*4, weight*2, weight, weight*2)
    case 5: // Dash-Dot-Dot
        return fmt.Sprintf("stroke-dasharray=\"%.2f %.2f %.2f %.2f %.2f %.2f\"", 
            weight*4, weight*2, weight, weight*2, weight, weight*2)
    // Patterns 6-23: various other patterns per MS-VSDX spec
    }
}
```

**Benodigde stappen**:
1. Lees `LinePattern` cell waarde in SVG renderer
2. Map Visio patterns (0-23) naar SVG stroke-dasharray
3. Schaal dash lengths naar line weight
4. Voeg toe aan path style string
5. Tests voor alle patterns

**Geschatte effort**: 2-3 uur

### 1.2 Arrow/Marker Varianten

**Status**: Basis arrows werken, maar Visio heeft 45+ arrow types

**Locatie**: `vsdx/svg.go` (huidige `arrowMarkerSVG` functie)

**Huidige code ondersteunt**:
- BeginArrow / EndArrow (aan/uit)
- BeginArrowSize / EndArrowSize (0-6)

**Ontbreekt**:
- Arrow types (filled, open, stealth, diamond, circle, etc.)
- Arrow positioning (inside/outside line)

**MS-VSDX Arrow Types** (uit spec sectie 2.4.4):
```
0  = None
1  = Triangle (filled)
2  = Stealth
3  = Triangle (open)
4  = Line
5  = Oval (filled)
6  = Diamond (filled)
7  = Diamond (open)
8  = Oval (open)
9  = Double triangle
10 = Triangle (45°)
... (tot 45)
```

**Implementatie**:
```go
type ArrowDef struct {
    Path      string  // SVG path data
    Width     float64 // Relative to line weight
    Height    float64
    RefX      float64 // Attachment point
    RefY      float64
    Filled    bool
}

var arrowTypes = map[int]ArrowDef{
    0:  {}, // None
    1:  {Path: "M 0 0 L 10 5 L 0 10 z", Width: 10, Height: 10, RefX: 10, RefY: 5, Filled: true},
    2:  {Path: "M 0 0 L 10 5 L 0 10 L 3 5 z", ...}, // Stealth
    3:  {Path: "M 0 0 L 10 5 L 0 10", Filled: false}, // Open triangle
    // etc.
}
```

**Benodigde stappen**:
1. Definieer alle 45 arrow types als SVG paths
2. Lees BeginArrow/EndArrow cell waarden (type, niet alleen aan/uit)
3. Genereer `<marker>` definitie per type
4. Handle arrow sizing (6 size levels)
5. Tests met visuele verificatie

**Geschatte effort**: 4-6 uur

### 1.3 Text Block Positioning

**Status**: Text wordt gecentreerd, maar Visio heeft complexe text block controls

**Relevante cells**:
```
TxtPinX, TxtPinY      - Text block anchor point
TxtLocPinX, TxtLocPinY - Text block local pin
TxtWidth, TxtHeight    - Text block dimensions  
TxtAngle              - Text rotation
VerticalAlign         - Top/Middle/Bottom (in shape)
HorzAlign             - Left/Center/Right (per paragraph)
TopMargin, BottomMargin, LeftMargin, RightMargin
```

**Implementatie in SVG**:
```go
func (e *svgExporter) renderText(shape *Shape, transform string) string {
    // 1. Get text block position relative to shape
    txtPinX := shape.CellValueFloat("TxtPinX", shape.Width()/2)
    txtPinY := shape.CellValueFloat("TxtPinY", shape.Height()/2)
    
    // 2. Get text alignment
    vertAlign := shape.CellValue("VerticalAlign") // 0=top, 1=middle, 2=bottom
    
    // 3. Get margins
    leftMargin := shape.CellValueFloat("LeftMargin", 0)
    
    // 4. Calculate SVG text position
    // 5. Apply TxtAngle rotation
    // 6. Handle multi-line with <tspan> elements
}
```

**Benodigde stappen**:
1. Lees alle TxtXxx cells
2. Bereken text positie in SVG coordinaten
3. Implementeer vertical alignment
4. Voeg text rotation toe
5. Handle margins
6. Multi-line text met correcte line spacing

**Geschatte effort**: 4-5 uur

---

## Fase 2: Visual Enhancements (P1)

### 2.1 Gradient Fills

**Status**: Niet geïmplementeerd

**MS-VSDX Sections**:
- `FillGradientStops` - Gradient stop definities
- `LineGradientStops` - Line gradient (minder belangrijk)

**XML Structuur**:
```xml
<Section N="FillGradientStops">
  <Row IX="0">
    <Cell N="Color" V="#FF0000"/>
    <Cell N="Position" V="0"/>
    <Cell N="Trans" V="0"/>
  </Row>
  <Row IX="1">
    <Cell N="Color" V="#0000FF"/>
    <Cell N="Position" V="1"/>
    <Cell N="Trans" V="0"/>
  </Row>
</Section>
```

**Relevante cells**:
```
FillGradientEnabled  - 0/1
FillGradientDir      - Gradient direction (angle or type)
FillGradientAngle    - Angle in radians
```

**SVG Output**:
```xml
<defs>
  <linearGradient id="grad1" x1="0%" y1="0%" x2="100%" y2="0%">
    <stop offset="0%" style="stop-color:#FF0000"/>
    <stop offset="100%" style="stop-color:#0000FF"/>
  </linearGradient>
</defs>
<path fill="url(#grad1)" .../>
```

**Implementatie**:

```go
// Nieuw bestand: vsdx/gradient.go

type GradientStop struct {
    Position float64 // 0.0 - 1.0
    Color    string  // #RRGGBB
    Trans    float64 // Transparency 0.0 - 1.0
}

type Gradient struct {
    Enabled bool
    Type    string  // "linear" or "radial"
    Angle   float64 // In radians
    Stops   []GradientStop
}

func (s *Shape) FillGradient() *Gradient {
    section := s.xml.FindElement("Section[@N='FillGradient']")
    // Parse stops...
}

// In svg.go:
func (e *svgExporter) renderGradientDef(id string, grad *Gradient) string {
    // Generate <linearGradient> or <radialGradient>
}
```

**Benodigde stappen**:
1. Nieuwe `Gradient` en `GradientStop` types
2. Parser voor FillGradientStops section
3. Lees FillGradientEnabled, FillGradientDir, FillGradientAngle
4. SVG gradient definition generator
5. Koppel aan shape fill in SVG renderer
6. Support voor radial gradients
7. Tests

**Geschatte effort**: 6-8 uur

### 2.2 Shadow Effects

**Status**: Niet geïmplementeerd

**Relevante cells**:
```
ShdwType        - 0=none, 1=simple, 2=oblique
ShdwOffsetX     - Shadow X offset
ShdwOffsetY     - Shadow Y offset  
ShdwObliqueAngle - Angle for oblique shadow
ShdwScaleFactor - Scale for oblique
ShdwForegnd     - Shadow color
ShdwForegndTrans - Shadow transparency
ShdwBlur        - Blur radius (newer Visio)
```

**SVG Output**:
```xml
<defs>
  <filter id="shadow1">
    <feDropShadow dx="3" dy="3" stdDeviation="2" flood-color="#000000" flood-opacity="0.3"/>
  </filter>
</defs>
<g filter="url(#shadow1)">
  <path .../>
</g>
```

**Implementatie**:
```go
type Shadow struct {
    Type       int     // 0=none, 1=simple, 2=oblique
    OffsetX    float64
    OffsetY    float64
    Color      string
    Opacity    float64
    Blur       float64
}

func (s *Shape) Shadow() *Shadow {
    if s.CellValue("ShdwType") == "0" {
        return nil
    }
    // Parse shadow cells...
}
```

**Benodigde stappen**:
1. Nieuwe `Shadow` type
2. Parser voor shadow cells
3. SVG filter generator
4. Apply filter to shape group
5. Handle oblique shadows (meer complex)
6. Tests

**Geschatte effort**: 4-5 uur

### 2.3 Master Shapes Aanmaken

**Status**: We kunnen masters gebruiken, maar niet aanmaken

**Huidige flow**:
```go
// Werkt:
master := vis.GetMasterPage("Rectangle")
page.AddShape(master, x, y)

// Ontbreekt:
newMaster := vis.CreateMaster("MyShape")
newMaster.AddGeometry(...)
newMaster.SetFillColor(...)
vis.SaveMaster(newMaster)
```

**Benodigde wijzigingen in `vsdxfile.go`**:

```go
// CreateMaster maakt een nieuwe master shape
func (v *VisioFile) CreateMaster(name string) (*Page, error) {
    // 1. Genereer nieuwe master ID
    maxID := v.getMaxMasterID()
    newID := maxID + 1
    
    // 2. Maak master XML document
    masterDoc := etree.NewDocument()
    masterDoc.CreateElement("MasterContents")
    // Add Shapes container, PageSheet, etc.
    
    // 3. Voeg toe aan masters.xml
    masterElem := v.mastersXML.CreateElement("Master")
    masterElem.CreateAttr("ID", strconv.Itoa(newID))
    masterElem.CreateAttr("Name", name)
    masterElem.CreateAttr("NameU", name)
    masterElem.CreateAttr("UniqueID", generateUUID())
    
    // 4. Voeg master page toe aan files map
    filename := fmt.Sprintf("visio/masters/master%d.xml", newID)
    
    // 5. Update [Content_Types].xml
    
    // 6. Update relationships
    
    return newPage(masterDoc, filename, name, strconv.Itoa(newID), "", v), nil
}

// SaveMaster slaat wijzigingen aan een master op
func (v *VisioFile) SaveMaster(master *Page) error {
    // Serialize master XML
    // Update in files map
}

// DeleteMaster verwijdert een master
func (v *VisioFile) DeleteMaster(name string) error {
    // Remove from masters.xml
    // Remove master file
    // Update relationships
}
```

**Benodigde stappen**:
1. `CreateMaster()` functie
2. UUID generator voor UniqueID
3. Content_Types.xml updater
4. Relationship (.rels) updater
5. `SaveMaster()` functie
6. `DeleteMaster()` functie
7. `DuplicateMaster()` voor kopieën
8. Tests

**Geschatte effort**: 6-8 uur

### 2.4 Stencil (.vssx) Bestanden

**Status**: Niet ondersteund

**Verschil met .vsdx**:
- .vssx bevat alleen masters, geen pages
- Andere Content_Types
- Geen page XML files

**Implementatie**:

```go
// Nieuw bestand: vsdx/stencil.go

type Stencil struct {
    files      map[string][]byte
    mastersXML *etree.Element
    Masters    []*Page
}

// OpenStencil opent een .vssx bestand
func OpenStencil(filename string) (*Stencil, error) {
    // Similar to Open() but stencil-specific
}

// CreateStencil maakt een nieuwe lege stencil
func CreateStencil() *Stencil {
    // Initialize empty stencil structure
}

// AddMaster voegt een master toe aan de stencil
func (s *Stencil) AddMaster(master *Page) error {
    // Copy master to stencil
}

// SaveVssx slaat de stencil op
func (s *Stencil) SaveVssx(filename string) error {
    // Write .vssx file
}

// ImportToDocument importeert stencil masters naar een document
func (s *Stencil) ImportToDocument(vis *VisioFile) error {
    // Copy all masters to document
}
```

**Benodigde stappen**:
1. Nieuwe `Stencil` type
2. `OpenStencil()` parser
3. `CreateStencil()` factory
4. `AddMaster()` functie
5. `SaveVssx()` writer
6. Content_Types voor .vssx
7. Import/export tussen stencil en document
8. Tests

**Geschatte effort**: 8-10 uur

---

## Fase 3: Advanced Features (P2)

### 3.1 Auto-Routing Connectors

**Status**: Connectors zijn rechte lijnen of handmatige paden

**Doel**: Automatisch pad vinden dat andere shapes vermijdt

**Algoritme opties**:
1. A* pathfinding met grid
2. Orthogonal routing (alleen horizontaal/verticaal)
3. Visibility graph

**Implementatie**:

```go
// Nieuw bestand: vsdx/routing.go

type Router struct {
    page      *Page
    obstacles []Rect  // Bounding boxes van shapes
    gridSize  float64
}

type RouteOptions struct {
    Orthogonal bool    // Alleen H/V lijnen
    Padding    float64 // Afstand tot obstacles
    MaxBends   int     // Maximum aantal bochten
}

// Route berekent een pad tussen twee punten
func (r *Router) Route(from, to Point, opts RouteOptions) []Point {
    // 1. Bouw obstacle grid
    // 2. A* of Dijkstra pathfinding
    // 3. Simplify path (remove unnecessary points)
    // 4. Return waypoints
}

// ApplyRoute past een route toe op een connector shape
func (r *Router) ApplyRoute(connector *Shape, points []Point) {
    // Clear existing geometry
    // Add MoveTo for start
    // Add LineTo for each waypoint
}

// AutoRouteConnectors route alle connectors op een pagina
func (p *Page) AutoRouteConnectors() {
    router := NewRouter(p)
    for _, conn := range p.GetConnectors() {
        from := conn.BeginPoint()
        to := conn.EndPoint()
        route := router.Route(from, to, DefaultRouteOptions)
        router.ApplyRoute(conn, route)
    }
}
```

**Benodigde stappen**:
1. Bounding box calculation voor obstacles
2. Grid-based collision detection
3. A* pathfinding implementatie
4. Path simplification
5. Orthogonal routing variant
6. Apply route to connector geometry
7. Incremental re-routing bij shape move
8. Tests met verschillende layouts

**Geschatte effort**: 12-16 uur

### 3.2 PNG/PDF Export

**Status**: Alleen SVG export

**Optie 1: Via externe tools**
```go
// SVG -> PNG via Inkscape of rsvg-convert
func ExportPNG(shape *Shape, filename string) error {
    svg, _ := ShapeToSVG(shape)
    tmpSVG := writeTempFile(svg)
    exec.Command("rsvg-convert", "-o", filename, tmpSVG).Run()
}
```

**Optie 2: Pure Go libraries**
```go
// Dependencies:
// - github.com/srwiley/oksvg (SVG parser)
// - github.com/srwiley/rasterx (rasterizer)
// - github.com/jung-kurt/gofpdf (PDF)

import (
    "github.com/srwiley/oksvg"
    "github.com/srwiley/rasterx"
    "github.com/jung-kurt/gofpdf"
)

func ExportPNG(shape *Shape, filename string, dpi float64) error {
    result, _ := ShapeToSVG(shape)
    icon, _ := oksvg.ReadIconStream(bytes.NewReader(result.SVG))
    
    w := int(result.Width * dpi / 72)
    h := int(result.Height * dpi / 72)
    
    img := image.NewRGBA(image.Rect(0, 0, w, h))
    scanner := rasterx.NewScannerGV(w, h, img, img.Bounds())
    raster := rasterx.NewDasher(w, h, scanner)
    
    icon.Draw(raster, 1.0)
    
    f, _ := os.Create(filename)
    png.Encode(f, img)
}

func ExportPDF(page *Page, filename string) error {
    pdf := gofpdf.New("P", "in", "A4", "")
    pdf.AddPage()
    
    for _, shape := range page.AllShapes() {
        svg, _ := ShapeToSVG(shape)
        // Convert SVG paths to PDF drawing commands
        // This is complex - may need custom implementation
    }
    
    return pdf.OutputFileAndClose(filename)
}
```

**Benodigde stappen**:
1. Kies approach (external tool vs pure Go)
2. Implementeer PNG export
3. Handle DPI/resolution
4. Implementeer PDF export
5. Multi-page PDF support
6. Page margins en sizing
7. Tests

**Geschatte effort**: 8-12 uur (afhankelijk van approach)

### 3.3 Theme Support

**Status**: Themes worden genegeerd

**MS-VSDX Theme Structuur**:
- Themes in `visio/theme/theme1.xml`
- ThemeIndex cell verwijst naar theme
- QuickStyleXxx cells voor varianten

**Relevante cells per shape**:
```
QuickStyleType        - Shape style category
QuickStyleVariation   - Style variant
QuickStyleLineColor   - Override line color
QuickStyleFillColor   - Override fill color
QuickStyleLineMatrix  - Line style matrix index
QuickStyleFillMatrix  - Fill style matrix index
```

**Implementatie**:

```go
// Nieuw bestand: vsdx/theme.go

type Theme struct {
    Name       string
    Colors     ThemeColors
    Fonts      ThemeFonts
    Effects    ThemeEffects
    Connectors ThemeConnectors
}

type ThemeColors struct {
    Dark1      string
    Light1     string
    Dark2      string
    Light2     string
    Accent1    string
    Accent2    string
    // ... etc
}

func (v *VisioFile) Theme() *Theme {
    // Parse theme XML
}

func (s *Shape) ResolveThemeColor(cellName string) string {
    // 1. Check for explicit color value
    // 2. Check QuickStyle references
    // 3. Look up in theme
    // 4. Return resolved color
}
```

**Benodigde stappen**:
1. Theme XML parser
2. ThemeColors, ThemeFonts, ThemeEffects types
3. QuickStyle cell reading
4. Color resolution met theme lookup
5. Apply theme to SVG rendering
6. Tests

**Geschatte effort**: 8-10 uur

### 3.4 Background Pages

**Status**: Basis support, niet volledig

**Hoe het werkt**:
- BackPage cell in PageSheet verwijst naar background page
- Background shapes worden onder foreground shapes gerenderd
- Backgrounds kunnen genest zijn

**Implementatie**:
```go
func (p *Page) BackgroundPage() *Page {
    ps := p.pagesheetXML()
    if ps == nil {
        return nil
    }
    backID := cellValue(ps, "BackPage")
    if backID == "" {
        return nil
    }
    return p.vis.GetPageByID(backID)
}

func (p *Page) AllShapesWithBackground() []*Shape {
    var shapes []*Shape
    
    // Recursively get background shapes first
    if bg := p.BackgroundPage(); bg != nil {
        shapes = append(shapes, bg.AllShapesWithBackground()...)
    }
    
    // Then foreground shapes
    shapes = append(shapes, p.AllShapes()...)
    return shapes
}

func (p *Page) SetBackgroundPage(bg *Page) {
    // Set BackPage cell
}
```

**Benodigde stappen**:
1. `BackgroundPage()` getter
2. `SetBackgroundPage()` setter
3. `AllShapesWithBackground()` voor rendering
4. Handle nested backgrounds
5. Tests

**Geschatte effort**: 3-4 uur

---

## Fase 4: Robustness & Polish

### 4.1 Error Recovery

**Status**: Errors bij malformed files crashen

**Implementatie**:
```go
type ParseOptions struct {
    StrictMode     bool          // Fail on any error
    ErrorHandler   func(error)   // Callback for non-fatal errors
    MaxErrors      int           // Stop after N errors
}

func OpenWithOptions(filename string, opts ParseOptions) (*VisioFile, error) {
    // Try to parse
    // Collect errors
    // Continue if possible
}
```

### 4.2 Schema Validation

**Status**: Geen validatie

**Implementatie**:
```go
func (v *VisioFile) Validate() []ValidationError {
    var errors []ValidationError
    
    // Check required elements exist
    // Validate cell values are in range
    // Check ID references are valid
    // Verify geometry is well-formed
    
    return errors
}
```

### 4.3 Memory Optimization

**Status**: Alle files in memory

**Opties**:
- Lazy loading van page XML
- Streaming voor grote files
- Cache invalidation

### 4.4 Concurrent Access

**Status**: Niet thread-safe

**Implementatie**:
- Mutex per VisioFile
- Read/write locks voor shapes
- Copy-on-write voor concurrent reads

---

## Fase 5: Geometry Completion

### 5.1 Ellipse Row Type

**Status**: Laatste ontbrekende geometry type

**XML**:
```xml
<Row T="Ellipse" IX="1">
  <Cell N="X" V="0.5"/>      <!-- Center X -->
  <Cell N="Y" V="0.5"/>      <!-- Center Y -->
  <Cell N="A" V="0.5"/>      <!-- Width semi-axis (to right) -->
  <Cell N="B" V="0.25"/>     <!-- End X of semi-axis -->
  <Cell N="C" V="0"/>        <!-- End Y of semi-axis -->
  <Cell N="D" V="0.75"/>     <!-- Height semi-axis (to top) -->
</Row>
```

**Implementatie**:
```go
// In geometry.go
func (g *Geometry) AddEllipse(centerX, centerY, a, b, c, d float64) {
    row := g.addRow("Ellipse")
    row.SetCell("X", centerX)
    row.SetCell("Y", centerY)
    row.SetCell("A", a)
    row.SetCell("B", b)
    row.SetCell("C", c)
    row.SetCell("D", d)
}

// In svg.go
case "ellipse":
    cx, cy := row.X(), row.Y()
    // Calculate radii from A,B,C,D
    // SVG: <ellipse cx="..." cy="..." rx="..." ry="..."/>
```

**Geschatte effort**: 2 uur

---

## Implementatie Volgorde

### Sprint 1: Core Rendering (Week 1-2)
1. ✅ Line patterns in SVG
2. ✅ Arrow varianten
3. ✅ Text block positioning
4. ✅ Ellipse geometry

**Deliverable**: Visueel correcte SVG export

### Sprint 2: Visual Polish (Week 3-4)
1. ✅ Gradient fills
2. ✅ Shadow effects
3. ✅ Background pages

**Deliverable**: Professioneel uitziende diagrams

### Sprint 3: Authoring (Week 5-6)
1. ✅ Master shapes aanmaken
2. ✅ Stencil (.vssx) support
3. ✅ Theme support

**Deliverable**: Eigen shapes en stencils maken

### Sprint 4: Advanced (Week 7-8)
1. ✅ Auto-routing connectors
2. ✅ PNG export
3. ✅ PDF export

**Deliverable**: Complete export opties

### Sprint 5: Polish (Week 9-10)
1. ✅ Error recovery
2. ✅ Schema validation
3. ✅ Performance optimization
4. ✅ Documentation

**Deliverable**: Production-ready library

---

## Test Strategy

### Unit Tests
- Elke nieuwe functie krijgt tests
- Edge cases voor rendering
- Round-trip tests (create -> save -> load -> verify)

### Visual Tests
- Golden file tests voor SVG output
- Compare tegen Visio-generated reference files
- Automated screenshot comparison

### Integration Tests
- Complete workflow tests
- Large file handling
- Compatibility met verschillende Visio versies

### Performance Tests
- Benchmark voor grote files (1000+ shapes)
- Memory usage tracking
- SVG rendering performance

---

## Dependencies

### Huidige dependencies
- `github.com/beevik/etree` - XML parsing

### Voorgestelde nieuwe dependencies

**Voor PNG export (optioneel)**:
```
github.com/srwiley/oksvg   - SVG parsing
github.com/srwiley/rasterx - Rasterization
```

**Voor PDF export (optioneel)**:
```
github.com/jung-kurt/gofpdf - PDF generation
```

**Alternatief**: Externe tools (Inkscape, rsvg-convert)

---

## Documentatie

### API Documentation
- GoDoc comments voor alle public functies
- Examples in `example_test.go`

### User Guide
- Getting started
- Common workflows
- Troubleshooting

### Migration Guide
- Upgraden van oude versies
- Breaking changes

---

## Geschatte Totale Effort

| Fase | Uren |
|------|------|
| Fase 1: Rendering | 14-19 |
| Fase 2: Enhancements | 26-33 |
| Fase 3: Advanced | 31-42 |
| Fase 4: Polish | 16-24 |
| Fase 5: Geometry | 2 |
| **Totaal** | **89-120 uur** |

Met een gemiddelde van ~20 uur/week: **5-6 weken** voor volledige implementatie.

---

---

## Coverage Targets per Fase

### Huidige Coverage Baseline: 88.6%

| Fase | Target | Strategie |
|------|--------|-----------|
| Fase 1 | 90% | Line patterns, arrows zijn makkelijk te testen |
| Fase 2 | 91% | Gradient/shadow hebben duidelijke output |
| Fase 3 | 92% | Stencils: round-trip tests |
| Fase 4 | 93% | Auto-routing: path verification tests |
| Fase 5 | 94% | Export: golden file comparison |
| Polish | **95%** | Edge cases, error paths |

### Coverage Regels voor Nieuwe Code

1. **Elke nieuwe public functie**: minimaal 90% coverage
2. **Elke nieuwe type**: constructor + alle methods getest
3. **SVG output**: golden file tests (expected output vergelijken)
4. **Round-trip tests**: create → save → load → verify
5. **Edge cases**: nil inputs, empty collections, invalid values

### Coverage Monitoring

```bash
# Run na elke fase:
go test ./vsdx/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep -v "100.0%" | sort -t'%' -k3 -n

# HTML report:
go tool cover -html=coverage.out -o coverage.html
```

---

## Quick Coverage Wins (Pre-Roadmap)

Deze functies hebben lage coverage en zijn snel te fixen:

### Prioriteit 1: 0% Coverage (moet gefixt)

| Functie | File | Reden | Fix |
|---------|------|-------|-----|
| `geometryCellParent` | geometry.go:483-484 | Marker interface | Accepteer als untestable OF test via type assertion |
| `extensionFromPath` | svg.go:892 | Unused? | Verwijder of test |
| `ConvertEMFToSVG` | svg.go:922 | Externe tool nodig | Mock of skip in CI |
| `MasterToSVG` | svg.go:948 | Niet aangeroepen | Test of verwijder |

### Prioriteit 2: <50% Coverage

| Functie | File | Coverage | Fix |
|---------|------|----------|-----|
| `toSlice` | template.go:433 | 13.3% | Test meer slice types |
| `isTruthy` | template.go:390 | 30.0% | Test alle value types |
| `cellString` | svg.go:762 | 33.3% | Test formula fallback |
| `toInterfaceFloat` | template.go:414 | 33.3% | Test int/float/string inputs |
| `resolvePageIndex` | vsdxfile.go:625 | 41.7% | Test negative index, out of bounds |
| `evalCellRef` | formula.go:486 | 53.3% | Test meer cell types |
| `SetValue` (DataProperty) | data_property.go:77 | 57.1% | Test type preservation |

### Prioriteit 3: 60-80% Coverage

| Functie | File | Coverage | Fix |
|---------|------|----------|-----|
| `clamp` | svg.go:645 | 60.0% | Test edge values |
| `MasterShape` | shape.go:153 | 69.2% | Test nil cases |
| `MasterPage` | shape.go:176 | 66.7% | Test missing master |
| `CellFormula` | shape.go:197 | 66.7% | Test formula inheritance |
| `SubstituteRefs` | formula.go:578 | 70.0% | Test function name skipping |
| `Page.Width/Height` | page.go:85,98 | 71.4% | Test missing PageSheet |

### Test Template voor Quick Fixes

```go
func TestLowCoverageFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   interface{}
        want    interface{}
        wantErr bool
    }{
        {"nil input", nil, defaultValue, false},
        {"empty input", "", defaultValue, false},
        {"valid input", validInput, expectedOutput, false},
        {"invalid input", invalidInput, nil, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := functionUnderTest(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Geschatte Impact

| Actie | Coverage Boost |
|-------|----------------|
| Fix 0% functies | +0.3% |
| Fix <50% functies | +0.8% |
| Fix 60-80% functies | +0.5% |
| **Totaal** | **+1.6%** → **90.2%** |

---

## Conclusie

Na implementatie van dit roadmap heeft vsdx-go:

- **Volledige MS-VSDX compatibiliteit** (alle sections, alle geometry types)
- **Professionele rendering** (gradients, shadows, arrows, themes)
- **Authoring capabilities** (create masters, stencils)
- **Export opties** (SVG, PNG, PDF)
- **Advanced features** (auto-routing, themes)

Dit is voldoende voor een professionele diagram-applicatie die kan concurreren met tools als draw.io en Lucidchart, met als extra dat alle output native compatible is met Microsoft Visio.
