package vsdx

import (
	"strconv"

	"github.com/beevik/etree"
)

// LineGradient represents a gradient stroke on a shape.
type LineGradient struct {
	Enabled bool
	Dir     int     // Gradient direction type
	Angle   float64 // Angle in radians
	Stops   []GradientStop
	shape   *Shape
}

// LineGradient returns the line gradient for this shape, if any.
func (s *Shape) LineGradient() *LineGradient {
	// Check if line gradient is enabled
	enabled := s.CellValue("LineGradientEnabled")
	if enabled != "1" && enabled != "TRUE" {
		return nil
	}

	grad := &LineGradient{
		Enabled: true,
		shape:   s,
	}

	// Parse direction and angle
	if dir := s.CellValue("LineGradientDir"); dir != "" {
		grad.Dir, _ = strconv.Atoi(dir)
	}
	if angle := s.CellValue("LineGradientAngle"); angle != "" {
		grad.Angle, _ = strconv.ParseFloat(angle, 64)
	}

	// Parse gradient stops
	grad.Stops = s.parseLineGradientStops()

	return grad
}

// parseLineGradientStops parses the LineGradient section for gradient stops.
func (s *Shape) parseLineGradientStops() []GradientStop {
	if s.xml == nil {
		return nil
	}

	section := s.xml.FindElement("Section[@N='LineGradient']")
	if section == nil {
		return nil
	}

	var stops []GradientStop
	for _, row := range section.SelectElements("Row") {
		stop := GradientStop{}

		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			value := cell.SelectAttrValue("V", "")

			switch name {
			case "GradientStopColor":
				stop.Color = value
			case "GradientStopPosition":
				stop.Position, _ = strconv.ParseFloat(value, 64)
			case "GradientStopColorTrans":
				stop.Trans, _ = strconv.ParseFloat(value, 64)
			}
		}

		stops = append(stops, stop)
	}

	return stops
}

// SetLineGradient sets a linear gradient stroke on this shape.
func (s *Shape) SetLineGradient(angle float64, stops []GradientStop) {
	if s.xml == nil {
		return
	}

	// Enable gradient
	s.SetCellValue("LineGradientEnabled", "1")
	s.SetCellValue("LineGradientDir", "0") // Linear
	s.SetCellFormula("LineGradientAngle", fmtFloat(angle))

	// Remove existing section
	if section := s.xml.FindElement("Section[@N='LineGradient']"); section != nil {
		s.xml.RemoveChild(section)
	}

	// Create new section
	section := s.xml.CreateElement("Section")
	section.CreateAttr("N", "LineGradient")

	for i, stop := range stops {
		row := section.CreateElement("Row")
		row.CreateAttr("IX", strconv.Itoa(i))

		colorCell := row.CreateElement("Cell")
		colorCell.CreateAttr("N", "GradientStopColor")
		colorCell.CreateAttr("V", stop.Color)

		posCell := row.CreateElement("Cell")
		posCell.CreateAttr("N", "GradientStopPosition")
		posCell.CreateAttr("V", fmtFloat(stop.Position))

		transCell := row.CreateElement("Cell")
		transCell.CreateAttr("N", "GradientStopColorTrans")
		transCell.CreateAttr("V", fmtFloat(stop.Trans))
	}
}

// ClearLineGradient removes the line gradient from this shape.
func (s *Shape) ClearLineGradient() {
	if s.xml == nil {
		return
	}

	s.SetCellValue("LineGradientEnabled", "0")

	if section := s.xml.FindElement("Section[@N='LineGradient']"); section != nil {
		s.xml.RemoveChild(section)
	}
}

// lineGradientToSVGDef generates an SVG gradient definition for line gradient.
func lineGradientToSVGDef(id string, grad *LineGradient) string {
	if grad == nil || len(grad.Stops) == 0 {
		return ""
	}

	return gradientToSVGDef(&Gradient{
		Enabled: grad.Enabled,
		Type:    "linear",
		Angle:   grad.Angle,
		Stops:   grad.Stops,
	}, id, 2)
}

// Reviewer represents a comment reviewer/author with more details.
type Reviewer struct {
	ID            int
	Name          string
	Initials      string
	Color         string
	ReviewerID    int
	CurrentIndex  int
	xml           *etree.Element
}

