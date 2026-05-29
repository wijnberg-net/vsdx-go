package vsdx

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
)

// buildStencilWithMaster builds an in-memory bundle with a single master
// holding the given cells and (optionally) an embedded image. Returns the
// receiver VisioFile plus the master *Page that callers can pass into
// ImportMaster. The bundle uses the bundled blank.vsdx as its scaffold so
// the resulting VisioFile is fully formed.
func buildStencilWithMaster(t *testing.T, name string, configure func(m *Page, v *VisioFile)) *Page {
	t.Helper()
	vis, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening blank: %v", err)
	}
	t.Cleanup(func() { _ = vis.Close() })
	master, err := vis.CreateMaster(name)
	if err != nil {
		t.Fatalf("CreateMaster(%q): %v", name, err)
	}
	if configure != nil {
		configure(master, vis)
	}
	return master
}

// pngBytes returns a minimal 1×1 PNG so tests can exercise media copy
// without depending on external fixtures.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, image.White)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// --- Test 1: simple master, basic import ---

func TestImportMaster_SimpleRect(t *testing.T) {
	stencil := buildStencilWithMaster(t, "RouterIcon", func(m *Page, v *VisioFile) {
		// Master comes pre-populated with one Shape (ID=1). Touch a couple
		// of cells so the master is non-trivial.
		shape := m.ChildShapes()[0]
		shape.SetFillColor("#3366cc")
		shape.SetLineColor("#222222")
	})

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	beforeCount := len(receiver.MasterPages)
	newID, err := receiver.ImportMaster(stencil)
	if err != nil {
		t.Fatalf("ImportMaster: %v", err)
	}
	if newID == "" {
		t.Fatal("ImportMaster returned empty ID")
	}
	if got := len(receiver.MasterPages); got != beforeCount+1 {
		t.Errorf("MasterPages count = %d, want %d", got, beforeCount+1)
	}
	imported := receiver.GetMasterPageByID(newID)
	if imported == nil {
		t.Fatalf("imported master ID %q not found via GetMasterPageByID", newID)
	}
	if imported.Name() != "RouterIcon" {
		t.Errorf("imported.Name() = %q, want %q", imported.Name(), "RouterIcon")
	}
	if imported.MasterUniqueID == "" {
		t.Error("MasterUniqueID not preserved")
	}
	// The master's shape should carry the colour we set.
	childs := imported.ChildShapes()
	if len(childs) == 0 {
		t.Fatal("imported master has no shapes")
	}
	if got := childs[0].FillColor(); got != "#3366cc" {
		t.Errorf("imported shape FillColor = %q, want %q", got, "#3366cc")
	}
}

// --- Test 2: UniqueID dedup ---

func TestImportMaster_DedupOnUniqueID(t *testing.T) {
	stencil := buildStencilWithMaster(t, "Cisco-2811", nil)

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	first, err := receiver.ImportMaster(stencil)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	beforeSecond := len(receiver.MasterPages)
	second, err := receiver.ImportMaster(stencil)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if first != second {
		t.Errorf("re-import returned %q, want %q (dedup)", second, first)
	}
	if got := len(receiver.MasterPages); got != beforeSecond {
		t.Errorf("MasterPages count after re-import = %d, want %d (no duplicate)", got, beforeSecond)
	}
}

// --- Test 3: AddShapeFromExternalMaster end-to-end ---

