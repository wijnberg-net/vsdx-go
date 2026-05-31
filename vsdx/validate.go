package vsdx

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// ValidationError represents a validation issue found in a Visio file.
type ValidationError struct {
	Path    string // Path within the file (e.g., "visio/pages/page1.xml")
	Element string // Element path (e.g., "Shapes/Shape[@ID='1']/Cell[@N='PinX']")
	Message string // Human-readable error message
	Level   string // "error", "warning", or "info"
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("[%s] %s: %s (%s)", e.Level, e.Path, e.Message, e.Element)
}

// ValidationResult contains all validation errors found.
type ValidationResult struct {
	Errors   []ValidationError
	Warnings []ValidationError
	Info     []ValidationError
}

// IsValid returns true if there are no errors.
func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// AllIssues returns all validation issues (errors, warnings, and info).
func (r *ValidationResult) AllIssues() []ValidationError {
	all := make([]ValidationError, 0, len(r.Errors)+len(r.Warnings)+len(r.Info))
	all = append(all, r.Errors...)
	all = append(all, r.Warnings...)
	all = append(all, r.Info...)
	return all
}

// Validate performs validation on the Visio file and returns any issues found.
func (v *VisioFile) Validate() *ValidationResult {
	result := &ValidationResult{}

	// OPC validation (MS-VSDX §2.1)
	v.validateOPC(result)

	// Validate required files exist.
	v.validateRequiredFiles(result)

	// Validate Content_Types consistency.
	v.validateContentTypes(result)

	// Validate pages.
	for _, page := range v.Pages {
		v.validatePage(page, result)
	}

	// Validate masters.
	for _, master := range v.MasterPages {
		v.validateMaster(master, result)
	}

	// Validate ID references.
	v.validateIDReferences(result)

	// Validate page references (backgrounds).
	v.validatePageReferences(result)

	return result
}

// validateOPC performs Open Packaging Conventions validation per MS-VSDX §2.1.
func (v *VisioFile) validateOPC(result *ValidationResult) {
	// Validate root relationships (_rels/.rels)
	v.validateRootRelationships(result)

	// Validate part-specific relationships
	v.validatePartRelationships(result)

	// Check for orphaned parts (no relationship pointing to them)
	v.validateOrphanedParts(result)
}

// validateRootRelationships validates _rels/.rels per OPC spec.
func (v *VisioFile) validateRootRelationships(result *ValidationResult) {
	relsPath := "_rels/.rels"
	relsData, ok := v.ZipFileContents[relsPath]
	if !ok {
		result.Errors = append(result.Errors, ValidationError{
			Path:    relsPath,
			Element: "",
			Message: "Required root relationships file is missing",
			Level:   "error",
		})
		return
	}

	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(relsData); err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Path:    relsPath,
			Element: "",
			Message: fmt.Sprintf("Failed to parse relationships XML: %v", err),
			Level:   "error",
		})
		return
	}

	// Check for duplicate relationship IDs
	seenIDs := make(map[string]bool)
	root := doc.Root()
	if root == nil {
		return
	}

	for _, rel := range root.SelectElements("Relationship") {
		id := rel.SelectAttrValue("Id", "")
		target := rel.SelectAttrValue("Target", "")

		if id == "" {
			result.Errors = append(result.Errors, ValidationError{
				Path:    relsPath,
				Element: fmt.Sprintf("Relationship[@Target='%s']", target),
				Message: "Relationship missing required Id attribute",
				Level:   "error",
			})
			continue
		}

		if seenIDs[id] {
			result.Errors = append(result.Errors, ValidationError{
				Path:    relsPath,
				Element: fmt.Sprintf("Relationship[@Id='%s']", id),
				Message: fmt.Sprintf("Duplicate relationship ID: %s", id),
				Level:   "error",
			})
		}
		seenIDs[id] = true

		// Validate target exists (relative to package root)
		if target != "" && !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			targetPath := strings.TrimPrefix(target, "/")
			if _, exists := v.ZipFileContents[targetPath]; !exists {
				result.Warnings = append(result.Warnings, ValidationError{
					Path:    relsPath,
					Element: fmt.Sprintf("Relationship[@Id='%s']", id),
					Message: fmt.Sprintf("Relationship target does not exist: %s", target),
					Level:   "warning",
				})
			}
		}
	}
}

