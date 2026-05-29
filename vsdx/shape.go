package vsdx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// fmtFloat formats a float64 as a string with no trailing zeros.
func fmtFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// toFloat converts a string to float64. Returns 0.0 if empty or invalid.
func toFloat(val string) float64 {
	if val == "" {
		return 0
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0
	}
	return f
}

// collectText recursively collects all text content from an etree element,
// equivalent to Python's "".join(element.itertext()).
func collectText(e *etree.Element) string {
	var buf strings.Builder
	for _, token := range e.Child {
		switch t := token.(type) {
		case *etree.CharData:
			buf.WriteString(t.Data)
		case *etree.Element:
			buf.WriteString(collectText(t))
		}
	}
	return buf.String()
}

// ShapeParent is the interface implemented by types that can contain shapes (*Page or *Shape).
type ShapeParent interface {
	removeChildShape(s *Shape)
}

// Shape represents a single shape or a group shape containing other shapes.
type Shape struct {
	xml           *etree.Element
	Parent        ShapeParent // *Page or *Shape
	Page          *Page
	ID            string
	MasterShapeID string // MasterShape attribute
	MasterPageID  string // Master attribute - reference to master page ID
	ShapeType     string // Type attribute (e.g., "Shape", "Group")
	ShapeName     string // NameU or Name attribute
	Cells         map[string]*Cell
	Geometry      *Geometry   // first geometry section (for backward compat)
	Geometries    []*Geometry // all geometry sections (Geometry IX=0, 1, 2, ...)

	dataProperties map[string]*DataProperty // lazy loaded
}

// newShape creates a Shape from an XML element.
func newShape(xml *etree.Element, parent ShapeParent, page *Page) *Shape {
	s := &Shape{
		xml:           xml,
		Parent:        parent,
		Page:          page,
		ID:            xml.SelectAttrValue("ID", ""),
		MasterShapeID: xml.SelectAttrValue("MasterShape", ""),
		MasterPageID:  xml.SelectAttrValue("Master", ""),
		ShapeType:     xml.SelectAttrValue("Type", ""),
		Cells:         make(map[string]*Cell),
	}

	s.ShapeName = xml.SelectAttrValue("NameU", "")
	if s.ShapeName == "" {
		s.ShapeName = xml.SelectAttrValue("Name", "")
	}

	// Inherit MasterPageID from parent shape if not set (for sub-shapes in groups)
	if s.MasterPageID == "" {
		if ps := s.ParentShape(); ps != nil {
			s.MasterPageID = ps.MasterPageID
		}
	}

	// Get Cells directly under Shape element
	for _, cellElem := range xml.SelectElements("Cell") {
		cell := newCell(cellElem, s)
		s.Cells[cell.Name()] = cell
	}

	// Get all Geometry sections (shapes can have multiple: IX=0, 1, 2, ...)
	for _, geometrySection := range xml.SelectElements("Section") {
		if geometrySection.SelectAttrValue("N", "") != "Geometry" {
			continue
		}
		// Parse geometry index from IX attribute, fallback to current slice length
		geomIndex := len(s.Geometries)
		if ixStr := geometrySection.SelectAttrValue("IX", ""); ixStr != "" {
			if ix, err := strconv.Atoi(ixStr); err == nil {
				geomIndex = ix
			}
		}
		g := newGeometry(geometrySection, s, geomIndex)
		s.Geometries = append(s.Geometries, g)
		if s.Geometry == nil {
			s.Geometry = g // backward compat: first section
		}
		// Also add geometry row cells to shape cells dict
		for _, rowElem := range geometrySection.SelectElements("Row") {
			rowType := rowElem.SelectAttrValue("T", "")
			if rowType != "" {
				for _, cellElem := range rowElem.SelectElements("Cell") {
					cell := newCell(cellElem, s)
					key := fmt.Sprintf("Geometry/%s/%s", rowType, cell.Name())
					s.Cells[key] = cell
				}
			}
		}
	}

	// If shape has no local geometry but references a master, inherit the
	// master's geometries. We DON'T alias `s.Geometries = ms.Geometries`
	// directly — that would make every Geometry struct on the instance point
	// at the master's XML/Rows, so a mutation on the instance corrupts the
	// master (and every sibling instance of the same master). Instead, build
	// instance-owned Geometry wrappers that initially reference master XML
	// for reads but localize to a deep-copy on first mutation via
	// (*Geometry).localize(). See master_isolation_test.go for the regression.
	if len(s.Geometries) == 0 {
		if ms := s.MasterShape(); ms != nil && len(ms.Geometries) > 0 {
			for _, mg := range ms.Geometries {
				ig := &Geometry{
					xml:   mg.xml,
					Cells: append([]*GeometryCell(nil), mg.Cells...),
					Rows:  make(map[string]*GeometryRow, len(mg.Rows)),
					shape: s,
				}
				// Wrap each master row so its `geometry` field points at the
				// instance's Geometry, not the master's. Without this,
				// (*GeometryRow).SetX / SetY would call localize on the
				// master's Geometry and corrupt every sibling instance — the
				// exact bug we're fixing here.
				for k, mr := range mg.Rows {
					ir := &GeometryRow{
						geometry: ig,
						xml:      mr.xml,
						Cells:    make(map[string]*GeometryCell, len(mr.Cells)),
					}
					for ck, cv := range mr.Cells {
						ir.Cells[ck] = cv
					}
					ig.Rows[k] = ir
				}
				s.Geometries = append(s.Geometries, ig)
			}
			if len(s.Geometries) > 0 {
				s.Geometry = s.Geometries[0]
			}
		}
	}

	// Get Control section
	controlSection := xml.FindElement("Section[@N='Control']")
	if controlSection != nil {
		for _, rowElem := range controlSection.SelectElements("Row") {
			rowName := rowElem.SelectAttrValue("N", "")
			if rowName != "" {
				for _, cellElem := range rowElem.SelectElements("Cell") {
					cell := newCell(cellElem, s)
					key := fmt.Sprintf("Control/%s/%s", rowName, cell.Name())
					s.Cells[key] = cell
				}
			}
		}
	}

	// Character / Paragraph sections: expose cells under the dotted
	// "Char.<N>" / "Para.<N>" keys that EffectiveStyle uses to drive font,
	// size, bold/italic, color and alignment. Without this the entire
	// formatting layer was invisible to the renderer — SetCharBold(true)
	// wrote Section/Row/Cell correctly but CellValue("Char.Style") returned
	// empty so EffectiveStyle.Bold stayed false.
	if charSection := xml.FindElement("Section[@N='Character']"); charSection != nil {
		if row := charSection.FindElement("Row"); row != nil {
			for _, cellElem := range row.SelectElements("Cell") {
				cell := newCell(cellElem, s)
				s.Cells["Char."+cell.Name()] = cell
			}
		}
	}
	if paraSection := xml.FindElement("Section[@N='Paragraph']"); paraSection != nil {
		if row := paraSection.FindElement("Row"); row != nil {
			for _, cellElem := range row.SelectElements("Cell") {
				cell := newCell(cellElem, s)
				s.Cells["Para."+cell.Name()] = cell
			}
		}
	}

	return s
}

// XML returns the underlying XML element of the shape.
func (s *Shape) XML() *etree.Element {
	return s.xml
}

func (s *Shape) String() string {
	return fmt.Sprintf("<Shape ID=%s type=%s text='%s'>", s.ID, s.ShapeType, s.Text())
}

// IsMasterShape returns true if the shape is on a master page.
func (s *Shape) IsMasterShape() bool {
	return s.Page.IsMasterPage()
}

// MasterShape returns this shape's master shape, or nil.
func (s *Shape) MasterShape() *Shape {
	if s.Page == nil || s.Page.vis == nil {
		return nil
	}
	masterPage := s.Page.vis.GetMasterPageByID(s.MasterPageID)
	if masterPage == nil {
		return nil
	}
	childShapes := masterPage.ChildShapes()
	if len(childShapes) == 0 {
		return nil
	}
	masterShape := childShapes[0]

	if s.MasterShapeID != "" {
		if sub := masterShape.FindShapeByID(s.MasterShapeID); sub != nil {
			return sub
		}
	}
	return masterShape
}

// MasterPage returns this shape's master page, or nil.
func (s *Shape) MasterPage() *Page {
	if s.Page == nil || s.Page.vis == nil {
		return nil
	}
	return s.Page.vis.GetMasterPageByID(s.MasterPageID)
}

// CellValue returns the value of a named cell. Falls back to master shape if not found locally.
func (s *Shape) CellValue(name string) string {
	if cell, ok := s.Cells[name]; ok {
		return cell.Value()
	}
	if s.MasterPageID != "" {
		if ms := s.MasterShape(); ms != nil {
			return ms.CellValue(name)
		}
	}
	return ""
}

// CellFormula returns the formula of a named cell. Falls back to master shape if not found locally.
func (s *Shape) CellFormula(name string) string {
	if cell, ok := s.Cells[name]; ok {
		return cell.Formula()
	}
	if s.MasterPageID != "" {
		if ms := s.MasterShape(); ms != nil {
			return ms.CellFormula(name)
		}
	}
	return ""
}

// HasCell returns true if the shape has a cell with the given name (locally or via master).
func (s *Shape) HasCell(name string) bool {
	return s.CellValue(name) != ""
}

// --- Cell editing ---

// ensureCell returns the cell with the given name, creating it if it doesn't exist.
// skipAttr is the attribute key to exclude when copying from a master cell (e.g., "V" or "F").
func (s *Shape) ensureCell(name, skipAttr string) *Cell {
	if cell, ok := s.Cells[name]; ok {
		return cell
	}
	// Create new cell element
	cellElem := etree.NewElement("Cell")
	cellElem.CreateAttr("N", name)

	// Copy attributes from master cell if available
	if s.MasterPageID != "" {
		if ms := s.MasterShape(); ms != nil {
			if masterCell, ok := ms.Cells[name]; ok {
				for _, attr := range masterCell.xml.Attr {
					if attr.Key != skipAttr {
						cellElem.CreateAttr(attr.Key, attr.Value)
					}
				}
			}
		}
	}

	// Insert after last Cell element in shape XML
	cells := s.xml.SelectElements("Cell")
	if len(cells) > 0 {
		lastCell := cells[len(cells)-1]
		insertAfter(s.xml, lastCell, cellElem)
	} else {
		s.xml.InsertChildAt(0, cellElem)
	}

	cell := newCell(cellElem, s)
	s.Cells[name] = cell
	return cell
}

// SetCellValue sets the value of a named cell, creating it if it doesn't exist.
// If a master shape has the cell, the new cell is copied from the master.
func (s *Shape) SetCellValue(name, value string) {
	s.ensureCell(name, "V").SetValue(value)
}

// SetCellFormula sets the formula of a named cell, creating it if it doesn't exist.
func (s *Shape) SetCellFormula(name, formula string) {
	s.ensureCell(name, "F").SetFormula(formula)
}

// insertAfter inserts newElem after refElem in parent's children.
func insertAfter(parent, refElem, newElem *etree.Element) {
	// Insert after refElem by using its index in the token list
	refIndex := refElem.Index()
	parent.InsertChildAt(refIndex+1, newElem)
}

// --- Position and size properties ---

