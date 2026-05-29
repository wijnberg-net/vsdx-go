// comprehensive-gen builds a feature-rich VSDX intended to be opened in
// Microsoft Visio 2021, saved (which produces Visio's canonical XML), and
// exported to SVG. Three round-trip signals come out of that workflow:
//
//  1. vsdx-go's own SVG (rendered from THIS output)  vs. Visio's SVG export
//     → tells us about renderer fidelity per feature.
//  2. vsdx-go opening Visio's resaved .vsdx          vs. Visio's SVG export
//     → tells us whether our reader interprets Visio's canonical XML the
//     same way Visio renders it.
//  3. Byte-diff THIS output vs. Visio's resaved file
//     → tells us where our writer's XML deviates from Visio's canonical form.
//
// Each shape's TEXT is set to its feature-name so a later coverage tool can
// identify which feature each shape demonstrates without out-of-band labels.
//
// The output is intentionally LARGE (~9 pages, ~150 shapes). Layout is rough:
// shapes are placed on a grid per page; aesthetics don't matter.
package main

import (
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/beevik/etree"
	"wijnberg.net/vsdx-go/vsdx"
)

const (
	pageW = 11.0
	pageH = 8.5
)

// gridCell positions shape (col, row) on a regular grid.
// Visio Y-up: row 0 is at the top.
func gridCell(col, row int, cellW, cellH, marginX, marginTop float64) (float64, float64) {
	x := marginX + float64(col)*cellW + cellW/2
	y := pageH - marginTop - float64(row)*cellH - cellH/2
	return x, y
}

// baseShape drops a rectangular shape with neutral fill+border at the given
// position and writes `label` as its text. Returns the shape for chaining.
func baseShape(p *vsdx.Page, cx, cy, w, h float64, label string) *vsdx.Shape {
	s := p.AddShape()
	s.SetX(cx)
	s.SetY(cy)
	s.SetWidth(w)
	s.SetHeight(h)
	s.SetLocX(w / 2)
	s.SetLocY(h / 2)
	s.AddGeometryRect()
	s.SetFillColor("#f0f4ff")
	s.SetLineColor("#333333")
	s.SetLineWeight(0.014)
	s.SetText(label)
	return s
}

// borderedShape: like baseShape but builds the rectangle geometry manually
// (without NoLine=1), so the shape's LinePattern / LineWeight / LineCap are
// actually visible. Use this on pages that demonstrate line styling.
func borderedShape(p *vsdx.Page, sl slot, label string) *vsdx.Shape {
	s := p.AddShape()
	s.SetX(sl.cx)
	s.SetY(sl.cy)
	s.SetWidth(sl.w)
	s.SetHeight(sl.h)
	s.SetLocX(sl.w / 2)
	s.SetLocY(sl.h / 2)
	g := s.AddGeometry()
	g.AddMoveTo(0, 0)
	g.AddLineTo(sl.w, 0)
	g.AddLineTo(sl.w, sl.h)
	g.AddLineTo(0, sl.h)
	g.AddLineTo(0, 0)
	s.SetFillColor("#ffffff")
	s.SetLineColor("#333333")
	s.SetLineWeight(0.025)
	s.SetText(label)
	return s
}

// setPageSize stamps PageWidth/PageHeight on the page. We need the
// PageSheet inside pages.xml (Visio reads dimensions from there, not from
// each page's content XML); vsdx-go's Page.SetWidth/SetHeight already
// resolve to that location.
func setPageSize(p *vsdx.Page, w, h float64) {
	p.SetWidth(w)
	p.SetHeight(h)
}

// ---------- Page 1: Shapes & Geometry ----------

func buildShapesPage(v *vsdx.VisioFile) {
	p, err := v.AddPage("Shapes")
	if err != nil {
		fatal("AddPage: %v", err)
	}
	setPageSize(p, pageW, pageH)

	cellW, cellH := 2.0, 1.4
	mx, mt := 0.4, 0.4

	// Row 0: basic primitives via baseShape (rect)
	_baseShape(p, gx(0, 0, cellW, cellH, mx, mt), "shape-rectangle")
	addRoundedRect(p, gx(1, 0, cellW, cellH, mx, mt), "shape-rounded-rectangle", 0.15)
	addEllipse(p, gx(2, 0, cellW, cellH, mx, mt), "shape-ellipse")
	addPolygon(p, gx(3, 0, cellW, cellH, mx, mt), "shape-triangle", 3)
	addPolygon(p, gx(4, 0, cellW, cellH, mx, mt), "shape-pentagon", 5)

	// Row 1: polygons + star + diamond
	addPolygon(p, gx(0, 1, cellW, cellH, mx, mt), "shape-hexagon", 6)
	addPolygon(p, gx(1, 1, cellW, cellH, mx, mt), "shape-octagon", 8)
	addStar(p, gx(2, 1, cellW, cellH, mx, mt), "shape-star-5point", 5)
	addStar(p, gx(3, 1, cellW, cellH, mx, mt), "shape-star-6point", 6)
	addDiamond(p, gx(4, 1, cellW, cellH, mx, mt), "shape-diamond")

	// Row 2: multi-geometry icon + group + arrow-block
	addMultiGeomIcon(p, gx(0, 2, cellW, cellH, mx, mt), "shape-icon-multigeom")
	addGroupShape(p, gx(1, 2, cellW, cellH, mx, mt), "shape-group-3children")
	addBlockArrow(p, gx(2, 2, cellW, cellH, mx, mt), "shape-arrow-block")
	addCross(p, gx(3, 2, cellW, cellH, mx, mt), "shape-cross")
	addLightning(p, gx(4, 2, cellW, cellH, mx, mt), "shape-lightning")
}

// slot bundles a grid-cell position and size so per-shape helpers don't
// need 4 numeric arguments. gx() returns one.
type slot struct {
	cx, cy, w, h float64
}

func gx(col, row int, cellW, cellH, mx, mt float64) slot {
	cx, cy := gridCell(col, row, cellW, cellH, mx, mt)
	return slot{cx, cy, cellW * 0.75, cellH * 0.65}
}

// _baseShape: slot-based variant of baseShape.
func _baseShape(p *vsdx.Page, sl slot, label string) *vsdx.Shape {
	return baseShape(p, sl.cx, sl.cy, sl.w, sl.h, label)
}

