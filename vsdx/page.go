package vsdx

import (
	"fmt"
	"strconv"

	"github.com/beevik/etree"
)

// Page represents a page or a master page in a vsdx file.
type Page struct {
	xml      *etree.Document
	filename string
	name     string
	pageID   string
	relID    string
	vis      *VisioFile

	MasterUniqueID string
	MasterBaseID   string
	RelsXMLFile    string
	RelsXML        *etree.Document
	MaxID          int
}

func newPage(xml *etree.Document, filename, pageName, pageID, relID string, vis *VisioFile) *Page {
	return &Page{
		xml:      xml,
		filename: filename,
		name:     pageName,
		pageID:   pageID,
		relID:    relID,
		vis:      vis,
	}
}

// Name returns the page name.
func (p *Page) Name() string {
	return p.name
}

// PageID returns the page ID.
func (p *Page) PageID() string {
	return p.pageID
}

// IsMasterPage returns true if this page is a master page.
func (p *Page) IsMasterPage() bool {
	if p.vis.mastersXML != nil && p.MasterUniqueID != "" {
		for _, master := range p.vis.mastersXML.SelectElements("Master") {
			if master.SelectAttrValue("UniqueID", "") == p.MasterUniqueID {
				return true
			}
		}
	}
	return false
}

// pagesheetXML returns the PageSheet element from pages_xml or masters_xml.
func (p *Page) pagesheetXML() *etree.Element {
	// Search in pages_xml
	if p.vis.pagesXML != nil {
		for _, pageElem := range p.vis.pagesXML.Root().SelectElements("Page") {
			if pageElem.SelectAttrValue("ID", "") == p.pageID {
				if ps := pageElem.SelectElement("PageSheet"); ps != nil {
					return ps
				}
			}
		}
	}
	// Search in masters_xml
	if p.vis.mastersXML != nil {
		for _, masterElem := range p.vis.mastersXML.SelectElements("Master") {
			if masterElem.SelectAttrValue("ID", "") == p.pageID {
				if ps := masterElem.SelectElement("PageSheet"); ps != nil {
					return ps
				}
			}
		}
	}
	return nil
}

// Width returns the page width.
func (p *Page) Width() float64 {
	ps := p.pagesheetXML()
	if ps == nil {
		return 0
	}
	cell := ps.FindElement("Cell[@N='" + CellPageWidth + "']")
	if cell == nil {
		return 0
	}
	return toFloat(cell.SelectAttrValue("V", ""))
}

// Height returns the page height.
func (p *Page) Height() float64 {
	ps := p.pagesheetXML()
	if ps == nil {
		return 0
	}
	cell := ps.FindElement("Cell[@N='" + CellPageHeight + "']")
	if cell == nil {
		return 0
	}
	return toFloat(cell.SelectAttrValue("V", ""))
}

// BackgroundPage returns the background page for this page, or nil if none is set.
func (p *Page) BackgroundPage() *Page {
	ps := p.pagesheetXML()
	if ps == nil {
		return nil
	}
	cell := ps.FindElement("Cell[@N='BackPage']")
	if cell == nil {
		return nil
	}
	backPageID := cell.SelectAttrValue("V", "")
	if backPageID == "" {
		return nil
	}
	return p.vis.GetPageByID(backPageID)
}

// SetBackgroundPage sets the background page for this page.
// Pass nil to remove the background page.
func (p *Page) SetBackgroundPage(bg *Page) {
	ps := p.pagesheetXML()
	if ps == nil {
		return
	}

	cell := ps.FindElement("Cell[@N='BackPage']")
	if bg == nil {
		// Remove background page.
		if cell != nil {
			ps.RemoveChild(cell)
		}
		return
	}

	// Set background page.
	if cell == nil {
		cell = ps.CreateElement("Cell")
		cell.CreateAttr("N", "BackPage")
	}
	cell.CreateAttr("V", bg.PageID())
}

// AllShapesWithBackground returns all shapes including those from the background page.
// Background shapes are returned first, followed by foreground shapes.
func (p *Page) AllShapesWithBackground() []*Shape {
	var shapes []*Shape

	// Recursively get background shapes first.
	if bg := p.BackgroundPage(); bg != nil {
		shapes = append(shapes, bg.AllShapesWithBackground()...)
	}

	// Then foreground shapes.
	shapes = append(shapes, p.AllShapes()...)
	return shapes
}

// IsBackgroundPage returns true if this page is used as a background for other pages.
func (p *Page) IsBackgroundPage() bool {
	ps := p.pagesheetXML()
	if ps == nil {
		return false
	}
	cell := ps.FindElement("Cell[@N='Background']")
	if cell == nil {
		return false
	}
	return cell.SelectAttrValue("V", "") == "1"
}

