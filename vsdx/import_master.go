// Package vsdx — cross-bundle master copy.
//
// ImportMaster takes a *Page from one VisioFile / Stencil and embeds it as a
// new master in the receiver bundle, preserving geometry, cells, sub-shapes,
// theme-resolved cells, embedded media, connection points, sub-master
// references, and the source's UniqueID for dedup.
//
// This is the operation Visio performs when a user drags a shape from a
// stencil pane onto a drawing: the master is *promoted* into the drawing
// bundle on first use so subsequent instances reference it locally and the
// file remains self-contained.
//
// See FEATURE_REQUEST_cross_bundle_master_copy.md for the design rationale.

package vsdx

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/beevik/etree"
)

// ErrSourceMasterNil is returned when ImportMaster is called with a nil source.
var ErrSourceMasterNil = errors.New("source master is nil")

const (
	masterRelType = "http://schemas.microsoft.com/visio/2010/relationships/master"
)

// ImportOptions controls behaviour of ImportMaster.
type ImportOptions struct {
	// InlineTheme: when true (default), THEMEGUARD / THEMEVAL formulas in
	// the source master are evaluated against the source bundle's theme and
	// replaced with concrete cell values. The imported master becomes
	// theme-independent.
	//
	// Set to false to preserve the symbolic formulas. Use this when source
	// and receiver share a theme and you want imported instances to follow
	// receiver-bundle theme changes.
	InlineTheme bool
}

// DefaultImportOptions returns the recommended import behaviour: inline
// theme references so the imported master is self-contained.
func DefaultImportOptions() ImportOptions {
	return ImportOptions{InlineTheme: true}
}

// ImportMaster copies the master shape src (from any open VisioFile or
// Stencil) into v's MasterPages and returns the master ID assigned in v.
//
// The operation is idempotent on UniqueID: if a master with src's UniqueID
// is already registered in v, no copy happens and the existing local master
// ID is returned.
//
// Side effects on the receiver:
//   - master XML is added under visio/masters/master<n>.xml
//   - master rel is appended to visio/masters/masters.xml.rels
//   - masters.xml registers the new master with a fresh local ID, NameU /
//     UniqueID copied from src
//   - per-master rels file (visio/masters/_rels/master<n>.xml.rels) is
//     created when the master has media or sub-resource references
//   - any media files referenced by the master's rels (PNG / EMF / SVG /
//     JPEG) are copied into visio/media/ with re-assigned filenames when
//     they would otherwise collide
//   - theme references are inlined to concrete cells when InlineTheme is true
//   - sub-masters referenced via MasterShape attributes are recursively
//     imported with cycle detection
//   - the dynamic-connector master is reused (never duplicated) if one
//     already exists in the receiver bundle
//   - [Content_Types].xml gets Default Extension entries for any media
//     extension the receiver doesn't yet declare
func (v *VisioFile) ImportMaster(src *Page) (string, error) {
	return v.ImportMasterWithOptions(src, DefaultImportOptions())
}

// ImportMasterWithOptions is the configurable variant of ImportMaster.
func (v *VisioFile) ImportMasterWithOptions(src *Page, opts ImportOptions) (string, error) {
	if src == nil {
		return "", ErrSourceMasterNil
	}
	srcVis := src.vis
	if srcVis == nil {
		return "", fmt.Errorf("source master has no parent VisioFile")
	}

	ctx := newImportContext(v, srcVis, opts)
	return ctx.importMaster(src)
}

// AddShapeFromExternalMaster combines ImportMaster with AddShape on p.
// Imports src into the page's owning VisioFile (idempotent), then creates
// a new shape instance bound to the resulting local master ID.
func (p *Page) AddShapeFromExternalMaster(src *Page, x, y float64) (*Shape, error) {
	if p.vis == nil {
		return nil, fmt.Errorf("page has no parent VisioFile")
	}
	localID, err := p.vis.ImportMaster(src)
	if err != nil {
		return nil, fmt.Errorf("importing master: %w", err)
	}
	shape := p.AddShape()
	shape.SetMasterPageID(localID)
	// Pull natural size from the imported master so the instance lands at
	// the same dimensions Visio would use when dragging from a stencil.
	if mp := p.vis.GetMasterPageByID(localID); mp != nil {
		if root := mp.xml.Root(); root != nil {
			if firstShape := root.FindElement(".//Shape"); firstShape != nil {
				if w := readCellFloat(firstShape, "Width"); w > 0 {
					shape.SetWidth(w)
				}
				if h := readCellFloat(firstShape, "Height"); h > 0 {
					shape.SetHeight(h)
				}
			}
		}
	}
	shape.SetX(x)
	shape.SetY(y)
	return shape, nil
}

