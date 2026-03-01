package vsdx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/beevik/etree"
)

// VisioFile represents an open .vsdx file.
type VisioFile struct {
	Filename        string
	Pages           []*Page
	MasterPages     []*Page
	ZipFileContents map[string][]byte

	pagesXML        *etree.Document
	pagesXMLRels    *etree.Document
	contentTypesXML *etree.Document
	appXML          *etree.Document
	documentXML     *etree.Document
	documentXMLRels *etree.Document
	mastersXML      *etree.Element // root element of masters.xml
	masterIndex     map[string]*Page
	debug           bool
}

// Open opens a .vsdx or .vsdm file and returns a VisioFile.
func Open(filename string) (*VisioFile, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".vsdx" && ext != ".vsdm" {
		return nil, fmt.Errorf("invalid file type: %s", ext)
	}

	v := &VisioFile{
		Filename:        filename,
		ZipFileContents: make(map[string][]byte),
		masterIndex:     make(map[string]*Page),
	}

	if err := v.loadZipContents(); err != nil {
		return nil, fmt.Errorf("loading zip contents: %w", err)
	}

	if err := v.loadPages(); err != nil {
		return nil, fmt.Errorf("loading pages: %w", err)
	}

	v.loadMasterPages()

	return v, nil
}

// Close releases resources associated with the VisioFile.
func (v *VisioFile) Close() {
	v.ZipFileContents = nil
}

// loadZipContents reads all files from the ZIP archive into memory.
func (v *VisioFile) loadZipContents() error {
	r, err := zip.OpenReader(v.Filename)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("opening %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("reading %s: %w", f.Name, err)
		}
		v.ZipFileContents[f.Name] = data
	}
	return nil
}

// fileToXML parses an XML file from the zip contents.
func (v *VisioFile) fileToXML(filename string) (*etree.Document, error) {
	data, ok := v.ZipFileContents[filename]
	if !ok {
		return nil, nil
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filename, err)
	}
	return doc, nil
}

// loadPages loads page objects from the ZIP contents.
func (v *VisioFile) loadPages() error {
	// Load pages.xml.rels to get page filename mappings
	relsDoc, err := v.fileToXML("visio/pages/_rels/pages.xml.rels")
	if err != nil {
		return fmt.Errorf("loading pages.xml.rels: %w", err)
	}
	if relsDoc == nil {
		return fmt.Errorf("pages.xml.rels not found")
	}
	v.pagesXMLRels = relsDoc

	// Build relID -> page filename mapping
	relIDToPage := make(map[string]string)
	for _, rel := range relsDoc.Root().SelectElements("Relationship") {
		relID := rel.SelectAttrValue("Id", "")
		target := rel.SelectAttrValue("Target", "")
		relIDToPage[relID] = target
	}

	// Load pages.xml
	pagesDoc, err := v.fileToXML("visio/pages/pages.xml")
	if err != nil {
		return fmt.Errorf("loading pages.xml: %w", err)
	}
	if pagesDoc == nil {
		return fmt.Errorf("pages.xml not found")
	}
	v.pagesXML = pagesDoc

	// Create Page objects for each page
	for _, pageElem := range pagesDoc.Root().SelectElements("Page") {
		relElem := pageElem.SelectElement("Rel")
		if relElem == nil {
			continue
		}
		// The r:id attribute - in etree, namespace-qualified attributes
		// are stored with their namespace prefix removed in the Key field
		relID := relElem.SelectAttrValue("id", "")

		pageName := pageElem.SelectAttrValue("Name", "")
		pageID := pageElem.SelectAttrValue("ID", "")
		pageFile := relIDToPage[relID]
		pagePath := "visio/pages/" + pageFile

		pageDoc, err := v.fileToXML(pagePath)
		if err != nil {
			return fmt.Errorf("loading %s: %w", pagePath, err)
		}
		if pageDoc == nil {
			continue
		}

		page := newPage(pageDoc, pagePath, pageName, pageID, relID, v)

		// Check for page-level rels (e.g., master references)
		basePageFile := filepath.Base(pagePath)
		pageRelsPath := "visio/pages/_rels/" + basePageFile + ".rels"
		if _, ok := v.ZipFileContents[pageRelsPath]; ok {
			pageRelsDoc, err := v.fileToXML(pageRelsPath)
			if err == nil && pageRelsDoc != nil {
				page.RelsXMLFile = pageRelsPath
				page.RelsXML = pageRelsDoc
			}
		}

		v.Pages = append(v.Pages, page)
	}

	// Load other XML files
	v.contentTypesXML, _ = v.fileToXML("[Content_Types].xml")
	v.appXML, _ = v.fileToXML("docProps/app.xml")
	v.documentXML, _ = v.fileToXML("visio/document.xml")
	v.documentXMLRels, _ = v.fileToXML("visio/_rels/document.xml.rels")

	return nil
}