// SetAsBackgroundPage marks or unmarks this page as a background page.
func (p *Page) SetAsBackgroundPage(isBackground bool) {
	ps := p.pagesheetXML()
	if ps == nil {
		return
	}

	cell := ps.FindElement("Cell[@N='Background']")
	if cell == nil {
		cell = ps.CreateElement("Cell")
		cell.CreateAttr("N", "Background")
	}
	if isBackground {
		cell.CreateAttr("V", "1")
	} else {
		cell.CreateAttr("V", "0")
	}
}

// XML returns the page content XML document.
func (p *Page) XML() *etree.Document {
	return p.xml
}

// shapes returns the internal Shapes container objects.
func (p *Page) shapes() []*Shape {
	if p.xml == nil || p.xml.Root() == nil {
		return nil
	}
	var shapes []*Shape
	for _, shapesElem := range p.xml.Root().SelectElements("Shapes") {
		shapes = append(shapes, newShape(shapesElem, p, p))
	}
	return shapes
}

// ChildShapes returns the top-level shapes on this page.
func (p *Page) ChildShapes() []*Shape {
	s := p.shapes()
	if len(s) > 0 {
		return s[0].ChildShapes()
	}
	return nil
}

// AllShapes returns all shapes on the page, recursively.
func (p *Page) AllShapes() []*Shape {
	s := p.shapes()
	if len(s) > 0 {
		return s[0].AllShapes()
	}
	return nil
}

// SetMaxIDs calculates and stores the maximum shape ID for this page.
func (p *Page) SetMaxIDs() int {
	for _, shapes := range p.shapes() {
		for _, shape := range shapes.ChildShapes() {
			if id := shape.GetMaxID(); id > p.MaxID {
				p.MaxID = id
			}
		}
	}
	return p.MaxID
}

// IndexNum returns the zero-based index of this page in the parent VisioFile.
func (p *Page) IndexNum() int {
	for i, page := range p.vis.Pages {
		if page == p {
			return i
		}
	}
	return -1
}

// --- Connect methods ---

// Connects returns all Connect objects on this page.
func (p *Page) Connects() []*Connect {
	if p.xml == nil || p.xml.Root() == nil {
		return nil
	}
	var connects []*Connect
	for _, connectElem := range p.xml.Root().FindElements(".//Connect") {
		connects = append(connects, newConnect(connectElem, p))
	}
	return connects
}

// DisconnectShapes removes every <Connect> on the page whose FromSheet
// matches connector.ID and ToSheet matches terminal.ID. Both shapes
// remain on the page — only the binding(s) between them are stripped.
// Use this to detach a connector endpoint without deleting the connector
// shape (see Shape.Remove for full delete-with-cleanup).
//
// Returns the number of <Connect> elements removed. Pass nil for either
// shape to match any value on that side (e.g. DisconnectShapes(c, nil)
// drops every binding owned by connector c).
func (p *Page) DisconnectShapes(connector, terminal *Shape) int {
	if p.xml == nil || p.xml.Root() == nil {
		return 0
	}
	var fromID, toID string
	if connector != nil {
		fromID = connector.ID
	}
	if terminal != nil {
		toID = terminal.ID
	}
	removed := 0
	for _, connectElem := range append([]*etree.Element(nil), p.xml.Root().FindElements(".//Connect")...) {
		if fromID != "" && connectElem.SelectAttrValue("FromSheet", "") != fromID {
			continue
		}
		if toID != "" && connectElem.SelectAttrValue("ToSheet", "") != toID {
			continue
		}
		if parent := connectElem.Parent(); parent != nil {
			parent.RemoveChild(connectElem)
			removed++
		}
	}
	return removed
}

// --- Search methods ---

// FindShapeByID searches for a shape by ID on this page.
func (p *Page) FindShapeByID(shapeID string) *Shape {
	for _, s := range p.shapes() {
		if found := s.FindShapeByID(shapeID); found != nil {
			return found
		}
	}
	return nil
}

// FindShapeByText searches for a shape containing the given text on this page.
func (p *Page) FindShapeByText(text string) *Shape {
	for _, s := range p.shapes() {
		if found := s.FindShapeByText(text); found != nil {
			return found
		}
	}
	return nil
}

// FindShapesByText searches for all shapes containing the given text on this page.
func (p *Page) FindShapesByText(text string) []*Shape {
	var shapes []*Shape
	for _, s := range p.shapes() {
		shapes = append(shapes, s.FindShapesByText(text)...)
	}
	return shapes
}