// readCellFloat reads a numeric cell value from a Shape element by name.
// Returns 0 when the cell is absent or unparseable.
func readCellFloat(shape *etree.Element, cellName string) float64 {
	for _, c := range shape.SelectElements("Cell") {
		if c.SelectAttrValue("N", "") == cellName {
			v, _ := strconv.ParseFloat(c.SelectAttrValue("V", ""), 64)
			return v
		}
	}
	return 0
}

// importContext carries cross-bundle copy state through one ImportMaster
// invocation. Tracks the source-to-receiver master-ID remap and media
// rename map for cycle detection and rel-target rewriting.
type importContext struct {
	dst           *VisioFile
	src           *VisioFile
	opts          ImportOptions
	masterIDRemap map[string]string // src master ID → dst master ID
	mediaRemap    map[string]string // src media path → dst media path
	importingNow  map[string]bool   // src master ID → in-progress (cycle guard)
}

func newImportContext(dst, src *VisioFile, opts ImportOptions) *importContext {
	return &importContext{
		dst:           dst,
		src:           src,
		opts:          opts,
		masterIDRemap: make(map[string]string),
		mediaRemap:    make(map[string]string),
		importingNow:  make(map[string]bool),
	}
}

// importMaster is the recursive entry-point. Handles UniqueID dedup,
// dynamic-connector reuse, sub-master recursion, and orchestrates the
// XML / rels / media / content-types updates.
func (ctx *importContext) importMaster(srcMaster *Page) (string, error) {
	srcID := srcMaster.pageID

	// Same-source-bundle dedup via in-flight remap.
	if mapped, ok := ctx.masterIDRemap[srcID]; ok {
		return mapped, nil
	}

	// UniqueID dedup against receiver bundle.
	if uid := srcMaster.MasterUniqueID; uid != "" {
		for _, mp := range ctx.dst.MasterPages {
			if mp.MasterUniqueID == uid {
				ctx.masterIDRemap[srcID] = mp.pageID
				return mp.pageID, nil
			}
		}
	}

	// Dynamic-connector reuse: name-based since stencils often carry one
	// labelled "Dynamic connector" without a globally-fixed UniqueID.
	srcName := srcMaster.Name()
	if isDynamicConnectorName(srcName) {
		for _, mp := range ctx.dst.MasterPages {
			if isDynamicConnectorName(mp.Name()) {
				ctx.masterIDRemap[srcID] = mp.pageID
				return mp.pageID, nil
			}
		}
	}

	// Cycle detection — sub-master recursion should never reach the same
	// master twice in one chain.
	if ctx.importingNow[srcID] {
		return "", fmt.Errorf("cycle in master inheritance chain (master ID %s)", srcID)
	}
	ctx.importingNow[srcID] = true
	defer delete(ctx.importingNow, srcID)

	// Ensure the receiver has the masters.xml scaffolding.
	if ctx.dst.mastersXML == nil {
		if err := ctx.dst.initMastersXML(); err != nil {
			return "", fmt.Errorf("initializing masters.xml: %w", err)
		}
	}

	// Recurse into sub-masters BEFORE allocating our own ID. Allocating
	// first would race with nested allocations — each nested call uses
	// getMaxMasterID() against the receiver's mastersXML and a reservation
	// that we haven't yet registered would be invisible to that scan,
	// producing collisions where two masters both claim the same numeric
	// ID. Allocating after recursion gives each call its own monotonically
	// increasing ID.
	//
	// BaseID is the VSDX way of referencing a base master at the
	// Master-element level; recurse there first.
	if base := srcMaster.MasterBaseID; base != "" {
		if baseMaster := ctx.src.GetMasterPageByID(base); baseMaster != nil {
			if _, err := ctx.importMaster(baseMaster); err != nil {
				return "", fmt.Errorf("importing base master %s: %w", base, err)
			}
		}
	}
	// Then recurse into nested Shape Master references inside this
	// master's shape tree.
	if err := ctx.recurseIntoMasterRefs(srcMaster); err != nil {
		return "", err
	}

	// Now allocate a fresh master ID + rId in the receiver. Both are
	// monotonic against the receiver state after all nested imports have
	// landed, so no collision is possible.
	newID := ctx.dst.getMaxMasterID() + 1
	newIDStr := strconv.Itoa(newID)
	newRelID := fmt.Sprintf("rId%d", newID)
	newMasterFile := fmt.Sprintf("visio/masters/master%d.xml", newID)
	ctx.masterIDRemap[srcID] = newIDStr

	// Deep-copy the master content document so we can mutate IDs / cells
	// without touching the source.
	newDoc, err := deepCopyDoc(srcMaster.xml)
	if err != nil {
		return "", fmt.Errorf("copying master content: %w", err)
	}

	// Rewrite intra-master references: nested Shape Master attributes that
	// point at other masters imported earlier in this chain.
	ctx.rewriteMasterRefs(newDoc.Root())

	// Inline theme cells when requested. Must run on the deep-copied doc
	// (we resolve against src's theme, write into the new doc).
	if ctx.opts.InlineTheme {
		ctx.inlineThemeCells(newDoc.Root(), srcMaster)
	}

	// Copy per-master rels file and any referenced media. The rels file
	// uses image relationships; we collect every Target, copy the media
	// bytes to the receiver, renumber to avoid collisions, and rewrite the
	// rel targets.
	newRelsBytes, mediaCopied, err := ctx.copyMasterRels(srcMaster, newID)
	if err != nil {
		return "", fmt.Errorf("copying master rels: %w", err)
	}
	if newRelsBytes != nil {
		ctx.dst.ZipFileContents["visio/masters/_rels/"+fmt.Sprintf("master%d.xml.rels", newID)] = newRelsBytes
	}

	// Serialise the rewritten master content into the receiver ZIP.
	masterBytes, err := writeXMLBytes(newDoc)
	if err != nil {
		return "", fmt.Errorf("serializing imported master: %w", err)
	}
	ctx.dst.ZipFileContents[newMasterFile] = masterBytes

	// Register the master in masters.xml. Copy attributes from src
	// (NameU, IconSize, PatternFlags, Prompt, Hidden, BaseID, etc.) but
	// override ID + UniqueID handling: ID is fresh; UniqueID is preserved
	// from src so re-imports of the same source collapse to one.
	masterElem := ctx.dst.mastersXML.CreateElement("Master")
	masterElem.CreateAttr("ID", newIDStr)
	// Copy known attributes verbatim from source's Master element.
	if srcMasterElem := ctx.findSourceMasterElem(srcID); srcMasterElem != nil {
		for _, attr := range srcMasterElem.Attr {
			if attr.Key == "ID" {
				continue // already set
			}
			if attr.Key == "BaseID" {
				if remapped, ok := ctx.masterIDRemap[attr.Value]; ok {
					masterElem.CreateAttr("BaseID", remapped)
					continue
				}
			}
			masterElem.CreateAttr(attr.Key, attr.Value)
		}
		// Deep-copy non-Rel children (PageSheet, Icon, etc.) verbatim.
		for _, child := range srcMasterElem.ChildElements() {
			if child.Tag == "Rel" {
				continue
			}
			masterElem.AddChild(child.Copy())
		}
	} else {
		// Fallback when source Master element isn't accessible.
		if srcName != "" {
			masterElem.CreateAttr("NameU", srcName)
			masterElem.CreateAttr("Name", srcName)
		}
		if srcMaster.MasterUniqueID != "" {
			masterElem.CreateAttr("UniqueID", srcMaster.MasterUniqueID)
		}
		masterElem.CreateAttr("IconSize", "1")
		masterElem.CreateAttr("PatternFlags", "0")
		masterElem.CreateAttr("Prompt", "")
	}
	// Append the Rel child pointing at the new master file.
	relElem := masterElem.CreateElement("Rel")
	relElem.CreateAttr("r:id", newRelID)

	ctx.dst.updateMastersXMLInZip()

	// Add the rel + content-type entries.
	ctx.dst.addMasterRel(newRelID, fmt.Sprintf("master%d.xml", newID))
	ctx.dst.addMasterContentType("/" + newMasterFile)

	// Register any new media-extension Default Content_Types entries.
	for _, mediaPath := range mediaCopied {
		ext := strings.TrimPrefix(filepath.Ext(mediaPath), ".")
		if ct := mediaContentType(ext); ct != "" {
			ctx.dst.addContentTypeDefault(ext, ct)
		}
	}

	// Build the receiver-side Page object and register it.
	masterPage := newPage(newDoc, newMasterFile, srcName, newIDStr, newRelID, ctx.dst)
	masterPage.MasterUniqueID = srcMaster.MasterUniqueID
	if base := srcMaster.MasterBaseID; base != "" {
		if remapped, ok := ctx.masterIDRemap[base]; ok {
			masterPage.MasterBaseID = remapped
		}
	}
	if newRelsBytes != nil {
		masterPage.RelsXMLFile = "visio/masters/_rels/" + fmt.Sprintf("master%d.xml.rels", newID)
		// Re-parse so the in-memory Page sees the updated rels.
		relsDoc := etree.NewDocument()
		if err := relsDoc.ReadFromBytes(newRelsBytes); err == nil {
			masterPage.RelsXML = relsDoc
		}
	}
	ctx.dst.MasterPages = append(ctx.dst.MasterPages, masterPage)
	if ctx.dst.masterIndex == nil {
		ctx.dst.masterIndex = make(map[string]*Page)
	}
	if srcName != "" {
		ctx.dst.masterIndex[srcName] = masterPage
	}

	return newIDStr, nil
}

