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
		g := newGeometry(geometrySection, s)
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
func (s *Shape) SetWidth(v float64)  { s.SetCellValue(CellWidth, fmtFloat(v)) }
func (s *Shape) SetHeight(v float64) { s.SetCellValue(CellHeight, fmtFloat(v)) }
func (s *Shape) SetAngle(v float64)  { s.SetCellValue(CellAngle, fmtFloat(v)) }

func (s *Shape) SetBeginX(v float64) { s.SetCellValue(CellBeginX, fmtFloat(v)) }
func (s *Shape) SetBeginY(v float64) { s.SetCellValue(CellBeginY, fmtFloat(v)) }
func (s *Shape) SetEndX(v float64)   { s.SetCellValue(CellEndX, fmtFloat(v)) }
func (s *Shape) SetEndY(v float64)   { s.SetCellValue(CellEndY, fmtFloat(v)) }

// --- Style properties ---

func (s *Shape) LineWeight() float64 { return toFloat(s.CellValue(CellLineWeight)) }
func (s *Shape) LineColor() string   { return s.CellValue(CellLineColor) }
func (s *Shape) FillColor() string   { return s.CellValue(CellFillForegnd) }

// TextColor returns the first text color from the Character section.
func (s *Shape) TextColor() string {
	charSection := s.xml.FindElement("Section[@N='Character']")
	if charSection == nil {
		return ""
	}
	colorCell := charSection.FindElement("Row/Cell[@N='Color']")
	if colorCell != nil {
		return colorCell.SelectAttrValue("V", "")
	}
	return ""
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

func (s *Shape) SetLineWeight(v float64) {
	s.SetCellValue(CellLineWeight, fmtFloat(v))
}
func (s *Shape) SetLineColor(v string) { s.SetCellValue(CellLineColor, v) }
func (s *Shape) SetFillColor(v string) { s.SetCellValue(CellFillForegnd, v) }

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
func (s *Shape) SetCharSize(pt float64) {
	s.ensureCharacterCell("Size", fmtFloat(pt/72.0))
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

// ensureSectionCell sets a cell in a named section, creating section and row if needed.
func (s *Shape) ensureSectionCell(sectionName, cellName, value string) {
	section := s.xml.FindElement("Section[@N='" + sectionName + "']")
	if section == nil {
		section = s.xml.CreateElement("Section")
		section.CreateAttr("N", sectionName)
	}
	row := section.FindElement("Row")
	if row == nil {
		row = section.CreateElement("Row")
		row.CreateAttr("IX", "0")
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

// SetLinePattern sets the line pattern.
// Use LinePatternSolid (1), LinePatternDash (2), LinePatternDot (3), etc.
func (s *Shape) SetLinePattern(pattern int) {
	s.SetCellValue(CellLinePattern, strconv.Itoa(pattern))
}

// Line cap constants for SetLineCap.
const (
	LineCapRound    = 0
	LineCapSquare   = 1
	LineCapExtended = 2
)

// SetLineCap sets the line cap style.
// Use LineCapRound (0), LineCapSquare (1), or LineCapExtended (2).
func (s *Shape) SetLineCap(cap int) {
	s.SetCellValue(CellLineCap, strconv.Itoa(cap))
}

// SetBeginArrow sets the begin arrow style. Use 13 for standard arrow, 0 for none.
func (s *Shape) SetBeginArrow(v int) {
	s.SetCellValue(CellBeginArrow, strconv.Itoa(v))
}

// SetEndArrow sets the EndArrow cell value. Use 13 for standard arrow, 0 for none.
func (s *Shape) SetEndArrow(v int) {
	s.SetCellValue(CellEndArrow, strconv.Itoa(v))
}

// SetRounding sets the corner rounding radius in inches.
func (s *Shape) SetRounding(radius float64) {
	s.SetCellValue(CellRounding, fmtFloat(radius))
}

// --- Fill style ---

// SetFillPattern sets the fill pattern. 0=transparent, 1=solid, 2-24=hatches.
func (s *Shape) SetFillPattern(pattern int) {
	s.SetCellValue(CellFillPattern, strconv.Itoa(pattern))
}

// SetFillTransparency sets the foreground fill transparency (0.0=opaque, 1.0=fully transparent).
func (s *Shape) SetFillTransparency(v float64) {
	s.SetCellValue(CellFillForegndTrans, fmtFloat(v))
}

// SetFillBkgndColor sets the background fill color.
func (s *Shape) SetFillBkgndColor(v string) {
	s.SetCellValue(CellFillBkgnd, v)
}

// SetFillBkgndTransparency sets the background fill transparency (0.0=opaque, 1.0=fully transparent).
func (s *Shape) SetFillBkgndTransparency(v float64) {
	s.SetCellValue(CellFillBkgndTrans, fmtFloat(v))
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

// SetText sets the text content of the shape. Clears existing sub-element text.
// Creates a Text element if one doesn't already exist.
func (s *Shape) SetText(text string) {
	textElem := s.xml.FindElement("Text")
	if textElem == nil {
		textElem = s.xml.CreateElement("Text")
	} else {
		clearAllText(textElem)
	}
	textElem.SetText(text)
}

// clearAllText recursively clears text content from an element and its children.
func clearAllText(e *etree.Element) {
	e.SetText("")
	e.SetTail("")
	for _, child := range e.ChildElements() {
		clearAllText(child)
	}
}

// AddGeometry creates a new empty Geometry section on the shape and returns it.
// Use the returned Geometry's AddMoveTo/AddLineTo methods to define the path.
func (s *Shape) AddGeometry() *Geometry {
	section := s.xml.CreateElement("Section")
	section.CreateAttr("N", "Geometry")
	section.CreateAttr("IX", strconv.Itoa(len(s.Geometries)))

	g := newGeometry(section, s)
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

// Move moves the shape by the given deltas, updating position and geometry.
func (s *Shape) Move(xDelta, yDelta float64) {
	if s.Geometry != nil {
		s.Geometry.Move(xDelta, yDelta)
	}
	if s.HasBeginX() {
		s.SetBeginX(s.BeginX() + xDelta)
		s.SetBeginY(s.BeginY() + yDelta)
	}
	s.SetX(s.X() + xDelta)
	s.SetY(s.Y() + yDelta)
}

// Remove removes this shape from its parent XML element.
func (s *Shape) Remove() {
	s.Parent.removeChildShape(s)
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
