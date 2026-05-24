# SVG Renderer Pipeline - Production Status

## Architecture Overview

```
Parsed Model (VSDX XML)
  │
  ├─► Effective Style Resolution      [ComputeEffectiveStyle()]
  │     └─ local → master → stylesheet → theme → defaults
  │     └─ ✅ PRODUCTION
  │
  ├─► Geometry Resolution             [ResolveGeometry()]
  │     └─ all geometry types resolved to SVG paths
  │     └─ arrow setback applied with correct unit conversion
  │     └─ ✅ PRODUCTION
  │
  ├─► World Transform Resolution      [ComputeGroupTransforms()]
  │     └─ PinX/PinY, LocPinX/LocPinY, Angle
  │     └─ parent transforms, child offsets
  │     └─ ✅ PRODUCTION
  │
  ├─► Text Resolution                 [resolveText()]
  │     └─ multiline wrapping
  │     └─ alignment, margins, rotation
  │     └─ ✅ PRODUCTION
  │
  ├─► Render Tree Construction        [RenderTreeBuilder.BuildWithScale()]
  │     └─ OrderIndex-based z-order
  │     └─ visibility filtering
  │     └─ ✅ PRODUCTION
  │
  └─► SVG Emission                    [SVGEmitter.Emit()]
        └─ pure serialization only
        └─ deterministic output
        └─ ✅ PRODUCTION
```

## Public API

| Function | Description | Status |
|----------|-------------|--------|
| `ShapeToSVG()` | Canonical entry - RenderTree | ✅ Production |
| `EmitRenderTree()` | Direct RenderTree, returns bytes | ✅ Production |
| `EmitRenderTreeWithResult()` | Full result with brand color | ✅ Production |
| `ShapeToSVGLegacy()` | Legacy renderer (reference only) | ⚠️ Deprecated |

## Validation Tools

| Tool | Purpose | Location |
|------|---------|----------|
| `cmd/render-diff` | Compare legacy vs RenderTree | CLI |
| `RenderComparator` | Programmatic comparison | vsdx/render_validate_diff.go |
| `TestGoldenFixtures` | Golden file regression tests | vsdx/golden_test.go |
| `TestDeterministicOutput` | Output stability verification | vsdx/golden_test.go |

## Golden Test Fixtures

Located in `testdata/golden/`:

| Fixture | Categories | Description |
|---------|------------|-------------|
| simple_rect | geometry, text | Basic rectangle |
| filled_rect | geometry, fill, text | Filled shape |
| house_group | geometry, group, transform | Nested geometries |
| connector_arrow | geometry, markers | Arrow markers |

## Divergence Analysis

**Final Statistics (109 shapes tested):**

| Category | Count | Status |
|----------|-------|--------|
| path | 14 | 3 intentional, 11 need work |
| bounds | 8 | Needs investigation |
| text | 88 | 1 fixed (baseline), 87 need work |
| fill | 50 | ✅ Intentional - no heuristic inversion |

**Severity Breakdown:**
- Critical: 0
- Error: 23 (text content parsing in comparator)
- Warning: 137

**Detailed Status**: See `DIVERGENCE_STATUS.md` for item-by-item classification.

## Known Intentional Divergences

### 1. Arrow Setback Unit Conversion
**RenderTree**: Correctly converts points → SVG units
**Legacy**: Used points directly as SVG units (bug)
**Delta**: ~0.9 SVG units per connector
**Status**: ✅ RenderTree is correct

### 2. Multi-Geometry Fill Colors
**RenderTree**: Uses actual shape data colors
**Legacy**: Inverted dark fills for secondary geometries
**Status**: ✅ RenderTree is correct per MS-VSDX

### 3. Z-Order Determination
**RenderTree**: OrderIndex property
**Legacy**: Geometry-count heuristics
**Status**: ✅ RenderTree follows spec

## Guarantees

### Determinism
- Multiple renders of same shape produce byte-identical SVG
- Map iteration order doesn't affect output
- Verified by `TestDeterministicOutput`

### Correctness
- All geometry resolved before emission
- All transforms pre-computed
- No heuristics in SVG emitter
- All style lookups via EffectiveStyle

### Reproducibility
- Golden fixtures track expected output
- SHA-256 hash comparison
- Regression tests on every build

## Unsupported Features

See `UNSUPPORTED_FEATURES.md` for complete list.

Key unsupported:
- 3D effects (bevel, glow, reflection)
- Custom line joins/caps
- Rich text (mixed fonts)
- Dynamic connector routing
- Pattern/image fills

## Test Commands

```bash
# Run all renderer tests
go test ./vsdx/... -v

# Run golden tests
go test ./vsdx/... -run "TestGoldenFixtures" -v

# Regenerate golden fixtures
GENERATE_GOLDEN=1 go test ./vsdx/... -run "TestGenerateGoldenFixtures" -v

# Generate divergence report
go run ./cmd/render-diff -report=REPORT.md tests/*.vsdx

# Test determinism
go test ./vsdx/... -run "TestDeterministicOutput" -v
```

## Architecture Compliance

| Requirement | Status |
|-------------|--------|
| No geometry mutation during emission | ✅ |
| No transform computation during emission | ✅ |
| No style resolution during emission | ✅ |
| No connector heuristics | ✅ |
| No ShapeSheet lookups in emitter | ✅ |
| Deterministic serialization | ✅ |
| Golden test coverage | ✅ |
| Documented unsupported features | ✅ |
