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

// ensureCharacterCell sets a cell in the Character section, creating the
// section and row if they don't exist.
func (s *Shape) ensureCharacterCell(cellName, value string) {
	charSection := s.xml.FindElement("Section[@N='Character']")
	if charSection == nil {
		charSection = s.xml.CreateElement("Section")
		charSection.CreateAttr("N", "Character")
	}
	row := charSection.FindElement("Row")
	if row == nil {
		row = charSection.CreateElement("Row")
		row.CreateAttr("IX", "0")
	}
	cell := row.FindElement("Cell[@N='" + cellName + "']")
	if cell == nil {
		cell = row.CreateElement("Cell")
		cell.CreateAttr("N", cellName)
	}
	cell.CreateAttr("V", value)
}

// SetEndArrow sets the EndArrow cell value. Use 13 for standard arrow, 0 for none.
func (s *Shape) SetEndArrow(v int) {
	s.SetCellValue(CellEndArrow, strconv.Itoa(v))
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
	return Rect{BeginX: bx, BeginY: by, EndX: ex, EndY: ey}
}

// RelativeBoundsRect returns bounds relative to parent group shape as a Rect.
func (s *Shape) RelativeBoundsRect() Rect {
	bx, by, ex, ey := s.RelativeBounds()
	return Rect{BeginX: bx, BeginY: by, EndX: ex, EndY: ey}
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