// recurseIntoMasterRefs walks every Shape in the source master and imports
// any masters referenced via the Master attribute that haven't been
// imported yet. Required for composite masters whose sub-shapes derive
// from other masters in the source bundle.
func (ctx *importContext) recurseIntoMasterRefs(srcMaster *Page) error {
	if srcMaster.xml == nil || srcMaster.xml.Root() == nil {
		return nil
	}
	for _, shapeElem := range srcMaster.xml.Root().FindElements(".//Shape") {
		masterRef := shapeElem.SelectAttrValue("Master", "")
		if masterRef == "" {
			continue
		}
		if _, already := ctx.masterIDRemap[masterRef]; already {
			continue
		}
		refMaster := ctx.src.GetMasterPageByID(masterRef)
		if refMaster == nil {
			// Source master not loaded; skip and let the rewriter strip
			// the attribute later. The instance will fall back to its
			// own geometry — same behaviour as a missing master in Visio.
			continue
		}
		if _, err := ctx.importMaster(refMaster); err != nil {
			return fmt.Errorf("recursive import of master %s: %w", masterRef, err)
		}
	}
	return nil
}

// rewriteMasterRefs updates the deep-copied master content's intra-bundle
// references: every nested Shape whose Master attribute points at a master
// in the source bundle gets the attribute remapped to the receiver-side ID
// (via masterIDRemap, populated earlier by recurseIntoMasterRefs). When
// the referenced master couldn't be imported (not present in the source),
// the attribute is dropped so the shape falls back to its own geometry
// rather than carrying a phantom master ID.
//
// MasterShape attributes are left untouched: their scope is shape-IDs
// inside a target master, and shape IDs survive the import unchanged.
func (ctx *importContext) rewriteMasterRefs(root *etree.Element) {
	if root == nil {
		return
	}
	for _, shapeElem := range root.FindElements(".//Shape") {
		attr := shapeElem.SelectAttr("Master")
		if attr == nil {
			continue
		}
		if remapped, ok := ctx.masterIDRemap[attr.Value]; ok {
			attr.Value = remapped
			continue
		}
		shapeElem.RemoveAttr("Master")
	}
}