func TestAddShapeFromExternalMaster(t *testing.T) {
	stencil := buildStencilWithMaster(t, "DeviceIcon", func(m *Page, v *VisioFile) {
		shape := m.ChildShapes()[0]
		shape.SetCellValue("Width", "1.5")
		shape.SetCellValue("Height", "0.75")
	})

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	page := receiver.GetPage(0)
	if page == nil {
		t.Fatal("receiver has no first page")
	}
	beforeShapes := len(page.ChildShapes())

	instance, err := page.AddShapeFromExternalMaster(stencil, 3.0, 5.0)
	if err != nil {
		t.Fatalf("AddShapeFromExternalMaster: %v", err)
	}
	if instance.MasterPageID == "" {
		t.Error("instance.MasterPageID not set")
	}
	if got := len(page.ChildShapes()); got != beforeShapes+1 {
		t.Errorf("page shape count = %d, want %d", got, beforeShapes+1)
	}
	if instance.X() != 3.0 || instance.Y() != 5.0 {
		t.Errorf("instance position = (%v, %v), want (3, 5)", instance.X(), instance.Y())
	}
	// Master natural size should propagate.
	if got := instance.Width(); got != 1.5 {
		t.Errorf("instance.Width() = %v, want 1.5", got)
	}
	if got := instance.Height(); got != 0.75 {
		t.Errorf("instance.Height() = %v, want 0.75", got)
	}
}

// --- Test 4: foreign data (image) master copies media + rewrites rel ---

func TestImportMaster_ForeignDataMedia(t *testing.T) {
	pngData := []byte(nil)
	var srcImageName string

	stencil := buildStencilWithMaster(t, "ImageIcon", func(m *Page, v *VisioFile) {
		// Drop a master-level rels file with an image reference + the
		// matching media bytes so ImportMaster has something to copy.
		raw := pngBytes(t)
		pngData = raw
		// Locate the master XML's directory and craft a manual rels entry.
		v.ZipFileContents["visio/media/image42.png"] = raw
		srcImageName = "image42.png"
		// Create the per-master rels file by hand.
		relsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image42.png"/>` +
			`</Relationships>`
		relsPath := "visio/masters/_rels/" + masterBaseName(m.filename) + ".rels"
		v.ZipFileContents[relsPath] = []byte(relsXML)
		// Update Page so ImportMaster sees the rels.
		m.RelsXMLFile = relsPath
		if doc, err := v.fileToXML(relsPath); err == nil {
			m.RelsXML = doc
		}
	})

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	// Pre-populate receiver with image42.png so we can verify rename
	// happens on collision.
	receiver.ZipFileContents["visio/media/image42.png"] = []byte("not the same image")

	newID, err := receiver.ImportMaster(stencil)
	if err != nil {
		t.Fatalf("ImportMaster: %v", err)
	}

	// 1. New media file should exist and equal the source bytes.
	imported := receiver.GetMasterPageByID(newID)
	if imported == nil || imported.RelsXML == nil {
		t.Fatal("imported master has no rels")
	}
	relTarget := ""
	for _, rel := range imported.RelsXML.Root().SelectElements("Relationship") {
		if rel.SelectAttrValue("Type", "") == ImageRelType {
			relTarget = rel.SelectAttrValue("Target", "")
		}
	}
	if relTarget == "" {
		t.Fatal("imported master rels: no image relationship found")
	}
	// Rewritten target points at a renamed media file (not the colliding name).
	if strings.HasSuffix(relTarget, srcImageName) {
		t.Errorf("rel target %q was not renamed despite collision", relTarget)
	}
	mediaPath := "visio/media/" + strings.TrimPrefix(relTarget, "../media/")
	got, ok := receiver.ZipFileContents[mediaPath]
	if !ok {
		t.Fatalf("renamed media file %q not present in ZipFileContents", mediaPath)
	}
	if !bytes.Equal(got, pngData) {
		t.Error("renamed media bytes don't match source")
	}
	// 2. Content_Types should declare the png extension (was already
	//    present in blank.vsdx for many bundles, but assert idempotency).
	root := receiver.contentTypesXML.Root()
	hasPNG := false
	for _, def := range root.SelectElements("Default") {
		if strings.EqualFold(def.SelectAttrValue("Extension", ""), "png") {
			hasPNG = true
		}
	}
	if !hasPNG {
		t.Error("Content_Types missing Default Extension='png' after image import")
	}
}

// --- Test 5: theme inlining ---