// validatePartRelationships validates part-specific .rels files.
func (v *VisioFile) validatePartRelationships(result *ValidationResult) {
	// Find all .rels files
	for filePath, data := range v.ZipFileContents {
		if !strings.HasSuffix(filePath, ".rels") || filePath == "_rels/.rels" {
			continue
		}

		doc := etree.NewDocument()
		if err := doc.ReadFromBytes(data); err != nil {
			result.Warnings = append(result.Warnings, ValidationError{
				Path:    filePath,
				Element: "",
				Message: fmt.Sprintf("Failed to parse relationships XML: %v", err),
				Level:   "warning",
			})
			continue
		}

		// Determine the base path for resolving relative targets
		// For visio/pages/_rels/page1.xml.rels, base is visio/pages/
		basePath := path.Dir(path.Dir(filePath)) + "/"

		root := doc.Root()
		if root == nil {
			continue
		}

		seenIDs := make(map[string]bool)
		for _, rel := range root.SelectElements("Relationship") {
			id := rel.SelectAttrValue("Id", "")
			target := rel.SelectAttrValue("Target", "")

			if id == "" {
				continue
			}

			if seenIDs[id] {
				result.Errors = append(result.Errors, ValidationError{
					Path:    filePath,
					Element: fmt.Sprintf("Relationship[@Id='%s']", id),
					Message: fmt.Sprintf("Duplicate relationship ID: %s", id),
					Level:   "error",
				})
			}
			seenIDs[id] = true

			// Validate relative target exists
			if target != "" && !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") && !strings.HasPrefix(target, "/") {
				resolvedPath := path.Clean(basePath + target)
				if _, exists := v.ZipFileContents[resolvedPath]; !exists {
					result.Warnings = append(result.Warnings, ValidationError{
						Path:    filePath,
						Element: fmt.Sprintf("Relationship[@Id='%s']", id),
						Message: fmt.Sprintf("Relationship target does not exist: %s (resolved: %s)", target, resolvedPath),
						Level:   "warning",
					})
				}
			}
		}
	}
}

// validateOrphanedParts checks for parts that have no relationship pointing to them.
func (v *VisioFile) validateOrphanedParts(result *ValidationResult) {
	// Collect all relationship targets
	referencedParts := make(map[string]bool)

	// Always referenced parts (package-level)
	referencedParts["[Content_Types].xml"] = true
	referencedParts["_rels/.rels"] = true

	for filePath, data := range v.ZipFileContents {
		if !strings.HasSuffix(filePath, ".rels") {
			continue
		}

		doc := etree.NewDocument()
		if err := doc.ReadFromBytes(data); err != nil {
			continue
		}

		// Determine base path
		var basePath string
		if filePath == "_rels/.rels" {
			basePath = ""
		} else {
			basePath = path.Dir(path.Dir(filePath)) + "/"
		}

		root := doc.Root()
		if root == nil {
			continue
		}

		for _, rel := range root.SelectElements("Relationship") {
			target := rel.SelectAttrValue("Target", "")
			if target == "" || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
				continue
			}

			var resolvedPath string
			if strings.HasPrefix(target, "/") {
				resolvedPath = strings.TrimPrefix(target, "/")
			} else {
				resolvedPath = path.Clean(basePath + target)
			}
			referencedParts[resolvedPath] = true

			// Also mark the .rels file for this part as referenced
			relsPath := path.Dir(resolvedPath) + "/_rels/" + path.Base(resolvedPath) + ".rels"
			referencedParts[relsPath] = true
		}

		// The .rels file itself is referenced by its parent part
		referencedParts[filePath] = true
	}

	// Check for orphaned parts (excluding .rels files and known special cases)
	for filePath := range v.ZipFileContents {
		if strings.HasSuffix(filePath, ".rels") {
			continue
		}
		if strings.HasPrefix(filePath, "docProps/") {
			// Document properties may be referenced by root rels, already handled
			continue
		}
		if !referencedParts[filePath] {
			result.Info = append(result.Info, ValidationError{
				Path:    filePath,
				Element: "",
				Message: "Part has no relationship pointing to it (orphaned)",
				Level:   "info",
			})
		}
	}
}

