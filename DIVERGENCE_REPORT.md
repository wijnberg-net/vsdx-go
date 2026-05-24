# SVG Renderer Divergence Report

## Summary

- **Total shapes tested**: 109
- **Divergent shapes**: 38 (34.9%)
- **Total divergences**: 160

### By Severity

- error: 23
- warning: 137

### By Category

- path: 14
- bounds: 8
- text: 88
- fill: 50

## Known Divergences

### 1. Multi-Geometry Fill Inversion (Legacy Heuristic)

The legacy renderer inverts fill colors for secondary geometries in multi-geometry shapes.
RenderTree respects the actual shape data.

**Impact**: Shapes like House, Can, and other icons may show inverted window/detail colors.

### 2. Z-Order Heuristics

The legacy renderer uses geometry-count-based z-order heuristics.
RenderTree uses the OrderIndex property.

**Impact**: Path rendering order may differ but visual appearance should be similar.

### 3. Multiline Text Handling

The legacy renderer uses <tspan> elements for multiline text.
RenderTree outputs text with embedded newlines.

**Impact**: Text y-positioning differs slightly; multiline rendering varies by browser.

## Page Details

### Page: Page-1

# Divergence Report: Page-1 (ID: 0)

## Summary

- Total shapes: 2
- Identical: 1
- Divergent: 1
- Total diffs: 21

### By Severity

- error: 4
- warning: 17

### By Category

- path: 10
- text: 11

## Shape Details

### Shape 7

- Paths: legacy=4, rendertree=4
- Texts: legacy=4, rendertree=7
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | path | path[2].cmd[0] arg[0] mismatch | 57.5000 | 0.0000 | 57.5000 |
| warning | path | path[2].cmd[1] arg[0] mismatch | 100.0000 | 42.5000 | 57.5000 |
| warning | path | path[2].cmd[2] arg[0] mismatch | 100.0000 | 42.5000 | 57.5000 |
| warning | path | path[2].cmd[3] arg[0] mismatch | 57.5000 | 0.0000 | 57.5000 |
| warning | path | path[2].cmd[4] arg[0] mismatch | 57.5000 | 0.0000 | 57.5000 |
| warning | path | path[3].cmd[0] arg[0] mismatch | 57.5000 | 0.0000 | 57.5000 |
| warning | path | path[3].cmd[1] arg[0] mismatch | 100.0000 | 42.5000 | 57.5000 |
| warning | path | path[3].cmd[2] arg[0] mismatch | 100.0000 | 42.5000 | 57.5000 |
| warning | path | path[3].cmd[3] arg[0] mismatch | 57.5000 | 0.0000 | 57.5000 |
| warning | path | path[3].cmd[4] arg[0] mismatch | 57.5000 | 0.0000 | 57.5000 |
| warning | text | text element count mismatch | 4 texts | 7 texts |  |
| warning | text | text[0] x position mismatch | 21.2500 | 50.0000 | 28.7500 |
| warning | text | text[0] y position mismatch | 6.2500 | 14.3800 | 8.1300 |
| error | text | text[0] content mismatch | Shape 1.1.1 | Shape 1 |  |
| warning | text | text[1] y position mismatch | 22.5000 | 14.3700 | 8.1300 |
| error | text | text[1] content mismatch | Shape 1.1.2 | Shape 1.1 |  |
| warning | text | text[2] x position mismatch | 78.7500 | 21.2500 | 57.5000 |
| error | text | text[2] content mismatch | Shape 1.2.1 | Shape 1.1.1 |  |
| warning | text | text[3] x position mismatch | 78.7500 | 21.2500 | 57.5000 |
| warning | text | text[3] y position mismatch | 22.5000 | 6.2500 | 16.2500 |
| error | text | text[3] content mismatch | Shape 1.2.2 | Shape 1.1.2 |  |


### Page: Dynamic connector

# Divergence Report: Dynamic connector (ID: 2)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 1

### By Severity

- warning: 1

### By Category

- bounds: 1

## Shape Details

### Shape 5

- Paths: legacy=1, rendertree=1
- Texts: legacy=0, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | bounds | viewBox y mismatch | -100.0000 | -0.0100 | 99.9900 |


### Page: Page-1

# Divergence Report: Page-1 (ID: 0)

## Summary

- Total shapes: 6
- Identical: 1
- Divergent: 5
- Total diffs: 23

### By Severity

- error: 7
- warning: 16

### By Category

- text: 23

## Shape Details

### Shape 9