func (s *Shape) X() float64      { return toFloat(s.CellValue(CellPinX)) }
func (s *Shape) Y() float64      { return toFloat(s.CellValue(CellPinY)) }
func (s *Shape) LocX() float64   { return toFloat(s.CellValue(CellLocPinX)) }
func (s *Shape) LocY() float64   { return toFloat(s.CellValue(CellLocPinY)) }
func (s *Shape) Width() float64  { return toFloat(s.CellValue(CellWidth)) }
func (s *Shape) Height() float64 { return toFloat(s.CellValue(CellHeight)) }
func (s *Shape) Angle() float64  { return toFloat(s.CellValue(CellAngle)) }

func (s *Shape) BeginX() float64 { return toFloat(s.CellValue(CellBeginX)) }
func (s *Shape) BeginY() float64 { return toFloat(s.CellValue(CellBeginY)) }
func (s *Shape) EndX() float64   { return toFloat(s.CellValue(CellEndX)) }
func (s *Shape) EndY() float64   { return toFloat(s.CellValue(CellEndY)) }

func (s *Shape) HasBeginX() bool { return s.CellValue(CellBeginX) != "" }

func (s *Shape) LocXFormula() string { return s.CellFormula(CellLocPinX) }
func (s *Shape) LocYFormula() string { return s.CellFormula(CellLocPinY) }

// BoundingBox returns the bounding box of the shape in page coordinates.
func (s *Shape) BoundingBox() Rect {
	x := s.X()
	y := s.Y()
	w := s.Width()
	h := s.Height()
	locX := s.LocX()
	locY := s.LocY()

	// Calculate the lower-left corner of the shape.
	left := x - locX
	bottom := y - locY

	return Rect{
		X:      left,
		Y:      bottom,
		Width:  w,
		Height: h,
	}
}

// --- Position and size setters ---

func (s *Shape) SetX(v float64)      { s.SetCellValue(CellPinX, fmtFloat(v)) }
func (s *Shape) SetY(v float64)      { s.SetCellValue(CellPinY, fmtFloat(v)) }
func (s *Shape) SetLocX(v float64)   { s.SetCellValue(CellLocPinX, fmtFloat(v)) }
func (s *Shape) SetLocY(v float64)   { s.SetCellValue(CellLocPinY, fmtFloat(v)) }
// SetWidth changes the shape's Width cell AND scales every absolute X
// coordinate in its local geometry by the same factor — plus the LocPinX
// cell so the shape's anchor moves with the resize. Without the scaling,
// shapes whose geometry rows store absolute values (e.g. "MoveTo X=1.488")
// keep rendering at their old width even after the Width cell changes —
// because Visio normally re-evaluates the master's formulas to recompute
// those values, and we don't run a ShapeSheet evaluator at render time.
// Without the LocPin scaling, the rendered shape jumps sideways on resize
// because the anchor offset stays at the OLD width's value while the
// frontend assumes the pin tracks the bbox center.
// Relative-coord rows (RelMoveTo etc.) are left alone; their stored fractions
// already track the new width via the resolver's localW multiplication.
func (s *Shape) SetWidth(v float64) {
	old := s.Width()
	s.SetCellValue(CellWidth, fmtFloat(v))
	if old > 0 && v > 0 && v != old {
		scale := v / old
		scaleGeometryAxis(s.Geometry, "X", scale)
		if loc := s.LocX(); loc != 0 {
			s.SetLocX(loc * scale)
		}
		// Sub-shapes draw inside the parent's local coordinate space; when
		// the parent's Width grows, their absolute coordinates need to grow
		// proportionally too. Without this, a Visio "Can" instance resized
		// to 2x its master width still renders its cylinder body at master
		// width because the body lives on a child shape.
		for _, child := range s.ChildShapes() {
			scaleChildShapeAxis(child, "X", scale)
		}
	}
}

// SetHeight is the Y-axis sibling of SetWidth.
func (s *Shape) SetHeight(v float64) {
	old := s.Height()
	s.SetCellValue(CellHeight, fmtFloat(v))
	if old != 0 && v != 0 && v != old {
		// Y axes use signed values (Visio negative-height shapes), so use
		// magnitudes for the ratio to avoid flipping the geometry.
		ratio := absVal(v) / absVal(old)
		if ratio != 1.0 {
			scaleGeometryAxis(s.Geometry, "Y", ratio)
			if loc := s.LocY(); loc != 0 {
				s.SetLocY(loc * ratio)
			}
			for _, child := range s.ChildShapes() {
				scaleChildShapeAxis(child, "Y", ratio)
			}
		}
	}
}

// scaleChildShapeAxis multiplies a sub-shape's axis-related cells by scale,
// recursing into the sub-shape's own children. We touch:
//   - PinX (or PinY) — the child's position inside the parent's coord space
//   - Width (or Height) — the child's own size
//   - LocPinX (or LocPinY) — the child's pin offset (so it scales with width)
//   - geometry rows that store absolute coords in this axis
//
// We don't touch cells whose F attribute references "Width" / "Height"
// because they're driven by the parent's Width/Height already.
func scaleChildShapeAxis(s *Shape, axis string, scale float64) {
	if s == nil || scale == 1.0 {
		return
	}
	var pin, locPin, dim CellName
	if axis == "X" {
		pin, locPin, dim = CellPinX, CellLocPinX, CellWidth
	} else {
		pin, locPin, dim = CellPinY, CellLocPinY, CellHeight
	}
	scaleNonInhCell(s, pin, scale)
	scaleNonInhCell(s, dim, scale)
	scaleNonInhCell(s, locPin, scale)
	scaleGeometryAxis(s.Geometry, axis, scale)
	for _, child := range s.ChildShapes() {
		scaleChildShapeAxis(child, axis, scale)
	}
}

// widthHeightTokenRE matches a Width or Height TOKEN — not any substring —
// inside a ShapeSheet formula. Used by scaleNonInhCell to detect cells whose
// value is already a function of the parent's Width/Height so we don't
// double-scale them.
//
// EC-002: pre-fix this was strings.Contains(f, "Width"), which had false
// positives on user-defined cells like "WidthForegndColor" or
// "ScaleHeightFactor". The word-boundary form rejects those while still
// matching "Width", "Width*0.5", "Sheet.5!Width", "GUARD(Width)", etc.
var widthHeightTokenRE = regexp.MustCompile(`\b(Width|Height)\b`)

// scaleNonInhCell scales a cell's Value, including F="Inh" cells. We have
// to scale even Inh cells because the renderer reads Value, and Value for
// child shapes is captured at authoring time (when instance dims == master
// dims). After a parent resize, the inherited Value is stale. The formula
// stays untouched so Visio's own re-evaluation on reload is still correct.
//
// Cells whose formula references Width / Height of the SAME shape are
// already driven by that shape's resized cell, so we skip them to avoid
// double-scaling.
func scaleNonInhCell(s *Shape, name CellName, scale float64) {
	cell := s.Cells[string(name)]
	if cell == nil {
		return
	}
	if widthHeightTokenRE.MatchString(cell.Formula()) {
		return
	}
	cell.SetValue(fmtFloat(toFloat(cell.Value()) * scale))
}

