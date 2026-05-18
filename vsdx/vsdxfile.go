package vsdx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// PagePosition represents a relative position for inserting pages.
type PagePosition int

const (
	PageFirst  PagePosition = 0
	PageLast   PagePosition = -1
	PageAfter  PagePosition = -2
	PageBefore PagePosition = -3
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
	rootRelsXML     *etree.Document // _rels/.rels - package relationships
	coreXML         *etree.Document // docProps/core.xml - core properties
	customXML       *etree.Document // docProps/custom.xml - custom properties
	mastersXML      *etree.Element  // root element of masters.xml
	masterIndex     map[string]*Page
	cachedMedia     *Media
}

// IsStencil returns true if this file is a stencil (.vssx/.vssm) with master shapes but no pages.
func (v *VisioFile) IsStencil() bool {
	return len(v.Pages) == 0 && len(v.MasterPages) > 0
}

// Open opens a .vsdx, .vsdm, .vssx, or .vssm file and returns a VisioFile.
func Open(filename string) (*VisioFile, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".vsdx" && ext != ".vsdm" && ext != ".vssx" && ext != ".vssm" {
		return nil, &FileError{Path: filename, Err: ErrInvalidFileType}
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

// OpenBytes opens a .vsdx file from a byte slice (e.g. embedded data).
func OpenBytes(data []byte) (*VisioFile, error) {
	v := &VisioFile{
		Filename:        "(bytes)",
		ZipFileContents: make(map[string][]byte),
		masterIndex:     make(map[string]*Page),
	}

	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("opening zip from bytes: %w", err)
	}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f.Name, err)
		}
		v.ZipFileContents[f.Name] = content
	}

	if err := v.loadPages(); err != nil {
		return nil, fmt.Errorf("loading pages: %w", err)
	}
	v.loadMasterPages()

	return v, nil
}

// Close releases resources associated with the VisioFile.
// It implements the io.Closer interface.
func (v *VisioFile) Close() error {
	if v.cachedMedia != nil {
		v.cachedMedia.Close()
		v.cachedMedia = nil
	}
	v.ZipFileContents = nil
	return nil
}