- Paths: legacy=3, rendertree=3
- Texts: legacy=3, rendertree=4
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 3 texts | 4 texts |  |
| warning | text | text[0] y position mismatch | 36.3600 | 0.0000 | 36.3600 |
| error | text | text[0] content mismatch | Shape Text | Group shape text |  |
| warning | text | text[1] y position mismatch | 14.2900 | 36.3600 | 22.0700 |
| error | text | text[1] content mismatch | Sub-shape 1 | Shape Text |  |
| warning | text | text[2] x position mismatch | 50.0000 | 25.0000 | 25.0000 |
| warning | text | text[2] y position mismatch | 57.0900 | 7.7300 | 49.3600 |
| error | text | text[2] content mismatch | Sub-shape 2 | Sub-shape 1 |  |

### Shape 14

- Paths: legacy=3, rendertree=3
- Texts: legacy=2, rendertree=3
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 2 texts | 3 texts |  |
| warning | text | text[0] x position mismatch | 33.3600 | 50.0000 | 16.6400 |
| warning | text | text[0] y position mismatch | 50.3900 | 0.0000 | 50.3900 |
| error | text | text[0] content mismatch | Sub to copy | Shape to copy |  |
| warning | text | text[1] x position mismatch | 72.9100 | 18.1800 | 54.7300 |
| warning | text | text[1] y position mismatch | 52.4800 | 7.2100 | 45.2700 |
| error | text | text[1] content mismatch | To copy | Sub to copy |  |

### Shape 11

- Paths: legacy=2, rendertree=2
- Texts: legacy=2, rendertree=3
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 2 texts | 3 texts |  |
| warning | text | text[0] y position mismatch | 72.7300 | 36.3600 | 36.3700 |
| error | text | text[0] content mismatch | Sub-shape to remove | Shape to remove |  |
| warning | text | text[1] x position mismatch | 63.4100 | 50.0000 | 13.4100 |
| warning | text | text[1] y position mismatch | 13.9100 | 72.7300 | 58.8200 |
| error | text | text[1] content mismatch | Sub-shape 3 | Sub-shape to remove |  |

### Shape 16

- Paths: legacy=1, rendertree=1
- Texts: legacy=1, rendertree=1
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text[0] y position mismatch | 51.9800 | 25.9900 | 25.9900 |

### Shape 17

- Paths: legacy=1, rendertree=1
- Texts: legacy=1, rendertree=1
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text[0] y position mismatch | 54.4300 | 27.2100 | 27.2200 |


### Page: Page-3

# Divergence Report: Page-3 (ID: 5)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 7

### By Severity

- error: 2
- warning: 5

### By Category

- text: 7

## Shape Details

### Shape 1

- Paths: legacy=3, rendertree=3
- Texts: legacy=2, rendertree=3
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 2 texts | 3 texts |  |
| warning | text | text[0] x position mismatch | 33.3600 | 50.0000 | 16.6400 |
| warning | text | text[0] y position mismatch | 46.2300 | 0.0000 | 46.2300 |
| error | text | text[0] content mismatch | Sub already here | Shape already here |  |
| warning | text | text[1] x position mismatch | 72.9100 | 18.1800 | 54.7300 |
| warning | text | text[1] y position mismatch | 52.4800 | 3.0500 | 49.4300 |
| error | text | text[1] content mismatch | Already here | Sub already here |  |


### Page: Page-1

# Divergence Report: Page-1 (ID: 0)

## Summary

- Total shapes: 4
- Identical: 2
- Divergent: 2
- Total diffs: 10

### By Severity

- warning: 10

### By Category

- text: 2
- fill: 8

## Shape Details

### Shape 7 (House)

- Paths: legacy=7, rendertree=7
- Texts: legacy=0, rendertree=1
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | fill | path[1] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[2] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[3] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[4] fill mismatch | #FFFFFF | #000000 |  |
| warning | text | text element count mismatch | 0 texts | 1 texts |  |

### Shape 11 (House.11)

- Paths: legacy=7, rendertree=7
- Texts: legacy=0, rendertree=1
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | fill | path[1] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[2] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[3] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[4] fill mismatch | #FFFFFF | #000000 |  |
| warning | text | text element count mismatch | 0 texts | 1 texts |  |


### Page: House

# Divergence Report: House (ID: 2)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 4

### By Severity

- warning: 4

### By Category

- fill: 4

## Shape Details

### Shape 5