func absVal(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// scaleGeometryAxis multiplies every absolute-coord cell on the given axis
// ("X" or "Y") of every absolute-coord row in g by scale. For row types
// with multiple coord cells per axis (Ellipse: X+A+C on X, Y+B+D on Y;
// EllipticalArcTo: X+A and Y+B; InfiniteLine: X+A and Y+B) all of them
// scale together. Relative-coord rows are skipped (their fractions already
// track Width/Height at render time). Localizes the geometry first so the
// mutation lands on instance-owned XML, not on a master shared with every
// sibling instance.
func scaleGeometryAxis(g *Geometry, axis string, scale float64) {
	if g == nil {
		return
	}
	g.localize()
	for _, row := range g.Rows {
		if row.xml == nil || row.xml.Parent() != g.xml {
			continue
		}
		xCells, yCells := moveCoordCells(row.RowType())
		var cellNames []string
		if axis == "X" {
			cellNames = xCells
		} else {
			cellNames = yCells
		}
		for _, name := range cellNames {
			cell := row.Cells[name]
			if cell == nil {
				continue
			}
			newV := toFloat(cell.Value()) * scale
			cell.SetValue(fmtFloat(newV))
		}
	}
}
func (s *Shape) SetAngle(v float64)  { s.SetCellValue(CellAngle, fmtFloat(v)) }

// FlipX returns true if the shape is mirrored along its vertical axis.
// Per MS-VSDX §2.2.3.2.1 step 3 in the canonical 7-step transform: when
// FlipX=1, the shape's local geometry is mirrored about the y-axis before
// rotation and the pin is applied.
func (s *Shape) FlipX() bool { return s.CellValue(CellFlipX) == "1" }

// FlipY returns true if the shape is mirrored along its horizontal axis.
func (s *Shape) FlipY() bool { return s.CellValue(CellFlipY) == "1" }

// SetFlipX sets (or clears) the FlipX cell. Setting to true causes the
// renderer to mirror the shape along its vertical axis.
func (s *Shape) SetFlipX(v bool) {
	if v {
		s.SetCellValue(CellFlipX, "1")
	} else {
		s.SetCellValue(CellFlipX, "0")
	}
	s.clearCellFormula(CellFlipX)
}

// SetFlipY sets (or clears) the FlipY cell. Setting to true causes the
// renderer to mirror the shape along its horizontal axis.
func (s *Shape) SetFlipY(v bool) {
	if v {
		s.SetCellValue(CellFlipY, "1")
	} else {
		s.SetCellValue(CellFlipY, "0")
	}
	s.clearCellFormula(CellFlipY)
}

func (s *Shape) SetBeginX(v float64) { s.SetCellValue(CellBeginX, fmtFloat(v)) }
func (s *Shape) SetBeginY(v float64) { s.SetCellValue(CellBeginY, fmtFloat(v)) }
func (s *Shape) SetEndX(v float64)   { s.SetCellValue(CellEndX, fmtFloat(v)) }
func (s *Shape) SetEndY(v float64)   { s.SetCellValue(CellEndY, fmtFloat(v)) }

// SetMasterPageID sets (or clears) the shape's Master attribute and refreshes
// the cached MasterPageID. Pass "" to detach from any master.
func (s *Shape) SetMasterPageID(masterID string) {
	if masterID == "" {
		s.xml.RemoveAttr("Master")
	} else {
		if attr := s.xml.SelectAttr("Master"); attr != nil {
			attr.Value = masterID
		} else {
			s.xml.CreateAttr("Master", masterID)
		}
	}
	s.MasterPageID = masterID
}

// --- Style properties ---

func (s *Shape) LineWeight() float64 { return toFloat(s.CellValue(CellLineWeight)) }
func (s *Shape) LineColor() string   { return s.CellValue(CellLineColor) }
func (s *Shape) FillColor() string   { return s.CellValue(CellFillForegnd) }

// TextColor returns the first text color from the Character section.
func (s *Shape) TextColor() string {
	charSection := s.xml.FindElement("Section[@N='Character']")
	if charSection != nil {
		colorCell := charSection.FindElement("Row/Cell[@N='Color']")
		if colorCell != nil {
			return colorCell.SelectAttrValue("V", "")
		}
	}
	// Check master shape
	if master := s.MasterShape(); master != nil {
		return master.TextColor()
	}
	return ""
}

// TextSize returns the font size from the Character section in inches.
// Falls back to master shape if not found locally.
func (s *Shape) TextSize() float64 {
	charSection := s.xml.FindElement("Section[@N='Character']")
	if charSection != nil {
		sizeCell := charSection.FindElement("Row/Cell[@N='Size']")
		if sizeCell != nil {
			return toFloat(sizeCell.SelectAttrValue("V", ""))
		}
	}
	// Try master shape
	if s.MasterPageID != "" {
		if ms := s.MasterShape(); ms != nil {
			return ms.TextSize()
		}
	}
	return 0
}

// EndArrow returns the EndArrow cell value.
func (s *Shape) EndArrow() string { return s.CellValue(CellEndArrow) }

// LineStyleID returns the LineStyle attribute.
func (s *Shape) LineStyleID() string { return s.xml.SelectAttrValue("LineStyle", "") }

// FillStyleID returns the FillStyle attribute.
func (s *Shape) FillStyleID() string { return s.xml.SelectAttrValue("FillStyle", "") }

// TextStyleID returns the TextStyle attribute.
func (s *Shape) TextStyleID() string { return s.xml.SelectAttrValue("TextStyle", "") }

// --- Style setters ---

// SetLineWeight overrides the shape's line weight to a literal value and
// clears any formula on the cell (matches the SetFillColor / SetLineColor
// policy: explicit literal wins over inherited / theme-driven formula).
func (s *Shape) SetLineWeight(v float64) {
	s.SetCellValue(CellLineWeight, fmtFloat(v))
	s.clearCellFormula(CellLineWeight)
	// WRITER_AUDIT.md §4: Visio annotates LineWeight with U='PT' even
	// though the stored value is in inches. The U attribute is a display-
	// unit hint; emit it so byte-diffs against a Visio resave line up.
	if c, ok := s.Cells[string(CellLineWeight)]; ok {
		c.SetUnit("PT")
	}
}
// SetLineColor sets the line color to an explicit literal and clears any
// theme-binding formula on that cell. Without the F clear, an existing
// `F="THEMEGUARD(RGB(...))"` would survive on the local cell and Visio's
// next open would re-evaluate F, overwriting the V the user just set.
// Callers that want a theme-tracking color must use SetCellFormula
// explicitly after this.
func (s *Shape) SetLineColor(v string) {
	s.SetCellValue(CellLineColor, v)
	s.clearCellFormula(CellLineColor)
}

// SetFillColor sets the foreground fill color to an explicit literal and
// clears any theme-binding formula on that cell. See SetLineColor for the
// rationale.
func (s *Shape) SetFillColor(v string) {
	s.SetCellValue(CellFillForegnd, v)
	s.clearCellFormula(CellFillForegnd)
}

// clearCellFormula removes the F attribute from a local cell, if one exists.
// No-op if the cell has no local copy (still inherited from master).
func (s *Shape) clearCellFormula(name string) {
	if c, ok := s.Cells[name]; ok && c != nil && c.xml != nil && c.xml.Parent() == s.xml {
		c.xml.RemoveAttr("F")
	}
}

// SetTextColor sets the first text color in the Character section.
func (s *Shape) SetTextColor(v string) {
	s.ensureCharacterCell("Color", v)
}

// SetTextSize sets the font size in the Character section.
// Value is in inches (e.g., 0.111111 for 8pt = 8/72 in).
func (s *Shape) SetTextSize(v float64) {
	s.ensureCharacterCell("Size", fmtFloat(v))
}

// SetCharBold sets or clears bold formatting on the shape's text.
func (s *Shape) SetCharBold(bold bool) {
	s.setCharStyleBit(1, bold)
}

// SetCharItalic sets or clears italic formatting on the shape's text.
func (s *Shape) SetCharItalic(italic bool) {
	s.setCharStyleBit(2, italic)
}

// SetCharUnderline sets or clears underline formatting on the shape's text.
func (s *Shape) SetCharUnderline(underline bool) {
	s.setCharStyleBit(4, underline)
}

// setCharStyleBit sets or clears a bit in the Character Style cell.
// Bit 1=Bold, 2=Italic, 4=Underline.
func (s *Shape) setCharStyleBit(bit int, on bool) {
	val := 0
	charSection := s.xml.FindElement("Section[@N='Character']")
	if charSection != nil {
		if row := charSection.FindElement("Row"); row != nil {
			if cell := row.FindElement("Cell[@N='Style']"); cell != nil {
				val, _ = strconv.Atoi(cell.SelectAttrValue("V", "0"))
			}
		}
	}
	if on {
		val |= bit
	} else {
		val &^= bit
	}
	s.ensureCharacterCell("Style", strconv.Itoa(val))
}

// SetCharSize sets the font size in points (e.g., 12 for 12pt).
// Character.Size is stored as inches per MS-VSDX; Visio displays as points
// via the U='PT' annotation we emit alongside.
func (s *Shape) SetCharSize(pt float64) {
	s.ensureCharacterCell("Size", fmtFloat(pt/72.0))
	// Match Visio's canonical U='PT' annotation on Char.Size cells.
	if c, ok := s.Cells["Char.Size"]; ok {
		c.SetUnit("PT")
	}
}

// SetCharFont sets the font name for the shape's text.
func (s *Shape) SetCharFont(name string) {
	s.ensureCharacterCell("Font", name)
}

// ensureCharacterCell sets a cell in the Character section, creating the
// section and row if they don't exist.
func (s *Shape) ensureCharacterCell(cellName, value string) {
	s.ensureSectionCell("Character", cellName, value)
}

// ensureSectionCell sets a cell in a named section, creating the section and
// row when needed.
//
// EC-006: when we create a NEW local Row that shadows a master Row, we
// first copy every cell from the master Row into the new local Row. Visio
// interprets a partial local Row as fully local — i.e. every cell the
// master had but we didn't mirror is silently dropped on the next open. By
// pre-populating the master's siblings (with their full attribute set,
// including F formulas) we keep the inheritance semantics intact while
// still letting the caller override the one cell they actually care about.
func (s *Shape) ensureSectionCell(sectionName, cellName, value string) {
	section := s.xml.FindElement("Section[@N='" + sectionName + "']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", sectionName)
	}
	row := section.FindElement("Row")
	rowIsNew := row == nil
	if rowIsNew {
		row = section.CreateElement("Row")
		row.CreateAttr("IX", "0")
		// Copy master's same-section, same-row cells into this newly-local
		// row so siblings survive Visio's "partial row = fully local" rule.
		if ms := s.MasterShape(); ms != nil {
			msSection := ms.XML().FindElement("Section[@N='" + sectionName + "']")
			if msSection != nil {
				msRow := msSection.SelectElement("Row")
				if msRow != nil {
					for _, mc := range msRow.SelectElements("Cell") {
						if mc.SelectAttrValue("N", "") == cellName {
							continue // caller is about to set this one
						}
						clone := row.CreateElement("Cell")
						for _, attr := range mc.Attr {
							clone.CreateAttr(attr.Key, attr.Value)
						}
					}
				}
			}
		}
	}
	cell := row.FindElement("Cell[@N='" + cellName + "']")
	if cell == nil {
		cell = row.CreateElement("Cell")
		cell.CreateAttr("N", cellName)
	}
	cell.CreateAttr("V", value)
}

// Paragraph alignment constants.
const (
	AlignLeft    = 0
	AlignCenter  = 1
	AlignRight   = 2
	AlignJustify = 3
)

// SetParagraphAlign sets the horizontal text alignment.
// Use AlignLeft (0), AlignCenter (1), AlignRight (2), or AlignJustify (3).
func (s *Shape) SetParagraphAlign(align int) {
	s.ensureSectionCell("Paragraph", "HorzAlign", strconv.Itoa(align))
}

// Line pattern constants.
const (
	LinePatternNone       = 0
	LinePatternSolid      = 1
	LinePatternDash       = 2
	LinePatternDot        = 3
	LinePatternDashDot    = 4
	LinePatternDashDotDot = 5
)

// SetLinePattern sets the line pattern and clears any inherited formula.
// Use LinePatternSolid (1), LinePatternDash (2), LinePatternDot (3), etc.
func (s *Shape) SetLinePattern(pattern int) {
	s.SetCellValue(CellLinePattern, strconv.Itoa(pattern))
	s.clearCellFormula(CellLinePattern)
}

// Line cap constants for SetLineCap.
const (
	LineCapRound    = 0
	LineCapSquare   = 1
	LineCapExtended = 2
)

// SetLineCap sets the line cap style and clears any inherited formula.
// Use LineCapRound (0), LineCapSquare (1), or LineCapExtended (2).
func (s *Shape) SetLineCap(cap int) {
	s.SetCellValue(CellLineCap, strconv.Itoa(cap))
	s.clearCellFormula(CellLineCap)
}

// SetBeginArrow sets the begin arrow style and clears any inherited formula.
// Use 13 for standard arrow, 0 for none.
func (s *Shape) SetBeginArrow(v int) {
	s.SetCellValue(CellBeginArrow, strconv.Itoa(v))
	s.clearCellFormula(CellBeginArrow)
}

// SetEndArrow sets the EndArrow cell value and clears any inherited formula.
// Use 13 for standard arrow, 0 for none.
func (s *Shape) SetEndArrow(v int) {
	s.SetCellValue(CellEndArrow, strconv.Itoa(v))
	s.clearCellFormula(CellEndArrow)
}

// SetRounding sets the corner rounding radius in inches and clears any
// inherited formula.
func (s *Shape) SetRounding(radius float64) {
	s.SetCellValue(CellRounding, fmtFloat(radius))
	s.clearCellFormula(CellRounding)
}

// --- Fill style ---

// SetFillPattern sets the fill pattern and clears any inherited formula.
// 0=transparent, 1=solid, 2-24=hatches.
func (s *Shape) SetFillPattern(pattern int) {
	s.SetCellValue(CellFillPattern, strconv.Itoa(pattern))
	s.clearCellFormula(CellFillPattern)
}

// SetFillTransparency sets the foreground fill transparency (0.0=opaque,
// 1.0=fully transparent) and clears any inherited formula.
func (s *Shape) SetFillTransparency(v float64) {
	s.SetCellValue(CellFillForegndTrans, fmtFloat(v))
	s.clearCellFormula(CellFillForegndTrans)
}

// SetFillBkgndColor sets the background fill color and clears any inherited
// formula (see SetFillColor for the rationale).
func (s *Shape) SetFillBkgndColor(v string) {
	s.SetCellValue(CellFillBkgnd, v)
	s.clearCellFormula(CellFillBkgnd)
}

// SetFillBkgndTransparency sets the background fill transparency
// (0.0=opaque, 1.0=fully transparent) and clears any inherited formula.
func (s *Shape) SetFillBkgndTransparency(v float64) {
	s.SetCellValue(CellFillBkgndTrans, fmtFloat(v))
	s.clearCellFormula(CellFillBkgndTrans)
}

// --- Text block positioning ---

// SetTxtPinX sets the X position of the text block's pin relative to the shape origin.
func (s *Shape) SetTxtPinX(v float64) { s.SetCellValue(CellTxtPinX, fmtFloat(v)) }

// SetTxtPinY sets the Y position of the text block's pin relative to the shape origin.
func (s *Shape) SetTxtPinY(v float64) { s.SetCellValue(CellTxtPinY, fmtFloat(v)) }

// SetTxtLocPinX sets the X local pin within the text block.
func (s *Shape) SetTxtLocPinX(v float64) { s.SetCellValue(CellTxtLocPinX, fmtFloat(v)) }

