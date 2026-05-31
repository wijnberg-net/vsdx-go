package vsdx

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExportOptions configures image export.
type ExportOptions struct {
	DPI    float64 // Resolution in dots per inch (default: 96)
	Width  int     // Output width in pixels (0 = auto from DPI)
	Height int     // Output height in pixels (0 = auto from DPI)
}

// DefaultExportOptions returns sensible export defaults.
func DefaultExportOptions() ExportOptions {
	return ExportOptions{
		DPI: 96,
	}
}

// ExportPNG exports a shape to a PNG file.
// Requires rsvg-convert or inkscape to be installed.
func (s *Shape) ExportPNG(filename string, opts ...ExportOptions) error {
	opt := DefaultExportOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Generate SVG.
	result, err := ShapeToSVG(s)
	if err != nil {
		return fmt.Errorf("generating SVG: %w", err)
	}

	return svgToPNG(result.SVG, filename, opt)
}

// ExportPNG exports a page to a PNG file.
// Requires rsvg-convert or inkscape to be installed.
func (p *Page) ExportPNG(filename string, opts ...ExportOptions) error {
	opt := DefaultExportOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Generate SVG for the entire page.
	var svgParts []string
	pageW := p.Width()
	pageH := p.Height()
	if pageW <= 0 {
		pageW = 8.5 // Default letter width
	}
	if pageH <= 0 {
		pageH = 11 // Default letter height
	}

	// Scale to reasonable output size.
	scale := 100.0 / pageW
	viewW := pageW * scale
	viewH := pageH * scale

	svgParts = append(svgParts, fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.2f %.2f" width="%.0f" height="%.0f">`,
		viewW, viewH, viewW*opt.DPI/96, viewH*opt.DPI/96))

	// Render each shape.
	for _, shape := range p.AllShapes() {
		result, err := ShapeToSVG(shape, WithSize(viewW, viewH))
		if err != nil {
			continue
		}
		// Extract the content between <svg> and </svg>.
		content := extractSVGContent(string(result.SVG))
		svgParts = append(svgParts, content)
	}

	svgParts = append(svgParts, "</svg>")
	svgData := []byte(strings.Join(svgParts, "\n"))

	return svgToPNG(svgData, filename, opt)
}

// svgToPNG converts SVG data to a PNG file using external tools.
func svgToPNG(svgData []byte, filename string, opts ExportOptions) error {
	// Try rsvg-convert first (faster, more common).
	if hasCommand("rsvg-convert") {
		return rsvgConvert(svgData, filename, opts, "png")
	}

	// Fall back to Inkscape.
	if hasCommand("inkscape") {
		return inkscapeConvert(svgData, filename, opts, "png")
	}

	return fmt.Errorf("PNG export requires rsvg-convert or inkscape to be installed")
}

// ExportPDF exports a page to a PDF file.
// Requires rsvg-convert or inkscape to be installed.
func (p *Page) ExportPDF(filename string, opts ...ExportOptions) error {
	opt := DefaultExportOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Generate SVG for the entire page.
	var svgParts []string
	pageW := p.Width()
	pageH := p.Height()
	if pageW <= 0 {
		pageW = 8.5
	}
	if pageH <= 0 {
		pageH = 11
	}

	// PDF dimensions in points (72 per inch).
	pdfW := pageW * 72
	pdfH := pageH * 72

	svgParts = append(svgParts, fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.2f %.2f" width="%.2fpt" height="%.2fpt">`,
		pdfW, pdfH, pdfW, pdfH))

	// Render each shape.
	for _, shape := range p.AllShapes() {
		result, err := ShapeToSVG(shape, WithSize(pdfW, pdfH))
		if err != nil {
			continue
		}
		content := extractSVGContent(string(result.SVG))
		svgParts = append(svgParts, content)
	}

	svgParts = append(svgParts, "</svg>")
	svgData := []byte(strings.Join(svgParts, "\n"))

	return svgToPDF(svgData, filename, opt)
}

// svgToPDF converts SVG data to a PDF file using external tools.
func svgToPDF(svgData []byte, filename string, opts ExportOptions) error {
	// Try rsvg-convert first.
	if hasCommand("rsvg-convert") {
		return rsvgConvert(svgData, filename, opts, "pdf")
	}

	// Fall back to Inkscape.
	if hasCommand("inkscape") {
		return inkscapeConvert(svgData, filename, opts, "pdf")
	}

	return fmt.Errorf("PDF export requires rsvg-convert or inkscape to be installed")
}