// validateRequiredFiles checks that required ZIP entries exist per MS-VSDX spec §2.1.
func (v *VisioFile) validateRequiredFiles(result *ValidationResult) {
	// Required OPC parts
	requiredOPC := []string{
		"[Content_Types].xml",
		"_rels/.rels",
	}

	// Required VSDX parts (MS-VSDX §2.1.1)
	requiredVSDX := []string{
		"visio/document.xml",
		"visio/pages/pages.xml",
	}

	for _, p := range requiredOPC {
		if _, ok := v.ZipFileContents[p]; !ok {
			result.Errors = append(result.Errors, ValidationError{
				Path:    p,
				Element: "",
				Message: "Required OPC file is missing",
				Level:   "error",
			})
		}
	}

	for _, p := range requiredVSDX {
		if _, ok := v.ZipFileContents[p]; !ok {
			result.Errors = append(result.Errors, ValidationError{
				Path:    p,
				Element: "",
				Message: "Required VSDX file is missing",
				Level:   "error",
			})
		}
	}

	// Optional but common parts (warnings if missing)
	optionalParts := []string{
		"docProps/core.xml",
		"docProps/app.xml",
	}

	for _, p := range optionalParts {
		if _, ok := v.ZipFileContents[p]; !ok {
			result.Info = append(result.Info, ValidationError{
				Path:    p,
				Element: "",
				Message: "Common optional file is missing",
				Level:   "info",
			})
		}
	}
}

// validateContentTypes checks that [Content_Types].xml is consistent with ZIP contents.
func (v *VisioFile) validateContentTypes(result *ValidationResult) {
	ctData, ok := v.ZipFileContents["[Content_Types].xml"]
	if !ok {
		return
	}

	content := string(ctData)

	// Check that page files are registered.
	for _, page := range v.Pages {
		if page.filename != "" {
			partName := "/" + page.filename
			if !strings.Contains(content, partName) && !strings.Contains(content, "Extension=\"xml\"") {
				result.Info = append(result.Info, ValidationError{
					Path:    "[Content_Types].xml",
					Element: "Override[@PartName='" + partName + "']",
					Message: fmt.Sprintf("Page file %q not explicitly registered (using extension default)", page.filename),
					Level:   "info",
				})
			}
		}
	}

	// Check that master files are registered.
	for _, master := range v.MasterPages {
		if master.filename != "" {
			partName := "/" + master.filename
			if !strings.Contains(content, partName) && !strings.Contains(content, "Extension=\"xml\"") {
				result.Info = append(result.Info, ValidationError{
					Path:    "[Content_Types].xml",
					Element: "Override[@PartName='" + partName + "']",
					Message: fmt.Sprintf("Master file %q not explicitly registered (using extension default)", master.filename),
					Level:   "info",
				})
			}
		}
	}
}

// validatePage validates a single page.
func (v *VisioFile) validatePage(page *Page, result *ValidationResult) {
	if page.xml == nil {
		result.Errors = append(result.Errors, ValidationError{
			Path:    page.filename,
			Element: "",
			Message: "Page XML is nil",
			Level:   "error",
		})
		return
	}

	// Validate shapes.
	for _, shape := range page.AllShapes() {
		v.validateShape(shape, page.filename, result)
	}
}

// validateMaster validates a master page.
func (v *VisioFile) validateMaster(master *Page, result *ValidationResult) {
	if master.xml == nil {
		result.Warnings = append(result.Warnings, ValidationError{
			Path:    master.filename,
			Element: "",
			Message: "Master XML is nil",
			Level:   "warning",
		})
		return
	}
}