// Use the slot-style helpers everywhere:
func addRoundedRect(p *vsdx.Page, sl slot, label string, rounding float64) *vsdx.Shape {
	s := _baseShape(p, sl, label)
	s.SetRounding(rounding)
	return s
}

func addEllipse(p *vsdx.Page, sl slot, label string) *vsdx.Shape {
	s := p.AddShape()
	s.SetX(sl.cx)
	s.SetY(sl.cy)
	s.SetWidth(sl.w)
	s.SetHeight(sl.h)
	s.SetLocX(sl.w / 2)
	s.SetLocY(sl.h / 2)
	g := s.AddGeometry()
	// Ellipse(x, y, a, b, c, d): center (x,y), semi-axis a in X at (a, y),
	// semi-axis b in Y at (x, b)
	g.AddEllipse(sl.w/2, sl.h/2, sl.w, sl.h/2, sl.w/2, 0)
	s.SetFillColor("#cfe2ff")
	s.SetLineColor("#333333")
	s.SetLineWeight(0.014)
	s.SetText(label)
	return s
}

// addPolygon builds an n-sided regular polygon inscribed in the shape bbox.
func addPolygon(p *vsdx.Page, sl slot, label string, n int) *vsdx.Shape {
	s := p.AddShape()
	s.SetX(sl.cx)
	s.SetY(sl.cy)
	s.SetWidth(sl.w)
	s.SetHeight(sl.h)
	s.SetLocX(sl.w / 2)
	s.SetLocY(sl.h / 2)
	g := s.AddGeometry()
	cx, cy := sl.w/2, sl.h/2
	rx, ry := sl.w*0.45, sl.h*0.45
	for i := 0; i <= n; i++ {
		a := math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		x := cx + rx*math.Cos(a)
		y := cy + ry*math.Sin(a)
		if i == 0 {
			g.AddMoveTo(x, y)
		} else {
			g.AddLineTo(x, y)
		}
	}
	s.SetFillColor("#d4ecd4")
	s.SetLineColor("#333333")
	s.SetLineWeight(0.014)
	s.SetText(label)
	return s
}

// addStar builds an N-pointed star.
func addStar(p *vsdx.Page, sl slot, label string, points int) *vsdx.Shape {
	s := p.AddShape()
	s.SetX(sl.cx)
	s.SetY(sl.cy)
	s.SetWidth(sl.w)
	s.SetHeight(sl.h)
	s.SetLocX(sl.w / 2)
	s.SetLocY(sl.h / 2)
	g := s.AddGeometry()
	cx, cy := sl.w/2, sl.h/2
	rOuter := math.Min(sl.w, sl.h) * 0.45
	rInner := rOuter * 0.4
	n := points * 2
	for i := 0; i <= n; i++ {
		a := math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		r := rOuter
		if i%2 == 1 {
			r = rInner
		}
		x := cx + r*math.Cos(a)
		y := cy + r*math.Sin(a)
		if i == 0 {
			g.AddMoveTo(x, y)
		} else {
			g.AddLineTo(x, y)
		}
	}
	s.SetFillColor("#ffe8a0")
	s.SetLineColor("#333333")
	s.SetLineWeight(0.014)
	s.SetText(label)
	return s
}

func addDiamond(p *vsdx.Page, sl slot, label string) *vsdx.Shape {
	s := p.AddShape()
	s.SetX(sl.cx)
	s.SetY(sl.cy)
	s.SetWidth(sl.w)
	s.SetHeight(sl.h)
	s.SetLocX(sl.w / 2)
	s.SetLocY(sl.h / 2)
	g := s.AddGeometry()
	g.AddMoveTo(sl.w/2, 0)
	g.AddLineTo(sl.w, sl.h/2)
	g.AddLineTo(sl.w/2, sl.h)
	g.AddLineTo(0, sl.h/2)
	g.AddLineTo(sl.w/2, 0)
	s.SetFillColor("#ffd4ec")
	s.SetLineColor("#333333")
	s.SetLineWeight(0.014)
	s.SetText(label)
	return s
}

// addMultiGeomIcon: one shape with TWO geometry sections — outer rectangle
// + inner cross. Exercises multi-geometry rendering / OrderIndex z-order.
func addMultiGeomIcon(p *vsdx.Page, sl slot, label string) *vsdx.Shape {
	s := p.AddShape()
	s.SetX(sl.cx)
	s.SetY(sl.cy)
	s.SetWidth(sl.w)
	s.SetHeight(sl.h)
	s.SetLocX(sl.w / 2)
	s.SetLocY(sl.h / 2)
	s.AddGeometryRect()
	// Second geometry: an X inside.
	g2 := s.AddGeometry()
	g2.AddMoveTo(0.1*sl.w, 0.1*sl.h)
	g2.AddLineTo(0.9*sl.w, 0.9*sl.h)
	g2.AddMoveTo(0.9*sl.w, 0.1*sl.h)
	g2.AddLineTo(0.1*sl.w, 0.9*sl.h)
	s.SetFillColor("#bfdfff")
	s.SetLineColor("#003366")
	s.SetLineWeight(0.02)
	s.SetText(label)
	return s
}

func addGroupShape(p *vsdx.Page, sl slot, label string) *vsdx.Shape {
	// Three small shapes, then GroupShapes them.
	mk := func(dx, dy float64, color string) *vsdx.Shape {
		s := p.AddShape()
		s.SetX(sl.cx + dx)
		s.SetY(sl.cy + dy)
		s.SetWidth(sl.w * 0.3)
		s.SetHeight(sl.h * 0.4)
		s.SetLocX(sl.w * 0.15)
		s.SetLocY(sl.h * 0.2)
		s.AddGeometryRect()
		s.SetFillColor(color)
		s.SetLineColor("#222222")
		s.SetLineWeight(0.012)
		return s
	}
	a := mk(-sl.w*0.25, sl.h*0.15, "#ff9966")
	b := mk(sl.w*0.25, sl.h*0.15, "#66ccff")
	c := mk(0, -sl.h*0.2, "#99dd66")
	g := p.GroupShapes([]*vsdx.Shape{a, b, c}, 0.02)
	if g != nil {
		g.SetText(label)
	}
	return g
}

