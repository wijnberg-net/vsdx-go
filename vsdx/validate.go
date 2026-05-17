package vsdx

import (
	"fmt"
	"strconv"
	"strings"
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

	// Validate required files exist.
	v.validateRequiredFiles(result)

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

	return result
}

// validateRequiredFiles checks that required ZIP entries exist.
func (v *VisioFile) validateRequiredFiles(result *ValidationResult) {
	required := []string{
		"[Content_Types].xml",
		"visio/pages/pages.xml",
	}

	for _, path := range required {
		if _, ok := v.ZipFileContents[path]; !ok {
			result.Errors = append(result.Errors, ValidationError{
				Path:    path,
				Element: "",
				Message: "Required file is missing",
				Level:   "error",
			})
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
		if v.GetMasterPageByID(shape.MasterPageID) == nil {
			result.Warnings = append(result.Warnings, ValidationError{
				Path:    path,
				Element: element,
				Message: fmt.Sprintf("Master reference %q not found", shape.MasterPageID),
				Level:   "warning",
			})
		}
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

// ParseOptions configures error handling during file parsing.
type ParseOptions struct {
	StrictMode   bool                  // If true, fail on any error
	ErrorHandler func(error)           // Callback for non-fatal errors
	MaxErrors    int                   // Stop after this many errors (0 = unlimited)
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