// SetTxtLocPinY sets the Y local pin within the text block.
func (s *Shape) SetTxtLocPinY(v float64) { s.SetCellValue(CellTxtLocPinY, fmtFloat(v)) }

// SetTxtWidth sets the width of the text block.
func (s *Shape) SetTxtWidth(v float64) { s.SetCellValue(CellTxtWidth, fmtFloat(v)) }

// SetTxtHeight sets the height of the text block.
func (s *Shape) SetTxtHeight(v float64) { s.SetCellValue(CellTxtHeight, fmtFloat(v)) }

// SetTxtAngle sets the rotation angle of the text block in radians.
func (s *Shape) SetTxtAngle(v float64) { s.SetCellValue(CellTxtAngle, fmtFloat(v)) }

// --- Protection ---

func (s *Shape) setLock(cell CellName, locked bool) {
	v := "0"
	if locked {
		v = "1"
	}
	s.ensureSectionCell("Protection", cell, v)
}

// SetLockMove locks or unlocks shape movement (both X and Y).
func (s *Shape) SetLockMove(locked bool) {
	s.setLock(CellLockMoveX, locked)
	s.setLock(CellLockMoveY, locked)
}

// SetLockSize locks or unlocks shape resizing (both width and height).
func (s *Shape) SetLockSize(locked bool) {
	s.setLock(CellLockWidth, locked)
	s.setLock(CellLockHeight, locked)
}

// SetLockDelete locks or unlocks shape deletion.
func (s *Shape) SetLockDelete(locked bool) {
	s.setLock(CellLockDelete, locked)
}

// SetLockRotate locks or unlocks shape rotation.
func (s *Shape) SetLockRotate(locked bool) {
	s.setLock(CellLockRotate, locked)
}

// SetLockAspect locks or unlocks the aspect ratio when resizing.
func (s *Shape) SetLockAspect(locked bool) {
	s.setLock(CellLockAspect, locked)
}

// --- User-defined cells ---

// AddUserCell adds a user-defined cell to the shape.
// User cells store custom metadata without appearing in the Shape Data pane.
func (s *Shape) AddUserCell(name, value string) {
	section := s.xml.FindElement("Section[@N='User']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", "User")
	}

	row := section.CreateElement("Row")
	row.CreateAttr("N", name)
	addCellXML(row, "Value", value, "")
	addCellXML(row, "Prompt", "", "")
}

// UserCellValue returns the value of a user-defined cell, or "" if not found.
func (s *Shape) UserCellValue(name string) string {
	section := s.xml.FindElement("Section[@N='User']")
	if section == nil {
		return ""
	}
	row := section.FindElement("Row[@N='" + name + "']")
	if row == nil {
		return ""
	}
	cell := row.FindElement("Cell[@N='Value']")
	if cell == nil {
		return ""
	}
	return cell.SelectAttrValue("V", "")
}

// Hyperlink returns the first hyperlink on the shape as (address, description).
// Returns empty strings if the shape has no hyperlink.
func (s *Shape) Hyperlink() (address, description string) {
	section := s.xml.FindElement("Section[@N='Hyperlink']")
	if section == nil {
		return "", ""
	}
	row := section.FindElement("Row")
	if row == nil {
		return "", ""
	}
	for _, c := range row.SelectElements("Cell") {
		switch c.SelectAttrValue("N", "") {
		case "Address":
			address = c.SelectAttrValue("V", "")
		case "Description":
			description = c.SelectAttrValue("V", "")
		}
	}
	return address, description
}

// AddHyperlink adds a hyperlink to the shape.
func (s *Shape) AddHyperlink(address, description string) {
	section := s.xml.FindElement("Section[@N='Hyperlink']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", "Hyperlink")
	}

	maxRow := 0
	for _, row := range section.SelectElements("Row") {
		rowName := row.SelectAttrValue("N", "")
		if strings.HasPrefix(rowName, "Row_") {
			if n, err := strconv.Atoi(rowName[4:]); err == nil && n > maxRow {
				maxRow = n
			}
		}
	}

	row := section.CreateElement("Row")
	row.CreateAttr("N", fmt.Sprintf("Row_%d", maxRow+1))
	addCellXML(row, "Address", address, "")
	addCellXML(row, "Description", description, "")
	addCellXML(row, "SubAddress", "", "")
	addCellXML(row, "ExtraInfo", "", "")
	addCellXML(row, "Frame", "", "")
	addCellXML(row, "Default", "0", "")
	addCellXML(row, "Invisible", "0", "")
}

// AddConnectionPoint adds a connection point to the shape at the given local coordinates.
// x and y are in the shape's coordinate space (e.g., Width*0.5 / Height*0.5 for center).
func (s *Shape) AddConnectionPoint(x, y float64) {
	section := s.xml.FindElement("Section[@N='Connection']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", "Connection")
	}

	maxIX := -1
	for _, row := range section.SelectElements("Row") {
		if ix, err := strconv.Atoi(row.SelectAttrValue("IX", "")); err == nil && ix > maxIX {
			maxIX = ix
		}
	}

	row := section.CreateElement("Row")
	row.CreateAttr("IX", strconv.Itoa(maxIX+1))
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "DirX", "0", "")
	addCellXML(row, "DirY", "0", "")
	addCellXML(row, "Type", "0", "")
}

// SetLayerMember sets which layers this shape belongs to.
// layers is a semicolon-separated list of layer indices (e.g., "0" or "0;1").
func (s *Shape) SetLayerMember(layers string) {
	s.SetCellValue(CellLayerMember, layers)
}

// --- Scratch Section ---

// ScratchCell represents a scratch cell row in the Scratch section.
type ScratchCell struct {
	Row   int
	X     string
	Y     string
	A     string
	B     string
	C     string
	D     string
}

// ScratchCells returns all scratch cells from this shape.
func (s *Shape) ScratchCells() []ScratchCell {
	section := s.xml.FindElement("Section[@N='Scratch']")
	if section == nil {
		return nil
	}
	var result []ScratchCell
	for _, row := range section.SelectElements("Row") {
		ix, _ := strconv.Atoi(row.SelectAttrValue("IX", "0"))
		sc := ScratchCell{Row: ix}
		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			val := cell.SelectAttrValue("V", "")
			switch name {
			case "X":
				sc.X = val
			case "Y":
				sc.Y = val
			case "A":
				sc.A = val
			case "B":
				sc.B = val
			case "C":
				sc.C = val
			case "D":
				sc.D = val
			}
		}
		result = append(result, sc)
	}
	return result
}

// AddScratchCell adds a scratch cell row with the given values.
// Scratch cells are used for intermediate calculations and temporary storage.
func (s *Shape) AddScratchCell(x, y, a, b, c, d string) int {
	section := s.xml.FindElement("Section[@N='Scratch']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", "Scratch")
	}

	maxIX := -1
	for _, row := range section.SelectElements("Row") {
		if ix, err := strconv.Atoi(row.SelectAttrValue("IX", "")); err == nil && ix > maxIX {
			maxIX = ix
		}
	}
	ix := maxIX + 1

	row := section.CreateElement("Row")
	row.CreateAttr("IX", strconv.Itoa(ix))
	if x != "" {
		addCellXML(row, "X", x, "")
	}
	if y != "" {
		addCellXML(row, "Y", y, "")
	}
	if a != "" {
		addCellXML(row, "A", a, "")
	}
	if b != "" {
		addCellXML(row, "B", b, "")
	}
	if c != "" {
		addCellXML(row, "C", c, "")
	}
	if d != "" {
		addCellXML(row, "D", d, "")
	}
	return ix
}

// --- Actions Section ---

// Action represents a row in the Actions section (right-click menu items).
type Action struct {
	Name        string
	Menu        string
	Action      string
	Checked     bool
	Disabled    bool
	ReadOnly    bool
	Invisible   bool
	BeginGroup  bool
	TagName     string
	ButtonFace  string
	SortKey     string
}

// Actions returns all action rows from this shape.
func (s *Shape) Actions() []Action {
	section := s.xml.FindElement("Section[@N='Actions']")
	if section == nil {
		return nil
	}
	var result []Action
	for _, row := range section.SelectElements("Row") {
		a := Action{Name: row.SelectAttrValue("N", "")}
		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			val := cell.SelectAttrValue("V", "")
			switch name {
			case "Menu":
				a.Menu = val
			case "Action":
				a.Action = val
			case "Checked":
				a.Checked = val == "1"
			case "Disabled":
				a.Disabled = val == "1"
			case "ReadOnly":
				a.ReadOnly = val == "1"
			case "Invisible":
				a.Invisible = val == "1"
			case "BeginGroup":
				a.BeginGroup = val == "1"
			case "TagName":
				a.TagName = val
			case "ButtonFace":
				a.ButtonFace = val
			case "SortKey":
				a.SortKey = val
			}
		}
		result = append(result, a)
	}
	return result
}

// AddAction adds an action (right-click menu item) to the shape.
// name is a unique identifier, menu is the display text, action is the formula to execute.
func (s *Shape) AddAction(name, menu, action string) {
	section := s.xml.FindElement("Section[@N='Actions']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", "Actions")
	}

	row := section.CreateElement("Row")
	row.CreateAttr("N", name)
	addCellXML(row, "Menu", menu, "")
	addCellXML(row, "Action", "", action) // Action is typically a formula
	addCellXML(row, "Checked", "0", "")
	addCellXML(row, "Disabled", "0", "")
	addCellXML(row, "ReadOnly", "0", "")
	addCellXML(row, "Invisible", "0", "")
	addCellXML(row, "BeginGroup", "0", "")
}

// --- Field Section ---

// Field represents a text field in the shape.
type Field struct {
	Row       int
	Value     string
	Format    string
	Type      int // 0=String, 2=Numeric, 5=DateTime, 7=Duration
	UICategory int
	UICode    int
	UIFormat  int
	Calendar  int
	ObjectKind int
}

// Fields returns all field rows from this shape.
func (s *Shape) Fields() []Field {
	section := s.xml.FindElement("Section[@N='Field']")
	if section == nil {
		return nil
	}
	var result []Field
	for _, row := range section.SelectElements("Row") {
		ix, _ := strconv.Atoi(row.SelectAttrValue("IX", "0"))
		f := Field{Row: ix}
		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			val := cell.SelectAttrValue("V", "")
			switch name {
			case "Value":
				f.Value = val
			case "Format":
				f.Format = val
			case "Type":
				f.Type, _ = strconv.Atoi(val)
			case "UICat":
				f.UICategory, _ = strconv.Atoi(val)
			case "UICod":
				f.UICode, _ = strconv.Atoi(val)
			case "UIFmt":
				f.UIFormat, _ = strconv.Atoi(val)
			case "Calendar":
				f.Calendar, _ = strconv.Atoi(val)
			case "ObjectKind":
				f.ObjectKind, _ = strconv.Atoi(val)
			}
		}
		result = append(result, f)
	}
	return result
}

// AddField adds a text field to the shape.
// fieldType: 0=custom string, 2=numeric, 5=date/time
// value is the field value or formula.
func (s *Shape) AddField(fieldType int, value, format string) int {
	section := s.xml.FindElement("Section[@N='Field']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", "Field")
	}

	maxIX := -1
	for _, row := range section.SelectElements("Row") {
		if ix, err := strconv.Atoi(row.SelectAttrValue("IX", "")); err == nil && ix > maxIX {
			maxIX = ix
		}
	}
	ix := maxIX + 1

	row := section.CreateElement("Row")
	row.CreateAttr("IX", strconv.Itoa(ix))
	addCellXML(row, "Value", value, "")
	addCellXML(row, "Format", format, "")
	addCellXML(row, "Type", strconv.Itoa(fieldType), "")
	return ix
}