func TestImportMaster_ThemeInlining(t *testing.T) {
	stencil := buildStencilWithMaster(t, "ThemedShape", func(m *Page, v *VisioFile) {
		shape := m.ChildShapes()[0]
		// Set a concrete value and a THEMEGUARD formula. After import
		// with InlineTheme=true the formula should be removed but V kept.
		shape.SetCellValue("FillForegnd", "#abcdef")
		shape.SetCellFormula("FillForegnd", "THEMEGUARD(THEMEVAL())")
	})

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	newID, err := receiver.ImportMaster(stencil)
	if err != nil {
		t.Fatalf("ImportMaster: %v", err)
	}
	imported := receiver.GetMasterPageByID(newID)
	if imported == nil {
		t.Fatal("imported master not found")
	}
	shape := imported.ChildShapes()[0]
	// Walk the XML directly to verify the F attribute is gone and V remains.
	cell := shape.xml.FindElement("Cell[@N='FillForegnd']")
	if cell == nil {
		t.Fatal("FillForegnd cell not present in imported master")
	}
	if got := cell.SelectAttrValue("F", ""); got != "" {
		t.Errorf("FillForegnd F attribute survived inlining: %q", got)
	}
	if got := cell.SelectAttrValue("V", ""); got == "" {
		t.Error("FillForegnd V attribute lost during inlining")
	}
}

// --- Test 6: theme inlining can be disabled ---

func TestImportMaster_PreserveThemeWhenOptedOut(t *testing.T) {
	stencil := buildStencilWithMaster(t, "KeepThemed", func(m *Page, v *VisioFile) {
		shape := m.ChildShapes()[0]
		shape.SetCellValue("FillForegnd", "#000000")
		shape.SetCellFormula("FillForegnd", "THEMEGUARD(THEMEVAL())")
	})

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	newID, err := receiver.ImportMasterWithOptions(stencil, ImportOptions{InlineTheme: false})
	if err != nil {
		t.Fatalf("ImportMaster: %v", err)
	}
	imported := receiver.GetMasterPageByID(newID)
	cell := imported.ChildShapes()[0].xml.FindElement("Cell[@N='FillForegnd']")
	if got := cell.SelectAttrValue("F", ""); got == "" {
		t.Error("F attribute stripped even though InlineTheme=false")
	}
}

// --- Test 7: dynamic connector reuse ---

func TestImportMaster_DynamicConnectorReuse(t *testing.T) {
	// Two stencils each carrying a "Dynamic connector" master. After
	// importing both, the receiver should have only one connector master.
	stencilA := buildStencilWithMaster(t, "Dynamic connector", nil)
	stencilB := buildStencilWithMaster(t, "Dynamic connector", nil)

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	idA, err := receiver.ImportMaster(stencilA)
	if err != nil {
		t.Fatalf("import A: %v", err)
	}
	idB, err := receiver.ImportMaster(stencilB)
	if err != nil {
		t.Fatalf("import B: %v", err)
	}
	if idA != idB {
		t.Errorf("second connector import returned %q, want same as first %q (reuse)", idB, idA)
	}
	connectorCount := 0
	for _, mp := range receiver.MasterPages {
		if isDynamicConnectorName(mp.Name()) {
			connectorCount++
		}
	}
	if connectorCount != 1 {
		t.Errorf("receiver has %d dynamic-connector masters after two imports, want 1", connectorCount)
	}
}

// --- Test 8: round-trip survives save / reload ---