func addBlockArrow(p *vsdx.Page, sl slot, label string) *vsdx.Shape {
	s := p.AddShape()
	s.SetX(sl.cx)
	s.SetY(sl.cy)
	s.SetWidth(sl.w)
	s.SetHeight(sl.h)
	s.SetLocX(sl.w / 2)
	s.SetLocY(sl.h / 2)
	g := s.AddGeometry()
	w, h := sl.w, sl.h
	// Block arrow pointing right (concave on left, pointy on right).
	g.AddMoveTo(0, 0.3*h)
	g.AddLineTo(0.6*w, 0.3*h)
	g.AddLineTo(0.6*w, 0.1*h)
	g.AddLineTo(w, 0.5*h)
	g.AddLineTo(0.6*w, 0.9*h)
	g.AddLineTo(0.6*w, 0.7*h)
	g.AddLineTo(0, 0.7*h)
	g.AddLineTo(0, 0.3*h)
	s.SetFillColor("#ffcc99")
	s.SetLineColor("#222222")
	s.SetLineWeight(0.014)
	s.SetText(label)
	return s
}

func addCross(p *vsdx.Page, sl slot, label string) *vsdx.Shape {
	s := p.AddShape()
	s.SetX(sl.cx)
	s.SetY(sl.cy)
	s.SetWidth(sl.w)
	s.SetHeight(sl.h)
	s.SetLocX(sl.w / 2)
	s.SetLocY(sl.h / 2)
	g := s.AddGeometry()
	w, h := sl.w, sl.h
	g.AddMoveTo(0.35*w, 0)
	g.AddLineTo(0.65*w, 0)
	g.AddLineTo(0.65*w, 0.35*h)
	g.AddLineTo(w, 0.35*h)
	g.AddLineTo(w, 0.65*h)
	g.AddLineTo(0.65*w, 0.65*h)
	g.AddLineTo(0.65*w, h)
	g.AddLineTo(0.35*w, h)
	g.AddLineTo(0.35*w, 0.65*h)
	g.AddLineTo(0, 0.65*h)
	g.AddLineTo(0, 0.35*h)
	g.AddLineTo(0.35*w, 0.35*h)
	g.AddLineTo(0.35*w, 0)
	s.SetFillColor("#ffaaaa")
	s.SetLineColor("#222222")
	s.SetLineWeight(0.014)
	s.SetText(label)
	return s
}

func addLightning(p *vsdx.Page, sl slot, label string) *vsdx.Shape {
	s := p.AddShape()
	s.SetX(sl.cx)
	s.SetY(sl.cy)
	s.SetWidth(sl.w)
	s.SetHeight(sl.h)
	s.SetLocX(sl.w / 2)
	s.SetLocY(sl.h / 2)
	g := s.AddGeometry()
	w, h := sl.w, sl.h
	g.AddMoveTo(0.5*w, h)
	g.AddLineTo(0.2*w, 0.55*h)
	g.AddLineTo(0.4*w, 0.55*h)
	g.AddLineTo(0.25*w, 0)
	g.AddLineTo(0.65*w, 0.45*h)
	g.AddLineTo(0.45*w, 0.45*h)
	g.AddLineTo(0.5*w, h)
	s.SetFillColor("#fff066")
	s.SetLineColor("#222222")
	s.SetLineWeight(0.014)
	s.SetText(label)
	return s
}

// ---------- Page 2: Fills ----------

func buildFillsPage(v *vsdx.VisioFile) {
	p, err := v.AddPage("Fills")
	if err != nil {
		fatal("AddPage: %v", err)
	}
	setPageSize(p, pageW, pageH)

	cellW, cellH := 2.0, 1.0
	mx, mt := 0.4, 0.4

	// Row 0: solid colors
	s := _baseShape(p, gx(0, 0, cellW, cellH, mx, mt), "fill-solid-red")
	s.SetFillColor("#cc0000")
	s = _baseShape(p, gx(1, 0, cellW, cellH, mx, mt), "fill-solid-blue")
	s.SetFillColor("#0044cc")
	s = _baseShape(p, gx(2, 0, cellW, cellH, mx, mt), "fill-solid-green")
	s.SetFillColor("#22aa44")
	s = _baseShape(p, gx(3, 0, cellW, cellH, mx, mt), "fill-solid-rgb-custom")
	s.SetFillColor("#eaa759")
	s = _baseShape(p, gx(4, 0, cellW, cellH, mx, mt), "fill-none-outline-only")
	s.SetFillPattern(0)

	// Row 1: gradients (raw cell write because no SetFillGradient API exists)
	addFillGradient(p, gx(0, 1, cellW, cellH, mx, mt), "fill-gradient-linear-2stops-horiz", 0,
		[]vsdx.GradientStop{{Position: 0, Color: "#ff0000"}, {Position: 1, Color: "#0000ff"}})
	addFillGradient(p, gx(1, 1, cellW, cellH, mx, mt), "fill-gradient-linear-2stops-vert", math.Pi/2,
		[]vsdx.GradientStop{{Position: 0, Color: "#ff0000"}, {Position: 1, Color: "#0000ff"}})
	addFillGradient(p, gx(2, 1, cellW, cellH, mx, mt), "fill-gradient-linear-3stops-diag", math.Pi/4,
		[]vsdx.GradientStop{{Position: 0, Color: "#ff0000"}, {Position: 0.5, Color: "#ffff00"}, {Position: 1, Color: "#00aa00"}})
	addFillGradient(p, gx(3, 1, cellW, cellH, mx, mt), "fill-gradient-linear-4stops", 0,
		[]vsdx.GradientStop{
			{Position: 0, Color: "#ff0000"},
			{Position: 0.33, Color: "#ffff00"},
			{Position: 0.66, Color: "#00ff00"},
			{Position: 1, Color: "#0000ff"},
		})
	addFillGradientRadial(p, gx(4, 1, cellW, cellH, mx, mt), "fill-gradient-radial",
		[]vsdx.GradientStop{{Position: 0, Color: "#ffff00"}, {Position: 1, Color: "#cc0000"}})

	// Rows 2-3: patterns 1-8 (Visio's standard hatches)
	patternLabels := []string{
		"fill-pattern-2", "fill-pattern-3", "fill-pattern-4", "fill-pattern-5",
		"fill-pattern-6", "fill-pattern-7", "fill-pattern-8", "fill-pattern-9",
		"fill-pattern-25-fine-dotted", "fill-pattern-26-medium-dotted",
	}
	patternIDs := []int{2, 3, 4, 5, 6, 7, 8, 9, 25, 26}
	for i, label := range patternLabels {
		col, row := i%5, 2+i/5
		s := _baseShape(p, gx(col, row, cellW, cellH, mx, mt), label)
		s.SetFillColor("#3366cc")
		s.SetFillBkgndColor("#ffffff")
		s.SetFillPattern(patternIDs[i])
	}

	// Row 4: transparency
	s = _baseShape(p, gx(0, 4, cellW, cellH, mx, mt), "fill-transparent-25pct")
	s.SetFillColor("#cc0000")
	s.SetFillTransparency(0.25)
	s = _baseShape(p, gx(1, 4, cellW, cellH, mx, mt), "fill-transparent-50pct")
	s.SetFillColor("#cc0000")
	s.SetFillTransparency(0.5)
	s = _baseShape(p, gx(2, 4, cellW, cellH, mx, mt), "fill-transparent-75pct")
	s.SetFillColor("#cc0000")
	s.SetFillTransparency(0.75)
	s = _baseShape(p, gx(3, 4, cellW, cellH, mx, mt), "fill-bkgnd-yellow-pattern-2")
	s.SetFillColor("#0044cc")
	s.SetFillBkgndColor("#ffff00")
	s.SetFillPattern(2)
}