// loadMasterPages loads master page objects from the ZIP contents.
func (v *VisioFile) loadMasterPages() {
	// Load masters.xml.rels
	masterRelsDoc, err := v.fileToXML("visio/masters/_rels/masters.xml.rels")
	if err != nil || masterRelsDoc == nil {
		return // No masters
	}

	// Build relID -> master path mapping
	relIDToPath := make(map[string]string)
	for _, rel := range masterRelsDoc.Root().SelectElements("Relationship") {
		masterID := rel.SelectAttrValue("Id", "")
		target := rel.SelectAttrValue("Target", "")
		masterPath := "visio/masters/" + target
		relIDToPath[masterID] = masterPath
	}

	// Load masters.xml
	mastersDoc, err := v.fileToXML("visio/masters/masters.xml")
	if err != nil || mastersDoc == nil {
		return
	}
	v.mastersXML = mastersDoc.Root()

	// Create Page objects for each master
	for _, masterElem := range v.mastersXML.SelectElements("Master") {
		masterName := masterElem.SelectAttrValue("NameU", "")
		if masterName == "" {
			masterName = masterElem.SelectAttrValue("Name", "")
		}
		if masterName == "" {
			masterName = "Unknown"
		}

		relElem := masterElem.SelectElement("Rel")
		if relElem == nil {
			continue
		}
		relID := relElem.SelectAttrValue("id", "")
		masterID := masterElem.SelectAttrValue("ID", "")
		masterUniqueID := masterElem.SelectAttrValue("UniqueID", "")
		masterBaseID := masterElem.SelectAttrValue("BaseID", "")

		masterPath := relIDToPath[relID]
		masterDoc, err := v.fileToXML(masterPath)
		if err != nil || masterDoc == nil {
			continue
		}

		masterPage := newPage(masterDoc, masterPath, masterName, masterID, relID, v)
		masterPage.MasterUniqueID = masterUniqueID
		masterPage.MasterBaseID = masterBaseID

		v.MasterPages = append(v.MasterPages, masterPage)
		v.masterIndex[masterName] = masterPage
	}
}

// GetPage returns the page at the given zero-based index, or nil if out of range.
func (v *VisioFile) GetPage(n int) *Page {
	if n < 0 || n >= len(v.Pages) {
		return nil
	}
	return v.Pages[n]
}

// GetPageNames returns a list of page names.
func (v *VisioFile) GetPageNames() []string {
	names := make([]string, len(v.Pages))
	for i, p := range v.Pages {
		names[i] = p.Name()
	}
	return names
}

// GetPageByName returns the first page with the given name, or nil.
func (v *VisioFile) GetPageByName(name string) *Page {
	for _, p := range v.Pages {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

// GetMasterPageByID returns the master page with the given ID, or nil.
func (v *VisioFile) GetMasterPageByID(id string) *Page {
	for _, m := range v.MasterPages {
		if m.pageID == id {
			return m
		}
	}
	return nil
}

// AppXML returns the app.xml document.
func (v *VisioFile) AppXML() *etree.Document {
	return v.appXML
}

// PagesXML returns the pages.xml document.
func (v *VisioFile) PagesXML() *etree.Document {
	return v.pagesXML
}

// MastersXML returns the masters.xml root element.
func (v *VisioFile) MastersXML() *etree.Element {
	return v.mastersXML
}

// PrettyPrintElement returns a pretty-printed XML string of an etree element.
func PrettyPrintElement(elem *etree.Element) string {
	doc := etree.NewDocument()
	doc.SetRoot(elem.Copy())
	doc.Indent(2)
	s, _ := doc.WriteToString()
	return s
}

// SaveVsdx saves the VisioFile to a new .vsdx file.
// This updates all modified XML back into the zip contents and writes to disk.
func (v *VisioFile) SaveVsdx(filename string) error {
	// Update pages XML back to zip contents
	if v.pagesXML != nil {
		data, err := v.pagesXML.WriteToBytes()
		if err != nil {
			return fmt.Errorf("serializing pages.xml: %w", err)
		}
		v.ZipFileContents["visio/pages/pages.xml"] = data
	}
	if v.pagesXMLRels != nil {
		data, err := v.pagesXMLRels.WriteToBytes()
		if err != nil {
			return fmt.Errorf("serializing pages.xml.rels: %w", err)
		}
		v.ZipFileContents["visio/pages/_rels/pages.xml.rels"] = data
	}

	// Update each page XML
	for _, page := range v.Pages {
		if page.xml != nil {
			data, err := page.xml.WriteToBytes()
			if err != nil {
				return fmt.Errorf("serializing %s: %w", page.filename, err)
			}
			v.ZipFileContents[page.filename] = data
		}
	}

	// Update other XML files
	if v.contentTypesXML != nil {
		data, _ := v.contentTypesXML.WriteToBytes()
		v.ZipFileContents["[Content_Types].xml"] = data
	}
	if v.appXML != nil {
		data, _ := v.appXML.WriteToBytes()
		v.ZipFileContents["docProps/app.xml"] = data
	}
	if v.documentXML != nil {
		data, _ := v.documentXML.WriteToBytes()
		v.ZipFileContents["visio/document.xml"] = data
	}
	if v.documentXMLRels != nil {
		data, _ := v.documentXMLRels.WriteToBytes()
		v.ZipFileContents["visio/_rels/document.xml.rels"] = data
	}

	// Write zip file
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for path, content := range v.ZipFileContents {
		fw, err := w.Create(path)
		if err != nil {
			return fmt.Errorf("creating %s in zip: %w", path, err)
		}
		if _, err := fw.Write(content); err != nil {
			return fmt.Errorf("writing %s to zip: %w", path, err)
		}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing zip: %w", err)
	}

	return writeFile(filename, buf.Bytes())
}

// writeFile writes data to a file, creating directories as needed.
func writeFile(filename string, data []byte) error {
	return writeFileBytes(filename, data)
}