func TestImportMaster_RoundTrip(t *testing.T) {
	stencil := buildStencilWithMaster(t, "PersistedMaster", func(m *Page, v *VisioFile) {
		m.ChildShapes()[0].SetFillColor("#7fbf00")
	})

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	page := receiver.GetPage(0)
	_, err = page.AddShapeFromExternalMaster(stencil, 2.5, 4.5)
	if err != nil {
		t.Fatalf("AddShapeFromExternalMaster: %v", err)
	}

	saved, err := receiver.SaveVsdxBytes()
	if err != nil {
		t.Fatalf("SaveVsdxBytes: %v", err)
	}

	reopened, err := OpenBytes(saved)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	foundMaster := false
	for _, mp := range reopened.MasterPages {
		if mp.Name() == "PersistedMaster" {
			foundMaster = true
		}
	}
	if !foundMaster {
		t.Error("imported master not present after save+reload")
	}
	reopenedPage := reopened.GetPage(0)
	if reopenedPage == nil {
		t.Fatal("reopened bundle missing first page")
	}
	foundInstance := false
	for _, shape := range reopenedPage.ChildShapes() {
		if shape.MasterPageID != "" {
			// The master ID may have been renumbered during save, so just
			// confirm the instance still references *some* master.
			if mp := reopened.GetMasterPageByID(shape.MasterPageID); mp != nil && mp.Name() == "PersistedMaster" {
				foundInstance = true
			}
		}
	}
	if !foundInstance {
		t.Error("instance lost its master binding after save+reload")
	}
}

// --- Test 9: nil source rejected cleanly ---

func TestImportMaster_NilSourceError(t *testing.T) {
	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })
	_, err = receiver.ImportMaster(nil)
	if err == nil {
		t.Error("ImportMaster(nil) returned no error")
	}
}

// --- Test 10: sub-master recursion (composite master imports its base) ---

// TestImportMaster_SubMasterRecursion covers the recurseIntoMasterRefs +
// rewriteMasterRefs cooperation. Source bundle holds two masters: a
// standalone "Base" master and a "Composite" master whose internal shape
// carries Master="<base ID>". Importing Composite alone should pull in
// Base as well and remap the nested Master attribute.
func TestImportMaster_SubMasterRecursion(t *testing.T) {
	source, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening source: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })

	base, err := source.CreateMaster("Base")
	if err != nil {
		t.Fatalf("CreateMaster Base: %v", err)
	}
	composite, err := source.CreateMaster("Composite")
	if err != nil {
		t.Fatalf("CreateMaster Composite: %v", err)
	}
	// Point Composite's interior shape at Base via the Master attribute.
	compShape := composite.ChildShapes()[0]
	compShape.xml.CreateAttr("Master", base.pageID)

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	beforeCount := len(receiver.MasterPages)
	newID, err := receiver.ImportMaster(composite)
	if err != nil {
		t.Fatalf("ImportMaster: %v", err)
	}
	// Both masters should be present (Base via recursion, Composite directly).
	if got := len(receiver.MasterPages); got != beforeCount+2 {
		t.Errorf("MasterPages count after recursion = %d, want %d", got, beforeCount+2)
	}
	// The composite's nested Master attribute must point at Base's
	// receiver-side ID. We can't assert "different from source ID" because
	// blank.vsdx starts with zero masters, so Base — the first to import
	// via recursion — gets receiver ID "1", which coincidentally equals
	// its source ID. The robust check is that the attribute resolves to
	// a receiver master with the expected name.
	compReceiver := receiver.GetMasterPageByID(newID)
	if compReceiver == nil {
		t.Fatal("imported Composite not retrievable")
	}
	nestedShape := compReceiver.ChildShapes()[0]
	nestedMaster := nestedShape.xml.SelectAttrValue("Master", "")
	if nestedMaster == "" {
		t.Fatal("nested Master attribute was dropped instead of remapped")
	}
	baseReceiver := receiver.GetMasterPageByID(nestedMaster)
	if baseReceiver == nil {
		t.Fatalf("nested Master=%q does not resolve to any receiver master", nestedMaster)
	}
	if baseReceiver.Name() != "Base" {
		t.Errorf("nested Master=%q resolves to %q, want %q", nestedMaster, baseReceiver.Name(), "Base")
	}
	// Sanity: importing into a non-empty receiver and observing remap
	// is the same as the above with a guaranteed different numeric range.
	receiverWithExistingMasters, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening secondary receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiverWithExistingMasters.Close() })
	// Pre-populate with a couple of unrelated masters so getMaxMasterID
	// returns 2 before our import. Base's receiver ID will then be "3",
	// which is unambiguously different from source ID "1".
	if _, err := receiverWithExistingMasters.CreateMaster("Filler1"); err != nil {
		t.Fatalf("CreateMaster Filler1: %v", err)
	}
	if _, err := receiverWithExistingMasters.CreateMaster("Filler2"); err != nil {
		t.Fatalf("CreateMaster Filler2: %v", err)
	}
	newID2, err := receiverWithExistingMasters.ImportMaster(composite)
	if err != nil {
		t.Fatalf("secondary ImportMaster: %v", err)
	}
	comp2 := receiverWithExistingMasters.GetMasterPageByID(newID2)
	if comp2 == nil {
		t.Fatal("secondary import: composite not retrievable")
	}
	nested2 := comp2.ChildShapes()[0].xml.SelectAttrValue("Master", "")
	if nested2 == base.pageID {
		t.Errorf("nested Master=%q still equals source ID (no remap happened)", nested2)
	}
}