- Paths: legacy=7, rendertree=7
- Texts: legacy=0, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | fill | path[1] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[2] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[3] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[4] fill mismatch | #FFFFFF | #000000 |  |


### Page: Page-1

# Divergence Report: Page-1 (ID: 0)

## Summary

- Total shapes: 5
- Identical: 4
- Divergent: 1
- Total diffs: 1

### By Severity

- warning: 1

### By Category

- bounds: 1

## Shape Details

### Shape 7

- Paths: legacy=1, rendertree=1
- Texts: legacy=0, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | bounds | viewBox y mismatch | -100.0000 | -0.0100 | 99.9900 |


### Page: Page-2

# Divergence Report: Page-2 (ID: 4)

## Summary

- Total shapes: 5
- Identical: 4
- Divergent: 1
- Total diffs: 2

### By Severity

- warning: 2

### By Category

- bounds: 1
- text: 1

## Shape Details

### Shape 7

- Paths: legacy=1, rendertree=1
- Texts: legacy=1, rendertree=1
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text[0] y position mismatch | 127.8600 | 27.8600 | 100.0000 |
| warning | bounds | viewBox y mismatch | -100.0000 | -0.0100 | 99.9900 |


### Page: Page-3

# Divergence Report: Page-3 (ID: 7)

## Summary

- Total shapes: 4
- Identical: 2
- Divergent: 2
- Total diffs: 21

### By Severity

- warning: 21

### By Category

- text: 2
- fill: 19

## Shape Details

### Shape 6 (Router)

- Paths: legacy=15, rendertree=15
- Texts: legacy=0, rendertree=1
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | fill | path[3] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[4] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[5] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[6] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[7] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[8] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[9] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[11] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[12] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[13] fill mismatch | #FFFFFF | #000000 |  |
| warning | text | text element count mismatch | 0 texts | 1 texts |  |

### Shape 1 (Switch)

- Paths: legacy=14, rendertree=14
- Texts: legacy=0, rendertree=1
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | fill | path[3] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[4] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[5] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[6] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[7] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[8] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[9] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[10] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[11] fill mismatch | #FFFFFF | #000000 |  |
| warning | text | text element count mismatch | 0 texts | 1 texts |  |


### Page: Dynamic connector

# Divergence Report: Dynamic connector (ID: 2)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 1

### By Severity

- warning: 1

### By Category

- bounds: 1

## Shape Details

### Shape 5

- Paths: legacy=1, rendertree=1
- Texts: legacy=0, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | bounds | viewBox y mismatch | -100.0000 | -0.0100 | 99.9900 |


### Page: Switch

# Divergence Report: Switch (ID: 6)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 9

### By Severity

- warning: 9

### By Category

- fill: 9

## Shape Details

### Shape 5

- Paths: legacy=14, rendertree=14
- Texts: legacy=0, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | fill | path[3] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[4] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[5] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[6] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[7] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[8] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[9] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[10] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[11] fill mismatch | #FFFFFF | #000000 |  |


### Page: Router

# Divergence Report: Router (ID: 7)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 10

### By Severity

- warning: 10

### By Category

- fill: 10

## Shape Details

### Shape 5

- Paths: legacy=15, rendertree=15
- Texts: legacy=0, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | fill | path[3] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[4] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[5] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[6] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[7] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[8] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[9] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[11] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[12] fill mismatch | #FFFFFF | #000000 |  |
| warning | fill | path[13] fill mismatch | #FFFFFF | #000000 |  |


### Page: Page 1

# Divergence Report: Page 1 (ID: 0)

## Summary

- Total shapes: 5
- Identical: 0
- Divergent: 5
- Total diffs: 12

### By Severity

- warning: 12

### By Category

- path: 3
- bounds: 1
- text: 8

## Shape Details

### Shape 6 (com.lucidchart.Line.6)

- Paths: legacy=2, rendertree=2
- Texts: legacy=1, rendertree=1
- Markers: legacy=1, rendertree=1

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | path | path[1].cmd[1] arg[0] mismatch | -96.4000 | -97.3100 | 0.9100 |
| warning | text | text[0] x position mismatch | -56.9600 | 50.0000 | 106.9600 |
| warning | text | text[0] y position mismatch | 53.8700 | 26.9400 | 26.9300 |
| warning | bounds | viewBox x mismatch | -103.1500 | -3.1500 | 100.0000 |

### Shape 5 (com.lucidchart.Line.5)