// --- Control Section ---

// Control represents a control handle on the shape.
type Control struct {
	Name      string
	X         float64
	Y         float64
	XDyn      string
	YDyn      string
	XCon      int
	YCon      int
	CanGlue   bool
	Tip       string
}

// Controls returns all control handles from this shape.
func (s *Shape) Controls() []Control {
	section := s.xml.FindElement("Section[@N='Control']")
	if section == nil {
		return nil
	}
	var result []Control
	for _, row := range section.SelectElements("Row") {
		c := Control{Name: row.SelectAttrValue("N", "")}
		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			val := cell.SelectAttrValue("V", "")
			switch name {
			case "X":
				c.X = toFloat(val)
			case "Y":
				c.Y = toFloat(val)
			case "XDyn":
				c.XDyn = val
			case "YDyn":
				c.YDyn = val
			case "XCon":
				c.XCon, _ = strconv.Atoi(val)
			case "YCon":
				c.YCon, _ = strconv.Atoi(val)
			case "CanGlue":
				c.CanGlue = val == "1"
			case "Prompt":
				c.Tip = val
			}
		}
		result = append(result, c)
	}
	return result
}

// AddControl adds a control handle to the shape at the given coordinates.
// name is a unique identifier, tip is the tooltip shown when hovering.
func (s *Shape) AddControl(name string, x, y float64, tip string) {
	section := s.xml.FindElement("Section[@N='Control']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", "Control")
	}

	row := section.CreateElement("Row")
	row.CreateAttr("N", name)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "XDyn", fmtFloat(x), "")
	addCellXML(row, "YDyn", fmtFloat(y), "")
	addCellXML(row, "XCon", "0", "")
	addCellXML(row, "YCon", "0", "")
	addCellXML(row, "CanGlue", "0", "")
	if tip != "" {
		addCellXML(row, "Prompt", tip, "")
	}
}

// --- Tabs Section ---

// TabStop represents a tab stop in the shape's text.
type TabStop struct {
	Row       int
	Position  float64
	Alignment int // 0=left, 1=center, 2=right, 3=decimal, 4=bar
	Leader    int // 0=none, 1=dots, 2=dashes, 3=underline
}

// TabStops returns all tab stops from this shape.
func (s *Shape) TabStops() []TabStop {
	section := s.xml.FindElement("Section[@N='Tabs']")
	if section == nil {
		return nil
	}
	var result []TabStop
	for _, row := range section.SelectElements("Row") {
		ix, _ := strconv.Atoi(row.SelectAttrValue("IX", "0"))
		t := TabStop{Row: ix}
		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			val := cell.SelectAttrValue("V", "")
			switch name {
			case "Position":
				t.Position = toFloat(val)
			case "Alignment":
				t.Alignment, _ = strconv.Atoi(val)
			case "Leader":
				t.Leader, _ = strconv.Atoi(val)
			}
		}
		result = append(result, t)
	}
	return result
}

// AddTabStop adds a tab stop to the shape.
// position is in inches, alignment: 0=left, 1=center, 2=right, 3=decimal.
func (s *Shape) AddTabStop(position float64, alignment int) int {
	section := s.xml.FindElement("Section[@N='Tabs']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", "Tabs")
	}

	maxIX := -1
	for _, row := range section.SelectElements("Row") {
		if ix, err := strconv.Atoi(row.SelectAttrValue("IX", "")); err == nil && ix > maxIX {
			maxIX = ix
		}
	}
	ix := maxIX + 1

	row := section.CreateElement("Row")
	row.CreateAttr("IX", strconv.Itoa(ix))
	addCellXML(row, "Position", fmtFloat(position), "")
	addCellXML(row, "Alignment", strconv.Itoa(alignment), "")
	return ix
}

// SetComment sets the Comment cell on the shape (visible as tooltip in Visio).
func (s *Shape) SetComment(text string) {
	s.SetCellValue("Comment", text)
}

// SetLineStyleID sets the LineStyle attribute on the shape element.
func (s *Shape) SetLineStyleID(v string) { s.xml.CreateAttr("LineStyle", v) }

// SetFillStyleID sets the FillStyle attribute on the shape element.
func (s *Shape) SetFillStyleID(v string) { s.xml.CreateAttr("FillStyle", v) }

// SetTextStyleID sets the TextStyle attribute on the shape element.
func (s *Shape) SetTextStyleID(v string) { s.xml.CreateAttr("TextStyle", v) }

// --- Text ---

// Text returns the text content of the shape. Falls back to master shape text.
func (s *Shape) Text() string {
	textElem := s.xml.FindElement("Text")
	if textElem != nil {
		return collectText(textElem)
	}
	if s.MasterPageID != "" {
		if ms := s.MasterShape(); ms != nil {
			return ms.Text()
		}
	}
	return ""
}

// SetText sets the text content of the shape. Replaces the entire Text
// element body: any existing <cp>/<pp>/<fld> format-marker children are
// removed because their indices reference text positions that no longer
// exist. The Character/Paragraph/Field SECTIONS on the shape are NOT
// touched — only the in-text markers. Callers who want format-rich text
// must re-insert markers themselves.
// Creates a Text element if one doesn't already exist.
func (s *Shape) SetText(text string) {
	textElem := s.xml.FindElement("Text")
	if textElem == nil {
		textElem = s.xml.CreateElement("Text")
	} else {
		for _, child := range textElem.ChildElements() {
			textElem.RemoveChild(child)
		}
		textElem.SetTail("")
	}
	textElem.SetText(text)
}

// AddGeometry creates a new empty Geometry section on the shape and returns it.
// Use the returned Geometry's AddMoveTo/AddLineTo methods to define the path.
func (s *Shape) AddGeometry() *Geometry {
	geomIndex := len(s.Geometries)
	section := s.xml.CreateElement("Section")
	section.CreateAttr("N", "Geometry")
	section.CreateAttr("IX", strconv.Itoa(geomIndex))

	// WRITER_AUDIT.md §3: Visio writes NoShow/NoSnap/NoQuickDrag at the
	// top of every Geometry section with V='0' F='No Formula'. They're
	// defaults but Visio's canonical form always includes them.
	addCellWithFormula(section, "NoShow", "0", "No Formula", "")
	addCellWithFormula(section, "NoSnap", "0", "No Formula", "")
	addCellWithFormula(section, "NoQuickDrag", "0", "No Formula", "")

	g := newGeometry(section, s, geomIndex)
	s.Geometries = append(s.Geometries, g)
	if s.Geometry == nil {
		s.Geometry = g
	}
	return g
}

// AddGeometryRect adds a rectangular geometry using relative coordinates (0-1).
// The rectangle fills the shape's Width x Height. Sets NoFill=0 (enable fill)
// and NoLine=1 (hide geometry border) so the shape's FillPattern/FillForegnd
// cells control the appearance.
func (s *Shape) AddGeometryRect() *Geometry {
	g := s.AddGeometry()
	// Explicit NoFill=0 is required — without it, Visio may inherit NoFill
	// from the stylesheet/theme and suppress the fill entirely.
	addCellXML(g.xml, "NoFill", "0", "")
	addCellXML(g.xml, "NoLine", "1", "")
	g.AddRelMoveTo(0, 0)
	g.AddRelLineTo(1, 0)
	g.AddRelLineTo(1, 1)
	g.AddRelLineTo(0, 1)
	g.AddRelLineTo(0, 0)
	return g
}

// --- Shape manipulation ---

// Move translates the shape by the given deltas. For 2D shapes only the
// pin (PinX/PinY) and geometry rows move. For 1D shapes (connectors) both
// endpoints (BeginX/Y AND EndX/Y) move as well — leaving EndX/Y anchored
// while BeginX/Y followed the pin would stretch the connector across the
// drawing.
func (s *Shape) Move(xDelta, yDelta float64) {
	if s.Geometry != nil {
		s.Geometry.Move(xDelta, yDelta)
	}
	if s.HasBeginX() {
		s.SetBeginX(s.BeginX() + xDelta)
		s.SetBeginY(s.BeginY() + yDelta)
		// EndX/Y travel with BeginX/Y for 1D shapes. Only update them when a
		// local EndX cell actually exists, so we don't materialize endpoint
		// cells on shapes that don't track them.
		if s.CellValue(CellEndX) != "" {
			s.SetEndX(s.EndX() + xDelta)
			s.SetEndY(s.EndY() + yDelta)
		}
	}
	s.SetX(s.X() + xDelta)
	s.SetY(s.Y() + yDelta)
}

// Remove removes this shape from its parent XML element AND strips any
// <Connect> elements on the page that reference this shape (or any of its
// descendant child shapes) as either the connector or the terminal shape.
//
// Without the Connect cleanup, removing a shape leaves dangling references
// in the page's <Connects> section: Visio's open-time validator flags this
// as a structural error and triggers a repair prompt. The regression-test
// lives in remove_contracts_test.go.
//
// Note: removing a TERMINAL shape leaves the connector shape itself intact
// at its current geometry — only the Connect bindings to the now-gone shape
// are stripped. Callers that want full cascade-delete must walk and remove
// the dangling connector shapes themselves.
func (s *Shape) Remove() {
	s.removeOrphanConnects()
	s.Parent.removeChildShape(s)
}

// removeOrphanConnects walks the page's <Connect> elements and removes any
// that reference this shape — or any of its descendant child shapes —
// through either FromSheet (connector role) or ToSheet (terminal role).
func (s *Shape) removeOrphanConnects() {
	if s.Page == nil || s.Page.xml == nil || s.Page.xml.Root() == nil {
		return
	}
	// Collect every shape ID about to disappear (this shape + recursive
	// child shapes). A group Remove takes all descendants with it; their
	// Connects need cleanup too.
	doomed := map[string]bool{s.ID: true}
	for _, desc := range s.AllShapes() {
		doomed[desc.ID] = true
	}
	// Iterate over a snapshot — we mutate the parent during iteration.
	for _, connectElem := range append([]*etree.Element(nil), s.Page.xml.Root().FindElements(".//Connect")...) {
		from := connectElem.SelectAttrValue("FromSheet", "")
		to := connectElem.SelectAttrValue("ToSheet", "")
		if doomed[from] || doomed[to] {
			if parent := connectElem.Parent(); parent != nil {
				parent.RemoveChild(connectElem)
			}
		}
	}
}

// removeChildShape removes a child shape from this group shape's XML.
func (s *Shape) removeChildShape(child *Shape) {
	s.xml.RemoveChild(child.xml)
}

// FindReplace finds and replaces text in this shape and all sub-shapes.
func (s *Shape) FindReplace(old, new string) {
	text := s.Text()
	s.SetText(strings.ReplaceAll(text, old, new))
	for _, child := range s.ChildShapes() {
		child.FindReplace(old, new)
	}
}

// ApplyTextFilter replaces {{key}} placeholders in shape text with context values.
func (s *Shape) ApplyTextFilter(context map[string]string) {
	text := s.Text()
	for key, value := range context {
		rKey := "{{" + key + "}}"
		text = strings.ReplaceAll(text, rKey, value)
	}
	s.SetText(text)
	for _, child := range s.ChildShapes() {
		child.ApplyTextFilter(context)
	}
}

