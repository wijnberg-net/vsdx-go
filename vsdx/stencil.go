package vsdx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strconv"

	"github.com/beevik/etree"
)

// Stencil represents a Visio stencil file (.vssx/.vssm).
// A stencil contains only master shapes, no pages.
type Stencil struct {
	Filename        string
	Masters         []*Page
	ZipFileContents map[string][]byte

	mastersXML      *etree.Element
	contentTypesXML *etree.Document
	masterIndex     map[string]*Page
}

// OpenStencil opens a .vssx or .vssm stencil file.
func OpenStencil(filename string) (*Stencil, error) {
	// Read zip contents.
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, &FileError{Path: filename, Err: err}
	}
	defer reader.Close()

	st := &Stencil{
		Filename:        filename,
		ZipFileContents: make(map[string][]byte),
		masterIndex:     make(map[string]*Page),
	}

	// Read all files into memory.
	for _, f := range reader.File {
		rc, err := f.Open()
		if err != nil {
			return nil, &FileError{Path: f.Name, Err: err}
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, &FileError{Path: f.Name, Err: err}
		}
		st.ZipFileContents[f.Name] = data
	}

	// Parse [Content_Types].xml.
	if ctData, ok := st.ZipFileContents["[Content_Types].xml"]; ok {
		st.contentTypesXML = etree.NewDocument()
		if err := st.contentTypesXML.ReadFromBytes(ctData); err != nil {
			return nil, fmt.Errorf("parsing [Content_Types].xml: %w", err)
		}
	}

	// Parse masters.xml.
	mastersData, ok := st.ZipFileContents["visio/masters/masters.xml"]
	if !ok {
		return nil, fmt.Errorf("stencil has no masters.xml")
	}

	mastersDoc := etree.NewDocument()
	if err := mastersDoc.ReadFromBytes(mastersData); err != nil {
		return nil, fmt.Errorf("parsing masters.xml: %w", err)
	}
	st.mastersXML = mastersDoc.Root()

	// Load master pages.
	for _, masterElem := range st.mastersXML.SelectElements("Master") {
		masterID := masterElem.SelectAttrValue("ID", "")
		masterName := masterElem.SelectAttrValue("NameU", "")
		if masterName == "" {
			masterName = masterElem.SelectAttrValue("Name", "")
		}
		uniqueID := masterElem.SelectAttrValue("UniqueID", "")

		// Load master content XML.
		masterFilename := fmt.Sprintf("visio/masters/master%s.xml", masterID)
		masterData, ok := st.ZipFileContents[masterFilename]
		if !ok {
			continue
		}

		masterDoc := etree.NewDocument()
		if err := masterDoc.ReadFromBytes(masterData); err != nil {
			continue
		}

		mp := &Page{
			xml:            masterDoc,
			filename:       masterFilename,
			name:           masterName,
			pageID:         masterID,
			MasterUniqueID: uniqueID,
		}
		st.Masters = append(st.Masters, mp)
		st.masterIndex[masterName] = mp
	}

	return st, nil
}

// CreateStencil creates a new empty stencil.
func CreateStencil() *Stencil {
	st := &Stencil{
		ZipFileContents: make(map[string][]byte),
		masterIndex:     make(map[string]*Page),
	}

	// Initialize [Content_Types].xml.
	st.contentTypesXML = etree.NewDocument()
	st.contentTypesXML.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	types := st.contentTypesXML.CreateElement("Types")
	types.CreateAttr("xmlns", ContentNS)

	// Add default content types.
	addDefault := func(ext, ct string) {
		def := types.CreateElement("Default")
		def.CreateAttr("Extension", ext)
		def.CreateAttr("ContentType", ct)
	}
	addDefault("rels", "application/vnd.openxmlformats-package.relationships+xml")
	addDefault("xml", "application/xml")

	addOverride := func(part, ct string) {
		ov := types.CreateElement("Override")
		ov.CreateAttr("PartName", part)
		ov.CreateAttr("ContentType", ct)
	}
	addOverride("/visio/masters/masters.xml", "application/vnd.ms-visio.masters+xml")

	// Initialize masters.xml.
	mastersDoc := etree.NewDocument()
	mastersDoc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	masters := mastersDoc.CreateElement("Masters")
	masters.CreateAttr("xmlns", MainNS)
	masters.CreateAttr("xmlns:r", RelNS)
	st.mastersXML = masters

	// Initialize _rels/.rels.
	relsDoc := etree.NewDocument()
	relsDoc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	relationships := relsDoc.CreateElement("Relationships")
	relationships.CreateAttr("xmlns", PkgRelNS)

	rel := relationships.CreateElement("Relationship")
	rel.CreateAttr("Id", "rId1")
	rel.CreateAttr("Type", "http://schemas.microsoft.com/visio/2010/relationships/masters")
	rel.CreateAttr("Target", "visio/masters/masters.xml")

	if relsBytes, err := writeXMLBytes(relsDoc); err == nil {
		st.ZipFileContents["_rels/.rels"] = relsBytes
	}

	// Initialize masters/_rels/masters.xml.rels.
	masterRelsDoc := etree.NewDocument()
	masterRelsDoc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	masterRels := masterRelsDoc.CreateElement("Relationships")
	masterRels.CreateAttr("xmlns", PkgRelNS)

	if masterRelsBytes, err := writeXMLBytes(masterRelsDoc); err == nil {
		st.ZipFileContents["visio/masters/_rels/masters.xml.rels"] = masterRelsBytes
	}

	return st
}