- Paths: legacy=2, rendertree=2
- Texts: legacy=1, rendertree=1
- Markers: legacy=1, rendertree=1

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | path | path[1].cmd[1] arg[0] mismatch | 96.4000 | 97.3100 | 0.9100 |
| warning | text | text[0] x position mismatch | 56.5100 | 50.0000 | 6.5100 |
| warning | text | text[0] y position mismatch | 53.8700 | 26.9400 | 26.9300 |

### Shape 7 (com.lucidchart.Line.7)

- Paths: legacy=2, rendertree=2
- Texts: legacy=1, rendertree=1
- Markers: legacy=1, rendertree=1

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | path | path[1].cmd[1] arg[0] mismatch | 96.4000 | 97.3100 | 0.9100 |
| warning | text | text[0] x position mismatch | 42.6000 | 50.0000 | 7.4000 |
| warning | text | text[0] y position mismatch | 53.8700 | 26.9400 | 26.9300 |

### Shape 1 (com.lucidchart.ProcessBlock.1)

- Paths: legacy=1, rendertree=1
- Texts: legacy=0, rendertree=1
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 0 texts | 1 texts |  |

### Shape 3 (com.lucidchart.ProcessBlock.3)

- Paths: legacy=1, rendertree=1
- Texts: legacy=0, rendertree=1
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 0 texts | 1 texts |  |


### Page: com.lucidchart.ProcessBlock21.f71da818dccc6284be74c036b29d4164

# Divergence Report: com.lucidchart.ProcessBlock21.f71da818dccc6284be74c036b29d4164 (ID: 1)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 1

### By Severity

- warning: 1

### By Category

- text: 1

## Shape Details

### Shape 5

- Paths: legacy=1, rendertree=1
- Texts: legacy=0, rendertree=1
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 0 texts | 1 texts |  |


### Page: Page-2

# Divergence Report: Page-2 (ID: 4)

## Summary

- Total shapes: 2
- Identical: 1
- Divergent: 1
- Total diffs: 5

### By Severity

- error: 2
- warning: 3

### By Category

- text: 5

## Shape Details

### Shape 3

- Paths: legacy=2, rendertree=2
- Texts: legacy=2, rendertree=3
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 2 texts | 3 texts |  |
| warning | text | text[0] y position mismatch | 19.7700 | 46.5100 | 26.7400 |
| error | text | text[0] content mismatch | Shape A | Container |  |
| warning | text | text[1] y position mismatch | 77.3300 | 19.7700 | 57.5600 |
| error | text | text[1] content mismatch | Shape B | Shape A |  |


### Page: Unknown

# Divergence Report: Unknown (ID: 6)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 1

### By Severity

- warning: 1

### By Category

- path: 1

## Shape Details

### Shape 5

- Paths: legacy=1, rendertree=0
- Texts: legacy=0, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | path | path count mismatch | 1 paths | 0 paths |  |


### Page: Page-1

# Divergence Report: Page-1 (ID: 0)

## Summary

- Total shapes: 7
- Identical: 6
- Divergent: 1
- Total diffs: 1

### By Severity

- warning: 1

### By Category

- bounds: 1

## Shape Details

### Shape 7 (Dynamic connector)

- Paths: legacy=1, rendertree=1
- Texts: legacy=0, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | bounds | viewBox x mismatch | -100.0000 | -0.0100 | 99.9900 |


### Page: Dynamic connector

# Divergence Report: Dynamic connector (ID: 2)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 1

### By Severity

- warning: 1

### By Category

- bounds: 1

## Shape Details

### Shape 5

- Paths: legacy=1, rendertree=1
- Texts: legacy=0, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | bounds | viewBox y mismatch | -100.0000 | -0.0100 | 99.9900 |


### Page: Page-1

# Divergence Report: Page-1 (ID: 0)

## Summary

- Total shapes: 3
- Identical: 2
- Divergent: 1
- Total diffs: 1

### By Severity

- warning: 1

### By Category

- text: 1

## Shape Details

### Shape 2

- Paths: legacy=1, rendertree=1
- Texts: legacy=1, rendertree=1
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text[0] y position mismatch | 27.5500 | 13.7800 | 13.7700 |


### Page: Dynamic connector

# Divergence Report: Dynamic connector (ID: 2)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 1

### By Severity

- warning: 1

### By Category

- bounds: 1

## Shape Details

### Shape 5

- Paths: legacy=1, rendertree=1
- Texts: legacy=0, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | bounds | viewBox y mismatch | -100.0000 | -0.0100 | 99.9900 |


### Page: Page-1

# Divergence Report: Page-1 (ID: 0)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 6

