// Package renderpage emits a full-page SVG by iterating a Page's shapes
// and stitching their per-shape SVGs into a positioned page-level document.
//
// Extracted from cmd/render-compare so both render-compare and the
// mutation-corpus generator can use the same render path. Both tools need
// page-level SVG strings; the existing public vsdx.ShapeToSVG only emits
// per-shape SVG, requiring the caller to assemble the page.
//
// Keep this file API-stable: changes here can flip SSIM scores across the
// entire mutation-render corpus.
package renderpage

import (
	"fmt"
	"math"
	"strings"

	"wijnberg.net/vsdx-go/vsdx"
)

// Render builds a complete SVG document for the page at the given
// dimensions (in inches). Returns an error if any shape's geometry can't be
// resolved.
func Render(page *vsdx.Page, pageW, pageH float64) (string, error) {
	var sb strings.Builder
	ppi := 72.0
	viewW := pageW * ppi
	viewH := pageH * ppi

	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="%.4fin" height="%.4fin" viewBox="0 0 %.2f %.2f">`, pageW, pageH, viewW, viewH))
	sb.WriteString("\n")

	for _, shape := range page.ChildShapes() {
		shapeW := shape.Width()
		shapeH := shape.Height()
		shapeAngle := shape.Angle()
		isRotated1D := shapeH == 0 && shapeAngle != 0

		if isRotated1D {
			emitRotated1D(&sb, shape, pageH, ppi)
			continue
		}

		if shapeW == 0 {
			shapeW = 1
		}
		if shapeH == 0 {
			shapeH = 1
		}
		pixelW := math.Abs(shapeW) * ppi
		pixelH := math.Abs(shapeH) * ppi

		result, err := vsdx.ShapeToSVG(shape,
			vsdx.WithSize(pixelW, pixelH),
			vsdx.WithPrecision(2))
		if err != nil {
			continue
		}
		inner := extractSVGContent(string(result.SVG))

		x := (shape.X() - shape.LocX()) * ppi
		var y float64
		if shapeH < 0 {
			y = viewH - (shape.Y()-shape.LocY())*ppi
		} else {
			y = viewH - (shape.Y()+(shape.Height()-shape.LocY()))*ppi
		}

		// FlipX/FlipY mirror the shape's GEOMETRY within its local frame
		// (MS-VSDX §2.2.3.2.1 steps 3-4). Crucially, text content is NOT
		// mirrored — Visio's UI shows flipped shapes with text upright so
		// labels stay legible. We achieve this by splitting the inner SVG
		// into text elements and the rest: only the non-text geometry is
		// wrapped in the flipping group; text is placed in a sibling
		// non-flipped translate so it lands at the same on-page position
		// without being mirror-flipped.
		flipX := shape.FlipX()
		flipY := shape.FlipY()
		// Wrap the shape in <a xlink:href> when it carries a Hyperlink.
		// Matches Visio's SVG export which emits <a> wrappers around
		// hyperlink-bearing shapes so the rendered SVG is interactive.
		hyperHref, hyperTitle := shape.Hyperlink()
		if hyperHref != "" {
			if hyperTitle != "" {
				sb.WriteString(fmt.Sprintf(`<a xlink:href=%q xlink:title=%q>`, hyperHref, hyperTitle))
			} else {
				sb.WriteString(fmt.Sprintf(`<a xlink:href=%q>`, hyperHref))
			}
		}
		if !flipX && !flipY {
			sb.WriteString(fmt.Sprintf(`<g transform="translate(%.2f %.2f)">`, x, y))
			sb.WriteString(inner)
			sb.WriteString("</g>\n")
		} else {
			text, geom := splitTextAndGeometry(inner)
			fx, fy := 1.0, 1.0
			dx, dy := 0.0, 0.0
			if flipX {
				fx = -1
				dx = pixelW
			}
			if flipY {
				fy = -1
				dy = pixelH
			}
			// Outer non-flipped group establishes the shape's top-left at
			// (x, y). Inner flipped group mirrors only the geometry. Text
			// sits in the outer group, in its original local-coord
			// position — so it stays upright at the same page location.
			sb.WriteString(fmt.Sprintf(`<g transform="translate(%.2f %.2f)">`, x, y))
			sb.WriteString(fmt.Sprintf(`<g transform="translate(%.2f %.2f) scale(%.0f %.0f)">`, dx, dy, fx, fy))
			sb.WriteString(geom)
			sb.WriteString(`</g>`)
			sb.WriteString(text)
			sb.WriteString("</g>\n")
		}
		if hyperHref != "" {
			sb.WriteString("</a>\n")
		}
	}

	sb.WriteString("</svg>\n")
	return sb.String(), nil
}

// emitRotated1D writes the rendered SVG for a rotated 1D shape (line or
// connector with Height=0 and non-zero Angle) directly from BeginX/Y and
// EndX/Y rather than going through ShapeToSVG.
func emitRotated1D(sb *strings.Builder, shape *vsdx.Shape, pageH, ppi float64) {
	x1 := shape.BeginX() * ppi
	y1 := (pageH - shape.BeginY()) * ppi
	x2 := shape.EndX() * ppi
	y2 := (pageH - shape.EndY()) * ppi

	style := shape.ComputeEffectiveStyle()
	lineColor := style.EffectiveLineColor()
	if lineColor == "" {
		lineColor = "#000000"
	}
	linePattern := style.LinePattern
	strokeWidth := style.LineWeight

	dashArray := ""
	if linePattern == 4 {
		dashArray = fmt.Sprintf(`stroke-dasharray="%.2f %.2f %.2f %.2f"`,
			strokeWidth*7, strokeWidth*5, 0.0, strokeWidth*5)
	}

	endArrow := style.EndArrow
	beginArrow := style.BeginArrow
	hasEndArrow := endArrow > 0
	hasBeginArrow := beginArrow > 0

	if hasEndArrow || hasBeginArrow {
		arrowSizeMultipliers := []float64{0.5, 0.75, 1.0, 1.5, 2.0, 2.5, 3.0}
		sizeMult := 1.0
		if style.EndArrowSize >= 0 && style.EndArrowSize < len(arrowSizeMultipliers) {
			sizeMult = arrowSizeMultipliers[style.EndArrowSize]
		}
		baseScale := 0.36 * sizeMult
		minVisualWidth := 7.0 * sizeMult
		baseVisualWidth := 10 * baseScale * strokeWidth
		var markerSize float64
		if baseVisualWidth >= minVisualWidth {
			markerSize = 10 * baseScale
		} else {
			markerSize = minVisualWidth / strokeWidth
		}

		markerID := fmt.Sprintf("arrow_%s_%s", strings.ReplaceAll(lineColor, "#", ""), shape.ID)
		sb.WriteString("<defs>\n")
		if hasEndArrow {
			sb.WriteString(fmt.Sprintf(`  <marker id="%s_end" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="%.2f" markerHeight="%.2f" markerUnits="strokeWidth" orient="auto"><path d="M0 0 L10 5 L0 10 z" fill="%s" stroke="none"/></marker>`,
				markerID, markerSize, markerSize, lineColor))
			sb.WriteString("\n")
		}
		if hasBeginArrow {
			sb.WriteString(fmt.Sprintf(`  <marker id="%s_start" viewBox="0 0 10 10" refX="0" refY="5" markerWidth="%.2f" markerHeight="%.2f" markerUnits="strokeWidth" orient="auto-start-reverse"><path d="M0 0 L10 5 L0 10 z" fill="%s" stroke="none"/></marker>`,
				markerID, markerSize, markerSize, lineColor))
			sb.WriteString("\n")
		}
		sb.WriteString("</defs>\n")

		markerAttrs := ""
		if hasBeginArrow {
			markerAttrs += fmt.Sprintf(` marker-start="url(#%s_start)"`, markerID)
		}
		if hasEndArrow {
			markerAttrs += fmt.Sprintf(` marker-end="url(#%s_end)"`, markerID)
		}

		sb.WriteString(fmt.Sprintf(`<path d="M%.2f %.2fL%.2f %.2f" fill="none" stroke="%s" stroke-width="%.2f" %s stroke-linecap="round"%s/>`,
			x1, y1, x2, y2, lineColor, strokeWidth, dashArray, markerAttrs))
	} else {
		sb.WriteString(fmt.Sprintf(`<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" stroke="%s" stroke-width="%.2f" %s stroke-linecap="round"/>`,
			x1, y1, x2, y2, lineColor, strokeWidth, dashArray))
	}
	sb.WriteString("\n")
}