// inlineThemeCells walks every cell in the master content document, and
// when its F-attribute is a THEMEGUARD or THEMEVAL formula, evaluates the
// formula against the source's theme and replaces the cell's V with the
// concrete value. The F attribute is stripped so the imported master no
// longer follows the source theme.
//
// This walks ALL shapes in the master (top-level + nested) and ALL cells
// regardless of name; the THEMEGUARD / THEMEVAL detection is the gate.
func (ctx *importContext) inlineThemeCells(root *etree.Element, srcMaster *Page) {
	if root == nil {
		return
	}
	// Build a synthetic shape per Shape element using the source page as
	// the context. We can re-use the source's existing Shape objects by
	// looking them up by ID, which preserves master inheritance during
	// theme resolution.
	srcByID := make(map[string]*Shape)
	if srcMaster != nil {
		for _, s := range srcMaster.AllShapes() {
			srcByID[s.ID] = s
		}
	}
	for _, shapeElem := range root.FindElements(".//Shape") {
		shapeID := shapeElem.SelectAttrValue("ID", "")
		srcShape, hasSrc := srcByID[shapeID]
		for _, cellElem := range shapeElem.SelectElements("Cell") {
			formula := cellElem.SelectAttrValue("F", "")
			if formula == "" {
				continue
			}
			upper := strings.ToUpper(strings.TrimSpace(formula))
			if !strings.HasPrefix(upper, "THEMEGUARD(") && !strings.HasPrefix(upper, "THEMEVAL(") {
				continue
			}
			// Use the source shape's effective-style resolution when
			// available; fall back to keeping the current V value.
			if hasSrc {
				cellName := cellElem.SelectAttrValue("N", "")
				resolved := resolveCellViaEffectiveStyle(srcShape, cellName)
				if resolved != "" {
					cellElem.CreateAttr("V", resolved)
					cellElem.RemoveAttr("F")
					continue
				}
			}
			// Fall back: strip the theme formula so the cached V wins.
			cellElem.RemoveAttr("F")
		}
	}
}