// FindShapeByAttr searches for a shape by attribute name and value.
func (p *Page) FindShapeByAttr(attr, attrValue string) *Shape {
	for _, s := range p.shapes() {
		if found := s.FindShapeByAttr(attr, attrValue); found != nil {
			return found
		}
	}
	return nil
}

// FindShapeByPropertyLabel searches for a shape by data property label.
func (p *Page) FindShapeByPropertyLabel(label string) *Shape {
	s := p.shapes()
	if len(s) > 0 {
		return s[0].FindShapeByPropertyLabel(label)
	}
	return nil
}

// FindShapesByPropertyLabel searches for all shapes by data property label.
func (p *Page) FindShapesByPropertyLabel(label string) []*Shape {
	var shapes []*Shape
	for _, s := range p.shapes() {
		shapes = append(shapes, s.FindShapesByPropertyLabel(label)...)
	}
	return shapes
}

// FindShapeByPropertyLabelValue searches for a shape by property label and value.
func (p *Page) FindShapeByPropertyLabelValue(label, value string) *Shape {
	for _, s := range p.shapes() {
		if found := s.FindShapeByPropertyLabelValue(label, value); found != nil {
			return found
		}
	}
	return nil
}

// FindShapesByPropertyLabelValue searches for all shapes by property label and value.
func (p *Page) FindShapesByPropertyLabelValue(label, value string) []*Shape {
	var shapes []*Shape
	for _, s := range p.shapes() {
		shapes = append(shapes, s.FindShapesByPropertyLabelValue(label, value)...)
	}
	return shapes
}

// FindShapesByID searches for all shapes with the given ID on this page.
func (p *Page) FindShapesByID(shapeID string) []*Shape {
	for _, s := range p.shapes() {
		if found := s.FindShapesByID(shapeID); len(found) > 0 {
			return found
		}
	}
	return nil
}

// FindShapesWithSameMaster returns all shapes that share the same master as the given shape.
func (p *Page) FindShapesWithSameMaster(shape *Shape) []*Shape {
	var result []*Shape
	for _, s := range p.AllShapes() {
		if s.MasterShapeID == shape.MasterShapeID && s.MasterPageID == shape.MasterPageID {
			result = append(result, s)
		}
	}
	return result
}

// FindShapesByRegex searches for all shapes whose text matches the given regex pattern on this page.
func (p *Page) FindShapesByRegex(pattern string) ([]*Shape, error) {
	s := p.shapes()
	if len(s) > 0 {
		return s[0].FindShapesByRegex(pattern)
	}
	return nil, nil
}

// GetConnectorsBetween finds connector shapes between two shapes identified by ID or text.
// For each shape, specify either an ID or text to find it (ID takes priority).
func (p *Page) GetConnectorsBetween(shapeAID, shapeAText, shapeBID, shapeBText string) ([]*Shape, error) {
	var shapeA, shapeB *Shape
	if shapeAID != "" {
		shapeA = p.FindShapeByID(shapeAID)
	} else {
		shapeA = p.FindShapeByText(shapeAText)
	}
	if shapeBID != "" {
		shapeB = p.FindShapeByID(shapeBID)
	} else {
		shapeB = p.FindShapeByText(shapeBText)
	}

	if shapeA == nil {
		return nil, fmt.Errorf("shape A (id=%q text=%q): %w", shapeAID, shapeAText, ErrShapeNotFound)
	}
	if shapeB == nil {
		return nil, fmt.Errorf("shape B (id=%q text=%q): %w", shapeBID, shapeBText, ErrShapeNotFound)
	}

	// Get connected shape IDs for each shape
	connectedA := make(map[string]bool)
	for _, s := range shapeA.ConnectedShapes() {
		connectedA[s.ID] = true
	}

	// Find intersection: shapes connected to both A and B
	connectorIDs := make(map[string]bool)
	for _, s := range shapeB.ConnectedShapes() {
		if connectedA[s.ID] {
			connectorIDs[s.ID] = true
		}
	}

	// Look up connector shapes
	var connectors []*Shape
	for id := range connectorIDs {
		if shape := p.FindShapeByID(id); shape != nil {
			connectors = append(connectors, shape)
		}
	}
	return connectors, nil
}

// --- Page editing methods ---

// SetName sets the page name (updates Name and NameU attributes in pages.xml).
func (p *Page) SetName(name string) {
	p.name = name
	if p.vis.pagesXML != nil {
		for _, pageElem := range p.vis.pagesXML.Root().SelectElements("Page") {
			if pageElem.SelectAttrValue("ID", "") == p.pageID {
				pageElem.CreateAttr("Name", name)
				pageElem.CreateAttr("NameU", name)
				break
			}
		}
	}
}