// SetStartAndFinish sets the start and end positions of a connector or line shape.
// start is (x, y) of the beginning point, finish is (x, y) of the ending point.
func (s *Shape) SetStartAndFinish(startX, startY, finishX, finishY float64) {
	if !s.HasBeginX() {
		return // only apply to lines and connector shapes
	}

	isConnector := s.UniversalName() == "Dynamic connector"

	s.SetBeginX(startX)
	s.SetBeginY(startY)
	s.SetEndX(finishX)
	s.SetEndY(finishY)

	width := finishX - startX
	s.SetWidth(width)
	if isConnector {
		s.SetHeight(finishY - startY)
	} else {
		s.SetHeight(0)
	}

	s.SetX(startX)
	s.SetY(startY)

	if s.Geometry != nil {
		s.Geometry.SetMoveTo(0, 0, 0)
		s.Geometry.SetLineTo(width, finishY-startY, 0)
	}

	// Update text pin positions
	txtPinX := s.Cells[CellTxtPinX]
	txtPinY := s.Cells[CellTxtPinY]
	if txtPinX != nil && txtPinY != nil {
		if isConnector {
			txtPinX.SetValue(fmtFloat(width / 2))
			txtPinY.SetValue(fmtFloat((finishY - startY) / 2))
		} else {
			cx, cy := s.CenterXY()
			txtPinX.SetValue(fmtFloat(cx))
			txtPinY.SetValue(fmtFloat(cy))
		}
		pinXStr := txtPinX.Value()
		pinYStr := txtPinY.Value()
		s.SetCellValue("Control/TextPosition/X", pinXStr)
		s.SetCellValue("Control/TextPosition/Y", pinYStr)
		s.SetCellValue("Control/TextPosition/XDyn", pinXStr)
		s.SetCellValue("Control/TextPosition/YDyn", pinYStr)
	}

	// Evaluate formulas on all cells (shape cells + geometry cells)
	var cellElems []*etree.Element
	for _, c := range s.Cells {
		cellElems = append(cellElems, c.xml)
	}
	if s.Geometry != nil {
		for _, gc := range s.Geometry.Cells {
			cellElems = append(cellElems, gc.xml)
		}
		for _, row := range s.Geometry.Rows {
			for _, gc := range row.Cells {
				cellElems = append(cellElems, gc.xml)
			}
		}
	}
	for _, elem := range cellElems {
		formula := elem.SelectAttrValue("F", "")
		if formula != "" {
			cellName := elem.SelectAttrValue("N", "")
			if formula == "Inh" && s.MasterPageID != "" {
				if ms := s.MasterShape(); ms != nil {
					if masterCell, ok := ms.Cells[cellName]; ok {
						formula = masterCell.Formula()
					}
				}
			}
			if v, ok := CalcValue(s, formula); ok {
				elem.CreateAttr("V", fmtFloat(v))
			}
		}
	}
}

// RelativeBounds returns bounds relative to parent group shape.
func (s *Shape) RelativeBounds() (float64, float64, float64, float64) {
	bx, by, ex, ey := s.Bounds()
	if parentShape, ok := s.Parent.(*Shape); ok && parentShape.ShapeType == "Group" {
		pbx, pby, _, _ := parentShape.Bounds()
		bx += pbx
		by += pby
		ex += pbx
		ey += pby
	}
	return bx, by, ex, ey
}

// ParentShape returns the parent as a *Shape, or nil if the parent is a Page.
func (s *Shape) ParentShape() *Shape {
	if ps, ok := s.Parent.(*Shape); ok {
		return ps
	}
	return nil
}

// --- Child shapes ---

// ChildShapes returns the direct child Shape objects.
func (s *Shape) ChildShapes() []*Shape {
	var parentElement *etree.Element
	if s.ShapeType == "Group" {
		parentElement = s.xml.FindElement("Shapes")
	} else {
		parentElement = s.xml
	}
	if parentElement == nil {
		return nil
	}
	var shapes []*Shape
	for _, shapeElem := range parentElement.SelectElements("Shape") {
		shapes = append(shapes, newShape(shapeElem, s, s.Page))
	}
	return shapes
}

// AllShapes returns all shapes within this shape, recursively.
func (s *Shape) AllShapes() []*Shape {
	var shapes []*Shape
	for _, shape := range s.ChildShapes() {
		shapes = append(shapes, shape)
		if shape.ShapeType == "Group" {
			shapes = append(shapes, shape.AllShapes()...)
		}
	}
	return shapes
}

// GetMaxID returns the maximum shape ID within this shape and its children.
func (s *Shape) GetMaxID() int {
	maxID, _ := strconv.Atoi(s.ID)
	if s.ShapeType == "Group" {
		for _, shape := range s.ChildShapes() {
			if newMax := shape.GetMaxID(); newMax > maxID {
				maxID = newMax
			}
		}
	}
	return maxID
}

// --- Search methods ---

// FindShapeByID recursively searches for a shape by ID.
func (s *Shape) FindShapeByID(shapeID string) *Shape {
	for _, shape := range s.AllShapes() {
		if shape.ID == shapeID {
			return shape
		}
	}
	return nil
}

// FindShapeByText recursively searches for a shape containing the given text.
func (s *Shape) FindShapeByText(text string) *Shape {
	for _, shape := range s.AllShapes() {
		if strings.Contains(shape.Text(), text) {
			return shape
		}
	}
	return nil
}

// FindShapesByText recursively searches for all shapes containing the given text.
func (s *Shape) FindShapesByText(text string) []*Shape {
	var result []*Shape
	for _, shape := range s.AllShapes() {
		if strings.Contains(shape.Text(), text) {
			result = append(result, shape)
		}
	}
	return result
}

// FindShapeByPropertyLabel recursively searches for a shape by property label.
func (s *Shape) FindShapeByPropertyLabel(label string) *Shape {
	for _, shape := range s.AllShapes() {
		if _, ok := shape.DataProperties()[label]; ok {
			return shape
		}
	}
	return nil
}

// FindShapesByPropertyLabel recursively searches for all shapes by property label.
func (s *Shape) FindShapesByPropertyLabel(label string) []*Shape {
	var result []*Shape
	for _, shape := range s.AllShapes() {
		if _, ok := shape.DataProperties()[label]; ok {
			result = append(result, shape)
		}
	}
	return result
}

// FindShapeByPropertyLabelValue recursively searches for a shape by property label and value.
func (s *Shape) FindShapeByPropertyLabelValue(label, value string) *Shape {
	for _, shape := range s.AllShapes() {
		if prop, ok := shape.DataProperties()[label]; ok {
			if prop.Value() == value {
				return shape
			}
		}
	}
	return nil
}

// FindShapesByPropertyLabelValue recursively searches for all shapes by property label and value.
func (s *Shape) FindShapesByPropertyLabelValue(label, value string) []*Shape {
	var result []*Shape
	for _, shape := range s.AllShapes() {
		if prop, ok := shape.DataProperties()[label]; ok {
			if prop.Value() == value {
				result = append(result, shape)
			}
		}
	}
	return result
}

// ShapeValue returns the value of an XML attribute on the shape element.
func (s *Shape) ShapeValue(name string) string {
	return s.xml.SelectAttrValue(name, "")
}

// FindShapesByID recursively searches for all shapes with the given ID.
func (s *Shape) FindShapesByID(shapeID string) []*Shape {
	var result []*Shape
	for _, shape := range s.AllShapes() {
		if shape.ID == shapeID {
			result = append(result, shape)
		}
	}
	return result
}

// FindShapesByMaster recursively searches for all shapes with the given master page and shape IDs.
func (s *Shape) FindShapesByMaster(masterPageID, masterShapeID string) []*Shape {
	var result []*Shape
	for _, shape := range s.AllShapes() {
		if shape.MasterShapeID == masterShapeID && shape.MasterPageID == masterPageID {
			result = append(result, shape)
		}
	}
	return result
}

// FindShapesByRegex recursively searches for all shapes whose text matches the given regex pattern.
func (s *Shape) FindShapesByRegex(pattern string) ([]*Shape, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex %q: %w", pattern, err)
	}
	var result []*Shape
	for _, shape := range s.AllShapes() {
		if re.FindString(shape.Text()) != "" {
			result = append(result, shape)
		}
	}
	return result, nil
}

// FindShapeByAttr searches for a shape by XML attribute name and value.
func (s *Shape) FindShapeByAttr(attr, attrValue string) *Shape {
	for _, shape := range s.AllShapes() {
		if shape.xml.SelectAttrValue(attr, "") == attrValue {
			return shape
		}
	}
	return nil
}

// --- Data properties ---

// DataProperties returns the data properties of the shape, indexed by label.
// Results are cached after the first call.
// RecalculateDependents walks every cell on this shape, identifies those
// whose F-formula references the named cell as a word-boundary token, and
// re-evaluates them via CalcValue, writing the new V-attribute. Returns the
// number of cells whose V was actually updated.
//
// EC-012 minimum-viable dependency walker — one level deep, single shape.
// Does NOT handle:
//   - Transitive chains (A→B→C): call recursively / fixed-point yourself
//   - Cross-shape references (Sheet.N!Cell)
//   - Cycle detection (cells with circular F refs blow the stack)
//
// Typical use: after SetX(newPin) on a 1D shape whose Width has F=EndX-PinX,
// call shape.RecalculateDependents(CellPinX) so Width's V picks up the new
// derivation without waiting for Visio's next open.
func (s *Shape) RecalculateDependents(cellName CellName) int {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(string(cellName)) + `\b`)
	count := 0
	for name, cell := range s.Cells {
		if name == string(cellName) || cell == nil {
			continue
		}
		f := cell.Formula()
		if f == "" || !pattern.MatchString(f) {
			continue
		}
		if v, ok := CalcValue(s, f); ok {
			cell.SetValue(fmtFloat(v))
			count++
		}
	}
	return count
}

// InvalidateInheritanceCaches resets every lazily-cached state that depends
// on master inheritance. Call this after directly mutating a master shape
// (or any state that instances may have cached) so subsequent reads of this
// shape pick up the new master state. EC-003.
//
// Safe to call on any shape, including ones without a master. Today this
// only clears the data-properties cache, but the method is the single
// public hook future caches should subscribe to.
func (s *Shape) InvalidateInheritanceCaches() {
	s.InvalidateDataPropertiesCache()
}

// InvalidateInstanceCachesForMaster walks every shape on every page of the
// VisioFile and clears inheritance-derived caches on those whose
// MasterPageID matches the given master ID. EC-003 — useful after mutating
// a master shape so all dependent instances re-resolve at next read.
//
// NOTE on semantics: this clears caches on the *Shape pointers reachable
// via AllShapes() at call time. Page.AllShapes() rebuilds Shape structs on
// each call, so callers who hold their own *Shape pointer must invoke
// (*Shape).InvalidateInheritanceCaches() on it directly — the file-level
// helper has no way to reach private pointers.
//
// O(shapes) traversal; intended for explicit "I just edited a master" calls,
// not for hot paths.
func (v *VisioFile) InvalidateInstanceCachesForMaster(masterPageID string) {
	for _, p := range v.Pages {
		for _, s := range p.AllShapes() {
			if s.MasterPageID == masterPageID {
				s.InvalidateInheritanceCaches()
			}
		}
	}
}