// --- Test 11: deep master chain (3 levels) imports without stack issues ---

// TestImportMaster_DeepChain exercises the recursive import path for a
// chain C → B → A (each Master attribute references the next). All three
// masters end up in the receiver with their Master attributes remapped
// onto receiver-side IDs.
//
// A "true" cycle test (A → B → A mutually) is intentionally NOT added:
// vsdx-go's existing Shape construction (newShape → MasterShape →
// ChildShapes → newShape) recurses through Master attributes for
// geometry inheritance and stack-overflows on mutual references, which
// is a pre-existing limitation independent of the import path. importMaster
// itself short-circuits via masterIDRemap (the receiver-side ID of an
// already-being-imported master is returned on re-entry), so mutual
// imports would succeed if the source bundle were itself constructible.
func TestImportMaster_DeepChain(t *testing.T) {
	source, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening source: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })

	a, err := source.CreateMaster("DeepA")
	if err != nil {
		t.Fatalf("CreateMaster A: %v", err)
	}
	b, err := source.CreateMaster("DeepB")
	if err != nil {
		t.Fatalf("CreateMaster B: %v", err)
	}
	c, err := source.CreateMaster("DeepC")
	if err != nil {
		t.Fatalf("CreateMaster C: %v", err)
	}
	// C → B → A (no back-edges, no cycle).
	c.ChildShapes()[0].xml.CreateAttr("Master", b.pageID)
	b.ChildShapes()[0].xml.CreateAttr("Master", a.pageID)

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	beforeCount := len(receiver.MasterPages)
	if _, err := receiver.ImportMaster(c); err != nil {
		t.Fatalf("ImportMaster: %v", err)
	}
	if got := len(receiver.MasterPages); got != beforeCount+3 {
		t.Errorf("MasterPages = %d after deep import, want %d", got, beforeCount+3)
	}
	// Verify all three names land in the receiver.
	for _, want := range []string{"DeepA", "DeepB", "DeepC"} {
		if receiver.GetMasterPage(want) == nil {
			t.Errorf("master %q not imported", want)
		}
	}
}

// --- Test 12: Connection / ConnectionABCD sections survive import ---

