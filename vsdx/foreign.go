package vsdx

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// OPC image relationship type — used for PNG/JPEG images embedded in VSDX shapes.
const ImageRelType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"

// AddImage stores a PNG image in the VSDX media directory and registers it
// in the page's relationship file. Returns the relationship ID (e.g. "rId1")
// that can be passed to Shape.SetForeignData.
//
// The image is stored at visio/media/imageN.png where N is auto-incremented.
// The PNG content type is registered in [Content_Types].xml if not already present.
func (v *VisioFile) AddImage(page *Page, pngData []byte) (string, error) {
	if page == nil {
		return "", fmt.Errorf("page is nil")
	}
	if len(pngData) == 0 {
		return "", fmt.Errorf("empty image data")
	}

	// Find next available media index by scanning ZipFileContents
	nextIdx := 1
	for path := range v.ZipFileContents {
		if strings.HasPrefix(path, "visio/media/image") && strings.HasSuffix(path, ".png") {
			base := filepath.Base(path)                 // "image3.png"
			numStr := strings.TrimPrefix(base, "image") // "3.png"
			numStr = strings.TrimSuffix(numStr, ".png") // "3"
			if n, err := strconv.Atoi(numStr); err == nil && n >= nextIdx {
				nextIdx = n + 1
			}
		}
	}

	// Store image bytes
	mediaPath := fmt.Sprintf("visio/media/image%d.png", nextIdx)
	v.ZipFileContents[mediaPath] = pngData

	// Register PNG content type (idempotent)
	v.addContentTypeDefault("png", "image/png")

	// Ensure page has a rels document
	if page.RelsXML == nil {
		page.RelsXML = etree.NewDocument()
		page.RelsXML.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
		root := page.RelsXML.CreateElement("Relationships")
		root.CreateAttr("xmlns", PkgRelNS)
	}
	if page.RelsXMLFile == "" {
		page.RelsXMLFile = "visio/pages/_rels/" + filepath.Base(page.filename) + ".rels"
	}

	// Find next available rId in page rels
	root := page.RelsXML.Root()
	maxRelID := 0
	for _, rel := range root.SelectElements("Relationship") {
		id := rel.SelectAttrValue("Id", "")
		if strings.HasPrefix(id, "rId") {
			if n, err := strconv.Atoi(id[3:]); err == nil && n > maxRelID {
				maxRelID = n
			}
		}
	}
	relID := fmt.Sprintf("rId%d", maxRelID+1)

	// Add relationship entry
	rel := root.CreateElement("Relationship")
	rel.CreateAttr("Id", relID)
	rel.CreateAttr("Type", ImageRelType)
	rel.CreateAttr("Target", "../media/"+filepath.Base(mediaPath))

	return relID, nil
}

// addContentTypeDefault adds a <Default Extension="ext" ContentType="ct"/> entry
// to [Content_Types].xml if one doesn't already exist for this extension.
func (v *VisioFile) addContentTypeDefault(extension, contentType string) {
	if v.contentTypesXML == nil {
		return
	}
	root := v.contentTypesXML.Root()

	// Check if already registered
	for _, elem := range root.SelectElements("Default") {
		if strings.EqualFold(elem.SelectAttrValue("Extension", ""), extension) {
			return
		}
	}

	def := etree.NewElement("Default")
	def.CreateAttr("Extension", extension)
	def.CreateAttr("ContentType", contentType)

	// Insert after last Default element for clean ordering
	var lastDefault *etree.Element
	for _, elem := range root.SelectElements("Default") {
		lastDefault = elem
	}
	if lastDefault != nil {
		root.InsertChildAt(lastDefault.Index()+1, def)
	} else {
		root.InsertChildAt(0, def)
	}
}

