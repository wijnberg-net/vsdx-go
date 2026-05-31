# Unsupported Visio Features

**Scope**: this document tracks Visio features and how vsdx-go handles them
across **three planes** that often diverge:

| Plane | Question |
|---|---|
| **Model** | Can the library read/write the underlying VSDX cells? |
| **API**   | Does the library expose a typed Go method/struct for it? |
| **Render**| Does `ShapeToSVG` actually visualize the feature in SVG output? |

The matrices below mark each plane independently. A row tagged
`Model ✓ / Render ✗` means the feature round-trips through .vsdx
correctly but the SVG export currently doesn't show it.

**Last reviewed**: 2026-05-29 (writer canonicalization sweep)

**Cross-references**:
- `DIVERGENCE_STATUS.md` — per-divergence resolution + evidence
- `RENDER_AUDIT.md` — render pipeline architecture
- `WRITER_AUDIT.md` — writer canonical-form audit

---

## Geometry

| Feature | Model | API | Render |
|---|---|---|---|
| MoveTo / LineTo / RelMoveTo / RelLineTo | ✓ | ✓ | ✓ |
| ArcTo / EllipticalArcTo / RelEllipticalArcTo | ✓ | ✓ | ✓ |
| RelCubBezTo / RelQuadBezTo | ✓ | ✓ | ✓ |
| NURBSTo (B-spline) | ✓ | ✓ | ✓ (converted to beziers) |
| PolylineTo | ✓ | ✓ | ✓ |
| SplineStart / SplineKnot | ✓ | ✓ | ✓ |
| InfiniteLine | ✓ | ✓ | ✓ (clipped to bounds) |
| Ellipse row type | ✓ | ✓ | ✓ |
| Clipping paths (ClipPath property) | ✗ | ✗ | ✗ |
| Custom LineJoin per row | ✗ | ✗ | ✗ |

## Style — Fills

| Feature | Model | API | Render |
|---|---|---|---|
| Solid fill (FillForegnd, FillPattern=1) | ✓ | ✓ | ✓ |
| Fill transparency (FillForegndTrans) | ✓ | ✓ | ✓ |
| Linear gradient (FillGradient section) | ✓ | ✓ | ✓ |
| Radial gradient (FillGradientDir 7-11) | ✓ | ✓ | ✓ |
| Hatch patterns 2-9 (8×8 bitmap emit) | ✓ | ✓ | ✓ |
| Dot patterns 25-26 (fine / medium) | ✓ | ✓ | ✓ |
| Hatch patterns 10-24 | ✓ | ✓ | ⚠️ (rendered as solid) |
| Image / texture fills | ✓ (cells) | partial | ✗ |
| Picture fills with crop / tile | ⚠️ (cells only) | ✗ | ✗ |
| Compound lines (DoubleLine, ParaLine) | ✗ | ✗ | ✗ |
| Embossed / debossed effects | ✗ | ✗ | ✗ |

## Style — Strokes

| Feature | Model | API | Render |
|---|---|---|---|
| Line color (LineColor) | ✓ | ✓ | ✓ |
| Line weight (LineWeight) | ✓ | ✓ | ✓ |
| Line transparency (LineColorTrans) | ✓ | ✓ | ✓ |
| LineCap (round / square / extended) | ✓ | ✓ | ✓ |
| LinePattern 1-23 (dash-arrays) | ✓ | ✓ | ✓ |
| LineGradient (stroke gradient + section) | ✓ | ✓ | ✓ |
| Reviewer / Annotation stroke gradient | ✓ | ✓ | ✓ |

## Effects

| Feature | Model | API | Render |
|---|---|---|---|
| Drop shadow (ShdwForegnd, ShdwOffsetX/Y, ShdwType) | ✓ | ✓ | ✓ (feDropShadow filter) |
| Soft edges (SoftEdgesSize) | ✓ | ✓ | ✓ (feGaussianBlur filter) |
| Bevel effect (13 cells) | ✓ | ✓ `SetBevelEffect` | ✗ (cell round-trip only) |
| Glow effect (3 cells) | ✓ | ✓ `SetGlowEffect` | ✗ |
| Reflection effect (4 cells) | ✓ | ✓ `SetReflectionEffect` | ✗ |
| Sketch effect (6 cells) | ✓ | ✓ `SetSketchEffect` | ✗ |
| 3D rotation (7 cells) | ✓ | ✓ `SetRotation3DEffect` | ✗ |

## Text