// ExportPDFMultiPage exports multiple pages to a single PDF file.
// Requires rsvg-convert or inkscape to be installed.
func (v *VisioFile) ExportPDFMultiPage(filename string, opts ...ExportOptions) error {
	if len(v.Pages) == 0 {
		return fmt.Errorf("no pages to export")
	}

	opt := DefaultExportOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Create temporary directory for individual page PDFs.
	tmpDir, err := os.MkdirTemp("", "visio-export-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Export each page to a temporary PDF.
	var pagePDFs []string
	for i, page := range v.Pages {
		tmpFile := filepath.Join(tmpDir, fmt.Sprintf("page%d.pdf", i+1))
		if err := page.ExportPDF(tmpFile, opt); err != nil {
			return fmt.Errorf("exporting page %d: %w", i+1, err)
		}
		pagePDFs = append(pagePDFs, tmpFile)
	}

	// Merge PDFs (requires pdfunite or similar).
	return mergePDFs(pagePDFs, filename)
}

// rsvgConvert uses rsvg-convert to convert SVG to the target format.
func rsvgConvert(svgData []byte, filename string, opts ExportOptions, format string) error {
	args := []string{"-f", format, "-o", filename}

	if opts.DPI > 0 {
		args = append(args, "-d", fmt.Sprintf("%.0f", opts.DPI))
		args = append(args, "-p", fmt.Sprintf("%.0f", opts.DPI))
	}
	if opts.Width > 0 {
		args = append(args, "-w", fmt.Sprintf("%d", opts.Width))
	}
	if opts.Height > 0 {
		args = append(args, "-h", fmt.Sprintf("%d", opts.Height))
	}

	cmd := exec.Command("rsvg-convert", args...)
	cmd.Stdin = bytes.NewReader(svgData)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsvg-convert: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// inkscapeConvert uses Inkscape to convert SVG to the target format.
func inkscapeConvert(svgData []byte, filename string, opts ExportOptions, format string) error {
	// Write SVG to temp file (Inkscape reads from file).
	tmpFile, err := os.CreateTemp("", "visio-*.svg")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(svgData); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	tmpFile.Close()

	var args []string
	switch format {
	case "png":
		args = []string{tmpPath, "--export-type=png", "--export-filename=" + filename}
		if opts.DPI > 0 {
			args = append(args, fmt.Sprintf("--export-dpi=%.0f", opts.DPI))
		}
		if opts.Width > 0 {
			args = append(args, fmt.Sprintf("--export-width=%d", opts.Width))
		}
		if opts.Height > 0 {
			args = append(args, fmt.Sprintf("--export-height=%d", opts.Height))
		}
	case "pdf":
		args = []string{tmpPath, "--export-type=pdf", "--export-filename=" + filename}
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	cmd := exec.Command("inkscape", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("inkscape: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// mergePDFs merges multiple PDF files into one.
func mergePDFs(inputs []string, output string) error {
	if len(inputs) == 1 {
		// Just copy the single file.
		data, err := os.ReadFile(inputs[0])
		if err != nil {
			return err
		}
		return os.WriteFile(output, data, 0644)
	}

	// Try pdfunite (from poppler-utils).
	if hasCommand("pdfunite") {
		args := append(inputs, output)
		cmd := exec.Command("pdfunite", args...)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("pdfunite: %w", err)
		}
		return nil
	}

	// Try pdftk.
	if hasCommand("pdftk") {
		args := append(inputs, "cat", "output", output)
		cmd := exec.Command("pdftk", args...)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("pdftk: %w", err)
		}
		return nil
	}

	return fmt.Errorf("PDF merge requires pdfunite or pdftk to be installed")
}

// hasCommand checks if a command is available on the system.
func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// extractSVGContent extracts the content between <svg> and </svg> tags.
func extractSVGContent(svg string) string {
	start := strings.Index(svg, ">")
	if start < 0 {
		return ""
	}
	end := strings.LastIndex(svg, "</svg>")
	if end < 0 || end <= start {
		return ""
	}
	return svg[start+1 : end]
}
