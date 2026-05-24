# Text Rendering Divergence Report

**Status Tracking**: See `DIVERGENCE_STATUS.md` items #15-19 for current status.

## Summary

Text element counts match between generated and golden SVGs (9/9 for logical-architecture).
However, significant coordinate and structural divergences exist.

## Concrete Measurements

### Shape 1 (Devices) - Exact Coordinate Comparison

| Metric | Golden | Generated | Delta | Category |
|--------|--------|-----------|-------|----------|
| Group Transform X | 42.52 | 42.52 | 0 | match |
| Group Transform Y | -510.24 | 42.52 | 552.76 | **transform mismatch** |
| Text Local X | 14.57 | 42.52 | -27.95 | **geometry mismatch** |
| Text Local Y | 579.42 | 21.26 | 558.16 | **geometry mismatch** |
| Text Absolute X | 57.09 | 85.04 | **+27.95 pts** | **renderer bug** |
| Text Absolute Y | 69.18 | 63.78 | **-5.40 pts** | **renderer bug** |

### Coordinate System Analysis

**Golden SVG approach:**
- Uses Visio's native coordinates (Y near page bottom: 579.42)
- Applies negative Y transform (-510.236) to flip to SVG top-down
- Final position: 579.42 - 510.24 = 69.18

**Generated SVG approach:**
- Pre-converts to SVG top-down coordinates (Y=42.52)
- Uses local shape coordinates for text (21.26)
- Final position: 42.52 + 21.26 = 63.78

**Root cause:** Different coordinate and anchor approaches:
1. X difference (~28pt): Generated uses `text-anchor="middle"` at shape center.
   Golden pre-calculates left edge position with default `text-anchor="start"`.
2. Y difference (~5pt): Generated uses `dominant-baseline="middle"`.
   Golden calculates explicit Y for alphabetic baseline.

**Visual equivalence:** The X difference is visually equivalent (both center the text).
The Y difference of 5.4pt is NOT visually equivalent - text appears shifted.

## Divergence Classification

### 1. Coordinate System Mismatch
**Category**: transform mismatch  
**Severity**: High  
**Evidence**:
- Generated: `x="59.528" y="33.307"` (shape-local coordinates)
- Golden: `x="14.57" y="579.42"` (page-absolute coordinates)

**Root Cause**: Generated SVG places text within shape `<g>` elements using local
coordinates. Golden SVG uses page-absolute coordinates within each shape group.

**Fix Required**: Transform text coordinates to page-absolute space or ensure
consistent group transforms.

### 2. Multi-line Text Handling
**Category**: text layout mismatch  
**Severity**: High  
**Evidence**:
- Generated: `<text>Cloud Gateway</text>` (single line, collapsed)
- Golden: `<text>Cloud <tspan dy="1.2em">Gateway</tspan></text>` (multi-line)

**Root Cause**: Renderer does not implement tspan-based line breaking for text
that exceeds shape width or contains explicit line breaks.

**Fix Required**: Implement text wrapping with `<tspan>` elements and proper
`dy` attributes for line spacing.

### 3. Baseline Handling
**Category**: text layout mismatch  
**Severity**: High (causes 5.4pt visual shift)  
**Status**: FIXED

**Evidence**:
- Generated: `dominant-baseline="alphabetic"` with Y offset +0.3×fontSize
- Golden: No dominant-baseline attribute, Y offset by ~5.4pt below center

**Numeric Analysis (Shape 1 "Devices"):**
- Shape bounds Y: 552.76 to 595.28 (height=42.52)
- Shape center Y: 574.02
- Golden text Y: 579.42 (5.4pt below center)
- Font size: 18pt (1.5em)
- Baseline offset applied: 0.3 × font_size = 5.4pt

**Root Cause**: Visio positions text Y at alphabetic baseline, offset below
visual center by approximately `0.3 × font_size`.

**Fix Applied**: 
1. Changed `dominant-baseline` from "middle" to "alphabetic"
2. Added Y offset: `y += fontSize * 0.3`
3. Files modified: `vsdx/svg.go`, `vsdx/render_tree.go`, `svg-compare/cmd/render/render_page.go`

### 4. Font Size Units
**Category**: SVG standards variation  
**Severity**: Low  
**Evidence**:
- Generated: `font-size="18.000"` (interpreted as px)
- Golden: `font-size:1.5em` (relative to parent)

**Root Cause**: Different unit conventions. Both are valid SVG.

**Fix Required**: None required for visual fidelity, but could normalize to
match Visio output for diff minimization.

### 5. Text Fill Color
**Category**: renderer bug (fixed)  
**Severity**: Medium  
**Evidence**:
- Generated: `fill="#000000"`
- Golden: Uses themed color via CSS class

**Status**: Partially fixed by theme color resolution. Need to verify text color
inheritance follows same path.

## Required Implementation Work

### Phase 1: Text Coordinate Transform (High Priority)
1. Extract text position in page coordinates, not shape-local
2. Account for shape transforms when placing text
3. Match Visio's text block positioning algorithm

### Phase 2: Multi-line Text (High Priority)
1. Parse text content for line breaks (&#10; or \n)
2. Calculate line count and spacing
3. Generate `<tspan>` elements with proper `dy` values
4. Handle text wrapping based on shape width

### Phase 3: Baseline Calculation (Medium Priority)
1. Use font metrics (ascent/descent) for baseline calculation
2. Remove reliance on `dominant-baseline`
3. Calculate Y position as: textBlockY + ascent + (lineIndex * lineHeight)

### Phase 4: Font Metrics (Medium Priority)
1. Implement font metric lookup or estimation
2. Handle em-to-px conversion based on parent font size
3. Support Visio's font table references

## Metrics

| File | Text Elements | Coord Match | Layout Match | Notes |
|------|---------------|-------------|--------------|-------|
| logical-architecture | 9/9 | 0/9 | 0/9 | All use multi-line |
| ad-hoc-exploration | 6/6 | TBD | TBD | |
| physical-* | varies | TBD | TBD | |

## Acceptance Criteria

A text element is considered "matching" when:
1. X coordinate within 0.5pt of golden
2. Y coordinate within 0.5pt of golden
3. Same number of tspan elements
4. Line spacing (dy) matches within 0.1em
5. Font size matches within 1pt
6. Fill color matches exactly (after normalization)