// resolveCellViaEffectiveStyle returns the value the source shape would
// render with for the given cell, walking the theme / stylesheet / master
// chain. Returns "" when the cell has no themed value.
func resolveCellViaEffectiveStyle(s *Shape, cellName string) string {
	if s == nil {
		return ""
	}
	es := s.ComputeEffectiveStyle()
	switch cellName {
	case "FillForegnd":
		return es.FillForegnd
	case "FillBkgnd":
		return es.FillBkgnd
	case "LineColor":
		return es.LineColor
	case "Char.Color", "Color":
		return es.TextColor
	case "ShdwForegnd":
		return es.ShdwForegnd
	}
	return ""
}

// findSourceMasterElem returns the <Master ID="..."/> element from the
// source bundle's masters.xml. Used to copy attributes + non-Rel children
// verbatim into the receiver's master entry.
func (ctx *importContext) findSourceMasterElem(srcID string) *etree.Element {
	if ctx.src.mastersXML == nil {
		return nil
	}
	for _, m := range ctx.src.mastersXML.SelectElements("Master") {
		if m.SelectAttrValue("ID", "") == srcID {
			return m
		}
	}
	return nil
}

// copyMasterRels walks the source master's rels file, copies referenced
// media into the receiver ZIP (renaming on collision), and returns the
// rewritten rels bytes plus the list of new media paths created.
// Returns (nil, nil, nil) when the source master has no rels file.
func (ctx *importContext) copyMasterRels(srcMaster *Page, newMasterID int) ([]byte, []string, error) {
	if srcMaster.RelsXML == nil {
		return nil, nil, nil
	}
	srcRoot := srcMaster.RelsXML.Root()
	if srcRoot == nil {
		return nil, nil, nil
	}

	// Deep-copy the rels doc.
	newRelsDoc := etree.NewDocument()
	srcBytes, err := writeXMLBytes(srcMaster.RelsXML)
	if err != nil {
		return nil, nil, err
	}
	if err := newRelsDoc.ReadFromBytes(srcBytes); err != nil {
		return nil, nil, err
	}
	newRoot := newRelsDoc.Root()

	var mediaCopied []string
	for _, rel := range newRoot.SelectElements("Relationship") {
		relType := rel.SelectAttrValue("Type", "")
		target := rel.SelectAttrValue("Target", "")
		// Image relationships point at ../media/imageN.ext relative to
		// the master XML location. We resolve, copy with rename, and
		// rewrite Target.
		if relType == ImageRelType {
			srcMediaPath := resolveMasterRelTarget(srcMaster.filename, target)
			if srcMediaPath == "" {
				continue
			}
			data, ok := ctx.src.ZipFileContents[srcMediaPath]
			if !ok {
				// Source missing — skip rather than error so partial
				// imports still produce something valid.
				continue
			}
			newMediaPath := ctx.allocateMediaPath(srcMediaPath)
			ctx.dst.ZipFileContents[newMediaPath] = data
			mediaCopied = append(mediaCopied, newMediaPath)
			// Rewrite Target to "../media/<newbase>".
			rel.CreateAttr("Target", "../media/"+filepath.Base(newMediaPath))
		}
	}
	out, err := writeXMLBytes(newRelsDoc)
	if err != nil {
		return nil, nil, err
	}
	return out, mediaCopied, nil
}

