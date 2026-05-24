# Unsupported Visio Features in SVG Renderer

**Status Tracking**: See `DIVERGENCE_STATUS.md` items #20-42 for classification status.

This document lists Visio features that are not currently supported by the SVG renderer.

## Geometry Features

### Fully Supported
- [x] MoveTo, LineTo, RelMoveTo, RelLineTo
- [x] ArcTo, EllipticalArcTo, RelEllipticalArcTo
- [x] RelCubBezTo, RelQuadBezTo
- [x] NURBSTo (converted to beziers)
- [x] PolylineTo
- [x] Ellipse (full ellipse geometry)
- [x] SplineStart, SplineKnot
- [x] InfiniteLine (clipped to bounds)

### Partially Supported
- [⚠️] 3D effects (BevelEffect, GlowEffect, ReflectionEffect) - ignored
- [⚠️] SketchEffect - ignored
- [⚠️] Rotation3DEffect - ignored

### Not Supported
- [ ] Clipping paths (ClipPath property)
- [ ] Custom line joins (LineJoin property)
- [ ] Custom line caps (LineCap property beyond markers)

## Style Features

### Fully Supported
- [x] Fill colors (solid, theme-resolved)
- [x] Stroke colors (solid, theme-resolved)
- [x] Line weight
- [x] Line patterns (24 dash patterns)
- [x] Fill patterns (solid fill)
- [x] Fill transparency
- [x] Stroke transparency

### Partially Supported
- [⚠️] Gradient fills - basic linear/radial only
- [⚠️] Pattern fills (hatching) - rendered as solid
- [⚠️] Image fills - not rendered

### Not Supported
- [ ] Embossed/debossed effects
- [ ] Texture fills
- [ ] Picture fills
- [ ] Compound lines

## Text Features

### Fully Supported
- [x] Single-line text
- [x] Multi-line text with word wrap
- [x] Horizontal alignment (left, center, right)
- [x] Vertical alignment (top, middle, bottom)
- [x] Font size
- [x] Bold/Italic
- [x] Text color
- [x] Text rotation

### Partially Supported
- [⚠️] Text margins - basic support
- [⚠️] Line spacing - 1.2x font size assumed

### Not Supported
- [ ] Rich text (mixed fonts/sizes within text)
- [ ] Subscript/superscript
- [ ] Text decoration (underline, strikethrough)
- [ ] Tabs and tab stops
- [ ] Bullet lists
- [ ] Text wrapping around shapes
- [ ] Vertical text orientation
- [ ] Double-byte character support (CJK layout)

## Marker (Arrow) Features

### Fully Supported
- [x] Standard arrow types (0-45)
- [x] Arrow size scaling
- [x] Arrow color inheritance
- [x] Path shortening for arrow placement

### Not Supported
- [ ] Custom arrow definitions
- [ ] Arrow placement along curved paths
- [ ] Compound arrow heads

## Transform Features

### Fully Supported
- [x] Shape position (PinX, PinY, LocPinX, LocPinY)
- [x] Shape dimensions (Width, Height)
- [x] Shape rotation (Angle)
- [x] Group transform propagation
- [x] Negative width/height (mirroring)

### Not Supported
- [ ] Flip transforms (FlipX, FlipY) - separate from negative dimensions
- [ ] Skew transforms
- [ ] Scale transforms beyond aspect-ratio fit

## Layer Features

### Partially Supported
- [⚠️] Layer visibility - shapes hidden via NoShow
- [⚠️] Layer membership - not used for z-ordering

### Not Supported
- [ ] Layer-based rendering order
- [ ] Layer-specific styles
- [ ] Layer lock/print settings

## Connector Features

### Fully Supported
- [x] Static connector geometry
- [x] Begin/End arrow markers
- [x] Connector line styles

### Not Supported
- [ ] Dynamic connector routing
- [ ] Connector jumps/bridges
- [ ] Connector glue behavior
- [ ] Automatic connector re-routing

## Group Features

### Fully Supported
- [x] Group transform propagation
- [x] Child shape positioning
- [x] Nested groups

### Not Supported
- [ ] Group clip bounds
- [ ] Group-level style overrides

## Page Features

### Fully Supported
- [x] Page shapes rendering
- [x] Master shape inheritance
- [x] Shape-to-master cell fallback

### Not Supported
- [ ] Background pages
- [ ] Page layers
- [ ] Page headers/footers
- [ ] Print areas

## Known Behavioral Differences from Visio

### 1. Arrow Setback Calculation
**Difference**: RenderTree correctly converts arrow setback from points to SVG units.
**Legacy behavior**: Treated points as SVG units directly.
**Impact**: ~0.9 SVG unit difference in connector endpoints.
**Resolution**: RenderTree is correct per MS-VSDX spec.

### 2. Multi-Geometry Fill Inversion
**Difference**: RenderTree uses actual fill colors from shape data.
**Legacy behavior**: Inverted dark fills to white for secondary geometries.
**Impact**: Icons like House may show black windows instead of white.
**Resolution**: RenderTree is correct; Visio files should set explicit colors.

### 3. Z-Order Determination
**Difference**: RenderTree uses OrderIndex property.
**Legacy behavior**: Used geometry-count-based heuristics.
**Impact**: Different rendering order for some shapes.
**Resolution**: RenderTree follows MS-VSDX spec.

## Version Information

- Renderer version: RenderTree 1.0
- MS-VSDX spec version: 2012/main
- Last updated: 2024-01-01