// RemoveCell deletes the named cell from this shape's local XML (if
// present). After this call, reading the cell via CellValue / CellFormula
// falls back to the master's value, restoring inheritance. No-op when no
// local cell exists. EC-010.
//
// Returns true if a local cell was actually removed.
func (s *Shape) RemoveCell(name CellName) bool {
	cell := s.xml.FindElement("Cell[@N='" + string(name) + "']")
	if cell == nil {
		return false
	}
	s.xml.RemoveChild(cell)
	delete(s.Cells, string(name))
	return true
}

// RevertToInherited is a semantic alias for RemoveCell: explicitly states
// "let the master drive this property again". Useful in style-editing UIs
// where the operation is conceptually "revert", not "delete". EC-010.
func (s *Shape) RevertToInherited(name CellName) bool {
	return s.RemoveCell(name)
}

// GeometryAt returns the Geometry section at the given IX index, or nil if
// out of range. EC-005: compound master shapes carry multiple geometry
// sections (IX=0, 1, 2, ...). Use this helper to target sections beyond
// IX=0; shape.Geometry is an alias for Geometries[0] and only ever touches
// the first section.
func (s *Shape) GeometryAt(idx int) *Geometry {
	if idx < 0 || idx >= len(s.Geometries) {
		return nil
	}
	return s.Geometries[idx]
}

// InvalidateDataPropertiesCache resets the lazy-loaded data-properties cache
// so the next DataProperties() call reads fresh state from XML. AddDataProperty
// already calls this; users who mutate the Property section directly via
// shape.XML() must call this themselves before re-reading DataProperties().
func (s *Shape) InvalidateDataPropertiesCache() {
	s.dataProperties = nil
}

func (s *Shape) DataProperties() map[string]*DataProperty {
	if s.dataProperties != nil {
		return s.dataProperties
	}

	properties := make(map[string]*DataProperty)

	// Start with master data properties
	if ms := s.MasterShape(); ms != nil {
		for k, v := range ms.DataProperties() {
			properties[k] = v
		}
	}

	// Add/overwrite with local properties
	propSection := s.xml.FindElement("Section[@N='Property']")
	if propSection != nil {
		for _, rowElem := range propSection.SelectElements("Row") {
			dp := newDataProperty(rowElem, s)
			properties[dp.Label] = dp
		}
	}

	s.dataProperties = properties
	return properties
}

// --- Bounds ---

// Bounds returns the absolute bounds (beginX, beginY, endX, endY) of the shape relative to the page.
func (s *Shape) Bounds() (float64, float64, float64, float64) {
	if !s.HasBeginX() && !s.HasCell(CellPinX) && !s.HasCell(CellLocPinX) {
		return 0, 0, 0, 0
	}
	bx := s.BeginX()
	if bx == 0 {
		bx = s.X() - s.LocX()
	}
	by := s.BeginY()
	if by == 0 {
		by = s.Y() - s.LocY()
	}
	ex := s.EndX()
	if ex == 0 {
		ex = bx + s.Width()
	}
	ey := s.EndY()
	if ey == 0 {
		ey = by + s.Height()
	}
	return bx, by, ex, ey
}

// CenterXY returns the center position of the shape.
func (s *Shape) CenterXY() (float64, float64) {
	if s.HasBeginX() {
		return s.BeginX() + s.Width()/2, s.BeginY() + s.Height()/2
	}
	return s.X(), s.Y()
}

// Center returns the center position of the shape as a Point.
func (s *Shape) Center() Point {
	x, y := s.CenterXY()
	return Point{X: x, Y: y}
}

// BoundsRect returns the absolute bounds of the shape as a Rect.
func (s *Shape) BoundsRect() Rect {
	bx, by, ex, ey := s.Bounds()
	return Rect{X: bx, Y: by, Width: ex - bx, Height: ey - by}
}

// RelativeBoundsRect returns bounds relative to parent group shape as a Rect.
func (s *Shape) RelativeBoundsRect() Rect {
	bx, by, ex, ey := s.RelativeBounds()
	return Rect{X: bx, Y: by, Width: ex - bx, Height: ey - by}
}

// --- SmartTag Section ---

// SmartTag represents a smart tag (action button) on a shape.
type SmartTag struct {
	Name        string
	X           float64
	Y           float64
	XJustify    int
	YJustify    int
	DisplayMode int
	ButtonFace  string
	Description string
	Disabled    bool
}

// SmartTags returns all smart tags from this shape.
func (s *Shape) SmartTags() []SmartTag {
	section := s.xml.FindElement("Section[@N='SmartTag']")
	if section == nil {
		return nil
	}
	var result []SmartTag
	for _, row := range section.SelectElements("Row") {
		st := SmartTag{Name: row.SelectAttrValue("N", "")}
		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			val := cell.SelectAttrValue("V", "")
			switch name {
			case "X":
				st.X, _ = strconv.ParseFloat(val, 64)
			case "Y":
				st.Y, _ = strconv.ParseFloat(val, 64)
			case "XJustify":
				st.XJustify, _ = strconv.Atoi(val)
			case "YJustify":
				st.YJustify, _ = strconv.Atoi(val)
			case "DisplayMode":
				st.DisplayMode, _ = strconv.Atoi(val)
			case "ButtonFace":
				st.ButtonFace = val
			case "Description":
				st.Description = val
			case "Disabled":
				st.Disabled = val == "1"
			}
		}
		result = append(result, st)
	}
	return result
}

// AddSmartTag adds a smart tag to the shape at the given position.
func (s *Shape) AddSmartTag(name string, x, y float64, description string) {
	section := s.xml.FindElement("Section[@N='SmartTag']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", "SmartTag")
	}

	row := section.CreateElement("Row")
	row.CreateAttr("N", name)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "XJustify", "1", "")
	addCellXML(row, "YJustify", "1", "")
	addCellXML(row, "DisplayMode", "0", "")
	addCellXML(row, "Description", description, "")
	addCellXML(row, "Disabled", "0", "")
}

// --- ActionTag Section ---

// ActionTag represents an action tag on a shape (similar to SmartTag but with action).
type ActionTag struct {
	Name        string
	X           float64
	Y           float64
	TagName     string
	XJustify    int
	YJustify    int
	DisplayMode int
	ButtonFace  string
	Description string
	Disabled    bool
}

// ActionTags returns all action tags from this shape.
func (s *Shape) ActionTags() []ActionTag {
	section := s.xml.FindElement("Section[@N='ActionTag']")
	if section == nil {
		return nil
	}
	var result []ActionTag
	for _, row := range section.SelectElements("Row") {
		at := ActionTag{Name: row.SelectAttrValue("N", "")}
		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			val := cell.SelectAttrValue("V", "")
			switch name {
			case "X":
				at.X, _ = strconv.ParseFloat(val, 64)
			case "Y":
				at.Y, _ = strconv.ParseFloat(val, 64)
			case "TagName":
				at.TagName = val
			case "XJustify":
				at.XJustify, _ = strconv.Atoi(val)
			case "YJustify":
				at.YJustify, _ = strconv.Atoi(val)
			case "DisplayMode":
				at.DisplayMode, _ = strconv.Atoi(val)
			case "ButtonFace":
				at.ButtonFace = val
			case "Description":
				at.Description = val
			case "Disabled":
				at.Disabled = val == "1"
			}
		}
		result = append(result, at)
	}
	return result
}

// AddActionTag adds an action tag to the shape.
func (s *Shape) AddActionTag(name string, x, y float64, tagName, description string) {
	section := s.xml.FindElement("Section[@N='ActionTag']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", "ActionTag")
	}

	row := section.CreateElement("Row")
	row.CreateAttr("N", name)
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "TagName", tagName, "")
	addCellXML(row, "XJustify", "1", "")
	addCellXML(row, "YJustify", "1", "")
	addCellXML(row, "DisplayMode", "0", "")
	addCellXML(row, "Description", description, "")
	addCellXML(row, "Disabled", "0", "")
}

// --- ConnectionABCD Section ---

// ConnectionABCD represents an extended connection point with directional info.
type ConnectionABCD struct {
	Row  int
	X    float64
	Y    float64
	A    float64 // Direction X
	B    float64 // Direction Y
	C    int     // Connection type
	D    int     // AutoGen flag
	DirX float64
	DirY float64
	Type int
}

// ConnectionsABCD returns extended connection points with directional information.
func (s *Shape) ConnectionsABCD() []ConnectionABCD {
	section := s.xml.FindElement("Section[@N='ConnectionABCD']")
	if section == nil {
		// Fall back to regular Connection section
		section = s.xml.FindElement("Section[@N='Connection']")
	}
	if section == nil {
		return nil
	}

	var result []ConnectionABCD
	for _, row := range section.SelectElements("Row") {
		ix, _ := strconv.Atoi(row.SelectAttrValue("IX", "0"))
		cp := ConnectionABCD{Row: ix}
		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			val := cell.SelectAttrValue("V", "")
			switch name {
			case "X":
				cp.X, _ = strconv.ParseFloat(val, 64)
			case "Y":
				cp.Y, _ = strconv.ParseFloat(val, 64)
			case "A":
				cp.A, _ = strconv.ParseFloat(val, 64)
			case "B":
				cp.B, _ = strconv.ParseFloat(val, 64)
			case "C":
				cp.C, _ = strconv.Atoi(val)
			case "D":
				cp.D, _ = strconv.Atoi(val)
			case "DirX":
				cp.DirX, _ = strconv.ParseFloat(val, 64)
			case "DirY":
				cp.DirY, _ = strconv.ParseFloat(val, 64)
			case "Type":
				cp.Type, _ = strconv.Atoi(val)
			}
		}
		result = append(result, cp)
	}
	return result
}

// AddConnectionABCD adds an extended connection point with direction.
func (s *Shape) AddConnectionABCD(x, y, dirX, dirY float64, connType int) int {
	section := s.xml.FindElement("Section[@N='ConnectionABCD']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", "ConnectionABCD")
	}

	maxIX := -1
	for _, row := range section.SelectElements("Row") {
		if ix, err := strconv.Atoi(row.SelectAttrValue("IX", "")); err == nil && ix > maxIX {
			maxIX = ix
		}
	}
	ix := maxIX + 1

	row := section.CreateElement("Row")
	row.CreateAttr("IX", strconv.Itoa(ix))
	addCellXML(row, "X", fmtFloat(x), "")
	addCellXML(row, "Y", fmtFloat(y), "")
	addCellXML(row, "DirX", fmtFloat(dirX), "")
	addCellXML(row, "DirY", fmtFloat(dirY), "")
	addCellXML(row, "Type", strconv.Itoa(connType), "")
	addCellXML(row, "A", "0", "")
	addCellXML(row, "B", "0", "")
	addCellXML(row, "C", "0", "")
	addCellXML(row, "D", "0", "")

	return ix
}

// --- Connects ---

// Connects returns all Connect objects related to this shape.
func (s *Shape) Connects() []*Connect {
	var connects []*Connect
	for _, c := range s.Page.Connects() {
		if s.ID == c.ShapeID() || s.ID == c.ConnectorShapeID() {
			connects = append(connects, c)
		}
	}
	return connects
}

// ConnectedShapes returns a list of shapes connected to this shape.
func (s *Shape) ConnectedShapes() []*Shape {
	var shapes []*Shape
	for _, c := range s.Connects() {
		if c.ConnectorShapeID() != s.ID {
			if shape := s.Page.FindShapeByID(c.ConnectorShapeID()); shape != nil {
				shapes = append(shapes, shape)
			}
		}
		if c.ShapeID() != s.ID {
			if shape := s.Page.FindShapeByID(c.ShapeID()); shape != nil {
				shapes = append(shapes, shape)
			}
		}
	}
	return shapes
}

