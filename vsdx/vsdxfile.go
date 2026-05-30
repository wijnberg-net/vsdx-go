package vsdx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"sort"
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

	// shapeResolveDepth guards (*Shape) construction against pathological
	// cyclic master inheritance. newShape inherits master geometry when the
	// instance has none of its own; that triggers MasterShape → ChildShapes
	// → newShape on the target master, which can recurse forever if two
	// masters reference each other via the Master attribute. The counter
	// is incremented on entry to the inheritance block and decremented on
	// exit; when it crosses shapeResolveDepthLimit we skip the inheritance
	// instead of recursing further. Not concurrent-safe by design — vsdx-go
	// itself isn't concurrent.
	shapeResolveDepth int
}

// shapeResolveDepthLimit caps cyclic master-inheritance recursion in
// newShape. A legitimate VSDX master chain rarely exceeds 5-10 levels;
// 64 is comfortably above that while still catching cycles within a
// handful of bounces.
const shapeResolveDepthLimit = 64

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

	// Remove the page's <Override> entry from [Content_Types].xml so the
	// package descriptor no longer advertises a part that doesn't exist.
	// Without this, Visio's content-type table accumulates phantom entries
	// for every removed page.
	pagePartName := "/" + page.filename
	if v.contentTypesXML != nil && v.contentTypesXML.Root() != nil {
		for _, ov := range v.contentTypesXML.Root().SelectElements("Override") {
			if ov.SelectAttrValue("PartName", "") == pagePartName {
				v.contentTypesXML.Root().RemoveChild(ov)
				break
			}
		}
	}

	// Remove the page's <Relationship> entry from pages.xml.rels so the
	// rels table mirrors the actual files in the package.
	relTarget := strings.TrimPrefix(page.filename, "visio/pages/")
	if v.pagesXMLRels != nil && v.pagesXMLRels.Root() != nil {
		for _, rel := range v.pagesXMLRels.Root().SelectElements("Relationship") {
			if rel.SelectAttrValue("Target", "") == relTarget {
				v.pagesXMLRels.Root().RemoveChild(rel)
				break
			}
		}
	}

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

	// WRITER_AUDIT.md §5: Visio writes 4 additional PageSheet defaults
	// we previously omitted (PageScale, DrawingScale, PageLockReplace,
	// PageLockDuplicate). Adding them keeps byte-diffs against a Visio
	// resave tight. PageScale / DrawingScale carry U='IN' on Visio's
	// canonical form.
	defaultCells := []struct {
		name, value, unit string
	}{
		{"PageWidth", "8.26771653543307", ""},
		{"PageHeight", "11.69291338582677", ""},
		{"ShdwOffsetX", "0.1181102362204724", ""},
		{"ShdwOffsetY", "-0.1181102362204724", ""},
		{"PageScale", "1", "IN"},
		{"DrawingScale", "1", "IN"},
		{"DrawingSizeType", "0", ""},
		{"DrawingScaleType", "0", ""},
		{"InhibitSnap", "0", ""},
		{"PageLockReplace", "0", ""},
		{"PageLockDuplicate", "0", ""},
		{"UIVisibility", "0", ""},
		{"ShdwType", "0", ""},
		{"ShdwObliqueAngle", "0", ""},
		{"ShdwScaleFactor", "1", ""},
		{"DrawingResizeType", "1", ""},
		{"PageShapeSplit", "1", ""},
	}
	for _, c := range defaultCells {
		cell := pagesheetElem.CreateElement("Cell")
		cell.CreateAttr("N", c.name)
		cell.CreateAttr("V", c.value)
		if c.unit != "" {
			cell.CreateAttr("U", c.unit)
		}
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
	pageContentBytes, err := writeXMLBytes(page.xml)
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

	// Update TitlesOfParts: add new lpstr element. Visio's canonical
	// ordering puts page titles FIRST and master titles at the END. If
	// the master title (e.g. "Dynamic connector" from ConnectShapes) was
	// added before this page (because ConnectShapes was called on an
	// earlier page), inserting at the absolute end interleaves the
	// master between pages. Instead, find the position of the last
	// page-title entry and insert immediately after it — before any
	// master titles.
	titlesOfParts := root.FindElement("TitlesOfParts")
	if titlesOfParts != nil {
		vector := titlesOfParts.FindElement(".//vector")
		if vector != nil {
			insertLpstrBeforeMasters(vector, pageName, v)
			size, _ := strconv.Atoi(vector.SelectAttrValue("size", "0"))
			vector.CreateAttr("size", strconv.Itoa(size+1))
		}
	}
}