// addFillGradient writes raw cells + a FillGradient section. Mirrors
// SetLineGradient's structure (see linegradient.go:80).
func addFillGradient(p *vsdx.Page, sl slot, label string, angle float64, stops []vsdx.GradientStop) *vsdx.Shape {
	s := _baseShape(p, sl, label)
	s.SetCellValue("FillGradientEnabled", "1")
	// FillGradientDir=0 (linear) is the default; Visio strips it on
	// resave. Omitting it here mirrors that canonical form.
	s.SetCellValue("FillGradientAngle", fmtFloat(angle))
	addGradientSection(s, "FillGradient", stops)
	return s
}

func addFillGradientRadial(p *vsdx.Page, sl slot, label string, stops []vsdx.GradientStop) *vsdx.Shape {
	s := _baseShape(p, sl, label)
	s.SetCellValue("FillGradientEnabled", "1")
	s.SetCellValue("FillGradientDir", "7") // radial
	addGradientSection(s, "FillGradient", stops)
	return s
}

func addGradientSection(s *vsdx.Shape, sectionName string, stops []vsdx.GradientStop) {
	root := s.XML()
	if root == nil {
		return
	}
	// Remove existing section
	for _, sec := range root.SelectElements("Section") {
		if sec.SelectAttrValue("N", "") == sectionName {
			root.RemoveChild(sec)
		}
	}
	section := root.CreateElement("Section")
	section.CreateAttr("N", sectionName)
	for i, stop := range stops {
		row := section.CreateElement("Row")
		row.CreateAttr("IX", strconv.Itoa(i))

		c := row.CreateElement("Cell")
		c.CreateAttr("N", "GradientStopColor")
		c.CreateAttr("V", stop.Color)

		// Visio's canonical resave omits Position when its value is 0
		// (the implicit default for the first stop) and omits ColorTrans
		// when 0. Mirror that to avoid emitting no-op cells.
		if stop.Position != 0 {
			c = row.CreateElement("Cell")
			c.CreateAttr("N", "GradientStopColorTrans")
			c.CreateAttr("V", fmtFloat(stop.Trans))
			c = row.CreateElement("Cell")
			c.CreateAttr("N", "GradientStopPosition")
			c.CreateAttr("V", fmtFloat(stop.Position))
		} else if stop.Trans != 0 {
			c = row.CreateElement("Cell")
			c.CreateAttr("N", "GradientStopColorTrans")
			c.CreateAttr("V", fmtFloat(stop.Trans))
		}
	}
}

// ---------- Page 3: Lines ----------

func buildLinesPage(v *vsdx.VisioFile) {
	p, err := v.AddPage("Lines")
	if err != nil {
		fatal("AddPage: %v", err)
	}
	setPageSize(p, pageW, pageH)

	cellW, cellH := 2.0, 0.7
	mx, mt := 0.4, 0.4

	// Row 0-1: line patterns 1-10 (Visio supports 1-23). Use borderedShape
	// so the dashed/dotted border is actually visible — _baseShape uses
	// AddGeometryRect which sets NoLine=1 on the geometry, hiding the
	// border regardless of the shape-level LineColor/LinePattern.
	for i, pat := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
		col, row := i%5, i/5
		s := borderedShape(p, gx(col, row, cellW, cellH, mx, mt),
			fmt.Sprintf("line-pattern-%02d", pat))
		s.SetLinePattern(pat)
	}

	// Row 2: weights
	weights := []float64{0.005, 0.014, 0.028, 0.055, 0.11}
	weightLabels := []string{
		"line-weight-0.25pt", "line-weight-1pt", "line-weight-2pt",
		"line-weight-4pt", "line-weight-8pt",
	}
	for i, w := range weights {
		s := borderedShape(p, gx(i, 2, cellW, cellH, mx, mt), weightLabels[i])
		s.SetLineWeight(w)
	}

	// Row 3: line caps + line gradient + color
	s := borderedShape(p, gx(0, 3, cellW, cellH, mx, mt), "line-cap-round")
	s.SetLineWeight(0.06)
	s.SetLineCap(vsdx.LineCapRound)
	s = borderedShape(p, gx(1, 3, cellW, cellH, mx, mt), "line-cap-square")
	s.SetLineWeight(0.06)
	s.SetLineCap(vsdx.LineCapSquare)
	s = borderedShape(p, gx(2, 3, cellW, cellH, mx, mt), "line-cap-extended")
	s.SetLineWeight(0.06)
	s.SetLineCap(vsdx.LineCapExtended)

	s = borderedShape(p, gx(3, 3, cellW, cellH, mx, mt), "line-gradient-red-to-blue")
	s.SetLineWeight(0.06)
	s.SetLineGradient(0,
		[]vsdx.GradientStop{{Position: 0, Color: "#cc0000"}, {Position: 1, Color: "#0044cc"}})

	s = borderedShape(p, gx(4, 3, cellW, cellH, mx, mt), "line-color-custom-rgb")
	s.SetLineWeight(0.04)
	s.SetLineColor("#ff8800")
}