### By Severity

- error: 2
- warning: 4

### By Category

- text: 6

## Shape Details

### Shape 9

- Paths: legacy=2, rendertree=2
- Texts: legacy=2, rendertree=3
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 2 texts | 3 texts |  |
| warning | text | text[0] y position mismatch | 7.6700 | 22.3400 | 14.6700 |
| error | text | text[0] content mismatch | Loop test {{ o }}... | {% for o in test_... |  |
| warning | text | text[1] x position mismatch | 49.5000 | 50.0000 | 0.5000 |
| warning | text | text[1] y position mismatch | 39.1100 | 7.6700 | 31.4400 |
| error | text | text[1] content mismatch | In this instance,... | Loop test {{ o }}... |  |


### Page: Page-1

# Divergence Report: Page-1 (ID: 0)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 6

### By Severity

- error: 2
- warning: 4

### By Category

- text: 6

## Shape Details

### Shape 9

- Paths: legacy=2, rendertree=2
- Texts: legacy=2, rendertree=3
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 2 texts | 3 texts |  |
| warning | text | text[0] y position mismatch | 4.2800 | 19.8200 | 15.5400 |
| error | text | text[0] content mismatch | Loop test {{ o }}... | {% for o in test_... |  |
| warning | text | text[1] x position mismatch | 49.5000 | 50.0000 | 0.5000 |
| warning | text | text[1] y position mismatch | 29.0300 | 4.2800 | 24.7500 |
| error | text | text[1] content mismatch | In this instance,... | Loop test {{ o }}... |  |


### Page: Page-1

# Divergence Report: Page-1 (ID: 0)

## Summary

- Total shapes: 2
- Identical: 1
- Divergent: 1
- Total diffs: 6

### By Severity

- error: 2
- warning: 4

### By Category

- text: 6

## Shape Details

### Shape 9

- Paths: legacy=2, rendertree=2
- Texts: legacy=2, rendertree=3
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 2 texts | 3 texts |  |
| warning | text | text[0] y position mismatch | -1.0000 | 8.4500 | 9.4500 |
| error | text | text[0] content mismatch | Loop test {{ o }}... | {% for o in test_... |  |
| warning | text | text[1] x position mismatch | 49.5000 | 50.0000 | 0.5000 |
| warning | text | text[1] y position mismatch | 11.0800 | -1.0000 | 12.0800 |
| error | text | text[1] content mismatch | In this instance,... | Loop test {{ o }}... |  |


### Page: showif test

# Divergence Report: showif test (ID: 4)

## Summary

- Total shapes: 3
- Identical: 0
- Divergent: 3
- Total diffs: 7

### By Severity

- error: 2
- warning: 5

### By Category

- text: 7

## Shape Details

### Shape 4

- Paths: legacy=1, rendertree=1
- Texts: legacy=1, rendertree=2
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 1 texts | 2 texts |  |
| warning | text | text[0] y position mismatch | 12.0500 | 0.0000 | 12.0500 |
| error | text | text[0] content mismatch | Only here if x is... | {% showif x &gt; ... |  |

### Shape 5

- Paths: legacy=1, rendertree=1
- Texts: legacy=1, rendertree=2
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 1 texts | 2 texts |  |
| warning | text | text[0] y position mismatch | 9.2900 | 0.0000 | 9.2900 |
| error | text | text[0] content mismatch | Only here if x is... | {% showif x %} |  |

### Shape 3

- Paths: legacy=1, rendertree=1
- Texts: legacy=1, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 1 texts | 0 texts |  |


### Page: Page-1

# Divergence Report: Page-1 (ID: 0)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 1

### By Severity

- warning: 1

### By Category

- text: 1

## Shape Details

### Shape 1

- Paths: legacy=10, rendertree=10
- Texts: legacy=1, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 1 texts | 0 texts |  |


### Page: AWS Step Functions workflow 

# Divergence Report: AWS Step Functions workflow  (ID: 2)

## Summary

- Total shapes: 1
- Identical: 0
- Divergent: 1
- Total diffs: 1

### By Severity

- warning: 1

### By Category

- text: 1

## Shape Details

### Shape 5

- Paths: legacy=10, rendertree=10
- Texts: legacy=1, rendertree=0
- Markers: legacy=0, rendertree=0

| Severity | Category | Message | Expected | Actual | Delta |
|----------|----------|---------|----------|--------|-------|
| warning | text | text element count mismatch | 1 texts | 0 texts |  |


