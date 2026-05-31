package vsdx

import (
	"os"
	"strings"
	"testing"
)

// TestMasterSave_MutationToMasterXMLPersistsThroughRoundTrip captures the
// second bug surfaced during the an earlier review: SaveVsdxBytes used to skip
// serializing the master XML files back into the zip. So any direct
// mutation of a master's XML — via vsdx-go's own master mutators, or by
// going through inst.MasterShape().SetX(...) — was thrown away on save.
//
// We mutate the master's NameU attribute (a benign, never-formula-driven
// scalar) and assert the saved zip's master*.xml contains the new value.
func TestMasterSave_MutationToMasterXMLPersistsThroughRoundTrip(t *testing.T) {
	const fixture = "testdata/architecture-sample.vsdx"
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("fixture %s not available: %v", fixture, err)
	}
	v, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}

	if len(v.MasterPages) == 0 {
		t.Skip("fixture has no master pages")
	}

	// Mutate the first master shape's NameU directly via its XML.
	target := v.MasterPages[0]
	shapeElem := target.xml.Root().FindElement(".//Shape")
	if shapeElem == nil {
		t.Fatal("no shape in first master")
	}
	const sentinel = "REGRESSION_TEST_NAMEU_MARKER_42"
	shapeElem.CreateAttr("NameU", sentinel)
	masterFilename := target.filename

	// Save and reopen.
	out, err := v.SaveVsdxBytes()
	if err != nil {
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	v.Close()

	// Verify the sentinel is in the saved zip's master XML.
	v2, err := OpenBytes(out)
	if err != nil {
		t.Fatalf("re-OpenBytes: %v", err)
	}
	defer v2.Close()

	rawMaster, ok := v2.ZipFileContents[masterFilename]
	if !ok {
		t.Fatalf("master file %s not present in saved zip", masterFilename)
	}
	if !strings.Contains(string(rawMaster), sentinel) {
		t.Errorf("master file %s did not preserve mutation: sentinel %q not found in serialized XML",
			masterFilename, sentinel)
	}
}

// TestMasterSave_MastersXMLIndexPersistsMutations checks that the
// visio/masters/masters.xml index file (the listing of all masters) also
// round-trips when mutated. Without explicit serialization the in-memory
// edits to MastersXML() would be lost.
func TestMasterSave_MastersXMLIndexPersistsMutations(t *testing.T) {
	const fixture = "testdata/architecture-sample.vsdx"
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("fixture %s not available: %v", fixture, err)
	}
	v, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}

	mx := v.MastersXML()
	if mx == nil {
		t.Skip("fixture has no masters.xml")
	}
	masterElem := mx.SelectElement("Master")
	if masterElem == nil {
		t.Skip("masters.xml has no Master elements")
	}
	const sentinel = "REGRESSION_TEST_INDEX_MARKER_42"
	masterElem.CreateAttr("NameU", sentinel)

	out, err := v.SaveVsdxBytes()
	if err != nil {
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	v.Close()

	v2, err := OpenBytes(out)
	if err != nil {
		t.Fatalf("re-OpenBytes: %v", err)
	}
	defer v2.Close()
	raw, ok := v2.ZipFileContents["visio/masters/masters.xml"]
	if !ok {
		t.Fatal("masters.xml not present in saved zip")
	}
	if !strings.Contains(string(raw), sentinel) {
		t.Errorf("masters.xml did not preserve mutation: sentinel %q not found", sentinel)
	}
}