// resolveMasterRelTarget takes a master XML file path and a rel Target
// (typically "../media/imageN.png" because the rels file lives under
// visio/masters/_rels/) and returns the absolute zip path of the
// referenced media file.
func resolveMasterRelTarget(masterFile, target string) string {
	dir := filepath.Dir(masterFile)        // visio/masters
	resolved := filepath.Join(dir, target) // visio/masters/../media/imageN.png
	return filepath.ToSlash(filepath.Clean(resolved))
}

// allocateMediaPath returns a media path in the receiver bundle that
// doesn't collide with existing media. When the source basename is free
// the same name is reused; otherwise the index in the basename is bumped
// to the next free integer.
func (ctx *importContext) allocateMediaPath(srcPath string) string {
	base := filepath.Base(srcPath) // e.g. "image3.png"
	candidate := "visio/media/" + base
	if remapped, ok := ctx.mediaRemap[srcPath]; ok {
		return remapped
	}
	if _, exists := ctx.dst.ZipFileContents[candidate]; !exists {
		ctx.mediaRemap[srcPath] = candidate
		return candidate
	}
	// Collision: parse imageN.ext, bump N until free.
	ext := filepath.Ext(base)             // ".png"
	stem := strings.TrimSuffix(base, ext) // "image3"
	prefix := stem
	for i := len(stem); i > 0; i-- {
		if stem[i-1] < '0' || stem[i-1] > '9' {
			prefix = stem[:i]
			break
		}
	}
	// Find the highest existing index for this prefix+ext in the receiver.
	maxIdx := 0
	for path := range ctx.dst.ZipFileContents {
		if !strings.HasPrefix(path, "visio/media/"+prefix) || !strings.HasSuffix(path, ext) {
			continue
		}
		idxStr := strings.TrimPrefix(filepath.Base(path), prefix)
		idxStr = strings.TrimSuffix(idxStr, ext)
		if n, err := strconv.Atoi(idxStr); err == nil && n > maxIdx {
			maxIdx = n
		}
	}
	newPath := fmt.Sprintf("visio/media/%s%d%s", prefix, maxIdx+1, ext)
	ctx.mediaRemap[srcPath] = newPath
	return newPath
}

// mediaContentType returns the MIME type for a media extension as Visio
// commonly registers it in [Content_Types].xml Default entries.
func mediaContentType(ext string) string {
	switch strings.ToLower(ext) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "bmp":
		return "image/bmp"
	case "emf":
		return "image/x-emf"
	case "wmf":
		return "image/x-wmf"
	case "svg":
		return "image/svg+xml"
	case "tif", "tiff":
		return "image/tiff"
	}
	return ""
}

// isDynamicConnectorName matches Visio's dynamic-connector master under
// its common locale names. Used by ImportMaster's reuse path so multiple
// stencil imports don't accumulate duplicate connector masters.
func isDynamicConnectorName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dynamic connector", "dynamische verbindingslijn", "verbinder":
		return true
	}
	return false
}

// deepCopyDoc returns a freshly parsed copy of doc so the caller can
// mutate without affecting the source bundle. etree's Copy method on
// Element is shallow w.r.t. shared text/tail strings; round-tripping
// through bytes guarantees full separation.
func deepCopyDoc(doc *etree.Document) (*etree.Document, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil document")
	}
	b, err := writeXMLBytes(doc)
	if err != nil {
		return nil, err
	}
	out := etree.NewDocument()
	if err := out.ReadFromBytes(b); err != nil {
		return nil, err
	}
	return out, nil
}