// GetMaster returns the master with the given name, or nil if not found.
func (st *Stencil) GetMaster(name string) *Page {
	return st.masterIndex[name]
}

// GetMasterNames returns the names of all masters in the stencil.
func (st *Stencil) GetMasterNames() []string {
	names := make([]string, len(st.Masters))
	for i, m := range st.Masters {
		names[i] = m.Name()
	}
	return names
}

// AddMaster adds a master to the stencil.
// If a master with the same name exists, it will be replaced.
func (st *Stencil) AddMaster(master *Page) error {
	if master == nil {
		return fmt.Errorf("master cannot be nil")
	}

	// Remove existing master with same name.
	for i, m := range st.Masters {
		if m.Name() == master.Name() {
			st.Masters = append(st.Masters[:i], st.Masters[i+1:]...)
			break
		}
	}

	// Generate new master ID.
	maxID := 0
	for _, m := range st.mastersXML.SelectElements("Master") {
		if idStr := m.SelectAttrValue("ID", ""); idStr != "" {
			if id, err := strconv.Atoi(idStr); err == nil && id > maxID {
				maxID = id
			}
		}
	}
	newID := maxID + 1
	idStr := strconv.Itoa(newID)

	// Generate unique ID if not present.
	uniqueID := master.MasterUniqueID
	if uniqueID == "" {
		uniqueID = generateUUID()
	}

	// Create master element in masters.xml.
	masterElem := st.mastersXML.CreateElement("Master")
	masterElem.CreateAttr("ID", idStr)
	masterElem.CreateAttr("Name", master.Name())
	masterElem.CreateAttr("NameU", master.Name())
	masterElem.CreateAttr("UniqueID", uniqueID)

	relElem := masterElem.CreateElement("Rel")
	relID := fmt.Sprintf("rId%d", newID)
	relElem.CreateAttr("r:id", relID)

	// Store master content XML.
	masterFilename := fmt.Sprintf("visio/masters/master%d.xml", newID)
	if master.xml != nil {
		if bytes, err := writeXMLBytes(master.xml); err == nil {
			st.ZipFileContents[masterFilename] = bytes
		}
	}

	// Add relationship.
	st.addMasterRel(relID, fmt.Sprintf("master%d.xml", newID))

	// Add content type.
	if st.contentTypesXML != nil {
		root := st.contentTypesXML.Root()
		if root != nil {
			override := root.CreateElement("Override")
			override.CreateAttr("PartName", fmt.Sprintf("/visio/masters/master%d.xml", newID))
			override.CreateAttr("ContentType", "application/vnd.ms-visio.master+xml")
		}
	}

	// Create new Page for the master.
	newMaster := &Page{
		xml:            master.xml,
		filename:       masterFilename,
		name:           master.Name(),
		pageID:         idStr,
		MasterUniqueID: uniqueID,
	}

	st.Masters = append(st.Masters, newMaster)
	st.masterIndex[master.Name()] = newMaster

	return nil
}

// addMasterRel adds a relationship entry for a master to masters.xml.rels.
func (st *Stencil) addMasterRel(relID, target string) {
	relsPath := "visio/masters/_rels/masters.xml.rels"
	relsData, ok := st.ZipFileContents[relsPath]
	if !ok {
		return
	}

	relsDoc := etree.NewDocument()
	if err := relsDoc.ReadFromBytes(relsData); err != nil {
		return
	}

	root := relsDoc.Root()
	if root == nil {
		return
	}

	rel := root.CreateElement("Relationship")
	rel.CreateAttr("Id", relID)
	rel.CreateAttr("Type", "http://schemas.microsoft.com/visio/2010/relationships/master")
	rel.CreateAttr("Target", target)

	if bytes, err := writeXMLBytes(relsDoc); err == nil {
		st.ZipFileContents[relsPath] = bytes
	}
}

// SaveVssx saves the stencil to a .vssx file.
func (st *Stencil) SaveVssx(filename string) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Update masters.xml in zip contents.
	if st.mastersXML != nil {
		doc := etree.NewDocument()
		doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
		doc.SetRoot(st.mastersXML.Copy())
		if bytes, err := writeXMLBytes(doc); err == nil {
			st.ZipFileContents["visio/masters/masters.xml"] = bytes
		}
	}

	// Update [Content_Types].xml.
	if st.contentTypesXML != nil {
		if bytes, err := writeXMLBytes(st.contentTypesXML); err == nil {
			st.ZipFileContents["[Content_Types].xml"] = bytes
		}
	}

	// Write all files to zip.
	for name, data := range st.ZipFileContents {
		w, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("creating %s: %w", name, err)
		}
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("closing zip: %w", err)
	}

	return writeFile(filename, buf.Bytes())
}

// ImportToDocument imports all masters from the stencil into a VisioFile.
func (st *Stencil) ImportToDocument(vis *VisioFile) error {
	for _, master := range st.Masters {
		// Create a new master in the document.
		newMaster, err := vis.CreateMaster(master.Name())
		if err != nil {
			return fmt.Errorf("creating master %q: %w", master.Name(), err)
		}

		// Copy content from stencil master.
		if master.xml != nil {
			if bytes, err := writeXMLBytes(master.xml); err == nil {
				newDoc := etree.NewDocument()
				if err := newDoc.ReadFromBytes(bytes); err == nil {
					newMaster.xml = newDoc
					vis.ZipFileContents[newMaster.filename] = bytes
				}
			}
		}
	}
	return nil
}
