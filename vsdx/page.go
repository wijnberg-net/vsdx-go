package vsdx

import (
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
	cell := ps.FindElement("Cell[@N='PageWidth']")
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
	cell := ps.FindElement("Cell[@N='PageHeight']")
	if cell == nil {
		return 0
	}
	return toFloat(cell.SelectAttrValue("V", ""))
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