// AddShape creates a new empty shape on the page and returns it.
// The shape gets the next available ID. Position and size default to (0, 0) and (1, 1).
// Use the returned Shape's Set methods to configure it before saving.
func (p *Page) AddShape() *Shape {
	if p.xml == nil || p.xml.Root() == nil {
		return nil
	}

	// Find or create Shapes container
	shapesElem := p.xml.Root().SelectElement("Shapes")
	if shapesElem == nil {
		shapesElem = p.xml.Root().CreateElement("Shapes")
	}

	// Assign next ID
	p.SetMaxIDs()
	p.MaxID++
	id := strconv.Itoa(p.MaxID)

	// Create minimal shape XML
	shapeElem := shapesElem.CreateElement("Shape")
	shapeElem.CreateAttr("ID", id)
	shapeElem.CreateAttr("Type", "Shape")

	// Default position and size cells
	addCellXML(shapeElem, CellPinX, "0", "")
	addCellXML(shapeElem, CellPinY, "0", "")
	addCellXML(shapeElem, CellWidth, "1", "")
	addCellXML(shapeElem, CellHeight, "1", "")
	addCellXML(shapeElem, CellLocPinX, "0.5", "Width*0.5")
	addCellXML(shapeElem, CellLocPinY, "0.5", "Height*0.5")

	return newShape(shapeElem, p, p)
}

// GroupShapes creates a group shape containing the given shapes.
// The shapes are removed from the page-level Shapes container and placed
// inside the new group with coordinates converted to group-relative space.
// padding expands the group bounding box (in inches) on all sides.
// Returns the new group shape, or nil if shapes is empty.
//
// Shape IDs are preserved — connectors that reference grouped shapes by ID
// (via Sheet.N! formulas and Connect FromSheet/ToSheet attributes) continue
// to work because Visio resolves IDs globally across the page, including
// inside group shapes.
func (p *Page) GroupShapes(shapes []*Shape, padding float64) *Shape {
	if len(shapes) == 0 {
		return nil
	}

	shapesElem := p.xml.Root().SelectElement("Shapes")
	if shapesElem == nil {
		return nil
	}

	// Compute axis-aligned bounding box of all shapes.
	// Use Width()/2 rather than LocX() because LocPinX's cached V attribute
	// can be stale after resize (the formula Width*0.5 is correct but V isn't
	// recalculated until Visio opens the file).
	minX := math.Inf(1)
	minY := math.Inf(1)
	maxX := math.Inf(-1)
	maxY := math.Inf(-1)
	for _, s := range shapes {
		x, y := s.X(), s.Y()
		hw, hh := s.Width()/2, s.Height()/2
		if x-hw < minX {
			minX = x - hw
		}
		if y-hh < minY {
			minY = y - hh
		}
		if x+hw > maxX {
			maxX = x + hw
		}
		if y+hh > maxY {
			maxY = y + hh
		}
	}

	// Expand bbox by padding on all sides.
	minX -= padding
	minY -= padding
	maxX += padding
	maxY += padding

	width := maxX - minX
	height := maxY - minY
	centerX := (minX + maxX) / 2
	centerY := (minY + maxY) / 2

	// Allocate next shape ID.
	p.SetMaxIDs()
	p.MaxID++
	groupID := strconv.Itoa(p.MaxID)

	// Create <Shape Type="Group"> element.
	groupElem := shapesElem.CreateElement("Shape")
	groupElem.CreateAttr("ID", groupID)
	groupElem.CreateAttr("Type", "Group")
	groupElem.CreateAttr("LineStyle", "3")
	groupElem.CreateAttr("FillStyle", "3")
	groupElem.CreateAttr("TextStyle", "3")

	// Position and size cells.
	addCellXML(groupElem, CellPinX, fmtFloat(centerX), "")
	addCellXML(groupElem, CellPinY, fmtFloat(centerY), "")
	addCellXML(groupElem, CellWidth, fmtFloat(width), "")
	addCellXML(groupElem, CellHeight, fmtFloat(height), "")
	addCellXML(groupElem, CellLocPinX, fmtFloat(width/2), "Width*0.5")
	addCellXML(groupElem, CellLocPinY, fmtFloat(height/2), "Height*0.5")

	// Invisible border: no line, no fill. ObjType=8 marks it as a group.
	addCellXML(groupElem, "LinePattern", "0", "")
	addCellXML(groupElem, "FillPattern", "0", "")
	addCellXML(groupElem, "ObjType", "8", "")

	// Create inner <Shapes> container for child shapes.
	innerShapes := groupElem.CreateElement("Shapes")

	// Move each child shape from page into group, converting coordinates.
	for _, s := range shapes {
		shapesElem.RemoveChild(s.xml)

		// Convert PinX/PinY from page-absolute to group-relative coordinates.
		// Group local origin (0,0) = group's bottom-left corner = (minX, minY).
		s.SetX(s.X() - minX)
		s.SetY(s.Y() - minY)

		innerShapes.AddChild(s.xml)
	}

	return newShape(groupElem, p, p)
}