// validateShape validates a single shape.
func (v *VisioFile) validateShape(shape *Shape, path string, result *ValidationResult) {
	element := fmt.Sprintf("Shape[@ID='%s']", shape.ID)

	// Validate shape ID.
	if shape.ID == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:    path,
			Element: element,
			Message: "Shape has no ID",
			Level:   "error",
		})
	}

	// Validate numeric cell values are valid.
	numericCells := []string{"PinX", "PinY", "Width", "Height", "LocPinX", "LocPinY", "Angle"}
	for _, cellName := range numericCells {
		if val := shape.CellValue(cellName); val != "" {
			if _, err := strconv.ParseFloat(val, 64); err != nil {
				// Check if it's a formula (contains function calls or refs).
				if !strings.Contains(val, "(") && !strings.Contains(val, "!") {
					result.Warnings = append(result.Warnings, ValidationError{
						Path:    path,
						Element: element + "/Cell[@N='" + cellName + "']",
						Message: fmt.Sprintf("Cell %s has non-numeric value: %q", cellName, val),
						Level:   "warning",
					})
				}
			}
		}
	}

	// Validate geometry if present.
	for _, geom := range shape.Geometries {
		v.validateGeometry(geom, shape.ID, path, result)
	}

	// Validate master reference if present.
	if shape.MasterPageID != "" {
		masterPage := v.GetMasterPageByID(shape.MasterPageID)
		if masterPage == nil {
			result.Warnings = append(result.Warnings, ValidationError{
				Path:    path,
				Element: element,
				Message: fmt.Sprintf("Master reference %q not found", shape.MasterPageID),
				Level:   "warning",
			})
		} else if shape.MasterShapeID != "" {
			// Validate MasterShapeID exists within the master page.
			found := false
			for _, ms := range masterPage.AllShapes() {
				if ms.ID == shape.MasterShapeID {
					found = true
					break
				}
			}
			if !found {
				result.Warnings = append(result.Warnings, ValidationError{
					Path:    path,
					Element: element,
					Message: fmt.Sprintf("MasterShape %q not found in Master %q", shape.MasterShapeID, shape.MasterPageID),
					Level:   "warning",
				})
			}
		}
	}

	// Validate layer references if present.
	if layerMember := shape.CellValue("LayerMember"); layerMember != "" {
		v.validateLayerReference(shape, layerMember, path, result)
	}
}

// validateGeometry validates a geometry section.
func (v *VisioFile) validateGeometry(geom *Geometry, shapeID, path string, result *ValidationResult) {
	element := fmt.Sprintf("Shape[@ID='%s']/Section[@N='Geometry']", shapeID)

	if len(geom.Rows) == 0 {
		result.Info = append(result.Info, ValidationError{
			Path:    path,
			Element: element,
			Message: "Geometry section has no rows",
			Level:   "info",
		})
		return
	}

	// Check for valid row types.
	validRowTypes := map[string]bool{
		"MoveTo": true, "LineTo": true, "RelMoveTo": true, "RelLineTo": true,
		"ArcTo": true, "EllipticalArcTo": true, "RelEllipticalArcTo": true,
		"RelCubBezTo": true, "RelQuadBezTo": true, "NURBSTo": true,
		"PolylineTo": true, "SplineStart": true, "SplineKnot": true,
		"InfiniteLine": true, "Ellipse": true,
	}

	for _, row := range geom.Rows {
		rowType := row.RowType()
		if rowType != "" && !validRowTypes[rowType] {
			result.Warnings = append(result.Warnings, ValidationError{
				Path:    path,
				Element: element + "/Row[@T='" + rowType + "']",
				Message: fmt.Sprintf("Unknown geometry row type: %q", rowType),
				Level:   "warning",
			})
		}
	}
}

// validateIDReferences checks that all ID references are valid.
func (v *VisioFile) validateIDReferences(result *ValidationResult) {
	// Collect all shape IDs.
	shapeIDs := make(map[string]bool)
	for _, page := range v.Pages {
		for _, shape := range page.AllShapes() {
			shapeIDs[shape.ID] = true
		}
	}

	// Check Connect references.
	for _, page := range v.Pages {
		for _, conn := range page.Connects() {
			fromID := conn.ShapeID()
			toID := conn.ConnectorShapeID()

			if fromID != "" && !shapeIDs[fromID] {
				result.Warnings = append(result.Warnings, ValidationError{
					Path:    page.filename,
					Element: fmt.Sprintf("Connect[@FromSheet='%s']", fromID),
					Message: fmt.Sprintf("Connect references non-existent shape %q", fromID),
					Level:   "warning",
				})
			}

			if toID != "" && !shapeIDs[toID] {
				result.Warnings = append(result.Warnings, ValidationError{
					Path:    page.filename,
					Element: fmt.Sprintf("Connect[@ToSheet='%s']", toID),
					Message: fmt.Sprintf("Connect references non-existent shape %q", toID),
					Level:   "warning",
				})
			}
		}
	}
}