// UniversalName returns the universal name of the shape.
func (s *Shape) UniversalName() string {
	nameUniv := s.xml.SelectAttrValue("NameU", "")
	if ms := s.MasterShape(); ms != nil {
		pageSheet := ms.Page.pagesheetXML()
		if pageSheet != nil {
			layer := pageSheet.FindElement("Section[@N='Layer']")
			if layer != nil {
				nameUnivCell := layer.FindElement("Cell[@N='NameUniv']")
				if nameUnivCell != nil {
					if v := nameUnivCell.SelectAttrValue("V", ""); v != "" {
						nameUniv = v
					}
				}
			}
		}
	}
	return nameUniv
}

// --- 3D/Bevel Effect Methods (MS-VSDX §2.2.7.3.2) ---

// BevelEffect holds 3D bevel effect properties for a shape.
type BevelEffect struct {
	TopType       int     // Bevel type for top face (0=none, 1-12=types)
	TopWidth      float64 // Width of top bevel
	TopHeight     float64 // Height of top bevel
	BottomType    int     // Bevel type for bottom face
	BottomWidth   float64 // Width of bottom bevel
	BottomHeight  float64 // Height of bottom bevel
	DepthColor    string  // Color of bevel depth
	DepthSize     float64 // Size of bevel depth
	ContourColor  string  // Color of bevel contour
	ContourSize   float64 // Size of bevel contour
	MaterialType  int     // Material type (1-10)
	LightingType  int     // Lighting type (1-15)
	LightingAngle float64 // Lighting angle in degrees
}

// BevelEffect returns the 3D bevel effect properties for this shape.
func (s *Shape) BevelEffect() *BevelEffect {
	return &BevelEffect{
		TopType:       int(toFloat(s.CellValue(CellBevelTopType))),
		TopWidth:      toFloat(s.CellValue(CellBevelTopWidth)),
		TopHeight:     toFloat(s.CellValue(CellBevelTopHeight)),
		BottomType:    int(toFloat(s.CellValue(CellBevelBottomType))),
		BottomWidth:   toFloat(s.CellValue(CellBevelBottomWidth)),
		BottomHeight:  toFloat(s.CellValue(CellBevelBottomHeight)),
		DepthColor:    s.CellValue(CellBevelDepthColor),
		DepthSize:     toFloat(s.CellValue(CellBevelDepthSize)),
		ContourColor:  s.CellValue(CellBevelContourColor),
		ContourSize:   toFloat(s.CellValue(CellBevelContourSize)),
		MaterialType:  int(toFloat(s.CellValue(CellBevelMaterialType))),
		LightingType:  int(toFloat(s.CellValue(CellBevelLightingType))),
		LightingAngle: toFloat(s.CellValue(CellBevelLightingAngle)),
	}
}

// SetBevelEffect sets the 3D bevel effect properties for this shape.
func (s *Shape) SetBevelEffect(effect *BevelEffect) {
	if effect == nil {
		return
	}
	s.SetCellValue(CellBevelTopType, strconv.Itoa(effect.TopType))
	s.SetCellValue(CellBevelTopWidth, fmtFloat(effect.TopWidth))
	s.SetCellValue(CellBevelTopHeight, fmtFloat(effect.TopHeight))
	s.SetCellValue(CellBevelBottomType, strconv.Itoa(effect.BottomType))
	s.SetCellValue(CellBevelBottomWidth, fmtFloat(effect.BottomWidth))
	s.SetCellValue(CellBevelBottomHeight, fmtFloat(effect.BottomHeight))
	if effect.DepthColor != "" {
		s.SetCellValue(CellBevelDepthColor, effect.DepthColor)
	}
	s.SetCellValue(CellBevelDepthSize, fmtFloat(effect.DepthSize))
	if effect.ContourColor != "" {
		s.SetCellValue(CellBevelContourColor, effect.ContourColor)
	}
	s.SetCellValue(CellBevelContourSize, fmtFloat(effect.ContourSize))
	s.SetCellValue(CellBevelMaterialType, strconv.Itoa(effect.MaterialType))
	s.SetCellValue(CellBevelLightingType, strconv.Itoa(effect.LightingType))
	s.SetCellValue(CellBevelLightingAngle, fmtFloat(effect.LightingAngle))
}

// --- Glow Effect Methods (MS-VSDX §2.2.7.3.3) ---

// GlowEffect holds glow effect properties for a shape.
type GlowEffect struct {
	Color      string  // Glow color
	ColorTrans float64 // Glow color transparency (0-1)
	Size       float64 // Glow size in points
}

// GlowEffect returns the glow effect properties for this shape.
func (s *Shape) GlowEffect() *GlowEffect {
	return &GlowEffect{
		Color:      s.CellValue(CellGlowColor),
		ColorTrans: toFloat(s.CellValue(CellGlowColorTrans)),
		Size:       toFloat(s.CellValue(CellGlowSize)),
	}
}

// SetGlowEffect sets the glow effect properties for this shape.
func (s *Shape) SetGlowEffect(effect *GlowEffect) {
	if effect == nil {
		return
	}
	if effect.Color != "" {
		s.SetCellValue(CellGlowColor, effect.Color)
	}
	s.SetCellValue(CellGlowColorTrans, fmtFloat(effect.ColorTrans))
	s.SetCellValue(CellGlowSize, fmtFloat(effect.Size))
}

// --- Reflection Effect Methods (MS-VSDX §2.2.7.3.4) ---

// ReflectionEffect holds reflection effect properties for a shape.
type ReflectionEffect struct {
	Size  float64 // Reflection size (0-100)
	Trans float64 // Reflection transparency (0-1)
	Dist  float64 // Distance from shape
	Blur  float64 // Blur amount
}

// ReflectionEffect returns the reflection effect properties for this shape.
func (s *Shape) ReflectionEffect() *ReflectionEffect {
	return &ReflectionEffect{
		Size:  toFloat(s.CellValue(CellReflectionSize)),
		Trans: toFloat(s.CellValue(CellReflectionTrans)),
		Dist:  toFloat(s.CellValue(CellReflectionDist)),
		Blur:  toFloat(s.CellValue(CellReflectionBlur)),
	}
}

// SetReflectionEffect sets the reflection effect properties for this shape.
func (s *Shape) SetReflectionEffect(effect *ReflectionEffect) {
	if effect == nil {
		return
	}
	s.SetCellValue(CellReflectionSize, fmtFloat(effect.Size))
	s.SetCellValue(CellReflectionTrans, fmtFloat(effect.Trans))
	s.SetCellValue(CellReflectionDist, fmtFloat(effect.Dist))
	s.SetCellValue(CellReflectionBlur, fmtFloat(effect.Blur))
}

// --- Soft Edges Effect Methods (MS-VSDX §2.2.7.3.5) ---

// SoftEdgesSize returns the soft edges size for this shape.
func (s *Shape) SoftEdgesSize() float64 {
	return toFloat(s.CellValue(CellSoftEdgesSize))
}

// SetSoftEdgesSize sets the soft edges size for this shape.
func (s *Shape) SetSoftEdgesSize(size float64) {
	s.SetCellValue(CellSoftEdgesSize, fmtFloat(size))
}

// --- Sketch Effect Methods (MS-VSDX §2.2.7.3.6) ---

// SketchEffect holds sketch effect properties for a shape.
type SketchEffect struct {
	Enabled    bool    // Whether sketch effect is enabled
	Seed       int     // Random seed for sketch
	Amount     float64 // Amount of sketch distortion
	LineWeight float64 // Line weight variation
	LineChange float64 // Line change variation
	FillChange float64 // Fill change variation
}

// SketchEffect returns the sketch effect properties for this shape.
func (s *Shape) SketchEffect() *SketchEffect {
	return &SketchEffect{
		Enabled:    toFloat(s.CellValue(CellSketchEnabled)) != 0,
		Seed:       int(toFloat(s.CellValue(CellSketchSeed))),
		Amount:     toFloat(s.CellValue(CellSketchAmount)),
		LineWeight: toFloat(s.CellValue(CellSketchLineWeight)),
		LineChange: toFloat(s.CellValue(CellSketchLineChange)),
		FillChange: toFloat(s.CellValue(CellSketchFillChange)),
	}
}

// SetSketchEffect sets the sketch effect properties for this shape.
func (s *Shape) SetSketchEffect(effect *SketchEffect) {
	if effect == nil {
		return
	}
	if effect.Enabled {
		s.SetCellValue(CellSketchEnabled, "1")
	} else {
		s.SetCellValue(CellSketchEnabled, "0")
	}
	s.SetCellValue(CellSketchSeed, strconv.Itoa(effect.Seed))
	s.SetCellValue(CellSketchAmount, fmtFloat(effect.Amount))
	s.SetCellValue(CellSketchLineWeight, fmtFloat(effect.LineWeight))
	s.SetCellValue(CellSketchLineChange, fmtFloat(effect.LineChange))
	s.SetCellValue(CellSketchFillChange, fmtFloat(effect.FillChange))
}

// --- 3D Rotation Effect Methods (MS-VSDX §2.2.7.3.7) ---

// Rotation3DEffect holds 3D rotation effect properties for a shape.
type Rotation3DEffect struct {
	XAngle           float64 // Rotation around X axis in degrees
	YAngle           float64 // Rotation around Y axis in degrees
	ZAngle           float64 // Rotation around Z axis in degrees
	RotationType     int     // Rotation type (0=parallel, 1=perspective)
	Perspective      float64 // Perspective field of view
	DistanceFromGround float64 // Distance from ground plane
	KeepTextFlat     bool    // Whether to keep text flat (not rotated)
}

// Rotation3DEffect returns the 3D rotation effect properties for this shape.
func (s *Shape) Rotation3DEffect() *Rotation3DEffect {
	return &Rotation3DEffect{
		XAngle:           toFloat(s.CellValue(CellRotationXAngle)),
		YAngle:           toFloat(s.CellValue(CellRotationYAngle)),
		ZAngle:           toFloat(s.CellValue(CellRotationZAngle)),
		RotationType:     int(toFloat(s.CellValue(CellRotationType))),
		Perspective:      toFloat(s.CellValue(CellPerspective)),
		DistanceFromGround: toFloat(s.CellValue(CellDistanceFromGround)),
		KeepTextFlat:     toFloat(s.CellValue(CellKeepTextFlat)) != 0,
	}
}

// SetRotation3DEffect sets the 3D rotation effect properties for this shape.
func (s *Shape) SetRotation3DEffect(effect *Rotation3DEffect) {
	if effect == nil {
		return
	}
	s.SetCellValue(CellRotationXAngle, fmtFloat(effect.XAngle))
	s.SetCellValue(CellRotationYAngle, fmtFloat(effect.YAngle))
	s.SetCellValue(CellRotationZAngle, fmtFloat(effect.ZAngle))
	s.SetCellValue(CellRotationType, strconv.Itoa(effect.RotationType))
	s.SetCellValue(CellPerspective, fmtFloat(effect.Perspective))
	s.SetCellValue(CellDistanceFromGround, fmtFloat(effect.DistanceFromGround))
	if effect.KeepTextFlat {
		s.SetCellValue(CellKeepTextFlat, "1")
	} else {
		s.SetCellValue(CellKeepTextFlat, "0")
	}
}