// ---------- Page 4: Arrows ----------

func buildArrowsPage(v *vsdx.VisioFile) {
	p, err := v.AddPage("Arrows")
	if err != nil {
		fatal("AddPage: %v", err)
	}
	setPageSize(p, pageW, pageH)

	cellW, cellH := 1.6, 0.5
	mx := 0.3

	// Each arrow demo is a REAL connector built via ConnectShapes (so it
	// gets the Dynamic Connector master attached). Standalone 1D shapes
	// with BeginX/EndX but no master are not classified as connectors by
	// Visio's SVG exporter, and arrows get dropped — that's why an
	// earlier orphan-shape variant exported with no markers at all.

	// Rows 0-3: arrow types 1-24 with end-arrow only
	for i := 1; i <= 24; i++ {
		col := (i - 1) % 6
		row := (i - 1) / 6
		x := mx + float64(col)*cellW + cellW/2
		y := pageH - 0.4 - float64(row)*cellH - cellH/2
		addArrowConnector(v, p, x, y, cellW*0.85,
			fmt.Sprintf("arrow-end-%02d", i), 0, i, 2)
	}

	// Row 4: arrows on both ends (selected combos)
	combos := []struct{ b, e int }{
		{1, 1}, {3, 3}, {5, 5}, {13, 13}, {39, 39}, {44, 44},
	}
	for i, c := range combos {
		col := i % 6
		row := 4
		x := mx + float64(col)*cellW + cellW/2
		y := pageH - 0.4 - float64(row)*cellH - cellH/2
		addArrowConnector(v, p, x, y, cellW*0.85,
			fmt.Sprintf("arrow-both-b%02d-e%02d", c.b, c.e),
			c.b, c.e, 2)
	}

	// Row 5: arrow sizes (size 0=xs, 1=s, ..., 5=xxl)
	sizes := []int{0, 1, 2, 3, 4, 5}
	for i, sz := range sizes {
		x := mx + float64(i)*cellW + cellW/2
		y := pageH - 0.4 - 5*cellH - cellH/2
		addArrowConnector(v, p, x, y, cellW*0.85,
			fmt.Sprintf("arrow-size-%d", sz), 0, 13, sz)
	}

	// Row 6: curved connector demos. ConnectShapes auto-routes between
	// the anchors; for variety we just place anchors further apart.
	for i, sz := range []int{2, 4} {
		x := mx + float64(i)*cellW*2 + cellW
		y := pageH - 0.4 - 6*cellH - cellH/2
		addArrowConnector(v, p, x, y, cellW*1.8,
			fmt.Sprintf("arrow-curved-size%d", sz), 0, 13, sz)
	}
}

// addArrowConnector spans (cx - length/2, cy) → (cx + length/2, cy) as a
// real Visio connector. Two invisible anchor shapes are dropped at the
// endpoints and ConnectShapes builds the connector between them; arrow
// type, size, and identifying text are applied to the resulting connector.
// Returns the connector shape.
//
// The anchor shapes are 0.001"×0.001", with FillPattern=0 and a geometry
// section explicitly marked NoFill=1 / NoLine=1 — visually inert but
// large enough that Visio still allocates connection points.
func addArrowConnector(v *vsdx.VisioFile, p *vsdx.Page, cx, cy, length float64,
	label string, beginArrow, endArrow, size int) *vsdx.Shape {
	mkAnchor := func(x, y float64) *vsdx.Shape {
		a := p.AddShape()
		a.SetX(x)
		a.SetY(y)
		a.SetWidth(0.001)
		a.SetHeight(0.001)
		a.SetLocX(0.0005)
		a.SetLocY(0.0005)
		a.AddGeometryRect()
		a.SetFillPattern(0)
		a.SetCellValue("LinePattern", "0")
		return a
	}
	src := mkAnchor(cx-length/2, cy)
	dst := mkAnchor(cx+length/2, cy)
	conn, err := v.ConnectShapes(p, src, dst)
	if err != nil {
		fatal("ConnectShapes(%s): %v", label, err)
	}
	conn.SetLineColor("#222222")
	conn.SetLineWeight(0.022)
	// Visio's resave strips BeginArrowSize / EndArrowSize when the size
	// equals the stylesheet default of 2. Mirror that to avoid carrying
	// no-op cells across round-trips.
	if beginArrow > 0 {
		conn.SetBeginArrow(beginArrow)
		if size != 2 {
			conn.SetCellValue("BeginArrowSize", strconv.Itoa(size))
		}
	}
	if endArrow > 0 {
		conn.SetEndArrow(endArrow)
		if size != 2 {
			conn.SetCellValue("EndArrowSize", strconv.Itoa(size))
		}
	}
	conn.SetText(label)
	return conn
}

// ---------- Page 5: Text ----------