// SetForeignData converts a shape into a Foreign (embedded image) shape.
// The relID parameter is the relationship ID returned by VisioFile.AddImage.
//
// This sets the shape type to "Foreign", adds the required Geometry section,
// image sizing cells, text positioning cells, and the ForeignData element
// that references the embedded PNG.
func (s *Shape) SetForeignData(relID string) {
	xml := s.xml

	// Set shape type to Foreign
	xml.CreateAttr("Type", "Foreign")
	s.ShapeType = "Foreign"

	// Set style inheritance
	xml.CreateAttr("LineStyle", "2")
	xml.CreateAttr("FillStyle", "2")
	xml.CreateAttr("TextStyle", "2")

	// Image sizing cells — formulas couple to Width/Height for resize behavior
	s.setCellValueFormula("ImgOffsetX", "0", "ImgWidth*0")
	s.setCellValueFormula("ImgOffsetY", "0", "ImgHeight*0")
	s.setCellValueFormula("ImgWidth", s.CellValue(CellWidth), "Width*1")
	s.setCellValueFormula("ImgHeight", s.CellValue(CellHeight), "Height*1")

	// ClippingPath with error attribute (standard Visio pattern)
	cpCell := s.ensureCell("ClippingPath", "V")
	cpCell.SetValue("")
	cpCell.SetFormula(`""`)
	cpCell.xml.CreateAttr("E", "#N/A")

	// Text label positioning — below the icon
	halfW := fmtFloat(toFloat(s.CellValue(CellWidth)) / 2)
	s.setCellValueFormula(CellTxtPinX, halfW, "Width*0.5")
	s.setCellValueFormula(CellTxtPinY, "0", "Height*0")
	s.setCellValueFormula("TxtWidth", s.CellValue(CellWidth), "Width*1")
	s.setCellValueFormula("TxtHeight", "0", "Height*0")
	s.setCellValueFormula("TxtLocPinX", halfW, "TxtWidth*0.5")
	s.setCellValueFormula("TxtLocPinY", "0", "TxtHeight*0.5")
	s.SetCellValue("TxtAngle", "0")
	s.SetCellValue("VerticalAlign", "0")
	s.SetCellValue("ObjType", "1")

	// Geometry section — required rectangle even for Foreign shapes
	// Remove existing geometry first
	if geo := xml.FindElement("Section[@N='Geometry']"); geo != nil {
		xml.RemoveChild(geo)
	}
	geo := xml.CreateElement("Section")
	geo.CreateAttr("N", "Geometry")
	geo.CreateAttr("IX", "0")
	addCellXML(geo, "NoFill", "0", "")
	addCellXML(geo, "NoLine", "0", "")
	addCellXML(geo, "NoShow", "0", "")
	addCellXML(geo, "NoSnap", "0", "")
	addCellXML(geo, "NoQuickDrag", "0", "")
	addGeoRowXML(geo, "RelMoveTo", "1", "0", "0")
	addGeoRowXML(geo, "RelLineTo", "2", "1", "0")
	addGeoRowXML(geo, "RelLineTo", "3", "1", "1")
	addGeoRowXML(geo, "RelLineTo", "4", "0", "1")
	addGeoRowXML(geo, "RelLineTo", "5", "0", "0")

	// ForeignData element — must be LAST child of Shape
	fd := xml.CreateElement("ForeignData")
	fd.CreateAttr("ForeignType", "Bitmap")
	fd.CreateAttr("CompressionType", "PNG")
	relElem := fd.CreateElement("Rel")
	relElem.Attr = append(relElem.Attr, etree.Attr{Space: "r", Key: "id", Value: relID})
}