// Reviewers returns all reviewers in the document.
func (v *VisioFile) Reviewers() []*Reviewer {
	if v.documentXML == nil {
		return nil
	}

	var reviewers []*Reviewer
	section := v.documentXML.FindElement("//Section[@N='Reviewer']")
	if section == nil {
		return nil
	}

	for _, row := range section.SelectElements("Row") {
		reviewer := &Reviewer{xml: row}
		reviewer.ID, _ = strconv.Atoi(row.SelectAttrValue("IX", "0"))

		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			value := cell.SelectAttrValue("V", "")

			switch name {
			case "Name":
				reviewer.Name = value
			case "Initials":
				reviewer.Initials = value
			case "Color":
				reviewer.Color = value
			case "ReviewerID":
				reviewer.ReviewerID, _ = strconv.Atoi(value)
			case "CurrentIndex":
				reviewer.CurrentIndex, _ = strconv.Atoi(value)
			}
		}

		reviewers = append(reviewers, reviewer)
	}

	return reviewers
}

// GetReviewer returns the reviewer with the given ID.
func (v *VisioFile) GetReviewer(id int) *Reviewer {
	for _, r := range v.Reviewers() {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// Annotation represents an annotation marker on a page.
type Annotation struct {
	ID         int
	X          float64
	Y          float64
	ReviewerID int
	MarkerID   int
	Comment    string
	Date       string
	xml        *etree.Element
}

// Annotations returns all annotations on a page.
func (p *Page) Annotations() []*Annotation {
	if p.xml == nil {
		return nil
	}

	var annotations []*Annotation
	section := p.xml.FindElement("//Section[@N='Annotation']")
	if section == nil {
		return nil
	}

	for _, row := range section.SelectElements("Row") {
		ann := &Annotation{xml: row}
		ann.ID, _ = strconv.Atoi(row.SelectAttrValue("IX", "0"))

		for _, cell := range row.SelectElements("Cell") {
			name := cell.SelectAttrValue("N", "")
			value := cell.SelectAttrValue("V", "")

			switch name {
			case "X":
				ann.X, _ = strconv.ParseFloat(value, 64)
			case "Y":
				ann.Y, _ = strconv.ParseFloat(value, 64)
			case "ReviewerID":
				ann.ReviewerID, _ = strconv.Atoi(value)
			case "MarkerIndex":
				ann.MarkerID, _ = strconv.Atoi(value)
			case "Comment":
				ann.Comment = value
			case "Date":
				ann.Date = value
			}
		}

		annotations = append(annotations, ann)
	}

	return annotations
}

// --- Reviewer Write Support ---

// AddReviewer adds a reviewer to the document and returns the new Reviewer.
func (v *VisioFile) AddReviewer(name, initials, color string) *Reviewer {
	if v.documentXML == nil {
		return nil
	}

	// Find or create Reviewer section
	section := v.documentXML.FindElement("//Section[@N='Reviewer']")
	if section == nil {
		docSheet := v.documentXML.FindElement("DocumentSheet")
		if docSheet == nil {
			docSheet = v.documentXML.Root()
		}
		if docSheet == nil {
			return nil
		}
		section = docSheet.CreateElement("Section")
		section.CreateAttr("N", "Reviewer")
	}

	// Find next ID
	maxID := -1
	for _, row := range section.SelectElements("Row") {
		if ix, err := strconv.Atoi(row.SelectAttrValue("IX", "0")); err == nil && ix > maxID {
			maxID = ix
		}
	}
	newID := maxID + 1

	// Create row
	row := section.CreateElement("Row")
	row.CreateAttr("IX", strconv.Itoa(newID))

	addReviewerCell(row, "Name", name)
	addReviewerCell(row, "Initials", initials)
	addReviewerCell(row, "Color", color)
	addReviewerCell(row, "ReviewerID", strconv.Itoa(newID+1))
	addReviewerCell(row, "CurrentIndex", "0")

	return &Reviewer{
		ID:           newID,
		Name:         name,
		Initials:     initials,
		Color:        color,
		ReviewerID:   newID + 1,
		CurrentIndex: 0,
		xml:          row,
	}
}

// addReviewerCell adds a cell to a reviewer row.
func addReviewerCell(row *etree.Element, name, value string) {
	cell := row.CreateElement("Cell")
	cell.CreateAttr("N", name)
	cell.CreateAttr("V", value)
}

// DeleteReviewer removes a reviewer from the document.
func (v *VisioFile) DeleteReviewer(id int) bool {
	if v.documentXML == nil {
		return false
	}

	section := v.documentXML.FindElement("//Section[@N='Reviewer']")
	if section == nil {
		return false
	}

	for _, row := range section.SelectElements("Row") {
		if ix, err := strconv.Atoi(row.SelectAttrValue("IX", "")); err == nil && ix == id {
			section.RemoveChild(row)
			return true
		}
	}

	return false
}

// --- Annotation Write Support ---

// AddAnnotation adds an annotation to the page and returns the new Annotation.
func (p *Page) AddAnnotation(x, y float64, reviewerID int, comment string) *Annotation {
	if p.xml == nil {
		return nil
	}

	// Find or create Annotation section in PageSheet
	pageSheet := p.pagesheetXML()
	if pageSheet == nil {
		pageSheet = p.xml.Root().CreateElement("PageSheet")
	}

	section := pageSheet.FindElement("Section[@N='Annotation']")
	if section == nil {
		section = pageSheet.CreateElement("Section")
		section.CreateAttr("N", "Annotation")
	}

	// Find next ID
	maxID := -1
	for _, row := range section.SelectElements("Row") {
		if ix, err := strconv.Atoi(row.SelectAttrValue("IX", "0")); err == nil && ix > maxID {
			maxID = ix
		}
	}
	newID := maxID + 1

	// Create row
	row := section.CreateElement("Row")
	row.CreateAttr("IX", strconv.Itoa(newID))

	addAnnotationCell(row, "X", strconv.FormatFloat(x, 'f', -1, 64))
	addAnnotationCell(row, "Y", strconv.FormatFloat(y, 'f', -1, 64))
	addAnnotationCell(row, "ReviewerID", strconv.Itoa(reviewerID))
	addAnnotationCell(row, "MarkerIndex", strconv.Itoa(newID))
	addAnnotationCell(row, "Comment", comment)
	addAnnotationCell(row, "Date", "") // Empty date, can be set later

	return &Annotation{
		ID:         newID,
		X:          x,
		Y:          y,
		ReviewerID: reviewerID,
		MarkerID:   newID,
		Comment:    comment,
		xml:        row,
	}
}

// addAnnotationCell adds a cell to an annotation row.
func addAnnotationCell(row *etree.Element, name, value string) {
	cell := row.CreateElement("Cell")
	cell.CreateAttr("N", name)
	cell.CreateAttr("V", value)
}

// DeleteAnnotation removes an annotation from the page.
func (p *Page) DeleteAnnotation(id int) bool {
	if p.xml == nil {
		return false
	}

	pageSheet := p.pagesheetXML()
	if pageSheet == nil {
		return false
	}

	section := pageSheet.FindElement("Section[@N='Annotation']")
	if section == nil {
		return false
	}

	for _, row := range section.SelectElements("Row") {
		if ix, err := strconv.Atoi(row.SelectAttrValue("IX", "")); err == nil && ix == id {
			section.RemoveChild(row)
			return true
		}
	}

	return false
}

// SetComment updates the comment text of an annotation.
func (a *Annotation) SetComment(comment string) {
	if a.xml == nil {
		return
	}
	for _, cell := range a.xml.SelectElements("Cell") {
		if cell.SelectAttrValue("N", "") == "Comment" {
			cell.CreateAttr("V", comment)
			a.Comment = comment
			return
		}
	}
	// Cell doesn't exist, create it
	addAnnotationCell(a.xml, "Comment", comment)
	a.Comment = comment
}

// SetDate updates the date of an annotation.
func (a *Annotation) SetDate(date string) {
	if a.xml == nil {
		return
	}
	for _, cell := range a.xml.SelectElements("Cell") {
		if cell.SelectAttrValue("N", "") == "Date" {
			cell.CreateAttr("V", date)
			a.Date = date
			return
		}
	}
	addAnnotationCell(a.xml, "Date", date)
	a.Date = date
}