func buildTextPage(v *vsdx.VisioFile) {
	p, err := v.AddPage("Text")
	if err != nil {
		fatal("AddPage: %v", err)
	}
	setPageSize(p, pageW, pageH)

	cellW, cellH := 2.0, 0.9
	mx, mt := 0.4, 0.4

	// Row 0: plain + multi-line + fonts
	s := _baseShape(p, gx(0, 0, cellW, cellH, mx, mt), "text-plain-singleline")
	_ = s
	s = _baseShape(p, gx(1, 0, cellW, cellH, mx, mt), "text-multiline\nLine 2\nLine 3")
	s = _baseShape(p, gx(2, 0, cellW, cellH, mx, mt), "text-font-arial")
	s.SetCharFont("Arial")
	s = _baseShape(p, gx(3, 0, cellW, cellH, mx, mt), "text-font-times")
	s.SetCharFont("Times New Roman")
	s = _baseShape(p, gx(4, 0, cellW, cellH, mx, mt), "text-font-courier")
	s.SetCharFont("Courier New")

	// Row 1: sizes
	for i, pt := range []float64{8, 12, 18, 24, 36} {
		s := _baseShape(p, gx(i, 1, cellW, cellH, mx, mt),
			fmt.Sprintf("text-size-%.0fpt", pt))
		s.SetCharSize(pt)
	}

	// Row 2: styles
	s = _baseShape(p, gx(0, 2, cellW, cellH, mx, mt), "text-style-bold")
	s.SetCharBold(true)
	s = _baseShape(p, gx(1, 2, cellW, cellH, mx, mt), "text-style-italic")
	s.SetCharItalic(true)
	s = _baseShape(p, gx(2, 2, cellW, cellH, mx, mt), "text-style-underline")
	s.SetCharUnderline(true)
	s = _baseShape(p, gx(3, 2, cellW, cellH, mx, mt), "text-style-bold-italic")
	s.SetCharBold(true)
	s.SetCharItalic(true)
	s = _baseShape(p, gx(4, 2, cellW, cellH, mx, mt), "text-color-red")
	s.SetTextColor("#cc0000")

	// Row 3: rotation / vertical
	s = _baseShape(p, gx(0, 3, cellW, cellH, mx, mt), "text-rotation-vertical-90")
	s.SetTxtAngle(math.Pi / 2)
	s = _baseShape(p, gx(1, 3, cellW, cellH, mx, mt), "text-rotation-vertical-270")
	s.SetTxtAngle(3 * math.Pi / 2)
	s = _baseShape(p, gx(2, 3, cellW, cellH, mx, mt), "text-rotation-45deg")
	s.SetTxtAngle(math.Pi / 4)
	s = _baseShape(p, gx(3, 3, cellW, cellH, mx, mt), "text-rotation-180")
	s.SetTxtAngle(math.Pi)
	s = _baseShape(p, gx(4, 3, cellW, cellH, mx, mt), "text-rotation-30")
	s.SetTxtAngle(math.Pi / 6)

	// Row 4: alignment
	for i, a := range []int{0, 1, 2, 3} {
		labels := []string{"left", "center", "right", "justify"}
		s := _baseShape(p, gx(i, 4, cellW, cellH, mx, mt),
			"text-align-"+labels[a]+"\nsecond line\nthird line")
		s.SetParagraphAlign(a)
	}

	// Row 4 col 4: rich text with cp markers
	addRichText(p, gx(4, 4, cellW, cellH, mx, mt), "text-rich-mixed-formatting")
}

// addRichText injects multiple cp markers + a Character section with multiple
// rows so the same shape's text has mixed bold/italic/color formatting.
func addRichText(p *vsdx.Page, sl slot, label string) *vsdx.Shape {
	s := _baseShape(p, sl, label)
	root := s.XML()
	if root == nil {
		return s
	}
	// Add a Character section with 3 rows (default + bold + italic+color).
	for _, sec := range root.SelectElements("Section") {
		if sec.SelectAttrValue("N", "") == "Character" {
			root.RemoveChild(sec)
		}
	}
	charSec := root.CreateElement("Section")
	charSec.CreateAttr("N", "Character")
	addCharRow(charSec, 0, false, false, "")
	addCharRow(charSec, 1, true, false, "")
	addCharRow(charSec, 2, false, true, "#cc0000")

	// Rebuild Text element: "label " + cp1 + "BOLD " + cp2 + "italic-red "
	for _, t := range root.SelectElements("Text") {
		root.RemoveChild(t)
	}
	textEl := root.CreateElement("Text")
	textEl.SetText(label + " ")
	cp1 := textEl.CreateElement("cp")
	cp1.CreateAttr("IX", "1")
	cp1.SetTail("BOLD ")
	cp2 := textEl.CreateElement("cp")
	cp2.CreateAttr("IX", "2")
	cp2.SetTail("italic-red")
	return s
}

func addCharRow(sec *etree.Element, ix int, bold, italic bool, color string) {
	row := sec.CreateElement("Row")
	row.CreateAttr("IX", strconv.Itoa(ix))
	style := 0
	if bold {
		style |= 1
	}
	if italic {
		style |= 2
	}
	if style != 0 {
		c := row.CreateElement("Cell")
		c.CreateAttr("N", "Style")
		c.CreateAttr("V", strconv.Itoa(style))
	}
	if color != "" {
		c := row.CreateElement("Cell")
		c.CreateAttr("N", "Color")
		c.CreateAttr("V", color)
	}
}

// ---------- Page 6: Transforms ----------

func buildTransformsPage(v *vsdx.VisioFile) {
	p, err := v.AddPage("Transforms")
	if err != nil {
		fatal("AddPage: %v", err)
	}
	setPageSize(p, pageW, pageH)

	cellW, cellH := 2.0, 1.5
	mx, mt := 0.4, 0.4

	angles := []struct {
		label string
		deg   float64
	}{
		{"transform-rotation-30", 30},
		{"transform-rotation-45", 45},
		{"transform-rotation-90", 90},
		{"transform-rotation-135", 135},
		{"transform-rotation-180", 180},
	}
	for i, a := range angles {
		s := _baseShape(p, gx(i, 0, cellW, cellH, mx, mt), a.label)
		s.SetFillColor("#cfe2ff")
		s.SetAngle(a.deg * math.Pi / 180)
	}

	flips := []struct {
		label string
		fx    bool
		fy    bool
	}{
		{"transform-flipx", true, false},
		{"transform-flipy", false, true},
		{"transform-flipx-flipy", true, true},
		{"transform-no-flip-reference", false, false},
	}
	for i, f := range flips {
		s := _baseShape(p, gx(i, 1, cellW, cellH, mx, mt), f.label)
		s.SetFillColor("#cfe2ff")
		// Use an asymmetric L-shape geometry so flip is visually obvious.
		root := s.XML()
		for _, sec := range root.SelectElements("Section") {
			if sec.SelectAttrValue("N", "") == "Geometry" {
				root.RemoveChild(sec)
			}
		}
		s.Geometries = nil
		s.Geometry = nil
		g := s.AddGeometry()
		w, h := cellW*0.75, cellH*0.65
		g.AddMoveTo(0, 0)
		g.AddLineTo(0.5*w, 0)
		g.AddLineTo(0.5*w, 0.5*h)
		g.AddLineTo(w, 0.5*h)
		g.AddLineTo(w, h)
		g.AddLineTo(0, h)
		g.AddLineTo(0, 0)
		if f.fx {
			s.SetFlipX(true)
		}
		if f.fy {
			s.SetFlipY(true)
		}
	}
}