// SetWidth sets the page width in the PageSheet.
func (p *Page) SetWidth(width float64) {
	ps := p.pagesheetXML()
	if ps == nil {
		return
	}
	cell := ps.FindElement("Cell[@N='" + CellPageWidth + "']")
	if cell != nil {
		cell.CreateAttr("V", fmtFloat(width))
	}
}

// SetHeight sets the page height in the PageSheet.
func (p *Page) SetHeight(height float64) {
	ps := p.pagesheetXML()
	if ps == nil {
		return
	}
	cell := ps.FindElement("Cell[@N='" + CellPageHeight + "']")
	if cell != nil {
		cell.CreateAttr("V", fmtFloat(height))
	}
}

// removeChildShape removes a child shape from this page's Shapes container XML.
func (p *Page) removeChildShape(s *Shape) {
	if p.xml != nil && p.xml.Root() != nil {
		for _, shapesElem := range p.xml.Root().SelectElements("Shapes") {
			shapesElem.RemoveChild(s.xml)
		}
	}
}

// AddConnect adds a Connect element to this page's XML.
func (p *Page) AddConnect(connect *Connect) {
	if p.xml == nil || p.xml.Root() == nil {
		return
	}
	// Find or create Connects element
	connectsElem := p.xml.Root().SelectElement("Connects")
	if connectsElem == nil {
		connectsElem = p.xml.Root().CreateElement("Connects")
	}
	connectsElem.AddChild(connect.xml)
}

// FindReplace finds and replaces text across all shapes on this page.
func (p *Page) FindReplace(old, new string) {
	for _, s := range p.shapes() {
		s.FindReplace(old, new)
	}
}

// ApplyTextContext applies text context/filter to all shapes on this page.
func (p *Page) ApplyTextContext(context map[string]string) {
	for _, s := range p.shapes() {
		s.ApplyTextFilter(context)
	}
}

// AddLayer adds a named layer to the page and returns the layer index.
// Use the index with Shape.SetLayerMember() to assign shapes to the layer.
func (p *Page) AddLayer(name string) int {
	if p.vis == nil || p.vis.pagesXML == nil {
		return -1
	}

	// Find the Page element in pages.xml and its PageSheet
	var pageSheet *etree.Element
	for _, pageElem := range p.vis.pagesXML.Root().SelectElements("Page") {
		if pageElem.SelectAttrValue("ID", "") == p.pageID {
			pageSheet = pageElem.SelectElement("PageSheet")
			if pageSheet == nil {
				pageSheet = pageElem.CreateElement("PageSheet")
			}
			break
		}
	}
	if pageSheet == nil {
		return -1
	}

	layerSection := pageSheet.FindElement("Section[@N='Layer']")
	if layerSection == nil {
		layerSection = pageSheet.CreateElement("Section")
		layerSection.CreateAttr("N", "Layer")
	}

	maxIX := -1
	for _, row := range layerSection.SelectElements("Row") {
		if ix, err := strconv.Atoi(row.SelectAttrValue("IX", "")); err == nil && ix > maxIX {
			maxIX = ix
		}
	}
	ix := maxIX + 1

	row := layerSection.CreateElement("Row")
	row.CreateAttr("IX", strconv.Itoa(ix))
	// Cell order mirrors Visio's canonical output: Name, Color, Status,
	// Visible, Print, Active, Lock, Snap, Glue, NameUniv, ColorTrans.
	// All cells that default to a value with no explicit formula carry
	// F='No Formula' to match Visio's resave shape.
	addCellXML(row, "Name", name, "")
	addCellWithFormula(row, "Color", "0", "No Formula", "")
	addCellWithFormula(row, "Status", "0", "No Formula", "")
	addCellXML(row, "Visible", "1", "")
	addCellXML(row, "Print", "1", "")
	addCellXML(row, "Active", "0", "")
	addCellXML(row, "Lock", "0", "")
	addCellXML(row, "Snap", "1", "")
	addCellXML(row, "Glue", "1", "")
	addCellWithFormula(row, "NameUniv", "", "No Formula", "")
	addCellWithFormula(row, "ColorTrans", "0", "No Formula", "")

	return ix
}

// AutoSize adjusts the page dimensions to fit all shapes with the given margin (in inches).
func (p *Page) AutoSize(margin float64) {
	var maxX, maxY float64
	for _, s := range p.AllShapes() {
		_, _, ex, ey := s.Bounds()
		if ex > maxX {
			maxX = ex
		}
		if ey > maxY {
			maxY = ey
		}
	}
	if maxX > 0 || maxY > 0 {
		p.SetWidth(maxX + margin)
		p.SetHeight(maxY + margin)
	}
}