// TestImportMaster_ConnectionsPreserved verifies that connection points,
// both T='Connection' and T='ConnectionABCD' rows, survive the deep-copy
// (and aren't accidentally touched by inlineThemeCells which walks every
// cell). Per MS-VSDX a Connection row carries X, Y, DirX, DirY, Type,
// AutoGen, Prompt; an ABCD row carries X, Y, A, B, C, D.
func TestImportMaster_ConnectionsPreserved(t *testing.T) {
	stencil := buildStencilWithMaster(t, "WithConnections", func(m *Page, v *VisioFile) {
		shape := m.ChildShapes()[0]
		shape.AddConnectionPoint(0.5, 0.0)
		shape.AddConnectionPoint(0.5, 1.0)
		shape.AddConnectionABCD(0.0, 0.5, 0, 0, 0)
	})

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	newID, err := receiver.ImportMaster(stencil)
	if err != nil {
		t.Fatalf("ImportMaster: %v", err)
	}
	imported := receiver.GetMasterPageByID(newID)
	shape := imported.ChildShapes()[0]
	section := shape.xml.FindElement("Section[@N='Connection']")
	if section == nil {
		t.Fatal("Connection section missing after import")
	}
	var connectionRows, abcdRows int
	for _, row := range section.SelectElements("Row") {
		switch row.SelectAttrValue("T", "") {
		case "Connection":
			connectionRows++
			// Sanity: every Connection row should have AutoGen + Prompt
			// emitted by AddConnectionPoint.
			if row.FindElement("Cell[@N='AutoGen']") == nil {
				t.Error("Connection row missing AutoGen cell")
			}
			if row.FindElement("Cell[@N='Prompt']") == nil {
				t.Error("Connection row missing Prompt cell")
			}
		case "ConnectionABCD":
			abcdRows++
			// ABCD rows carry X/Y/A/B/C/D — none of which should have
			// been mutated by theme inlining.
			for _, cellName := range []string{"X", "Y", "A", "B", "C", "D"} {
				if row.FindElement("Cell[@N='" + cellName + "']") == nil {
					t.Errorf("ConnectionABCD row missing %s cell", cellName)
				}
			}
		}
	}
	if connectionRows != 2 {
		t.Errorf("Connection rows = %d, want 2", connectionRows)
	}
	if abcdRows != 1 {
		t.Errorf("ConnectionABCD rows = %d, want 1", abcdRows)
	}
}

// --- Test 13: theme inlining actually performs theme resolution ---

// TestImportMaster_RealThemeResolution validates that inlineThemeCells
// doesn't just strip the F attribute and trust whatever V was already
// there — it must consult EffectiveStyle so a cell that only had a
// THEMEGUARD formula (no cached V) gets a concrete value written.
//
// We construct that case by hand: a Cell element with F='THEMEGUARD(...)'
// and NO V attribute on the source master. After import with the default
// InlineTheme=true the cell must carry a V — whatever the EffectiveStyle
// path resolved to from the source bundle's stylesheet / theme defaults.
func TestImportMaster_RealThemeResolution(t *testing.T) {
	stencil := buildStencilWithMaster(t, "BareTheme", func(m *Page, v *VisioFile) {
		shape := m.ChildShapes()[0]
		// Insert a Cell with only F, no V, so the F-strip code path
		// alone would leave V empty. The theme-resolution path must
		// fill it.
		cell := shape.xml.CreateElement("Cell")
		cell.CreateAttr("N", "LineColor")
		cell.CreateAttr("F", "THEMEGUARD(THEMEVAL())")
	})

	receiver, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening receiver: %v", err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	newID, err := receiver.ImportMaster(stencil)
	if err != nil {
		t.Fatalf("ImportMaster: %v", err)
	}
	imported := receiver.GetMasterPageByID(newID)
	cell := imported.ChildShapes()[0].xml.FindElement("Cell[@N='LineColor']")
	if cell == nil {
		t.Fatal("LineColor cell missing from imported master")
	}
	if got := cell.SelectAttrValue("F", ""); got != "" {
		t.Errorf("LineColor F attribute survived inlining: %q", got)
	}
	// V must be present (any non-empty value — the actual color depends
	// on the source bundle's stylesheet / theme defaults). What we're
	// asserting is that EffectiveStyle resolution kicked in, not just
	// the F-strip fallback.
	if got := cell.SelectAttrValue("V", ""); got == "" {
		t.Error("LineColor V is empty — theme resolution did not run")
	}
}

// masterBaseName returns the basename of a master file path (e.g.
// "visio/masters/master2.xml" → "master2.xml"). Tiny helper to avoid
// pulling filepath in the test code itself.
func masterBaseName(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx < 0 {
		return p
	}
	return p[idx+1:]
}