// insertLpstrBeforeMasters inserts a new vt:lpstr entry into the
// TitlesOfParts vector at the position right after the last page title
// — i.e. before any master titles. Used by addPageToAppXML so the page
// list stays contiguous and master entries stay at the tail.
func insertLpstrBeforeMasters(vector *etree.Element, title string, v *VisioFile) {
	// Build a set of master shape names (= titles we should insert BEFORE)
	masterNames := make(map[string]bool)
	for _, m := range v.MasterPages {
		if name := m.Name(); name != "" {
			masterNames[name] = true
		}
	}

	existing := vector.SelectElements("vt:lpstr")
	insertIdx := len(existing) // default: append
	for i, e := range existing {
		if masterNames[e.Text()] {
			insertIdx = i
			break
		}
	}

	lpstr := etree.NewElement("vt:lpstr")
	lpstr.SetText(title)
	if insertIdx >= len(existing) {
		vector.AddChild(lpstr)
	} else {
		// Insert before existing[insertIdx]
		vector.InsertChildAt(existing[insertIdx].Index(), lpstr)
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

// ConnectShapes creates a new straight connector between fromShape and toShape.
// For other styles, use ConnectShapesWithStyle.
func (v *VisioFile) ConnectShapes(page *Page, fromShape, toShape *Shape) (*Shape, error) {
	return v.ConnectShapesWithStyle(page, fromShape, toShape, "straight")
}

// ConnectShapesWithStyle creates a new connector of the requested style
// ("straight" or "curved") between fromShape and toShape on the given page.
func (v *VisioFile) ConnectShapesWithStyle(page *Page, fromShape, toShape *Shape, style string) (*Shape, error) {
	// Cache media template — opening embedded ZIP per call is expensive at scale.
	if v.cachedMedia == nil {
		m, err := NewMedia()
		if err != nil {
			return nil, fmt.Errorf("loading media template: %w", err)
		}
		v.cachedMedia = m
	}
	media := v.cachedMedia

	var connectorTemplate *Shape
	switch style {
	case "curved":
		connectorTemplate = media.CurvedConnector()
	case "straight", "":
		connectorTemplate = media.StraightConnector()
	default:
		return nil, fmt.Errorf("unknown connector style %q (want \"straight\" or \"curved\")", style)
	}
	if connectorTemplate == nil {
		return nil, fmt.Errorf("connector template %q not found in media", style)
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

	// Remap the connector's Master attribute. The template carried the master
	// ID from the embedded media file, which after CopyShape almost certainly
	// collides with an unrelated master in the destination (e.g. "Rounded
	// Rectangle" at ID 2). Without this, the new connector inherits the wrong
	// geometry and renders as a rounded blob instead of a line. We look up
	// the "Dynamic connector" master in the target — it's the right master
	// for both straight and curved variants because the style differences
	// live in the connector shape's own cells, not the master geometry.
	if dynMaster := v.GetMasterPage("Dynamic connector"); dynMaster != nil {
		connShape.SetMasterPageID(dynMaster.PageID())
	} else {
		// No matching master in target. Strip the Master attribute so the
		// connector falls back to its own geometry rather than inheriting
		// some unrelated shape's geometry.
		connShape.SetMasterPageID("")
	}

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

	// Ensure this page's rels file points at the master, regardless of
	// whether the masters folder was already populated. The if-block above
	// only fires on the FIRST ConnectShapes call (when hasMasters is
	// false); subsequent calls on a different page would leave that
	// page's rels missing. Without it Visio still opens the file but
	// shows the connector with no master inheritance.
	v.ensurePageMasterRel(page, "../masters/master1.xml")

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

// ensurePageMasterRel makes sure the given page has a <Relationship>
// pointing at the named master XML, creating the page's rels document
// from scratch if necessary. Idempotent — calling it twice with the
// same target doesn't duplicate the relationship.
func (v *VisioFile) ensurePageMasterRel(page *Page, masterTarget string) {
	const relType = "http://schemas.microsoft.com/visio/2010/relationships/master"

	if page.RelsXML == nil {
		page.RelsXML = etree.NewDocument()
		page.RelsXML.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
		root := page.RelsXML.CreateElement("Relationships")
		root.CreateAttr("xmlns", "http://schemas.openxmlformats.org/package/2006/relationships")
	}

	root := page.RelsXML.Root()

	// Already present? bail.
	for _, rel := range root.SelectElements("Relationship") {
		if rel.SelectAttrValue("Type", "") == relType &&
			rel.SelectAttrValue("Target", "") == masterTarget {
			page.RelsXMLFile = "visio/pages/_rels/" + filepath.Base(page.filename) + ".rels"
			return
		}
	}

	maxID := 0
	for _, rel := range root.SelectElements("Relationship") {
		id := rel.SelectAttrValue("Id", "")
		if strings.HasPrefix(id, "rId") {
			if n, err := strconv.Atoi(id[3:]); err == nil && n > maxID {
				maxID = n
			}
		}
	}
	newRel := root.CreateElement("Relationship")
	newRel.CreateAttr("Id", fmt.Sprintf("rId%d", maxID+1))
	newRel.CreateAttr("Type", relType)
	newRel.CreateAttr("Target", masterTarget)
	page.RelsXMLFile = "visio/pages/_rels/" + filepath.Base(page.filename) + ".rels"
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

	nameVariant := vector.CreateElement("vt:variant")
	lpstr := nameVariant.CreateElement("vt:lpstr")
	lpstr.SetText(name)

	valueVariant := vector.CreateElement("vt:variant")
	i4 := valueVariant.CreateElement("vt:i4")
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
	lpstr := vector.CreateElement("vt:lpstr")
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
	// WRITER_AUDIT.md §6: refresh document color palette so every #RRGGBB
	// used by a shape appears as a ColorEntry. Visio's resave does this
	// automatically; before this hook ran our document.xml shipped a static
	// 25-entry palette regardless of which colors the shapes used.
	v.refreshDocumentColorPalette()
	v.refreshFaceNames()
	v.refreshAppXMLHLinks()
	v.refreshPageRecalcTriggers()

	// Every shape carrying Character/Paragraph sections gets a leading
	// <cp IX="0"/> / <pp IX="0"/> marker on its <Text> element. Matches
	// Visio's canonical output where the initial run binds explicitly
	// to row 0 of the formatting section. Idempotent.
	for _, page := range v.Pages {
		for _, shape := range page.AllShapes() {
			shape.normalizeTextFormatMarkers()
		}
	}
	for _, master := range v.MasterPages {
		for _, shape := range master.AllShapes() {
			shape.normalizeTextFormatMarkers()
		}
	}

	// windows.xml describes the in-Visio session's open windows
	// (viewport position, ruler/grid toggles, etc.). Visio's resave
	// strips those <Window> children — the next open re-creates them
	// from defaults. Mirror that by removing the children but
	// preserving the root <Windows> with its ClientWidth/Height.
	if data, ok := v.ZipFileContents["visio/windows.xml"]; ok {
		if stripped, err := stripWindowsChildren(data); err == nil {
			v.ZipFileContents["visio/windows.xml"] = stripped
		}
	}

	// Update pages XML back to zip contents
	if v.pagesXML != nil {
		data, err := writeXMLBytes(v.pagesXML)
		if err != nil {
			return nil, fmt.Errorf("serializing pages.xml: %w", err)
		}
		v.ZipFileContents["visio/pages/pages.xml"] = data
	}
	if v.pagesXMLRels != nil {
		data, err := writeXMLBytes(v.pagesXMLRels)
		if err != nil {
			return nil, fmt.Errorf("serializing pages.xml.rels: %w", err)
		}
		v.ZipFileContents["visio/pages/_rels/pages.xml.rels"] = data
	}

	// Update each page XML and page-level rels
	for _, page := range v.Pages {
		if page.xml != nil {
			data, err := writeXMLBytes(page.xml)
			if err != nil {
				return nil, fmt.Errorf("serializing %s: %w", page.filename, err)
			}
			v.ZipFileContents[page.filename] = data
		}
		if page.RelsXML != nil && page.RelsXMLFile != "" {
			data, err := writeXMLBytes(page.RelsXML)
			if err != nil {
				return nil, fmt.Errorf("serializing %s: %w", page.RelsXMLFile, err)
			}
			v.ZipFileContents[page.RelsXMLFile] = data
		}
	}

	// Update each master page XML and master-level rels. Without this any
	// in-memory mutation of a master shape (or its descendants) would be
	// silently dropped on save — and instances referencing the master would
	// reload the original master at next open. Mirrors the pages loop above.
	for _, master := range v.MasterPages {
		if master.xml != nil {
			data, err := writeXMLBytes(master.xml)
			if err != nil {
				return nil, fmt.Errorf("serializing %s: %w", master.filename, err)
			}
			v.ZipFileContents[master.filename] = data
		}
		if master.RelsXML != nil && master.RelsXMLFile != "" {
			data, err := writeXMLBytes(master.RelsXML)
			if err != nil {
				return nil, fmt.Errorf("serializing %s: %w", master.RelsXMLFile, err)
			}
			v.ZipFileContents[master.RelsXMLFile] = data
		}
	}

	// Update masters.xml index. updateMastersXMLInZip is also called from
	// AddMaster/DeleteMaster at mutation time, but calling it again here
	// catches direct edits to v.MastersXML() that bypassed those entry
	// points.
	v.updateMastersXMLInZip()

	// Update other XML files
	if v.rootRelsXML != nil {
		data, err := writeXMLBytes(v.rootRelsXML)
		if err != nil {
			return nil, fmt.Errorf("serializing _rels/.rels: %w", err)
		}
		v.ZipFileContents["_rels/.rels"] = data
	}
	if v.contentTypesXML != nil {
		data, err := writeXMLBytes(v.contentTypesXML)
		if err != nil {
			return nil, fmt.Errorf("serializing [Content_Types].xml: %w", err)
		}
		v.ZipFileContents["[Content_Types].xml"] = data
	}
	if v.appXML != nil {
		data, err := writeXMLBytes(v.appXML)
		if err != nil {
			return nil, fmt.Errorf("serializing app.xml: %w", err)
		}
		v.ZipFileContents["docProps/app.xml"] = data
	}
	if v.coreXML != nil {
		data, err := writeXMLBytes(v.coreXML)
		if err != nil {
			return nil, fmt.Errorf("serializing core.xml: %w", err)
		}
		v.ZipFileContents["docProps/core.xml"] = data
	}
	if v.customXML != nil {
		data, err := writeXMLBytes(v.customXML)
		if err != nil {
			return nil, fmt.Errorf("serializing custom.xml: %w", err)
		}
		v.ZipFileContents["docProps/custom.xml"] = data
	}
	if v.documentXML != nil {
		data, err := writeXMLBytes(v.documentXML)
		if err != nil {
			return nil, fmt.Errorf("serializing document.xml: %w", err)
		}
		v.ZipFileContents["visio/document.xml"] = data
	}
	if v.documentXMLRels != nil {
		data, err := writeXMLBytes(v.documentXMLRels)
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

// refreshDocumentColorPalette appends a <ColorEntry> row to the document's
// <Colors> palette for every unique #RRGGBB found in any shape's colour-
// bearing cells. Visio's canonical save does the same: each shape colour
// gets an entry so the document picker / palette UI knows about it.
//
// Existing palette entries are left untouched (we only append). The scan
// covers FillForegnd, FillBkgnd, LineColor, ShdwForegnd, ShdwBkgnd, and
// Char.Color cells across every shape on every page and master.
func (v *VisioFile) refreshDocumentColorPalette() {
	if v.documentXML == nil || v.documentXML.Root() == nil {
		return
	}
	colorsElem := v.documentXML.Root().FindElement("Colors")
	if colorsElem == nil {
		return
	}

	// Track existing palette RGBs and the max IX so we can append.
	existing := make(map[string]bool)
	maxIX := -1
	for _, entry := range colorsElem.SelectElements("ColorEntry") {
		rgb := strings.ToUpper(entry.SelectAttrValue("RGB", ""))
		if rgb != "" {
			existing[rgb] = true
		}
		if ix := entry.SelectAttrValue("IX", ""); ix != "" {
			if n, err := strconv.Atoi(ix); err == nil && n > maxIX {
				maxIX = n
			}
		}
	}

	// Walk every shape on every page + master and collect colours.
	seen := make(map[string]bool)
	addIfNew := func(c string) {
		c = strings.ToUpper(strings.TrimSpace(c))
		if len(c) == 7 && c[0] == '#' && !existing[c] && !seen[c] {
			seen[c] = true
		}
	}
	scanCells := func(shape *Shape) {
		for _, name := range []string{
			"FillForegnd", "FillBkgnd", "LineColor",
			"ShdwForegnd", "ShdwBkgnd", "Char.Color",
		} {
			if c, ok := shape.Cells[name]; ok {
				addIfNew(c.Value())
			}
		}
	}
	for _, p := range v.Pages {
		for _, s := range p.AllShapes() {
			scanCells(s)
		}
	}
	for _, mp := range v.MasterPages {
		for _, s := range mp.AllShapes() {
			scanCells(s)
		}
	}

	// Stable order: sort by hex so successive runs produce identical output.
	added := make([]string, 0, len(seen))
	for c := range seen {
		added = append(added, c)
	}
	sort.Strings(added)
	for _, c := range added {
		maxIX++
		entry := colorsElem.CreateElement("ColorEntry")
		entry.CreateAttr("IX", strconv.Itoa(maxIX))
		entry.CreateAttr("RGB", c)
	}
}

// faceNameMetrics is a hard-coded table of font metric attributes Visio
// writes into <FaceName> entries when a font appears in a shape's
// Char.Font cell. Values lifted directly from Visio's canonical resave
// output. Fonts not in this table get a minimal entry — Visio accepts
// it but the font UI's preview tooling may degrade.
var faceNameMetrics = map[string]struct {
	UnicodeRanges string
	CharSets      string
	Panose        string
	Flags         string
}{
	"Calibri":         {"-469750017 -1040178053 9 0", "536871423 0", "2 15 5 2 2 2 4 3 2 4", "357"},
	"Arial":           {"-536858881 -1073711013 9 0", "1073742335 -65536", "2 11 6 4 2 2 2 2 2 4", "325"},
	"Times New Roman": {"-536858881 -1073711013 9 0", "1073742335 -65536", "2 2 6 3 5 4 5 2 3 4", "325"},
	"Courier New":     {"-536858881 -1073711037 9 0", "1073742335 -65536", "2 7 3 9 2 2 5 2 4 4", "324"},
	"Verdana":         {"-1610612033 1073750107 16 0", "536871423 0", "2 11 6 4 3 5 4 4 2 4", "325"},
	"Tahoma":          {"-520081665 -1073717157 9 0", "1074266367 -65536", "2 11 6 4 3 5 4 4 2 4", "325"},
	"Georgia":         {"-536858881 -1073711013 9 0", "1073742335 -65536", "2 4 5 2 5 4 5 2 3 3", "325"},
}

// refreshPageRecalcTriggers ensures every <PageSheet> in pages.xml has
// a <Trigger N='RecalcColor'> with a self-referencing RefBy. Visio's
// canonical resave injects this on every page so a theme change can
// fan out to all pages for color recalculation. Position matches
// Visio's layout: between DrawingScaleType and InhibitSnap. Idempotent.
func (v *VisioFile) refreshPageRecalcTriggers() {
	if v.pagesXML == nil || v.pagesXML.Root() == nil {
		return
	}
	for _, page := range v.pagesXML.Root().SelectElements("Page") {
		pageID := page.SelectAttrValue("ID", "")
		if pageID == "" {
			continue
		}
		sheet := page.FindElement("PageSheet")
		if sheet == nil {
			continue
		}
		// Skip if a RecalcColor trigger is already present.
		alreadyHas := false
		for _, t := range sheet.SelectElements("Trigger") {
			if t.SelectAttrValue("N", "") == "RecalcColor" {
				alreadyHas = true
				break
			}
		}
		if alreadyHas {
			continue
		}
		// Insert immediately after DrawingScaleType (or fall back to
		// appending if that cell is absent).
		trigger := etree.NewElement("Trigger")
		trigger.CreateAttr("N", "RecalcColor")
		refBy := trigger.CreateElement("RefBy")
		refBy.CreateAttr("T", "Page")
		refBy.CreateAttr("ID", pageID)

		var anchor *etree.Element
		for _, c := range sheet.SelectElements("Cell") {
			if c.SelectAttrValue("N", "") == "DrawingScaleType" {
				anchor = c
				break
			}
		}
		if anchor == nil {
			sheet.AddChild(trigger)
			continue
		}
		insertAfter(sheet, anchor, trigger)
	}
}

// refreshAppXMLHLinks rebuilds the <HLinks> element in docProps/app.xml
// from every Hyperlink section across all shapes. Visio's resave emits
// an HLinks vector tracking each hyperlink's address + subaddress so
// external tools (search indexers, document property panes) can list
// them without parsing every shape. Format per ECMA-376 §15.2.12.10:
//
//	<HLinks>
//	  <vt:vector size='N*6' baseType='variant'>
//	    <!-- per hyperlink -->
//	    <vt:variant><vt:i4>0</vt:i4></vt:variant>  (col)
//	    <vt:variant><vt:i4>0</vt:i4></vt:variant>  (row)
//	    <vt:variant><vt:i4>0</vt:i4></vt:variant>  (page)
//	    <vt:variant><vt:i4>4</vt:i4></vt:variant>  (hyperlink type flag)
//	    <vt:variant><vt:lpwstr>address</vt:lpwstr></vt:variant>
//	    <vt:variant><vt:lpwstr>subaddress</vt:lpwstr></vt:variant>
//	  </vt:vector>
//	</HLinks>
//
// HLinks is removed when there are no hyperlinks. Existing HLinks
// content is replaced wholesale.
func (v *VisioFile) refreshAppXMLHLinks() {
	if v.appXML == nil || v.appXML.Root() == nil {
		return
	}
	root := v.appXML.Root()

	type hl struct{ addr, sub string }
	var links []hl
	collect := func(s *Shape) {
		for _, sec := range s.xml.SelectElements("Section") {
			if sec.SelectAttrValue("N", "") != "Hyperlink" {
				continue
			}
			for _, row := range sec.SelectElements("Row") {
				addr, sub := "", ""
				for _, c := range row.SelectElements("Cell") {
					switch c.SelectAttrValue("N", "") {
					case "Address":
						addr = c.SelectAttrValue("V", "")
					case "SubAddress":
						sub = c.SelectAttrValue("V", "")
					}
				}
				if addr != "" || sub != "" {
					links = append(links, hl{addr, sub})
				}
			}
		}
	}
	for _, p := range v.Pages {
		for _, s := range p.AllShapes() {
			collect(s)
		}
	}

	// Sort by address — Visio's canonical resave emits the vector in
	// a deterministic order; alphabetical by address matches the
	// observed Page-1-before-https resave.
	sort.SliceStable(links, func(i, j int) bool {
		return links[i].addr < links[j].addr
	})

	// Remove existing HLinks element so we can rewrite it from scratch.
	if existing := root.FindElement("HLinks"); existing != nil {
		root.RemoveChild(existing)
	}

	if len(links) == 0 {
		return
	}

	hlinks := root.CreateElement("HLinks")
	vec := hlinks.CreateElement("vt:vector")
	vec.CreateAttr("size", strconv.Itoa(len(links)*6))
	vec.CreateAttr("baseType", "variant")

	emitI4 := func(n int) {
		variant := vec.CreateElement("vt:variant")
		i4 := variant.CreateElement("vt:i4")
		i4.SetText(strconv.Itoa(n))
	}
	emitLpwstr := func(s string) {
		variant := vec.CreateElement("vt:variant")
		lps := variant.CreateElement("vt:lpwstr")
		lps.SetText(s)
	}
	for _, l := range links {
		emitI4(0)
		emitI4(0)
		emitI4(0)
		emitI4(4)
		emitLpwstr(l.addr)
		emitLpwstr(l.sub)
	}
}

// stripWindowsChildren removes all <Window> children from windows.xml's
// root <Windows> element. Visio's canonical resave does this — open
// windows are session state, not document state, and get recreated on
// next open. Preserves the root element's attributes (ClientWidth,
// ClientHeight).
func stripWindowsChildren(data []byte) ([]byte, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, err
	}
	root := doc.Root()
	if root == nil {
		return data, nil
	}
	for _, c := range root.ChildElements() {
		root.RemoveChild(c)
	}
	return writeXMLBytes(doc)
}

// refreshFaceNames appends a <FaceName> entry to document.xml's <FaceNames>
// for every unique font referenced by a shape's Char.Font cell. Mirrors
// what Visio's resave does so the document registers all in-use fonts.
// Existing FaceName entries are left untouched (only appends).
func (v *VisioFile) refreshFaceNames() {
	if v.documentXML == nil || v.documentXML.Root() == nil {
		return
	}
	root := v.documentXML.Root()
	facesElem := root.FindElement("FaceNames")
	if facesElem == nil {
		// Insert <FaceNames> just before <StyleSheets> if absent. Visio's
		// canonical order is DocumentSettings, Colors, FaceNames, StyleSheets.
		facesElem = etree.NewElement("FaceNames")
		if styleSheets := root.FindElement("StyleSheets"); styleSheets != nil {
			styleSheetsIdx := -1
			for i, child := range root.ChildElements() {
				if child == styleSheets {
					styleSheetsIdx = i
					break
				}
			}
			if styleSheetsIdx >= 0 {
				root.InsertChildAt(styleSheetsIdx, facesElem)
			} else {
				root.AddChild(facesElem)
			}
		} else {
			root.AddChild(facesElem)
		}
	}

	existing := make(map[string]bool)
	for _, fn := range facesElem.SelectElements("FaceName") {
		name := fn.SelectAttrValue("NameU", "")
		if name != "" {
			existing[name] = true
		}
	}

	// Collect all in-use fonts across pages + masters.
	seen := make(map[string]bool)
	collect := func(shape *Shape) {
		if c, ok := shape.Cells["Char.Font"]; ok {
			f := strings.TrimSpace(c.Value())
			if f != "" && !existing[f] && !seen[f] {
				seen[f] = true
			}
		}
	}
	for _, p := range v.Pages {
		for _, s := range p.AllShapes() {
			collect(s)
		}
	}
	for _, mp := range v.MasterPages {
		for _, s := range mp.AllShapes() {
			collect(s)
		}
	}

	// Stable order.
	added := make([]string, 0, len(seen))
	for f := range seen {
		added = append(added, f)
	}
	sort.Strings(added)
	for _, f := range added {
		entry := facesElem.CreateElement("FaceName")
		entry.CreateAttr("NameU", f)
		if m, ok := faceNameMetrics[f]; ok {
			entry.CreateAttr("UnicodeRanges", m.UnicodeRanges)
			entry.CreateAttr("CharSets", m.CharSets)
			entry.CreateAttr("Panose", m.Panose)
			entry.CreateAttr("Flags", m.Flags)
		}
	}
}