// validateLayerReference checks that layer references are valid.
func (v *VisioFile) validateLayerReference(shape *Shape, layerMember, path string, result *ValidationResult) {
	element := fmt.Sprintf("Shape[@ID='%s']", shape.ID)

	// Get the page this shape belongs to.
	var page *Page
	switch p := shape.Parent.(type) {
	case *Page:
		page = p
	case *Shape:
		// Walk up to find the page.
		current := p
		for {
			switch pp := current.Parent.(type) {
			case *Page:
				page = pp
			case *Shape:
				current = pp
				continue
			}
			break
		}
	}

	if page == nil {
		return
	}

	// Count layers from page's Layer section.
	layerCount := 0
	if pageSheet := page.pagesheetXML(); pageSheet != nil {
		if layerSection := pageSheet.FindElement("Section[@N='Layer']"); layerSection != nil {
			layerCount = len(layerSection.SelectElements("Row"))
		}
	}

	for _, part := range strings.Split(layerMember, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx, err := strconv.Atoi(part)
		if err != nil {
			result.Warnings = append(result.Warnings, ValidationError{
				Path:    path,
				Element: element + "/Cell[@N='LayerMember']",
				Message: fmt.Sprintf("Invalid layer index %q", part),
				Level:   "warning",
			})
			continue
		}
		if idx < 0 || idx >= layerCount {
			result.Warnings = append(result.Warnings, ValidationError{
				Path:    path,
				Element: element + "/Cell[@N='LayerMember']",
				Message: fmt.Sprintf("Layer index %d does not exist (page has %d layers)", idx, layerCount),
				Level:   "warning",
			})
		}
	}
}

// validatePageReferences checks that page references (e.g., background pages) are valid.
func (v *VisioFile) validatePageReferences(result *ValidationResult) {
	// Collect all page IDs.
	pageIDs := make(map[string]bool)
	for _, page := range v.Pages {
		if id := page.PageID(); id != "" {
			pageIDs[id] = true
		}
	}

	// Check background page references.
	for _, page := range v.Pages {
		if page.xml == nil {
			continue
		}
		// Background page is referenced via BackPage attribute on PageSheet or Rel element.
		pageSheet := page.xml.FindElement("PageSheet")
		if pageSheet != nil {
			if backPage := pageSheet.SelectAttrValue("BackPage", ""); backPage != "" {
				if !pageIDs[backPage] {
					result.Warnings = append(result.Warnings, ValidationError{
						Path:    page.filename,
						Element: "PageSheet[@BackPage='" + backPage + "']",
						Message: fmt.Sprintf("Background page reference %q not found", backPage),
						Level:   "warning",
					})
				}
			}
		}

		// Also check Rel elements for page relationships.
		for _, rel := range page.xml.FindElements("//Rel") {
			if target := rel.SelectAttrValue("r:id", ""); target != "" {
				// Relationship IDs are validated by validateContentTypes.
			}
		}
	}
}

// ParseOptions configures error handling during file parsing.
type ParseOptions struct {
	StrictMode   bool        // If true, fail on any error
	ErrorHandler func(error) // Callback for non-fatal errors
	MaxErrors    int         // Stop after this many errors (0 = unlimited)
}

// OpenWithOptions opens a Visio file with custom parse options.
func OpenWithOptions(filename string, opts ParseOptions) (*VisioFile, []error, error) {
	vis, err := Open(filename)
	if err != nil {
		return nil, nil, err
	}

	// Collect non-fatal errors during validation.
	var parseErrors []error
	result := vis.Validate()

	for _, ve := range result.AllIssues() {
		parseErrors = append(parseErrors, ve)
		if opts.ErrorHandler != nil {
			opts.ErrorHandler(ve)
		}
		if opts.MaxErrors > 0 && len(parseErrors) >= opts.MaxErrors {
			break
		}
	}

	if opts.StrictMode && len(result.Errors) > 0 {
		return nil, parseErrors, result.Errors[0]
	}

	return vis, parseErrors, nil
}