// AddDataProperty adds a Shape Data property row to the shape.
// If the Property section doesn't exist, it is created.
// The row name is auto-incremented (Row_1, Row_2, etc.).
// Returns the created DataProperty.
func (s *Shape) AddDataProperty(name, label, value string) *DataProperty {
	xml := s.xml

	// Find or create Property section
	propSect := xml.FindElement("Section[@N='Property']")
	if propSect == nil {
		// Insert Property section before Geometry section (if it exists) for clean ordering
		propSect = etree.NewElement("Section")
		propSect.CreateAttr("N", "Property")
		if geo := xml.FindElement("Section[@N='Geometry']"); geo != nil {
			xml.InsertChildAt(geo.Index(), propSect)
		} else {
			xml.AddChild(propSect)
		}
	}

	// Find next row number
	maxRow := 0
	for _, row := range propSect.SelectElements("Row") {
		rowName := row.SelectAttrValue("N", "")
		if strings.HasPrefix(rowName, "Row_") {
			if n, err := strconv.Atoi(rowName[4:]); err == nil && n > maxRow {
				maxRow = n
			}
		}
	}
	rowName := fmt.Sprintf("Row_%d", maxRow+1)

	// Create row with all 11 required cells
	row := propSect.CreateElement("Row")
	if name != "" {
		row.CreateAttr("N", name)
	} else {
		row.CreateAttr("N", rowName)
	}

	addPropCellXML(row, "Value", value, "STR", "")
	addPropCellXML(row, "Prompt", "", "", "No Formula")
	addPropCellXML(row, "Label", label, "", "")
	addPropCellXML(row, "Format", "", "", "No Formula")
	addPropCellXML(row, "SortKey", "", "", "No Formula")
	addPropCellXML(row, "Type", "0", "", "")
	addPropCellXML(row, "Invisible", "0", "", "No Formula")
	addPropCellXML(row, "Verify", "0", "", "No Formula")
	addPropCellXML(row, "DataLinked", "0", "", "No Formula")
	addPropCellXML(row, "LangID", "en-US", "", "")
	addPropCellXML(row, "Calendar", "0", "", "No Formula")

	// Invalidate cached data properties
	s.dataProperties = nil

	return newDataProperty(row, s)
}

// --- Internal helpers ---

// setCellValueFormula sets both V and F attributes on a cell, creating if needed.
func (s *Shape) setCellValueFormula(name, value, formula string) {
	c := s.ensureCell(name, "V")
	c.SetValue(value)
	if formula != "" {
		c.SetFormula(formula)
	}
}

// addCellXML creates a Cell element under parent with N, V, and optional F attributes.
func addCellXML(parent *etree.Element, name, value, formula string) {
	c := parent.CreateElement("Cell")
	c.CreateAttr("N", name)
	c.CreateAttr("V", value)
	if formula != "" {
		c.CreateAttr("F", formula)
	}
}

// addGeoRowXML creates a geometry Row element (e.g. RelMoveTo, RelLineTo).
func addGeoRowXML(parent *etree.Element, rowType, ix, x, y string) {
	row := parent.CreateElement("Row")
	row.CreateAttr("T", rowType)
	row.CreateAttr("IX", ix)
	c1 := row.CreateElement("Cell")
	c1.CreateAttr("N", "X")
	c1.CreateAttr("V", x)
	c2 := row.CreateElement("Cell")
	c2.CreateAttr("N", "Y")
	c2.CreateAttr("V", y)
}

// addPropCellXML creates a property Cell element with optional U (unit) and F (formula).
func addPropCellXML(parent *etree.Element, name, value, unit, formula string) {
	c := parent.CreateElement("Cell")
	c.CreateAttr("N", name)
	c.CreateAttr("V", value)
	if unit != "" {
		c.CreateAttr("U", unit)
	}
	if formula != "" {
		c.CreateAttr("F", formula)
	}
}
