package vsdx

import (
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// StyleSheet represents a style sheet in a Visio document.
// Style sheets define default cell values that can be inherited by shapes.
type StyleSheet struct {
	ID        string // Style sheet ID
	Name      string // Language-dependent name
	NameU     string // Language-independent name
	LineStyle string // Reference to another style sheet for line properties
	FillStyle string // Reference to another style sheet for fill properties
	TextStyle string // Reference to another style sheet for text properties

	// Cell values defined in this style sheet
	cells map[string]string

	// Sections (Character, Paragraph, etc.)
	sections map[string][]*etree.Element

	vis *VisioFile
	xml *etree.Element
}

// StyleSheets returns all style sheets defined in the document.
func (v *VisioFile) StyleSheets() []*StyleSheet {
	if v.documentXML == nil {
		return nil
	}

	styleSheets := v.documentXML.FindElement("StyleSheets")
	if styleSheets == nil {
		return nil
	}

	var result []*StyleSheet
	for _, elem := range styleSheets.SelectElements("StyleSheet") {
		ss := parseStyleSheet(elem, v)
		if ss != nil {
			result = append(result, ss)
		}
	}

	return result
}

// StyleSheetByID returns the style sheet with the given ID, or nil.
func (v *VisioFile) StyleSheetByID(id string) *StyleSheet {
	for _, ss := range v.StyleSheets() {
		if ss.ID == id {
			return ss
		}
	}
	return nil
}

// StyleSheetByName returns the style sheet with the given name, or nil.
func (v *VisioFile) StyleSheetByName(name string) *StyleSheet {
	for _, ss := range v.StyleSheets() {
		if ss.NameU == name || ss.Name == name {
			return ss
		}
	}
	return nil
}

// parseStyleSheet parses a StyleSheet element.
func parseStyleSheet(elem *etree.Element, vis *VisioFile) *StyleSheet {
	ss := &StyleSheet{
		ID:        elem.SelectAttrValue("ID", ""),
		Name:      elem.SelectAttrValue("Name", ""),
		NameU:     elem.SelectAttrValue("NameU", ""),
		LineStyle: elem.SelectAttrValue("LineStyle", ""),
		FillStyle: elem.SelectAttrValue("FillStyle", ""),
		TextStyle: elem.SelectAttrValue("TextStyle", ""),
		cells:     make(map[string]string),
		sections:  make(map[string][]*etree.Element),
		vis:       vis,
		xml:       elem,
	}

	// Parse cells.
	for _, cell := range elem.SelectElements("Cell") {
		name := cell.SelectAttrValue("N", "")
		value := cell.SelectAttrValue("V", "")
		if name != "" {
			ss.cells[name] = value
		}
	}

	// Parse sections.
	for _, section := range elem.SelectElements("Section") {
		name := section.SelectAttrValue("N", "")
		if name != "" {
			ss.sections[name] = append(ss.sections[name], section)
		}
	}

	return ss
}

// CellValue returns the value of a cell in this style sheet.
// Returns empty string if not found.
func (ss *StyleSheet) CellValue(name string) string {
	if ss == nil {
		return ""
	}
	if v, ok := ss.cells[name]; ok {
		return v
	}
	return ""
}

// CellValueWithInheritance returns the value of a cell, following style inheritance.
func (ss *StyleSheet) CellValueWithInheritance(name string) string {
	if ss == nil {
		return ""
	}

	// Check this style sheet first.
	if v := ss.CellValue(name); v != "" {
		return v
	}

	// Determine which parent style to check based on cell category.
	parentID := ss.getParentStyleID(name)
	if parentID != "" && parentID != ss.ID {
		parent := ss.vis.StyleSheetByID(parentID)
		return parent.CellValueWithInheritance(name)
	}

	return ""
}

// getParentStyleID returns the appropriate parent style ID based on the cell name.
func (ss *StyleSheet) getParentStyleID(cellName string) string {
	// Line properties
	if isLineProperty(cellName) {
		return ss.LineStyle
	}
	// Fill properties
	if isFillProperty(cellName) {
		return ss.FillStyle
	}
	// Text properties
	if isTextProperty(cellName) {
		return ss.TextStyle
	}
	// Default to LineStyle for unknown properties
	return ss.LineStyle
}

// isLineProperty returns true if the cell name is a line property.
func isLineProperty(name string) bool {
	lineProps := map[string]bool{
		"LineWeight": true, "LineColor": true, "LinePattern": true,
		"Rounding": true, "EndArrow": true, "BeginArrow": true,
		"EndArrowSize": true, "BeginArrowSize": true, "LineCap": true,
		"LineColorTrans": true, "LineGradientEnabled": true,
		"LineGradientDir": true, "LineGradientAngle": true,
	}
	return lineProps[name]
}

// isFillProperty returns true if the cell name is a fill property.
func isFillProperty(name string) bool {
	fillProps := map[string]bool{
		"FillForegnd": true, "FillBkgnd": true, "FillPattern": true,
		"FillForegndTrans": true, "FillBkgndTrans": true,
		"FillGradientEnabled": true, "FillGradientDir": true,
		"FillGradientAngle": true, "ShdwForegnd": true, "ShdwBkgnd": true,
		"ShdwPattern": true, "ShdwForegndTrans": true, "ShdwBkgndTrans": true,
		"ShapeShdwType": true, "ShapeShdwOffsetX": true, "ShapeShdwOffsetY": true,
		"ShapeShdwObliqueAngle": true, "ShapeShdwScaleFactor": true, "ShapeShdwBlur": true,
	}
	return fillProps[name]
}

// isTextProperty returns true if the cell name is a text property.
func isTextProperty(name string) bool {
	textProps := map[string]bool{
		"VerticalAlign": true, "TextBkgnd": true, "TextBkgndTrans": true,
		"TextDirection": true, "LeftMargin": true, "RightMargin": true,
		"TopMargin": true, "BottomMargin": true,
	}
	// Also check for Character. or Paragraph. prefixes
	if strings.HasPrefix(name, "Char.") || strings.HasPrefix(name, "Para.") {
		return true
	}
	return textProps[name]
}

// Section returns the section rows for a given section name.
func (ss *StyleSheet) Section(name string) []*etree.Element {
	if ss == nil {
		return nil
	}
	return ss.sections[name]
}

// CharacterSection returns the Character section rows.
func (ss *StyleSheet) CharacterSection() []*etree.Element {
	if ss == nil {
		return nil
	}
	for _, section := range ss.xml.SelectElements("Section") {
		if section.SelectAttrValue("N", "") == "Character" {
			return section.SelectElements("Row")
		}
	}
	return nil
}

// ParagraphSection returns the Paragraph section rows.
func (ss *StyleSheet) ParagraphSection() []*etree.Element {
	if ss == nil {
		return nil
	}
	for _, section := range ss.xml.SelectElements("Section") {
		if section.SelectAttrValue("N", "") == "Paragraph" {
			return section.SelectElements("Row")
		}
	}
	return nil
}

// ResolveStyleValue resolves a cell value for a shape, following style inheritance.
// This is called when the shape doesn't have the cell value itself.
func (s *Shape) ResolveStyleValue(cellName string) string {
	if s.Page == nil || s.Page.vis == nil {
		return ""
	}

	// First check master shape.
	if master := s.MasterShape(); master != nil {
		if v := master.CellValue(cellName); v != "" {
			return v
		}
	}

	// Then check style sheets based on the shape's style references.
	vis := s.Page.vis

	// Determine which style attribute to use based on cell category.
	var styleID string
	if isLineProperty(cellName) {
		styleID = s.xml.SelectAttrValue("LineStyle", "")
	} else if isFillProperty(cellName) {
		styleID = s.xml.SelectAttrValue("FillStyle", "")
	} else if isTextProperty(cellName) {
		styleID = s.xml.SelectAttrValue("TextStyle", "")
	}

	if styleID != "" {
		ss := vis.StyleSheetByID(styleID)
		if v := ss.CellValueWithInheritance(cellName); v != "" {
			return v
		}
	}

	return ""
}

// ShapeLineStyle returns the line style sheet applied to the shape.
func (s *Shape) ShapeLineStyle() *StyleSheet {
	if s.Page == nil || s.Page.vis == nil {
		return nil
	}
	styleID := s.xml.SelectAttrValue("LineStyle", "")
	if styleID != "" {
		return s.Page.vis.StyleSheetByID(styleID)
	}
	return nil
}

// ShapeFillStyle returns the fill style sheet applied to the shape.
func (s *Shape) ShapeFillStyle() *StyleSheet {
	if s.Page == nil || s.Page.vis == nil {
		return nil
	}
	styleID := s.xml.SelectAttrValue("FillStyle", "")
	if styleID != "" {
		return s.Page.vis.StyleSheetByID(styleID)
	}
	return nil
}

// ShapeTextStyle returns the text style sheet applied to the shape.
func (s *Shape) ShapeTextStyle() *StyleSheet {
	if s.Page == nil || s.Page.vis == nil {
		return nil
	}
	styleID := s.xml.SelectAttrValue("TextStyle", "")
	if styleID != "" {
		return s.Page.vis.StyleSheetByID(styleID)
	}
	return nil
}

// SetLineStyle sets the line style sheet for the shape.
func (s *Shape) SetLineStyle(styleID string) {
	s.xml.CreateAttr("LineStyle", styleID)
}

// SetFillStyle sets the fill style sheet for the shape.
func (s *Shape) SetFillStyle(styleID string) {
	s.xml.CreateAttr("FillStyle", styleID)
}

// SetTextStyle sets the text style sheet for the shape.
func (s *Shape) SetTextStyle(styleID string) {
	s.xml.CreateAttr("TextStyle", styleID)
}

// CreateStyleSheet creates a new style sheet in the document.
func (v *VisioFile) CreateStyleSheet(name string) *StyleSheet {
	if v.documentXML == nil {
		return nil
	}

	styleSheets := v.documentXML.FindElement("StyleSheets")
	if styleSheets == nil {
		// Create StyleSheets element.
		root := v.documentXML.Root()
		if root == nil {
			return nil
		}
		styleSheets = root.CreateElement("StyleSheets")
	}

	// Find next available ID.
	maxID := 0
	for _, elem := range styleSheets.SelectElements("StyleSheet") {
		if id, err := strconv.Atoi(elem.SelectAttrValue("ID", "0")); err == nil && id > maxID {
			maxID = id
		}
	}
	newID := strconv.Itoa(maxID + 1)

	// Create new style sheet element.
	ssElem := styleSheets.CreateElement("StyleSheet")
	ssElem.CreateAttr("ID", newID)
	ssElem.CreateAttr("NameU", name)
	ssElem.CreateAttr("Name", name)
	ssElem.CreateAttr("LineStyle", "0")
	ssElem.CreateAttr("FillStyle", "0")
	ssElem.CreateAttr("TextStyle", "0")

	return parseStyleSheet(ssElem, v)
}

// SetCell sets a cell value in the style sheet.
func (ss *StyleSheet) SetCell(name, value string) {
	if ss == nil || ss.xml == nil {
		return
	}

	// Look for existing cell.
	for _, cell := range ss.xml.SelectElements("Cell") {
		if cell.SelectAttrValue("N", "") == name {
			cell.CreateAttr("V", value)
			ss.cells[name] = value
			return
		}
	}

	// Create new cell.
	cell := ss.xml.CreateElement("Cell")
	cell.CreateAttr("N", name)
	cell.CreateAttr("V", value)
	ss.cells[name] = value
}