| Feature | Model | API | Render |
|---|---|---|---|
| Plain text content | ✓ | ✓ | ✓ |
| Multi-line with word wrap | ✓ | ✓ | ✓ (hyphen-aware splitter) |
| Horizontal alignment (HorzAlign) | ✓ | ✓ | ✓ |
| Vertical alignment (VerticalAlign) | ✓ | ✓ | ✓ |
| Font name (Char.Font) | ✓ | ✓ `SetCharFont` | ✓ |
| Font size (Char.Size, U='PT') | ✓ | ✓ `SetCharSize` | ✓ |
| Bold / Italic (Char.Style bits) | ✓ | ✓ `SetCharBold` / `SetCharItalic` | ✓ |
| Underline (Char.Style bit 4) | ✓ | ✓ `SetCharUnderline` | ✓ |
| Text color (Char.Color) | ✓ | ✓ `SetTextColor` | ✓ |
| Text block rotation (TxtAngle) | ✓ | ✓ `SetTxtAngle` | ✓ |
| Inline cp/pp formatting runs | ✓ | ✓ raw cell write | ⚠️ (first-run honored) |
| Strikethru / DblUnderline / Overline | ✓ (cells) | ✗ | ✗ |
| Subscript / superscript (Char.Pos) | ✓ (cells) | ✗ | ✗ |
| Letter spacing (Char.Letterspace) | ✓ (cells) | ✗ | ✗ |
| Tab stops (Tabs section) | ✓ | ✓ `AddTabStop` | ✗ |
| Bullet lists (Para.Bullet) | ✓ (cells) | ✗ | ✗ |
| Text wrap around shapes | ✗ | ✗ | ✗ |
| CJK / double-byte layout | ⚠️ (AsianFont cell) | ✗ | ✗ |

## Arrows / Markers

| Feature | Model | API | Render |
|---|---|---|---|
| Begin / End arrow types 1-45 | ✓ | ✓ `SetBeginArrow` / `SetEndArrow` | ✓ |
| Arrow size index 0-6 | ✓ | ✓ | ✓ |
| Arrow color (inherits line color) | ✓ | — | ✓ |
| Setback (point→SVG unit conversion) | ✓ | — | ✓ |
| Custom arrow definitions | ✗ | ✗ | ✗ |
| Arrow placement along curved path | ✗ | ✗ | ✗ |
| Compound arrow heads | ✗ | ✗ | ✗ |

## Transforms

| Feature | Model | API | Render |
|---|---|---|---|
| PinX / PinY | ✓ | ✓ `SetX` / `SetY` | ✓ |
| Width / Height | ✓ | ✓ `SetWidth` / `SetHeight` | ✓ |
| LocPinX / LocPinY (with formula sync) | ✓ | ✓ (auto-managed) | ✓ |
| Angle (shape rotation) | ✓ | ✓ `SetAngle` | ✓ |
| FlipX / FlipY (mirror) | ✓ | ✓ `SetFlipX` / `SetFlipY` | ✓ (geometry mirrored, text upright) |
| Negative width / height | ✓ | ✓ | ✓ |
| Group transform propagation | ✓ | ✓ `GroupShapes` | ✓ |
| Hierarchical transforms (nested groups) | ✓ | ✓ | ✓ (render tree) |
| Skew transforms | ✗ | ✗ | ✗ |

## Layers

| Feature | Model | API | Render |
|---|---|---|---|
| Layer definition (Page Layer section) | ✓ | ✓ `Page.AddLayer` | — |
| Layer membership per shape (LayerMember) | ✓ | ✓ `SetLayerMember` | partial |
| Layer Color / ColorTrans / Status cells | ✓ | — (canonical defaults) | ✗ |
| Layer visibility (Visible cell) | ✓ | ✓ (canonical default 1) | partial |
| Layer-based z-order | ✗ | ✗ | ✗ |
| Layer-specific styles / locks | ✗ | ✗ | ✗ |

## Connectors

| Feature | Model | API | Render |
|---|---|---|---|
| Static connector geometry | ✓ | ✓ `ConnectShapes` | ✓ |
| Connection points (Connection section) | ✓ | ✓ `AddConnectionPoint` | ✓ |
| ConnectionABCD rows (T='ConnectionABCD') | ✓ | ✓ `AddConnectionABCD` | ✓ |
| Begin / End arrow markers | ✓ | ✓ | ✓ |
| Line styles on connectors | ✓ | ✓ | ✓ |
| Auto-routing (A* pathfinding) | ✓ | ✓ `Router` | ✓ |
| Dynamic re-routing on shape move | ⚠️ | ✗ | ✗ |
| Connector jumps / bridges (line crossings) | ✗ | ✗ | ✗ |

## Groups