// splitTextAndGeometry walks a per-shape SVG payload and partitions it into
// two strings: every `<text>...</text>` element (in document order) and all
// other elements (paths, rects, defs, etc.) in document order. Used by the
// FlipX/Y path so a mirror transform can be applied to geometry without
// flipping the human-readable text content.
//
// Naive but correct for vsdx-go's output, which never nests `<text>` inside
// arbitrary containers (text elements are always direct children of the
// per-shape SVG body). If that invariant is ever broken, this splitter will
// need to upgrade to a real XML parser.
func splitTextAndGeometry(inner string) (text, geom string) {
	var tb, gb strings.Builder
	i := 0
	for i < len(inner) {
		idx := strings.Index(inner[i:], "<text")
		if idx < 0 {
			gb.WriteString(inner[i:])
			break
		}
		gb.WriteString(inner[i : i+idx])
		j := i + idx
		end := strings.Index(inner[j:], "</text>")
		if end < 0 {
			// Malformed input — preserve as geometry rather than truncate.
			gb.WriteString(inner[j:])
			break
		}
		closeAt := j + end + len("</text>")
		tb.WriteString(inner[j:closeAt])
		i = closeAt
	}
	return tb.String(), gb.String()
}

// extractSVGContent strips the outer <svg ...> wrapper so the inner content
// can be repositioned inside a parent SVG via a translate group.
func extractSVGContent(svg string) string {
	svg = strings.TrimSpace(svg)
	start := strings.Index(svg, ">")
	if start == -1 {
		return svg
	}
	end := strings.LastIndex(svg, "</svg>")
	if end == -1 {
		return svg[start+1:]
	}
	return svg[start+1 : end]
}
