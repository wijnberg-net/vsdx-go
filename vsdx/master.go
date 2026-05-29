package vsdx

import (
	"crypto/rand"
	"fmt"
	"strconv"

	"github.com/beevik/etree"
)

// generateUUID generates a UUID in the format {XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX}.
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	// Set version 4 (random) and variant bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("{%08X-%04X-%04X-%04X-%012X}",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// getMaxMasterID returns the highest master ID currently in use.
func (v *VisioFile) getMaxMasterID() int {
	maxID := 0
	if v.mastersXML == nil {
		return maxID
	}
	for _, master := range v.mastersXML.SelectElements("Master") {
		if idStr := master.SelectAttrValue("ID", ""); idStr != "" {
			if id, err := strconv.Atoi(idStr); err == nil && id > maxID {
				maxID = id
			}
		}
	}
	return maxID
}

// CreateMaster creates a new empty master shape and returns it as a Page.
// The master can then be populated with shapes using AddShape or geometry methods.
func (v *VisioFile) CreateMaster(name string) (*Page, error) {
	if v.mastersXML == nil {
		// Initialize masters.xml if it doesn't exist.
		if err := v.initMastersXML(); err != nil {
			return nil, fmt.Errorf("initializing masters.xml: %w", err)
		}
	}

	// Generate new master ID.
	newID := v.getMaxMasterID() + 1
	idStr := strconv.Itoa(newID)

	// Generate unique ID.
	uniqueID := generateUUID()

	// Create master element in masters.xml.
	masterElem := v.mastersXML.CreateElement("Master")
	masterElem.CreateAttr("ID", idStr)
	masterElem.CreateAttr("Name", name)
	masterElem.CreateAttr("NameU", name)
	masterElem.CreateAttr("UniqueID", uniqueID)
	masterElem.CreateAttr("IconSize", "1")
	masterElem.CreateAttr("PatternFlags", "0")
	masterElem.CreateAttr("Prompt", "")

	// Create Rel element.
	relElem := masterElem.CreateElement("Rel")
	relID := fmt.Sprintf("rId%d", newID)
	relElem.CreateAttr("r:id", relID)

	// Create PageSheet for the master.
	pageSheet := masterElem.CreateElement("PageSheet")
	pageSheet.CreateAttr("LineStyle", "0")
	pageSheet.CreateAttr("FillStyle", "0")
	pageSheet.CreateAttr("TextStyle", "0")

	// Create master content XML.
	masterDoc := etree.NewDocument()
	masterDoc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	masterContents := masterDoc.CreateElement("MasterContents")
	masterContents.CreateAttr("xmlns", MainNS)
	masterContents.CreateAttr("xmlns:r", RelNS)

	// Create empty Shapes container.
	shapesElem := masterContents.CreateElement("Shapes")

	// Create default shape in the master.
	shapeElem := shapesElem.CreateElement("Shape")
	shapeElem.CreateAttr("ID", "1")
	shapeElem.CreateAttr("Type", "Shape")
	shapeElem.CreateAttr("NameU", name)

	// Add basic cells for shape dimensions.
	addCellXML(shapeElem, "PinX", "0.5", "Width*0.5")
	addCellXML(shapeElem, "PinY", "0.5", "Height*0.5")
	addCellXML(shapeElem, "Width", "1", "")
	addCellXML(shapeElem, "Height", "1", "")
	addCellXML(shapeElem, "LocPinX", "0.5", "Width*0.5")
	addCellXML(shapeElem, "LocPinY", "0.5", "Height*0.5")

	// Store master content XML.
	masterFilename := fmt.Sprintf("visio/masters/master%d.xml", newID)
	masterBytes, err := writeXMLBytes(masterDoc)
	if err != nil {
		return nil, fmt.Errorf("serializing master content: %w", err)
	}
	v.ZipFileContents[masterFilename] = masterBytes

	// Update masters.xml in zip contents.
	v.updateMastersXMLInZip()

	// Update masters/_rels/masters.xml.rels.
	v.addMasterRel(relID, fmt.Sprintf("master%d.xml", newID))

	// Update [Content_Types].xml.
	v.addMasterContentType(fmt.Sprintf("/visio/masters/master%d.xml", newID))

	// Create Page object for the master.
	masterPage := newPage(masterDoc, masterFilename, name, idStr, relID, v)
	masterPage.MasterUniqueID = uniqueID

	v.MasterPages = append(v.MasterPages, masterPage)
	if v.masterIndex == nil {
		v.masterIndex = make(map[string]*Page)
	}
	v.masterIndex[name] = masterPage

	return masterPage, nil
}