// ---------- Page 7: Effects ----------

func buildEffectsPage(v *vsdx.VisioFile) {
	p, err := v.AddPage("Effects")
	if err != nil {
		fatal("AddPage: %v", err)
	}
	setPageSize(p, pageW, pageH)

	cellW, cellH := 2.2, 1.6
	mx, mt := 0.4, 0.5

	// Row 0: shadows + soft edges
	s := _baseShape(p, gx(0, 0, cellW, cellH, mx, mt), "effect-shadow-default")
	s.SetFillColor("#cfe2ff")
	s.SetCellValue("ShdwForegnd", "#888888")
	s.SetCellValue("ShdwForegndTrans", "0.4")
	s.SetCellValue("ShdwOffsetX", "0.05")
	s.SetCellValue("ShdwOffsetY", "-0.05")
	s.SetCellValue("ShdwPattern", "1")
	s.SetCellValue("ShdwType", "1")

	s = _baseShape(p, gx(1, 0, cellW, cellH, mx, mt), "effect-shadow-color-red-large")
	s.SetFillColor("#ffe0e0")
	s.SetCellValue("ShdwForegnd", "#cc0000")
	s.SetCellValue("ShdwForegndTrans", "0.2")
	s.SetCellValue("ShdwOffsetX", "0.15")
	s.SetCellValue("ShdwOffsetY", "-0.15")
	s.SetCellValue("ShdwPattern", "1")

	s = _baseShape(p, gx(2, 0, cellW, cellH, mx, mt), "effect-soft-edges-5pt")
	s.SetFillColor("#cfe2ff")
	s.SetSoftEdgesSize(5)

	s = _baseShape(p, gx(3, 0, cellW, cellH, mx, mt), "effect-glow-blue-large")
	s.SetFillColor("#cfe2ff")
	s.SetGlowEffect(&vsdx.GlowEffect{Color: "#0044cc", ColorTrans: 0.2, Size: 12})

	// Row 1: bevel + reflection + 3D
	s = _baseShape(p, gx(0, 1, cellW, cellH, mx, mt), "effect-bevel-circle")
	s.SetFillColor("#ffcc99")
	s.SetBevelEffect(&vsdx.BevelEffect{
		TopType: 1, TopWidth: 6, TopHeight: 6,
		MaterialType: 1, LightingType: 1, LightingAngle: 0,
	})

	s = _baseShape(p, gx(1, 1, cellW, cellH, mx, mt), "effect-bevel-cross")
	s.SetFillColor("#99dd66")
	s.SetBevelEffect(&vsdx.BevelEffect{
		TopType: 7, TopWidth: 5, TopHeight: 5, BottomType: 7, BottomWidth: 5, BottomHeight: 5,
		MaterialType: 1, LightingType: 1,
	})

	s = _baseShape(p, gx(2, 1, cellW, cellH, mx, mt), "effect-reflection-default")
	s.SetFillColor("#cfe2ff")
	// ReflectionSize=50 is Visio's default and gets stripped on resave;
	// pick a non-default value so the cell survives a round trip.
	s.SetReflectionEffect(&vsdx.ReflectionEffect{Size: 30, Trans: 0.5, Dist: 0, Blur: 4})

	s = _baseShape(p, gx(3, 1, cellW, cellH, mx, mt), "effect-3d-rotation")
	s.SetFillColor("#cfe2ff")
	s.SetRotation3DEffect(&vsdx.Rotation3DEffect{
		RotationType: 1, XAngle: 20, YAngle: 20, ZAngle: 0,
		Perspective: 30, DistanceFromGround: 0, KeepTextFlat: false,
	})

	s = _baseShape(p, gx(0, 2, cellW, cellH, mx, mt), "effect-sketch-default")
	s.SetFillColor("#cfe2ff")
	s.SetSketchEffect(&vsdx.SketchEffect{
		Enabled: true, Seed: 42, Amount: 0.5, LineWeight: 1,
		LineChange: 0.5, FillChange: 0.5,
	})
}

// ---------- Page 8: Connectors ----------

func buildConnectorsPage(v *vsdx.VisioFile) {
	p, err := v.AddPage("Connectors")
	if err != nil {
		fatal("AddPage: %v", err)
	}
	setPageSize(p, pageW, pageH)

	// Pairs of anchors with connectors between them.
	mkAnchor := func(cx, cy float64, label string) *vsdx.Shape {
		s := p.AddShape()
		s.SetX(cx)
		s.SetY(cy)
		s.SetWidth(1.2)
		s.SetHeight(0.7)
		s.SetLocX(0.6)
		s.SetLocY(0.35)
		s.AddGeometryRect()
		s.SetFillColor("#cccccc")
		s.SetLineColor("#222222")
		s.SetLineWeight(0.014)
		s.SetText(label)
		return s
	}

	// Pair 1: straight static line
	a := mkAnchor(1.5, 7.0, "connector-static-line-A")
	b := mkAnchor(4.5, 7.0, "connector-static-line-B")
	conn, err := v.ConnectShapes(p, a, b)
	if err == nil {
		conn.SetText("connector-static-line")
	}

	// Pair 2: with begin+end arrows + label
	a = mkAnchor(6.5, 7.0, "connector-bidir-A")
	b = mkAnchor(9.5, 7.0, "connector-bidir-B")
	conn, err = v.ConnectShapes(p, a, b)
	if err == nil {
		conn.SetBeginArrow(13)
		conn.SetEndArrow(13)
		conn.SetText("connector-bidirectional-with-label")
	}

	// Pair 3: dashed connector
	a = mkAnchor(1.5, 5.0, "connector-dashed-A")
	b = mkAnchor(4.5, 5.0, "connector-dashed-B")
	conn, err = v.ConnectShapes(p, a, b)
	if err == nil {
		conn.SetLinePattern(vsdx.LinePatternDash)
		conn.SetEndArrow(13)
		conn.SetText("connector-dashed-with-arrow")
	}

	// Pair 4: thick connector with color
	a = mkAnchor(6.5, 5.0, "connector-thick-A")
	b = mkAnchor(9.5, 5.0, "connector-thick-B")
	conn, err = v.ConnectShapes(p, a, b)
	if err == nil {
		conn.SetLineWeight(0.06)
		conn.SetLineColor("#cc0000")
		conn.SetEndArrow(13)
		conn.SetText("connector-thick-red")
	}

	// Anchor with explicit connection points
	cp := mkAnchor(3.0, 2.5, "shape-with-4-connection-points")
	cp.AddConnectionPoint(0, 0.5)
	cp.AddConnectionPoint(1, 0.5)
	cp.AddConnectionPoint(0.5, 0)
	cp.AddConnectionPoint(0.5, 1)

	// Anchor with ABCD connection points (directional)
	abcd := mkAnchor(6.0, 2.5, "shape-with-ABCD-connections")
	abcd.AddConnectionABCD(0.5, 1, 0, 1, 1)
	abcd.AddConnectionABCD(1, 0.5, 1, 0, 1)
	abcd.AddConnectionABCD(0.5, 0, 0, -1, 1)
	abcd.AddConnectionABCD(0, 0.5, -1, 0, 1)
}

