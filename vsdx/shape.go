package vsdx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

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

// Shape represents a single shape or a group shape containing other shapes.
type Shape struct {
	xml           *etree.Element
	Parent        interface{} // *Page or *Shape
	Page          *Page
	ID            string
	MasterShapeID string // MasterShape attribute
	MasterPageID  string // Master attribute - reference to master page ID
	ShapeType     string // Type attribute (e.g., "Shape", "Group")
	ShapeName     string // NameU or Name attribute
	Cells         map[string]*Cell
	Geometry      *Geometry

	dataProperties map[string]*DataProperty // lazy loaded
}

// newShape creates a Shape from an XML element.
func newShape(xml *etree.Element, parent interface{}, page *Page) *Shape {
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
		if parentShape, ok := parent.(*Shape); ok {
			s.MasterPageID = parentShape.MasterPageID
		}
	}

	// Get Cells directly under Shape element
	for _, cellElem := range xml.SelectElements("Cell") {
		cell := newCell(cellElem, s)
		s.Cells[cell.Name()] = cell
	}

	// Get Geometry section
	geometrySection := xml.FindElement("Section[@N='Geometry']")
	if geometrySection != nil {
		s.Geometry = newGeometry(geometrySection, s)
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

// --- Position and size properties ---

func (s *Shape) X() float64      { return toFloat(s.CellValue("PinX")) }
func (s *Shape) Y() float64      { return toFloat(s.CellValue("PinY")) }
func (s *Shape) LocX() float64   { return toFloat(s.CellValue("LocPinX")) }
func (s *Shape) LocY() float64   { return toFloat(s.CellValue("LocPinY")) }
func (s *Shape) Width() float64  { return toFloat(s.CellValue("Width")) }
func (s *Shape) Height() float64 { return toFloat(s.CellValue("Height")) }
func (s *Shape) Angle() float64  { return toFloat(s.CellValue("Angle")) }

func (s *Shape) BeginX() float64 { return toFloat(s.CellValue("BeginX")) }
func (s *Shape) BeginY() float64 { return toFloat(s.CellValue("BeginY")) }
func (s *Shape) EndX() float64   { return toFloat(s.CellValue("EndX")) }
func (s *Shape) EndY() float64   { return toFloat(s.CellValue("EndY")) }

func (s *Shape) HasBeginX() bool { return s.CellValue("BeginX") != "" }

func (s *Shape) LocXFormula() string { return s.CellFormula("LocPinX") }
func (s *Shape) LocYFormula() string { return s.CellFormula("LocPinY") }

// --- Style properties ---

func (s *Shape) LineWeight() float64 { return toFloat(s.CellValue("LineWeight")) }
func (s *Shape) LineColor() string   { return s.CellValue("LineColor") }
func (s *Shape) FillColor() string   { return s.CellValue("FillForegnd") }

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
func (s *Shape) EndArrow() string { return s.CellValue("EndArrow") }

// LineStyleID returns the LineStyle attribute.
func (s *Shape) LineStyleID() string { return s.xml.SelectAttrValue("LineStyle", "") }

// FillStyleID returns the FillStyle attribute.
func (s *Shape) FillStyleID() string { return s.xml.SelectAttrValue("FillStyle", "") }

// TextStyleID returns the TextStyle attribute.
func (s *Shape) TextStyleID() string { return s.xml.SelectAttrValue("TextStyle", "") }

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
	if !s.HasBeginX() && !s.HasCell("PinX") && !s.HasCell("LocPinX") {
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