// loadZipContents reads all files from the ZIP archive into memory.
func (v *VisioFile) loadZipContents() error {
	r, err := zip.OpenReader(v.Filename)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close() //nolint:errcheck // best-effort close of ZIP reader

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("opening %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
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
// For stencil files (.vssx/.vssm), pages.xml.rels may not exist — this is normal.
func (v *VisioFile) loadPages() error {
	// Load pages.xml.rels to get page filename mappings
	relsDoc, err := v.fileToXML("visio/pages/_rels/pages.xml.rels")
	if err != nil {
		return fmt.Errorf("loading pages.xml.rels: %w", err)
	}

	// Stencils (.vssx/.vssm) have no pages — skip page loading
	if relsDoc == nil {
		// Still load other XML files needed for the file structure
		return v.loadCommonXML()
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
		return fmt.Errorf("%w: pages.xml not found", ErrInvalidFormat)
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

	return v.loadCommonXML()
}

// loadCommonXML loads XML files shared between regular documents and stencils.
func (v *VisioFile) loadCommonXML() error {
	var err error

	// Package relationships (root rels)
	v.rootRelsXML, err = v.fileToXML("_rels/.rels")
	if err != nil {
		return fmt.Errorf("loading _rels/.rels: %w", err)
	}

	v.contentTypesXML, err = v.fileToXML("[Content_Types].xml")
	if err != nil {
		return fmt.Errorf("loading [Content_Types].xml: %w", err)
	}
	v.appXML, err = v.fileToXML("docProps/app.xml")
	if err != nil {
		return fmt.Errorf("loading app.xml: %w", err)
	}

	// Core properties (optional)
	v.coreXML, _ = v.fileToXML("docProps/core.xml")

	// Custom properties (optional)
	v.customXML, _ = v.fileToXML("docProps/custom.xml")

	v.documentXML, err = v.fileToXML("visio/document.xml")
	if err != nil {
		return fmt.Errorf("loading document.xml: %w", err)
	}
	v.documentXMLRels, err = v.fileToXML("visio/_rels/document.xml.rels")
	if err != nil {
		return fmt.Errorf("loading document.xml.rels: %w", err)
	}

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

		// Load master-level rels (for Foreign image references)
		baseMasterFile := filepath.Base(masterPath)
		masterRelsPath := "visio/masters/_rels/" + baseMasterFile + ".rels"
		if _, ok := v.ZipFileContents[masterRelsPath]; ok {
			masterPageRels, err := v.fileToXML(masterRelsPath)
			if err == nil && masterPageRels != nil {
				masterPage.RelsXMLFile = masterRelsPath
				masterPage.RelsXML = masterPageRels
			}
		}

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

// GetPageByID returns the page with the given ID, or nil.
func (v *VisioFile) GetPageByID(id string) *Page {
	for _, p := range v.Pages {
		if p.pageID == id {
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

// --- Page management ---

// RemovePageByIndex removes the page at the given zero-based index.
func (v *VisioFile) RemovePageByIndex(index int) {
	if index < 0 || index >= len(v.Pages) {
		return
	}
	// Remove Page element from pages.xml
	pageElems := v.pagesXML.Root().SelectElements("Page")
	if index < len(pageElems) {
		v.pagesXML.Root().RemoveChild(pageElems[index])
	}

	page := v.Pages[index]

	// Remove from app.xml
	v.removePageFromAppXML(page.Name())

	// Remove page XML from zip contents
	delete(v.ZipFileContents, page.filename)

	// Remove from pages slice
	v.Pages = append(v.Pages[:index], v.Pages[index+1:]...)
}

// RemovePageByName removes the first page matching the given name.
func (v *VisioFile) RemovePageByName(name string) {
	for i, p := range v.Pages {
		if p.Name() == name {
			v.RemovePageByIndex(i)
			return
		}
	}
}

// AddPage adds a new empty page at the end of the VisioFile.
func (v *VisioFile) AddPage(name string) (*Page, error) {
	return v.AddPageAt(int(PageLast), name)
}

// AddPageAt adds a new empty page at the specified index (or PagePosition).
func (v *VisioFile) AddPageAt(index int, name string) (*Page, error) {
	// Determine page name
	if name == "" {
		name = fmt.Sprintf("Page-%d", len(v.Pages)+1)
	}
	name = v.getNewPageName(name)

	// Determine filename
	newPageFilename := fmt.Sprintf("page%d.xml", len(v.Pages)+1)

	// Add to pages.xml.rels
	newRelID := v.updatePagesXMLRels(newPageFilename)

	// Create PageSheet element with default values
	pagesheetElem := etree.NewElement("PageSheet")
	pagesheetElem.CreateAttr("FillStyle", "0")
	pagesheetElem.CreateAttr("LineStyle", "0")
	pagesheetElem.CreateAttr("TextStyle", "0")

	defaultCells := []struct{ name, value string }{
		{"PageWidth", "8.26771653543307"},
		{"PageHeight", "11.69291338582677"},
		{"ShdwOffsetX", "0.1181102362204724"},
		{"ShdwOffsetY", "-0.1181102362204724"},
		{"DrawingSizeType", "0"},
		{"DrawingScaleType", "0"},
		{"InhibitSnap", "0"},
		{"UIVisibility", "0"},
		{"ShdwType", "0"},
		{"ShdwObliqueAngle", "0"},
		{"ShdwScaleFactor", "1"},
		{"DrawingResizeType", "1"},
		{"PageShapeSplit", "1"},
	}
	for _, c := range defaultCells {
		cell := pagesheetElem.CreateElement("Cell")
		cell.CreateAttr("N", c.name)
		cell.CreateAttr("V", c.value)
	}

	// Create Page element for pages.xml
	newPageID := v.getMaxPageID() + 1
	pageElem := etree.NewElement("Page")
	pageElem.CreateAttr("ID", strconv.Itoa(newPageID))
	pageElem.CreateAttr("NameU", name)
	pageElem.CreateAttr("Name", name)
	pageElem.AddChild(pagesheetElem)

	relElem := pageElem.CreateElement("Rel")
	relElem.CreateAttr("r:id", newRelID)

	// Create empty page content XML
	pageContentXML := fmt.Sprintf("<?xml version='1.0' encoding='utf-8' ?><PageContents xmlns='%s' xmlns:r='%s' xml:space='preserve'/>", MainNS, RelNS)

	return v.createPage(pageContentXML, name, pageElem, index, nil)
}

// CopyPage copies an existing page and inserts at the given index (or PagePosition).
func (v *VisioFile) CopyPage(page *Page, index int, name string) (*Page, error) {
	if name == "" {
		name = page.Name()
	}
	name = v.getNewPageName(name)

	newPageFilename := fmt.Sprintf("page%d.xml", len(v.Pages)+1)
	newRelID := v.updatePagesXMLRels(newPageFilename)

	// Copy the source page element from pages.xml
	var sourcePageElem *etree.Element
	for _, pe := range v.pagesXML.Root().SelectElements("Page") {
		if pe.SelectAttrValue("Name", "") == page.Name() {
			sourcePageElem = pe
			break
		}
	}
	if sourcePageElem == nil {
		return nil, fmt.Errorf("source page %q not found in pages.xml", page.Name())
	}

	// Deep copy the page element
	newPageElem := sourcePageElem.Copy()
	newPageElem.CreateAttr("ID", strconv.Itoa(v.getMaxPageID()+1))
	newPageElem.CreateAttr("NameU", name)
	newPageElem.CreateAttr("Name", name)

	// Update Rel element with new relID
	relElem := newPageElem.SelectElement("Rel")
	if relElem != nil {
		relElem.CreateAttr("r:id", newRelID)
	}

	// Serialize source page content XML
	pageContentBytes, err := page.xml.WriteToBytes()
	if err != nil {
		return nil, fmt.Errorf("serializing page %q content: %w", page.Name(), err)
	}
	pageContentXML := string(pageContentBytes)

	newPage, err := v.createPage(pageContentXML, name, newPageElem, index, page)
	if err != nil {
		return nil, err
	}

	// Copy page rels if they exist
	origFilename := filepath.Base(page.filename)
	pageRelsPath := "visio/pages/_rels/" + origFilename + ".rels"
	if relsData, ok := v.ZipFileContents[pageRelsPath]; ok {
		newPageRelsPath := "visio/pages/_rels/" + newPageFilename + ".rels"
		relsDataCopy := make([]byte, len(relsData))
		copy(relsDataCopy, relsData)
		v.ZipFileContents[newPageRelsPath] = relsDataCopy
	}

	return newPage, nil
}

// CopyShape copies a shape element into the destination page, assigning new IDs.
// Returns the new shape element.
func (v *VisioFile) CopyShape(shape *etree.Element, page *Page) *etree.Element {
	newShape := shape.Copy()
	if page.MaxID == 0 {
		page.SetMaxIDs()
	}

	// Find or create Shapes container
	shapesTag := page.xml.Root().SelectElement("Shapes")
	if shapesTag == nil {
		shapesTag = page.xml.Root().CreateElement("Shapes")
	}

	idMap := v.incrementShapeIDs(newShape, page, nil)
	v.updateIDs(newShape, idMap)
	shapesTag.AddChild(newShape)

	return newShape
}

// --- Internal page helpers ---

// getNewPageName returns a unique page name by appending -1, -2, etc. if needed.
func (v *VisioFile) getNewPageName(name string) string {
	names := v.GetPageNames()
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet[name] {
		return name
	}
	i := 1
	for {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if !nameSet[candidate] {
			return candidate
		}
		i++
	}
}

// getMaxPageID returns the maximum page ID from pages.xml.
func (v *VisioFile) getMaxPageID() int {
	maxID := 0
	for _, pageElem := range v.pagesXML.Root().SelectElements("Page") {
		id, err := strconv.Atoi(pageElem.SelectAttrValue("ID", "0"))
		if err == nil && id > maxID {
			maxID = id
		}
	}
	return maxID
}

// updatePagesXMLRels adds a Relationship for the new page and returns the new relID.
func (v *VisioFile) updatePagesXMLRels(newPageFilename string) string {
	maxRelID := 0
	for _, rel := range v.pagesXMLRels.Root().SelectElements("Relationship") {
		relID := rel.SelectAttrValue("Id", "")
		if strings.HasPrefix(relID, "rId") {
			if num, err := strconv.Atoi(relID[3:]); err == nil && num > maxRelID {
				maxRelID = num
			}
		}
	}
	newRelID := fmt.Sprintf("rId%d", maxRelID+1)

	rel := v.pagesXMLRels.Root().CreateElement("Relationship")
	rel.CreateAttr("Target", newPageFilename)
	rel.CreateAttr("Type", "http://schemas.microsoft.com/visio/2010/relationships/page")
	rel.CreateAttr("Id", newRelID)

	return newRelID
}

// resolvePageIndex converts a PagePosition or int index to an actual insert index.
func (v *VisioFile) resolvePageIndex(index int, sourcePage *Page) int {
	switch PagePosition(index) {
	case PageLast:
		return len(v.Pages)
	case PageFirst:
		return 0
	case PageAfter:
		if sourcePage != nil {
			return sourcePage.IndexNum() + 1
		}
		return len(v.Pages)
	case PageBefore:
		if sourcePage != nil {
			return sourcePage.IndexNum()
		}
		return len(v.Pages)
	default:
		if index < 0 {
			return len(v.Pages)
		}
		return index
	}
}

// updateContentTypesXML adds an Override entry for the new page.
func (v *VisioFile) updateContentTypesXML(newPageFilename string) {
	if v.contentTypesXML == nil {
		return
	}
	root := v.contentTypesXML.Root()

	override := etree.NewElement("Override")
	override.CreateAttr("PartName", "/visio/pages/"+newPageFilename)
	override.CreateAttr("ContentType", "application/vnd.ms-visio.page+xml")

	// Find last page Override element and insert after it
	var lastPageOverride *etree.Element
	for _, elem := range root.SelectElements("Override") {
		if elem.SelectAttrValue("ContentType", "") == "application/vnd.ms-visio.page+xml" {
			lastPageOverride = elem
		}
	}
	if lastPageOverride != nil {
		insertAfter(root, lastPageOverride, override)
	} else {
		root.AddChild(override)
	}
}

// addPageToAppXML adds a page entry to app.xml (HeadingPairs and TitlesOfParts).
func (v *VisioFile) addPageToAppXML(pageName string) {
	if v.appXML == nil {
		return
	}
	root := v.appXML.Root()

	// Update HeadingPairs: increment page count (first i4 element)
	headingPairs := root.FindElement("HeadingPairs")
	if headingPairs != nil {
		i4 := headingPairs.FindElement(".//i4")
		if i4 != nil {
			count, _ := strconv.Atoi(i4.Text())
			i4.SetText(strconv.Itoa(count + 1))
		}
	}

	// Update TitlesOfParts: add new lpstr element
	titlesOfParts := root.FindElement("TitlesOfParts")
	if titlesOfParts != nil {
		vector := titlesOfParts.FindElement(".//vector")
		if vector != nil {
			lpstr := vector.CreateElement("lpstr")
			lpstr.SetText(pageName)
			// Increment vector size
			size, _ := strconv.Atoi(vector.SelectAttrValue("size", "0"))
			vector.CreateAttr("size", strconv.Itoa(size+1))
		}
	}
}

// removePageFromAppXML removes a page entry from app.xml.
func (v *VisioFile) removePageFromAppXML(pageName string) {
	if v.appXML == nil {
		return
	}
	root := v.appXML.Root()

	// Update HeadingPairs: decrement page count
	headingPairs := root.FindElement("HeadingPairs")
	if headingPairs != nil {
		i4 := headingPairs.FindElement(".//i4")
		if i4 != nil {
			count, _ := strconv.Atoi(i4.Text())
			if count > 0 {
				i4.SetText(strconv.Itoa(count - 1))
			}
		}
	}

	// Update TitlesOfParts: remove matching lpstr and decrement vector size
	titlesOfParts := root.FindElement("TitlesOfParts")
	if titlesOfParts != nil {
		vector := titlesOfParts.FindElement(".//vector")
		if vector != nil {
			for _, lpstr := range vector.SelectElements("lpstr") {
				if lpstr.Text() == pageName {
					vector.RemoveChild(lpstr)
					break
				}
			}
			size, _ := strconv.Atoi(vector.SelectAttrValue("size", "0"))
			if size > 0 {
				vector.CreateAttr("size", strconv.Itoa(size-1))
			}
		}
	}
}

// createPage is the internal method that creates a new page from XML content.
func (v *VisioFile) createPage(pageContentXML string, pageName string, pageElem *etree.Element, index int, sourcePage *Page) (*Page, error) {
	// Parse page content XML
	pageDoc := etree.NewDocument()
	if err := pageDoc.ReadFromString(pageContentXML); err != nil {
		return nil, fmt.Errorf("parsing page XML for %q: %w", pageName, err)
	}

	newPageFilename := fmt.Sprintf("page%d.xml", len(v.Pages)+1)
	newPagePath := "visio/pages/" + newPageFilename

	// Resolve index
	index = v.resolvePageIndex(index, sourcePage)

	// Insert page element in pages.xml at the correct position
	pageElems := v.pagesXML.Root().SelectElements("Page")
	if index >= len(pageElems) {
		v.pagesXML.Root().AddChild(pageElem)
	} else {
		// Insert before the element at index
		targetElem := pageElems[index]
		targetIndex := targetElem.Index()
		v.pagesXML.Root().InsertChildAt(targetIndex, pageElem)
	}

	// Update [Content_Types].xml
	v.updateContentTypesXML(newPageFilename)

	// Update app.xml
	v.addPageToAppXML(pageName)

	// Create Page object
	pageID := pageElem.SelectAttrValue("ID", "")
	relID := ""
	if rel := pageElem.SelectElement("Rel"); rel != nil {
		relID = rel.SelectAttrValue("id", "")
	}
	newPage := newPage(pageDoc, newPagePath, pageName, pageID, relID, v)

	// Insert into Pages slice at correct position
	if index >= len(v.Pages) {
		v.Pages = append(v.Pages, newPage)
	} else {
		v.Pages = append(v.Pages[:index+1], v.Pages[index:]...)
		v.Pages[index] = newPage
	}

	return newPage, nil
}

// --- Connector creation ---

// ConnectShapes creates a new connector shape between fromShape and toShape on the given page.
// Returns the connector Shape, or an error.
func (v *VisioFile) ConnectShapes(page *Page, fromShape, toShape *Shape) (*Shape, error) {
	// Cache media template — opening embedded ZIP per call is expensive at scale.
	if v.cachedMedia == nil {
		m, err := NewMedia()
		if err != nil {
			return nil, fmt.Errorf("loading media template: %w", err)
		}
		v.cachedMedia = m
	}
	media := v.cachedMedia

	// Copy straight connector template to destination page
	connectorTemplate := media.StraightConnector()
	if connectorTemplate == nil {
		return nil, fmt.Errorf("straight connector not found in media template")
	}

	connectorElem := v.CopyShape(connectorTemplate.XML(), page)

	// Clear the template text
	connShape := newShape(connectorElem, page, page)
	connShape.SetText("")

	// Set up master pages if needed
	v.ensureMasterPages(page, media, connShape)

	// Update BegTrigger and EndTrigger formulas to reference the from/to shapes
	if cell, ok := connShape.Cells[CellBegTrigger]; ok {
		formula := cell.Formula()
		formula = strings.Replace(formula, "Sheet.1!", "Sheet."+fromShape.ID+"!", 1)
		cell.SetFormula(formula)
	}
	if cell, ok := connShape.Cells[CellEndTrigger]; ok {
		formula := cell.Formula()
		formula = strings.Replace(formula, "Sheet.2!", "Sheet."+toShape.ID+"!", 1)
		cell.SetFormula(formula)
	}

	// Create Connect XML elements linking the connector to the shapes
	endConnectElem := etree.NewElement("Connect")
	endConnectElem.CreateAttr("FromSheet", connShape.ID)
	endConnectElem.CreateAttr("FromCell", ConnCellEndX)
	endConnectElem.CreateAttr("FromPart", PartEndX)
	endConnectElem.CreateAttr("ToSheet", toShape.ID)
	endConnectElem.CreateAttr("ToCell", ConnCellPinX)
	endConnectElem.CreateAttr("ToPart", PartWholeShape)

	begConnectElem := etree.NewElement("Connect")
	begConnectElem.CreateAttr("FromSheet", connShape.ID)
	begConnectElem.CreateAttr("FromCell", ConnCellBeginX)
	begConnectElem.CreateAttr("FromPart", PartBeginX)
	begConnectElem.CreateAttr("ToSheet", fromShape.ID)
	begConnectElem.CreateAttr("ToCell", ConnCellPinX)
	begConnectElem.CreateAttr("ToPart", PartWholeShape)

	page.AddConnect(newConnect(endConnectElem, page))
	page.AddConnect(newConnect(begConnectElem, page))

	// Position the connector between the shapes
	fromCX, fromCY := fromShape.CenterXY()
	toCX, toCY := toShape.CenterXY()
	connShape.SetStartAndFinish(fromCX, fromCY, toCX, toCY)

	return connShape, nil
}

// ensureMasterPages sets up master page references for a connector shape.
func (v *VisioFile) ensureMasterPages(page *Page, media *Media, connShape *Shape) {
	hasMasters := len(v.MasterPages) > 0

	if !hasMasters {
		// No masters folder - copy master files from media template
		mediaVis := media.VisioFile()
		for path, data := range mediaVis.ZipFileContents {
			if strings.HasPrefix(path, "visio/masters/") {
				dataCopy := make([]byte, len(data))
				copy(dataCopy, data)
				v.ZipFileContents[path] = dataCopy
			}
		}
		v.loadMasterPages()

		// Add document relationship for masters
		v.addDocumentRel("http://schemas.microsoft.com/visio/2010/relationships/masters", "masters/masters.xml")

		// Add content types overrides for masters
		v.addContentTypesOverride("/visio/masters/masters.xml", "application/vnd.ms-visio.masters+xml")
		v.addContentTypesOverride("/visio/masters/master1.xml", "application/vnd.ms-visio.master+xml")

		// Merge master rels from media template into page rels
		if relsData := media.RelsXML(); relsData != nil {
			mediaDoc := etree.NewDocument()
			_ = mediaDoc.ReadFromBytes(relsData)

			if page.RelsXML == nil {
				// No existing rels — use media template directly
				page.RelsXML = mediaDoc
			} else {
				// Merge: copy each media rel into existing page rels with new rId
				root := page.RelsXML.Root()
				for _, rel := range mediaDoc.Root().SelectElements("Relationship") {
					relType := rel.SelectAttrValue("Type", "")
					target := rel.SelectAttrValue("Target", "")

					// Skip if this exact type+target already exists
					found := false
					for _, existing := range root.SelectElements("Relationship") {
						if existing.SelectAttrValue("Type", "") == relType &&
							existing.SelectAttrValue("Target", "") == target {
							found = true
							break
						}
					}
					if found {
						continue
					}

					// Find next available rId
					maxID := 0
					for _, existing := range root.SelectElements("Relationship") {
						id := existing.SelectAttrValue("Id", "")
						if strings.HasPrefix(id, "rId") {
							if n, err := strconv.Atoi(id[3:]); err == nil && n > maxID {
								maxID = n
							}
						}
					}
					newRel := root.CreateElement("Relationship")
					newRel.CreateAttr("Id", fmt.Sprintf("rId%d", maxID+1))
					newRel.CreateAttr("Type", relType)
					newRel.CreateAttr("Target", target)
				}
			}
			page.RelsXMLFile = "visio/pages/_rels/" + filepath.Base(page.filename) + ".rels"
		}
	}

	// Update app.xml HeadingPairs and TitlesOfParts for Masters
	if v.appXML != nil {
		if v.getAppXMLValue("Masters") == "" {
			v.setAppXMLValue("Masters", "1")
		}
		if !v.titlesOfPartsContains(connShape.ShapeName) {
			v.addTitlesOfPartsItem(connShape.ShapeName)
		}
	}
}

// addDocumentRel adds a Relationship to document.xml.rels.
func (v *VisioFile) addDocumentRel(relType, target string) {
	if v.documentXMLRels == nil {
		return
	}
	root := v.documentXMLRels.Root()

	// Find max rId
	maxID := 0
	for _, rel := range root.SelectElements("Relationship") {
		relID := rel.SelectAttrValue("Id", "")
		if strings.HasPrefix(relID, "rId") {
			if num, err := strconv.Atoi(relID[3:]); err == nil && num > maxID {
				maxID = num
			}
		}
	}

	rel := root.CreateElement("Relationship")
	rel.CreateAttr("Id", fmt.Sprintf("rId%d", maxID+1))
	rel.CreateAttr("Type", relType)
	rel.CreateAttr("Target", target)
}

// addContentTypesOverride adds an Override entry to [Content_Types].xml.
func (v *VisioFile) addContentTypesOverride(partName, contentType string) {
	if v.contentTypesXML == nil {
		return
	}
	root := v.contentTypesXML.Root()

	override := etree.NewElement("Override")
	override.CreateAttr("PartName", partName)
	override.CreateAttr("ContentType", contentType)

	// Find last matching Override and insert after it
	var lastMatch *etree.Element
	for _, elem := range root.SelectElements("Override") {
		if elem.SelectAttrValue("ContentType", "") == contentType {
			lastMatch = elem
		}
	}
	if lastMatch != nil {
		insertAfter(root, lastMatch, override)
	} else {
		root.AddChild(override)
	}
}

// getAppXMLValue returns the i4 value for a HeadingPairs entry by name.
func (v *VisioFile) getAppXMLValue(name string) string {
	if v.appXML == nil {
		return ""
	}
	headingPairs := v.appXML.Root().FindElement("HeadingPairs")
	if headingPairs == nil {
		return ""
	}
	variants := headingPairs.FindElements(".//variant")
	for i := 0; i < len(variants)-1; i++ {
		lpstr := variants[i].FindElement(".//lpstr")
		if lpstr != nil && lpstr.Text() == name {
			i4 := variants[i+1].FindElement(".//i4")
			if i4 != nil {
				return i4.Text()
			}
		}
	}
	return ""
}

// setAppXMLValue sets the i4 value for a HeadingPairs entry by name (creates if not found).
func (v *VisioFile) setAppXMLValue(name, value string) {
	if v.appXML == nil {
		return
	}
	headingPairs := v.appXML.Root().FindElement("HeadingPairs")
	if headingPairs == nil {
		return
	}
	variants := headingPairs.FindElements(".//variant")
	for i := 0; i < len(variants)-1; i++ {
		lpstr := variants[i].FindElement(".//lpstr")
		if lpstr != nil && lpstr.Text() == name {
			i4 := variants[i+1].FindElement(".//i4")
			if i4 != nil {
				i4.SetText(value)
				return
			}
		}
	}

	// Not found - create new heading pair
	vector := headingPairs.FindElement(".//vector")
	if vector == nil {
		return
	}

	nameVariant := vector.CreateElement("variant")
	lpstr := nameVariant.CreateElement("lpstr")
	lpstr.SetText(name)

	valueVariant := vector.CreateElement("variant")
	i4 := valueVariant.CreateElement("i4")
	i4.SetText(value)

	// Increment vector size by 2
	size, _ := strconv.Atoi(vector.SelectAttrValue("size", "0"))
	vector.CreateAttr("size", strconv.Itoa(size+2))
}

// titlesOfPartsContains checks if a title exists in TitlesOfParts.
func (v *VisioFile) titlesOfPartsContains(title string) bool {
	if v.appXML == nil {
		return false
	}
	titlesOfParts := v.appXML.Root().FindElement("TitlesOfParts")
	if titlesOfParts == nil {
		return false
	}
	vector := titlesOfParts.FindElement(".//vector")
	if vector == nil {
		return false
	}
	for _, lpstr := range vector.SelectElements("lpstr") {
		if lpstr.Text() == title {
			return true
		}
	}
	return false
}

// addTitlesOfPartsItem adds a title to TitlesOfParts in app.xml.
func (v *VisioFile) addTitlesOfPartsItem(title string) {
	if v.appXML == nil {
		return
	}
	titlesOfParts := v.appXML.Root().FindElement("TitlesOfParts")
	if titlesOfParts == nil {
		return
	}
	vector := titlesOfParts.FindElement(".//vector")
	if vector == nil {
		return
	}
	lpstr := vector.CreateElement("lpstr")
	lpstr.SetText(title)
	size, _ := strconv.Atoi(vector.SelectAttrValue("size", "0"))
	vector.CreateAttr("size", strconv.Itoa(size+1))
}

// --- Shape ID management ---

// incrementShapeIDs recursively assigns new IDs to a shape and its children.
// Returns a map of old ID -> new ID.
func (v *VisioFile) incrementShapeIDs(shape *etree.Element, page *Page, idMap map[string]string) map[string]string {
	if idMap == nil {
		idMap = make(map[string]string)
	}
	v.setNewID(shape, page, idMap)
	for _, shapesElem := range shape.SelectElements("Shapes") {
		v.incrementShapeIDs(shapesElem, page, idMap)
	}
	for _, shapeElem := range shape.SelectElements("Shape") {
		v.setNewID(shapeElem, page, idMap)
	}
	return idMap
}

// setNewID assigns a new unique ID to an element, recording the mapping.
func (v *VisioFile) setNewID(element *etree.Element, page *Page, idMap map[string]string) {
	page.MaxID++
	newID := strconv.Itoa(page.MaxID)
	currentID := element.SelectAttrValue("ID", "")
	if currentID != "" {
		idMap[currentID] = newID
	}
	element.CreateAttr("ID", newID)
}

// updateIDs updates Sheet.N references in Cell formulas using the ID map.
func (v *VisioFile) updateIDs(shape *etree.Element, idMap map[string]string) {
	for _, shapesElem := range shape.SelectElements("Shapes") {
		v.updateIDs(shapesElem, idMap)
	}
	for _, shapeElem := range shape.SelectElements("Shape") {
		for _, cell := range shapeElem.SelectElements("Cell") {
			f := cell.SelectAttrValue("F", "")
			if strings.HasPrefix(f, "Sheet.") {
				// Extract shape ID from "Sheet.N!..."
				parts := strings.SplitN(f, "!", 2)
				sheetRef := parts[0] // "Sheet.N"
				shapeID := strings.TrimPrefix(sheetRef, "Sheet.")
				if newID, ok := idMap[shapeID]; ok {
					newF := strings.Replace(f, "Sheet."+shapeID, "Sheet."+newID, 1)
					cell.CreateAttr("F", newF)
				}
			}
		}
	}
}

// SaveVsdxBytes serializes the VisioFile to an in-memory .vsdx (ZIP) and returns the bytes.
// All modified XML documents are written back into the zip contents before building the archive.
func (v *VisioFile) SaveVsdxBytes() ([]byte, error) {
	// Update pages XML back to zip contents
	if v.pagesXML != nil {
		data, err := v.pagesXML.WriteToBytes()
		if err != nil {
			return nil, fmt.Errorf("serializing pages.xml: %w", err)
		}
		v.ZipFileContents["visio/pages/pages.xml"] = data
	}
	if v.pagesXMLRels != nil {
		data, err := v.pagesXMLRels.WriteToBytes()
		if err != nil {
			return nil, fmt.Errorf("serializing pages.xml.rels: %w", err)
		}
		v.ZipFileContents["visio/pages/_rels/pages.xml.rels"] = data
	}

	// Update each page XML and page-level rels
	for _, page := range v.Pages {
		if page.xml != nil {
			data, err := page.xml.WriteToBytes()
			if err != nil {
				return nil, fmt.Errorf("serializing %s: %w", page.filename, err)
			}
			v.ZipFileContents[page.filename] = data
		}
		if page.RelsXML != nil && page.RelsXMLFile != "" {
			data, err := page.RelsXML.WriteToBytes()
			if err != nil {
				return nil, fmt.Errorf("serializing %s: %w", page.RelsXMLFile, err)
			}
			v.ZipFileContents[page.RelsXMLFile] = data
		}
	}

	// Update other XML files
	if v.rootRelsXML != nil {
		data, err := v.rootRelsXML.WriteToBytes()
		if err != nil {
			return nil, fmt.Errorf("serializing _rels/.rels: %w", err)
		}
		v.ZipFileContents["_rels/.rels"] = data
	}
	if v.contentTypesXML != nil {
		data, err := v.contentTypesXML.WriteToBytes()
		if err != nil {
			return nil, fmt.Errorf("serializing [Content_Types].xml: %w", err)
		}
		v.ZipFileContents["[Content_Types].xml"] = data
	}
	if v.appXML != nil {
		data, err := v.appXML.WriteToBytes()
		if err != nil {
			return nil, fmt.Errorf("serializing app.xml: %w", err)
		}
		v.ZipFileContents["docProps/app.xml"] = data
	}
	if v.coreXML != nil {
		data, err := v.coreXML.WriteToBytes()
		if err != nil {
			return nil, fmt.Errorf("serializing core.xml: %w", err)
		}
		v.ZipFileContents["docProps/core.xml"] = data
	}
	if v.customXML != nil {
		data, err := v.customXML.WriteToBytes()
		if err != nil {
			return nil, fmt.Errorf("serializing custom.xml: %w", err)
		}
		v.ZipFileContents["docProps/custom.xml"] = data
	}
	if v.documentXML != nil {
		data, err := v.documentXML.WriteToBytes()
		if err != nil {
			return nil, fmt.Errorf("serializing document.xml: %w", err)
		}
		v.ZipFileContents["visio/document.xml"] = data
	}
	if v.documentXMLRels != nil {
		data, err := v.documentXMLRels.WriteToBytes()
		if err != nil {
			return nil, fmt.Errorf("serializing document.xml.rels: %w", err)
		}
		v.ZipFileContents["visio/_rels/document.xml.rels"] = data
	}

	// Build zip archive in memory
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for path, content := range v.ZipFileContents {
		fw, err := w.Create(path)
		if err != nil {
			return nil, fmt.Errorf("creating %s in zip: %w", path, err)
		}
		if _, err := fw.Write(content); err != nil {
			return nil, fmt.Errorf("writing %s to zip: %w", path, err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("closing zip: %w", err)
	}

	return buf.Bytes(), nil
}

// SaveVsdx saves the VisioFile to a new .vsdx file on disk.
func (v *VisioFile) SaveVsdx(filename string) error {
	data, err := v.SaveVsdxBytes()
	if err != nil {
		return err
	}
	return writeFile(filename, data)
}

// writeFile writes data to a file, creating directories as needed.
func writeFile(filename string, data []byte) error {
	return writeFileBytes(filename, data)
}

// --- Document Properties ---

// CoreProperties represents the core document properties (docProps/core.xml).
type CoreProperties struct {
	Title       string
	Subject     string
	Creator     string
	Keywords    string
	Description string
	LastModBy   string
	Revision    string
	Created     string
	Modified    string
}

// CoreProperties returns the core document properties.
func (v *VisioFile) CoreProperties() *CoreProperties {
	if v.coreXML == nil {
		return &CoreProperties{}
	}

	root := v.coreXML.Root()
	if root == nil {
		return &CoreProperties{}
	}

	props := &CoreProperties{}

	if elem := root.FindElement("dc:title"); elem != nil {
		props.Title = elem.Text()
	}
	if elem := root.FindElement("dc:subject"); elem != nil {
		props.Subject = elem.Text()
	}
	if elem := root.FindElement("dc:creator"); elem != nil {
		props.Creator = elem.Text()
	}
	if elem := root.FindElement("cp:keywords"); elem != nil {
		props.Keywords = elem.Text()
	}
	if elem := root.FindElement("dc:description"); elem != nil {
		props.Description = elem.Text()
	}
	if elem := root.FindElement("cp:lastModifiedBy"); elem != nil {
		props.LastModBy = elem.Text()
	}
	if elem := root.FindElement("cp:revision"); elem != nil {
		props.Revision = elem.Text()
	}
	if elem := root.FindElement("dcterms:created"); elem != nil {
		props.Created = elem.Text()
	}
	if elem := root.FindElement("dcterms:modified"); elem != nil {
		props.Modified = elem.Text()
	}

	return props
}

// SetCoreProperties sets the core document properties.
func (v *VisioFile) SetCoreProperties(props *CoreProperties) {
	if v.coreXML == nil {
		v.coreXML = etree.NewDocument()
		v.coreXML.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
		root := v.coreXML.CreateElement("cp:coreProperties")
		root.CreateAttr("xmlns:cp", CorePropNS)
		root.CreateAttr("xmlns:dc", DcNS)
		root.CreateAttr("xmlns:dcterms", DcTermsNS)
		root.CreateAttr("xmlns:dcmitype", "http://purl.org/dc/dcmitype/")
		root.CreateAttr("xmlns:xsi", "http://www.w3.org/2001/XMLSchema-instance")
	}

	root := v.coreXML.Root()

	setOrCreate := func(tag, value string) {
		if elem := root.FindElement(tag); elem != nil {
			elem.SetText(value)
		} else if value != "" {
			elem = root.CreateElement(tag)
			elem.SetText(value)
		}
	}

	setOrCreate("dc:title", props.Title)
	setOrCreate("dc:subject", props.Subject)
	setOrCreate("dc:creator", props.Creator)
	setOrCreate("cp:keywords", props.Keywords)
	setOrCreate("dc:description", props.Description)
	setOrCreate("cp:lastModifiedBy", props.LastModBy)
	setOrCreate("cp:revision", props.Revision)

	if props.Created != "" {
		if elem := root.FindElement("dcterms:created"); elem != nil {
			elem.SetText(props.Created)
		} else {
			elem = root.CreateElement("dcterms:created")
			elem.CreateAttr("xsi:type", "dcterms:W3CDTF")
			elem.SetText(props.Created)
		}
	}
	if props.Modified != "" {
		if elem := root.FindElement("dcterms:modified"); elem != nil {
			elem.SetText(props.Modified)
		} else {
			elem = root.CreateElement("dcterms:modified")
			elem.CreateAttr("xsi:type", "dcterms:W3CDTF")
			elem.SetText(props.Modified)
		}
	}
}

// CustomProperty represents a custom document property.
type CustomProperty struct {
	Name  string
	Value string
	Type  string // "lpwstr", "i4", "bool", "filetime", etc.
}

// CustomProperties returns all custom document properties.
func (v *VisioFile) CustomProperties() []CustomProperty {
	if v.customXML == nil {
		return nil
	}

	root := v.customXML.Root()
	if root == nil {
		return nil
	}

	var props []CustomProperty
	for _, prop := range root.SelectElements("property") {
		name := prop.SelectAttrValue("name", "")
		cp := CustomProperty{Name: name}

		// Find the value element (could be vt:lpwstr, vt:i4, vt:bool, etc.)
		for _, child := range prop.ChildElements() {
			cp.Type = strings.TrimPrefix(child.Tag, "vt:")
			cp.Value = child.Text()
			break
		}

		props = append(props, cp)
	}

	return props
}

// GetCustomProperty returns a custom property by name.
func (v *VisioFile) GetCustomProperty(name string) (string, bool) {
	for _, prop := range v.CustomProperties() {
		if prop.Name == name {
			return prop.Value, true
		}
	}
	return "", false
}

// SetCustomProperty sets a custom document property.
func (v *VisioFile) SetCustomProperty(name, value string) {
	if v.customXML == nil {
		v.customXML = etree.NewDocument()
		v.customXML.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
		root := v.customXML.CreateElement("Properties")
		root.CreateAttr("xmlns", "http://schemas.openxmlformats.org/officeDocument/2006/custom-properties")
		root.CreateAttr("xmlns:vt", VtNS)
	}

	root := v.customXML.Root()

	// Find existing property or create new one
	var propElem *etree.Element
	maxPID := 1
	for _, prop := range root.SelectElements("property") {
		if prop.SelectAttrValue("name", "") == name {
			propElem = prop
		}
		if pid, err := strconv.Atoi(prop.SelectAttrValue("pid", "0")); err == nil && pid > maxPID {
			maxPID = pid
		}
	}

	if propElem == nil {
		propElem = root.CreateElement("property")
		propElem.CreateAttr("fmtid", "{D5CDD505-2E9C-101B-9397-08002B2CF9AE}")
		propElem.CreateAttr("pid", strconv.Itoa(maxPID+1))
		propElem.CreateAttr("name", name)
	} else {
		// Remove existing value elements
		for _, child := range propElem.ChildElements() {
			propElem.RemoveChild(child)
		}
	}

	// Add value element
	valElem := propElem.CreateElement("vt:lpwstr")
	valElem.SetText(value)
}

// RootRelationships returns the package relationships from _rels/.rels.
func (v *VisioFile) RootRelationships() []struct {
	ID     string
	Type   string
	Target string
} {
	if v.rootRelsXML == nil {
		return nil
	}

	root := v.rootRelsXML.Root()
	if root == nil {
		return nil
	}

	var rels []struct {
		ID     string
		Type   string
		Target string
	}

	for _, rel := range root.SelectElements("Relationship") {
		rels = append(rels, struct {
			ID     string
			Type   string
			Target string
		}{
			ID:     rel.SelectAttrValue("Id", ""),
			Type:   rel.SelectAttrValue("Type", ""),
			Target: rel.SelectAttrValue("Target", ""),
		})
	}

	return rels
}