// initMastersXML creates a new masters.xml structure.
func (v *VisioFile) initMastersXML() error {
	// Create masters.xml document.
	mastersDoc := etree.NewDocument()
	mastersDoc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	masters := mastersDoc.CreateElement("Masters")
	masters.CreateAttr("xmlns", MainNS)
	masters.CreateAttr("xmlns:r", RelNS)

	v.mastersXML = masters

	// Serialize and store.
	mastersBytes, err := writeXMLBytes(mastersDoc)
	if err != nil {
		return err
	}
	v.ZipFileContents["visio/masters/masters.xml"] = mastersBytes

	// Create masters.xml.rels.
	relsDoc := etree.NewDocument()
	relsDoc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	relationships := relsDoc.CreateElement("Relationships")
	relationships.CreateAttr("xmlns", "http://schemas.openxmlformats.org/package/2006/relationships")

	relsBytes, err := writeXMLBytes(relsDoc)
	if err != nil {
		return err
	}
	v.ZipFileContents["visio/masters/_rels/masters.xml.rels"] = relsBytes

	// Add to Content_Types.xml.
	if v.contentTypesXML != nil {
		root := v.contentTypesXML.Root()
		if root != nil {
			override := root.CreateElement("Override")
			override.CreateAttr("PartName", "/visio/masters/masters.xml")
			override.CreateAttr("ContentType", "application/vnd.ms-visio.masters+xml")
		}
	}

	return nil
}

// updateMastersXMLInZip serializes the masters.xml element to the zip contents.
func (v *VisioFile) updateMastersXMLInZip() {
	if v.mastersXML == nil {
		return
	}
	// Wrap element in document for serialization.
	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8" standalone="yes"`)
	doc.SetRoot(v.mastersXML.Copy())
	if bytes, err := writeXMLBytes(doc); err == nil {
		v.ZipFileContents["visio/masters/masters.xml"] = bytes
	}
}

// addMasterRel adds a relationship entry for a master to masters.xml.rels.
func (v *VisioFile) addMasterRel(relID, target string) {
	relsPath := "visio/masters/_rels/masters.xml.rels"
	relsData, ok := v.ZipFileContents[relsPath]
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
		v.ZipFileContents[relsPath] = bytes
	}
}

// addMasterContentType adds a content type entry for a master file.
func (v *VisioFile) addMasterContentType(partName string) {
	if v.contentTypesXML == nil {
		return
	}
	root := v.contentTypesXML.Root()
	if root == nil {
		return
	}

	override := root.CreateElement("Override")
	override.CreateAttr("PartName", partName)
	override.CreateAttr("ContentType", "application/vnd.ms-visio.master+xml")
}

// DeleteMaster removes a master shape by name.
func (v *VisioFile) DeleteMaster(name string) error {
	if v.mastersXML == nil {
		return fmt.Errorf("no masters in this file")
	}

	// Find the master element.
	var masterElem *etree.Element
	var masterID string
	for _, master := range v.mastersXML.SelectElements("Master") {
		if master.SelectAttrValue("Name", "") == name || master.SelectAttrValue("NameU", "") == name {
			masterElem = master
			masterID = master.SelectAttrValue("ID", "")
			break
		}
	}

	if masterElem == nil {
		return fmt.Errorf("master %q not found", name)
	}

	// Remove from masters.xml.
	v.mastersXML.RemoveChild(masterElem)
	v.updateMastersXMLInZip()

	// Remove master content file.
	masterFilename := fmt.Sprintf("visio/masters/master%s.xml", masterID)
	delete(v.ZipFileContents, masterFilename)

	// Remove from MasterPages slice.
	for i, mp := range v.MasterPages {
		if mp.Name() == name {
			v.MasterPages = append(v.MasterPages[:i], v.MasterPages[i+1:]...)
			break
		}
	}

	// Remove from master index.
	delete(v.masterIndex, name)

	return nil
}

// DuplicateMaster creates a copy of an existing master with a new name.
func (v *VisioFile) DuplicateMaster(sourceName, newName string) (*Page, error) {
	source := v.GetMasterPage(sourceName)
	if source == nil {
		return nil, fmt.Errorf("master %q not found", sourceName)
	}

	// Create new master.
	newMaster, err := v.CreateMaster(newName)
	if err != nil {
		return nil, err
	}

	// Copy content from source.
	sourceBytes, err := writeXMLBytes(source.xml)
	if err != nil {
		return nil, fmt.Errorf("reading source master: %w", err)
	}

	newDoc := etree.NewDocument()
	if err := newDoc.ReadFromBytes(sourceBytes); err != nil {
		return nil, fmt.Errorf("parsing source master: %w", err)
	}

	newMaster.xml = newDoc

	// Update in zip contents.
	if bytes, err := writeXMLBytes(newDoc); err == nil {
		v.ZipFileContents[newMaster.filename] = bytes
	}

	return newMaster, nil
}

// GetMasterPage returns the master page with the given name, or nil if not found.
func (v *VisioFile) GetMasterPage(name string) *Page {
	// Check index first.
	if v.masterIndex != nil {
		if mp, ok := v.masterIndex[name]; ok {
			return mp
		}
	}
	// Fall back to linear search.
	for _, mp := range v.MasterPages {
		if mp.Name() == name {
			return mp
		}
	}
	return nil
}