// ---------- Page 9: Data & Metadata ----------

func buildDataPage(v *vsdx.VisioFile) {
	p, err := v.AddPage("Data")
	if err != nil {
		fatal("AddPage: %v", err)
	}
	setPageSize(p, pageW, pageH)

	cellW, cellH := 2.4, 1.4
	mx, mt := 0.4, 0.4

	// 1) Shape with 5 data properties
	s := _baseShape(p, gx(0, 0, cellW, cellH, mx, mt), "data-properties-5types")
	s.AddDataProperty("Status", "Status", "Active")
	s.AddDataProperty("Count", "Count", "42")
	s.AddDataProperty("Created", "Created", "2026-01-15")
	s.AddDataProperty("Priority", "Priority", "High")
	s.AddDataProperty("Approved", "Approved", "True")

	// 2) Hyperlink external
	s = _baseShape(p, gx(1, 0, cellW, cellH, mx, mt), "data-hyperlink-external")
	s.AddHyperlink("https://example.com", "External Link")

	// 3) Hyperlink internal
	s = _baseShape(p, gx(2, 0, cellW, cellH, mx, mt), "data-hyperlink-internal")
	s.AddHyperlink("Page-1", "Internal Page Link")

	// 4) User cells
	s = _baseShape(p, gx(3, 0, cellW, cellH, mx, mt), "data-user-cells")
	s.AddUserCell("MyCustomCell", "42.0")
	s.AddUserCell("AnotherCell", "Hello")

	// 5-7) Layer membership
	layerA := p.AddLayer("LayerA")
	layerB := p.AddLayer("LayerB")
	layerC := p.AddLayer("LayerC")

	s = _baseShape(p, gx(0, 1, cellW, cellH, mx, mt), "layer-membership-A-only")
	s.SetLayerMember(strconv.Itoa(layerA))
	s = _baseShape(p, gx(1, 1, cellW, cellH, mx, mt), "layer-membership-A-and-B")
	s.SetLayerMember(fmt.Sprintf("%d;%d", layerA, layerB))
	s = _baseShape(p, gx(2, 1, cellW, cellH, mx, mt), "layer-membership-C-only")
	s.SetLayerMember(strconv.Itoa(layerC))
	s = _baseShape(p, gx(3, 1, cellW, cellH, mx, mt), "layer-membership-no-layer")

	// 8) Shape with locks
	s = _baseShape(p, gx(0, 2, cellW, cellH, mx, mt), "data-locked-move")
	s.SetLockMove(true)
	s = _baseShape(p, gx(1, 2, cellW, cellH, mx, mt), "data-locked-size")
	s.SetLockSize(true)
	s = _baseShape(p, gx(2, 2, cellW, cellH, mx, mt), "data-locked-delete")
	s.SetLockDelete(true)
	s = _baseShape(p, gx(3, 2, cellW, cellH, mx, mt), "data-locked-rotate")
	s.SetLockRotate(true)

	// 9) Comment
	s = _baseShape(p, gx(0, 3, cellW, cellH, mx, mt), "data-shape-comment")
	s.SetComment("This shape has a comment attached.")
}

// ---------- Helpers ----------

func fmtFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// ---------- main ----------

func main() {
	out := "vsdx-svg/comprehensive/comprehensive-features.vsdx"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	source := "tests/blank.vsdx"
	data, err := os.ReadFile(source)
	if err != nil {
		fatal("read %s: %v", source, err)
	}
	v, err := vsdx.OpenBytes(data)
	if err != nil {
		fatal("open: %v", err)
	}
	defer v.Close()

	buildShapesPage(v)
	buildFillsPage(v)
	buildLinesPage(v)
	buildArrowsPage(v)
	buildTextPage(v)
	buildTransformsPage(v)
	buildEffectsPage(v)
	buildConnectorsPage(v)
	buildDataPage(v)

	// blank.vsdx ships with one empty default page at index 0. Drop it so
	// the output has just our 9 themed pages.
	v.RemovePageByIndex(0)

	bytes, err := v.SaveVsdxBytes()
	if err != nil {
		fatal("save: %v", err)
	}
	if err := os.WriteFile(out, bytes, 0644); err != nil {
		fatal("write %s: %v", out, err)
	}

	fmt.Printf("Wrote %s\n", out)
	fmt.Println()
	fmt.Println("Pages and feature counts:")
	fmt.Println("  Page 1 Shapes        — primitives, polygons, multi-geom, group, special")
	fmt.Println("  Page 2 Fills         — solid, gradient (linear/radial), patterns, transparency")
	fmt.Println("  Page 3 Lines         — patterns, weights, caps, gradient, color")
	fmt.Println("  Page 4 Arrows        — types 1-24, both-end combos, sizes, curved")
	fmt.Println("  Page 5 Text          — fonts, sizes, styles, rotation, align, rich-text")
	fmt.Println("  Page 6 Transforms    — rotation 30°-180°, flipX/Y combos")
	fmt.Println("  Page 7 Effects       — shadow, soft edges, glow, bevel, reflection, 3D, sketch")
	fmt.Println("  Page 8 Connectors    — straight, bidirectional, dashed, thick, ABCD points")
	fmt.Println("  Page 9 Data          — properties, hyperlinks, layers, locks, comments")
}