| Feature | Model | API | Render |
|---|---|---|---|
| Group creation | ✓ | ✓ `GroupShapes` | ✓ |
| Child shape positioning (group-relative) | ✓ | ✓ | ✓ |
| Nested groups | ✓ | ✓ | ✓ |
| Group clip bounds | ✗ | ✗ | ✗ |
| Group-level style overrides | ⚠️ (inherited) | ✗ | ✗ |

## Pages

| Feature | Model | API | Render |
|---|---|---|---|
| Page sheets (PageSheet element) | ✓ | ✓ | — |
| Multiple pages | ✓ | ✓ `AddPage` / `RemovePageByIndex` | ✓ |
| Master shape inheritance | ✓ | ✓ `MasterShape` | ✓ |
| Background pages | ✓ | ✓ `SetBackgroundPage` / `IsBackgroundPage` | ✓ |
| Page triggers (RecalcColor) | ✓ | — (canonical default) | — |
| Page-level layers | ✓ | ✓ `AddLayer` | partial |
| Page headers / footers | ✗ | ✗ | ✗ |
| Print areas | ✗ | ✗ | ✗ |

## Document-Level

| Feature | Model | API | Render |
|---|---|---|---|
| Themes (theme1.xml, variants, QuickStyle) | ✓ | ✓ `Theme` | ✓ |
| StyleSheets (StyleSheet inheritance) | ✓ | ✓ | ✓ |
| Document color palette (ColorEntry) | ✓ | ✓ auto-refresh | — |
| FaceName font registration | ✓ | ✓ auto-refresh | — |
| Comments (document + shape) | ✓ | ✓ | — |
| Reviewers / Annotations | ✓ | ✓ | — |
| Data connections / recordsets | ✓ | ✓ | — |
| Hyperlinks (Address, SubAddress, NewWindow, SortKey) | ✓ | ✓ `AddHyperlink` | ✓ (`<a xlink:href>` wrapper) |
| Markup Compatibility (mc:AlternateContent) | ✓ | ✓ | — |
| Custom file properties (docProps/custom.xml) | ✓ | ✓ | — |
| HLinks tracking in app.xml | ✓ | auto-refresh | — |

## Cross-bundle / Stencil import

| Feature | Model | API | Render |
|---|---|---|---|
| Create master from scratch (CreateMaster) | ✓ | ✓ | — |
| Duplicate master within bundle (DuplicateMaster) | ✓ | ✓ | — |
| Import master from another bundle (`ImportMaster`) | ✓ | ✓ `VisioFile.ImportMaster` / `ImportMasterWithOptions` | — |
| Convenience: import + instantiate in one call | ✓ | ✓ `Page.AddShapeFromExternalMaster` | — |
| UniqueID-based dedup (idempotent re-import) | ✓ | auto | — |
| Sub-master recursion (BaseID + nested Master refs) | ✓ | auto | — |
| Dynamic-connector master reuse (no duplication) | ✓ | auto | — |
| Foreign data media copy + collision-safe rename | ✓ | auto | — |
| Theme cell inlining (THEMEGUARD / THEMEVAL → V) | ✓ | `ImportOptions{InlineTheme: bool}` (default true) | — |
| Content_Types Default Extension auto-add | ✓ | auto | — |
| Master rels file deep-copy + Target rewrite | ✓ | auto | — |

## Formulas

| Feature | Model | API | Render |
|---|---|---|---|
| Cell formulas (F attribute) | ✓ | ✓ `SetCellFormula` | — |
| Formula evaluation (175+ functions) | ✓ | ✓ `FormulaEvaluator` | — |
| Cross-shape references (TheCel / Sheet.N!) | ✓ | ✓ | — |
| Page-level formula triggers (RefBy graph) | partial | ✗ | ✗ |

## Known Intentional Divergences from Visio

These are conscious choices documented in `DIVERGENCE_STATUS.md`:

1. **Arrow setback unit conversion** — RenderTree converts points to SVG
   units (÷72×96); legacy code treated points as SVG units directly.
   Result: ~0.91 SVG-unit delta on connector endpoints.
2. **Multi-geometry fill inversion** — RenderTree uses actual shape fill
   colors; legacy heuristically inverted dark fills to white for secondary
   geometries of icon shapes.
3. **Z-order determination** — RenderTree uses the OrderIndex property
   per spec; legacy used geometry-count heuristics.

## Versions

- Renderer: RenderTree 1.x (render_tree.go)
- MS-VSDX spec: 2012/main namespace
- Library version: post writer-canonicalization sweep (commit a5976f9)
- Last doc review: 2026-05-29
