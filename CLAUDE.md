# vsdx-go

Go library voor het lezen, bewerken en schrijven van Microsoft Visio (.vsdx) bestanden.
Dit project is een port van de Python [vsdx](https://github.com/dave-howard/vsdx) library (v0.6.1).

## Doel

De bestaande Python library omzetten naar een idiomatische Go library met dezelfde functionaliteit:
lezen/schrijven van .vsdx bestanden, shapes manipuleren, pagina's beheren, en Jinja2-achtige templating.

## VSDX Bestandsformaat

Een `.vsdx` bestand is een **ZIP-archief** met daarin XML-bestanden:

```
[Content_Types].xml          - Content type mappings
docProps/app.xml             - Document properties (pagina-telling, titels)
docProps/core.xml            - Core metadata
visio/document.xml           - Hoofddocument met stijlen/stylesheets
visio/_rels/document.xml.rels - Document relaties
visio/pages/pages.xml        - Paginadefinities (namen, IDs)
visio/pages/_rels/pages.xml.rels - Paginarelaties
visio/pages/page1.xml        - Individuele pagina-inhoud (shapes, connects)
visio/pages/page2.xml        - ...
visio/masters/masters.xml    - Master shape definities
visio/masters/master1.xml    - Individuele master shapes
```

XML namespace: `http://schemas.microsoft.com/office/visio/2012/main`

## Architectuur Python Library

### Kernklassen → Go structs

| Python klasse   | Bestand          | Verantwoordelijkheid                                    |
|-----------------|------------------|---------------------------------------------------------|
| `VisioFile`     | `vsdxfile.py`    | Hoofd-entrypoint: openen/opslaan ZIP, pagina-beheer     |
| `Page`          | `pages.py`       | Pagina of master-pagina: shapes, connects, afmetingen   |
| `Shape`         | `shapes.py`      | Shape of groep: tekst, positie, grootte, stijl, cellen  |
| `Cell`          | `shapes.py`      | Naam/waarde/formule paar uit XML Cell element            |
| `DataProperty`  | `shapes.py`      | Custom properties van een shape                         |
| `Connect`       | `connectors.py`  | Verbinding tussen twee shapes (from/to met relaties)    |
| `Geometry`      | `geometry.py`    | Shape pad-definitie (MoveTo, LineTo, ArcTo, etc.)       |
| `GeometryRow`   | `geometry.py`    | Rij in geometry (type + coördinaten)                    |
| `GeometryCell`  | `geometry.py`    | Cel in een geometry rij                                 |
| `Media`         | `media.py`       | Template shapes (connector, rechthoek, cirkel)          |
| `VisioFileDiff` | `vsdxdiff.py`    | Vergelijking van twee vsdx bestanden                    |

### Data Flow

**Openen:**
1. `.vsdx` (ZIP) → in-memory laden als `map[string][]byte`
2. XML bestanden parsen → ElementTree (Go: `encoding/xml` of `etree`)
3. Page objecten aanmaken vanuit page XML elementen
4. Shape objecten aanmaken vanuit Shape XML elementen (hiërarchisch)
5. Master pages apart laden
6. Connect objecten aanmaken

**Opslaan:**
1. Gewijzigde XML serialiseren
2. In-memory map → nieuw ZIP bestand schrijven naar disk

### Shape Hiërarchie

- Een `Page` bevat top-level shapes via `<Shapes>` element
- Een `Shape` kan een groep zijn met `child_shapes` (geneste `<Shapes>`)
- Shapes kunnen verwijzen naar een master shape (overerving van properties)
- Bij het opvragen van properties: eerst lokaal, dan master, dan master's master

### Belangrijke Shape Properties

- **Positie:** `PinX`, `PinY` (center), `BeginX/Y`, `EndX/Y` (connectoren)
- **Grootte:** `Width`, `Height`
- **Stijl:** `LineWeight`, `LineColor`, `FillForegnd`, `FillBkgnd`
- **Tekst:** apart `<Text>` element binnen shape
- **Cellen:** `<Cell N="naam" V="waarde" F="formule"/>` elementen
- **Data Properties:** `<Section N="Property"><Row>...</Row></Section>`
- **Geometry:** `<Section N="Geometry1"><Row T="MoveTo">...</Row></Section>`

### Jinja2 Templating (Python-specifiek)

De Python library ondersteunt Jinja2 templating in shape-teksten:
- `{{ variabele }}` - tekstvervanging
- `{% for item in list %}` - shapes dupliceren in een loop
- `{% showif conditie %}` - conditioneel tonen van shapes/pagina's
- `{% set self.x = waarde %}` - shape properties aanpassen

**Go-aanpak:** Go's `text/template` package als alternatief, of een aparte
template engine. Dit is een secundaire feature; eerst de kern-functionaliteit porten.

## Bronbestanden (Python)

| Bestand              | Regels | Beschrijving                              |
|----------------------|--------|-------------------------------------------|
| `vsdx/vsdxfile.py`   | ~700   | VisioFile klasse, ZIP/XML handling         |
| `vsdx/pages.py`      | ~350   | Page klasse                               |
| `vsdx/shapes.py`     | ~750   | Shape, Cell, DataProperty klassen         |
| `vsdx/connectors.py` | ~100   | Connect klasse                            |
| `vsdx/geometry.py`   | ~200   | Geometry klassen                          |
| `vsdx/formulae.py`   | ~50    | Formule-evaluatie                         |
| `vsdx/media.py`      | ~50    | Media template shapes                    |
| `vsdx/vsdxdiff.py`   | ~80    | Bestandsvergelijking                      |

## Go Package Structuur

```
vsdx-go/
├── go.mod
├── go.sum
├── README.md            # Library documentatie met voorbeelden
├── CLAUDE.md            # AI-assistentie context
├── vsdx/
│   ├── doc.go           # Package-level documentatie
│   ├── vsdxfile.go      # VisioFile struct + Open/Close/SaveVsdx
│   ├── page.go          # Page struct + search/edit methods
│   ├── shape.go         # Shape struct + position/text/style/search
│   ├── cell.go          # Cell struct (name/value/formula)
│   ├── cellname.go      # CellName type alias + cell/connect constants
│   ├── data_property.go # DataProperty struct (master inheritance)
│   ├── connect.go       # Connect struct (from/to relationships)
│   ├── geometry.go      # Geometry/GeometryRow/GeometryCell
│   ├── formula.go       # CalcValue formula evaluation
│   ├── namespace.go     # XML namespace constants
│   ├── errors.go        # Sentinel errors + FileError type
│   ├── types.go         # Point, Rect result structs
│   ├── util.go          # File writing helper
│   ├── media.go         # Media struct with embedded template shapes
│   ├── template.go      # RenderTemplate with Jinja2-style directives
│   ├── diff.go          # VisioFileDiff with LCS-based comparison
│   └── vsdx_test.go     # 95 test cases
├── tests/
│   └── *.vsdx           # Test fixtures (15+ files)
└── vsdx/*.py            # Original Python source files (reference, co-located)
```

## Go-specifieke Overwegingen

- **XML parsing:** `encoding/xml` of [`github.com/beevik/etree`](https://github.com/beevik/etree) voor XPath-achtige navigatie
  - `etree` is aan te raden omdat de Python library zwaar leunt op ElementTree met XPath
- **ZIP handling:** `archive/zip` standaard library
- **Embedded bestanden:** `embed` package voor media.vsdx
- **Error handling:** Go-idiomatisch met `error` return values (geen exceptions)
- **Namespace handling:** XML namespaces als constanten definiëren
- **Properties:** Go heeft geen Python-style properties; gebruik getter/setter methoden of exported fields

## XML Namespaces

```go
const (
    MainNS     = "http://schemas.microsoft.com/office/visio/2012/main"
    RelNS      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
    PkgRelNS   = "http://schemas.openxmlformats.org/package/2006/relationships"
    ContentNS  = "http://schemas.openxmlformats.org/package/2006/content-types"
    ExtPropNS  = "http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"
    VtNS       = "http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"
    CorePropNS = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
    DcNS       = "http://purl.org/dc/elements/1.1/"
    DcTermsNS  = "http://purl.org/dc/terms/"
)
```

## Prioriteit van Omzetting

1. **Fase 1 - Lezen:** ZIP openen, XML parsen, Page/Shape/Cell structs vullen - **DONE**
2. **Fase 2 - Navigatie:** Shapes zoeken (by ID, text, property, regex, master), hiërarchie doorlopen - **DONE**
3. **Fase 3 - Bewerken:** Shape properties wijzigen, tekst, stijl, positie, move, remove - **DONE**
4. **Fase 4 - Schrijven:** Gewijzigde XML opslaan, pagina's toevoegen/verwijderen/kopiëren, shapes kopiëren - **DONE**
5. **Fase 5 - Connectors:** ConnectShapes, SetStartAndFinish, Media template shapes - **DONE**
6. **Fase 6 - Templating:** RenderTemplate met Jinja2-achtige directives ({{key}}, for loops, showif, set self) - **DONE**
7. **Fase 7 - Diff:** VisioFileDiff met LCS-gebaseerde vergelijking van ZIP-inhoud - **DONE**

## Commando's

```bash
# Go tests
cd /home/michel/vsdx-go && go test ./vsdx/... -v

# Python tests (origineel, ter referentie)
cd /home/michel/vsdx-go && python -m pytest tests/ -v
```

## Huidige Status

- 17 Go source bestanden, ~2800 lines code + ~1850 lines tests = ~4650 total
- 95 test cases (alle passing)
- Fasen 1-7 compleet: lezen, navigatie, bewerken, schrijven, connectors, templating, diff
- Idiomatisch Go refactoring compleet: cell constants, error types, typed interfaces, deduplication
- Afhankelijkheid: `github.com/beevik/etree` v1.4.1
