package vsdx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/beevik/etree"
)

// This file is the foundation of the mutation-test framework. The audit found
// that vsdx-go's 490 existing tests are overwhelmingly "set the cell, read it
// back". That misses side effects (child shapes, geometry, LocPin), it misses
// round-trip persistence, and it misses master isolation. The helpers below
// codify the six contract categories from the audit:
//
//   1. SetAndRead          — covered by existing tests; not duplicated here
//   2. SideEffects         — snapshotShape + diffShapeSnapshots
//   3. RoundTripXML        — AssertRoundTripXML
//   4. RoundTripRender     — TODO Fase 2.5 (needs golden corpus)
//   5. Composition/Idempotent/Order — AssertIdempotent, AssertOrderIndependent
//   6. Inheritance-aware   — AssertMasterIsolation, AssertLocalGeometryCreated
//
// Each helper takes *testing.T and prints actionable failure messages with
// concrete cell-by-cell diffs so the failure mode is obvious.

// loadFixtureBytes reads a VSDX fixture from disk and skips the test if the
// file is missing. Tests use this rather than directly calling os.ReadFile so
// that CI gracefully skips when a fixture is not present in the checkout.
func loadFixtureBytes(t *testing.T, path string) []byte {
	t.Helper()
	if !filepath.IsAbs(path) {
		path = filepath.Join(testDir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fixture %s not available: %v", path, err)
	}
	return data
}

// openFromBytes is a thin wrapper that fails the test on parse errors, so
// callers don't have to write `if err != nil` boilerplate every time.
func openFromBytes(t *testing.T, data []byte) *VisioFile {
	t.Helper()
	v, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	return v
}

// findShape returns the first top-level shape on page 0 that matches the
// predicate, or fatals the test if not found. Existing test code repeats
// this loop in dozens of places; centralizing it makes intent obvious.
func findShape(t *testing.T, v *VisioFile, predicate func(*Shape) bool) *Shape {
	t.Helper()
	for _, p := range v.Pages {
		for _, s := range p.AllShapes() {
			if predicate(s) {
				return s
			}
		}
	}
	t.Fatal("findShape: no matching shape found")
	return nil
}

// findShapeByName is a common convenience: matches on ShapeName (NameU).
func findShapeByName(t *testing.T, v *VisioFile, name string) *Shape {
	t.Helper()
	return findShape(t, v, func(s *Shape) bool { return s.ShapeName == name })
}

// --- Shape snapshots ---

// shapeSnapshot captures the observable state of a Shape after a mutation —
// enough to diff against a "before" snapshot and report which fields changed.
// Add fields here as new mutators are written; tests pin down their entire
// expected delta rather than spot-checking a single getter.
type shapeSnapshot struct {
	ID        string
	X, Y      float64
	W, H      float64
	LocX      float64
	LocY      float64
	Angle     float64
	FillColor string
	LineColor string
	Text      string
	// Number of geometry sections / rows. Useful to catch silent geometry
	// addition or row duplication.
	GeometryCount int
	RowCount      int
	// SHA256 of the shape's XML for crude full-state comparison. Different
	// hashes mean *something* changed; identical hashes mean nothing did.
	XMLHash string
}

func snapshotShape(s *Shape) shapeSnapshot {
	snap := shapeSnapshot{
		ID:            s.ID,
		X:             s.X(),
		Y:             s.Y(),
		W:             s.Width(),
		H:             s.Height(),
		LocX:          s.LocX(),
		LocY:          s.LocY(),
		Angle:         s.Angle(),
		FillColor:     s.FillColor(),
		LineColor:     s.LineColor(),
		Text:          s.Text(),
		GeometryCount: len(s.Geometries),
	}
	for _, g := range s.Geometries {
		snap.RowCount += len(g.Rows)
	}
	doc := etree.NewDocument()
	doc.SetRoot(s.XML().Copy())
	if b, err := doc.WriteToBytes(); err == nil {
		sum := sha256.Sum256(b)
		snap.XMLHash = hex.EncodeToString(sum[:8])
	}
	return snap
}

// diffShapeSnapshots returns the fields that differ. Used by tests to assert
// "only Width and LocX changed", catching unintended side effects.
func diffShapeSnapshots(before, after shapeSnapshot) []string {
	var diffs []string
	if before.X != after.X {
		diffs = append(diffs, fmt.Sprintf("X: %v -> %v", before.X, after.X))
	}
	if before.Y != after.Y {
		diffs = append(diffs, fmt.Sprintf("Y: %v -> %v", before.Y, after.Y))
	}
	if before.W != after.W {
		diffs = append(diffs, fmt.Sprintf("W: %v -> %v", before.W, after.W))
	}
	if before.H != after.H {
		diffs = append(diffs, fmt.Sprintf("H: %v -> %v", before.H, after.H))
	}
	if before.LocX != after.LocX {
		diffs = append(diffs, fmt.Sprintf("LocX: %v -> %v", before.LocX, after.LocX))
	}
	if before.LocY != after.LocY {
		diffs = append(diffs, fmt.Sprintf("LocY: %v -> %v", before.LocY, after.LocY))
	}
	if before.Angle != after.Angle {
		diffs = append(diffs, fmt.Sprintf("Angle: %v -> %v", before.Angle, after.Angle))
	}
	if before.FillColor != after.FillColor {
		diffs = append(diffs, fmt.Sprintf("FillColor: %v -> %v", before.FillColor, after.FillColor))
	}
	if before.LineColor != after.LineColor {
		diffs = append(diffs, fmt.Sprintf("LineColor: %v -> %v", before.LineColor, after.LineColor))
	}
	if before.Text != after.Text {
		diffs = append(diffs, fmt.Sprintf("Text: %q -> %q", before.Text, after.Text))
	}
	if before.GeometryCount != after.GeometryCount {
		diffs = append(diffs, fmt.Sprintf("GeometryCount: %v -> %v", before.GeometryCount, after.GeometryCount))
	}
	if before.RowCount != after.RowCount {
		diffs = append(diffs, fmt.Sprintf("RowCount: %v -> %v", before.RowCount, after.RowCount))
	}
	return diffs
}

// assertOnlyTheseFieldsChanged checks that the diff produced by mutating a
// shape matches the expected set of fields exactly. Both unexpected changes
// and missing expected changes are reported.
func assertOnlyTheseFieldsChanged(t *testing.T, before, after shapeSnapshot, wantChanged []string) {
	t.Helper()
	got := diffShapeSnapshots(before, after)
	gotFields := make(map[string]string, len(got))
	for _, d := range got {
		field := strings.SplitN(d, ":", 2)[0]
		gotFields[field] = d
	}
	wantSet := make(map[string]bool, len(wantChanged))
	for _, w := range wantChanged {
		wantSet[w] = true
	}
	var extra, missing []string
	for f, line := range gotFields {
		if !wantSet[f] {
			extra = append(extra, line)
		}
	}
	for w := range wantSet {
		if _, ok := gotFields[w]; !ok {
			missing = append(missing, w)
		}
	}
	sort.Strings(extra)
	sort.Strings(missing)
	if len(extra) > 0 {
		t.Errorf("unexpected side-effects: %v", extra)
	}
	if len(missing) > 0 {
		t.Errorf("expected fields did not change: %v", missing)
	}
}

// --- Contract helpers ---

// AssertRoundTripXML runs mutate on a fresh copy of the source bytes, saves,
// reopens, then runs verify on the reopened VisioFile. Catches the entire
// class of "in-memory mutation that doesn't persist to disk" bugs.
func AssertRoundTripXML(t *testing.T, src []byte, mutate func(*VisioFile), verify func(*testing.T, *VisioFile)) {
	t.Helper()
	v := openFromBytes(t, src)
	mutate(v)
	out, err := v.SaveVsdxBytes()
	if err != nil {
		v.Close()
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	v.Close()
	v2 := openFromBytes(t, out)
	defer v2.Close()
	verify(t, v2)
}

// AssertIdempotent applies mutate twice and asserts the observable state
// after the second application equals the state after the first. The observe
// function determines what "observable state" means for this mutator (e.g.,
// just shape.Width(), or a full snapshot).
func AssertIdempotent[T comparable](t *testing.T, src []byte, mutate func(*VisioFile), observe func(*VisioFile) T) {
	t.Helper()
	v1 := openFromBytes(t, src)
	defer v1.Close()
	mutate(v1)
	first := observe(v1)
	mutate(v1)
	second := observe(v1)
	if first != second {
		t.Errorf("not idempotent: 1st application=%v, 2nd application=%v", first, second)
	}
}

// AssertOrderIndependent runs each permutation of the given mutators on a
// fresh VisioFile and asserts they all produce the same observable state.
// Use for non-trivial permutations only — the work is O(n!) so 5+ mutators
// gets expensive. For two mutators we just check A;B == B;A.
func AssertOrderIndependent(t *testing.T, src []byte, mutators []func(*VisioFile), observe func(*VisioFile) any) {
	t.Helper()
	if len(mutators) < 2 {
		t.Fatal("AssertOrderIndependent needs at least 2 mutators")
	}
	apply := func(perm []int) any {
		v := openFromBytes(t, src)
		defer v.Close()
		for _, i := range perm {
			mutators[i](v)
		}
		return observe(v)
	}
	identity := make([]int, len(mutators))
	for i := range identity {
		identity[i] = i
	}
	baseline := apply(identity)
	var permute func(arr []int, k int)
	permute = func(arr []int, k int) {
		if k == len(arr)-1 {
			got := apply(arr)
			if !reflect.DeepEqual(got, baseline) {
				t.Errorf("order %v produced %v; baseline order %v produced %v",
					arr, got, identity, baseline)
			}
			return
		}
		for i := k; i < len(arr); i++ {
			arr[k], arr[i] = arr[i], arr[k]
			permute(arr, k+1)
			arr[k], arr[i] = arr[i], arr[k]
		}
	}
	permute(append([]int(nil), identity...), 0)
}

// AssertMasterIsolation runs mutate on the named instance shape and asserts
// that the mutation didn't leak into either a) sibling instances sharing the
// same master, or b) the master XML itself. The second check matters because
// shape snapshots only capture a few high-level fields; lower-level edits
// (e.g. a geometry row's X cell) can corrupt the master without changing
// siblings' Width()/Height()/etc — those still read from their own cells.
func AssertMasterIsolation(t *testing.T, src []byte, mutateInstanceName string, mutate func(*Shape)) {
	t.Helper()
	v := openFromBytes(t, src)
	defer v.Close()
	target := findShapeByName(t, v, mutateInstanceName)
	if target.MasterPageID == "" {
		t.Skipf("shape %q has no master — isolation test does not apply", mutateInstanceName)
	}

	// Snapshot every sibling instance.
	type siblingState struct {
		id   string
		snap shapeSnapshot
	}
	var siblingsBefore []siblingState
	for _, p := range v.Pages {
		for _, s := range p.AllShapes() {
			if s.MasterPageID == target.MasterPageID && s.ID != target.ID {
				siblingsBefore = append(siblingsBefore, siblingState{id: s.ID, snap: snapshotShape(s)})
			}
		}
	}

	// Snapshot the master XML. Mutating it should be considered a violation
	// even if no sibling instance exists yet.
	masterHashBefore := masterXMLHash(v, target.MasterPageID)

	mutate(target)

	masterHashAfter := masterXMLHash(v, target.MasterPageID)
	if masterHashBefore != "" && masterHashBefore != masterHashAfter {
		t.Errorf("master isolation violated: mutation on instance %s changed master %s XML (hash %s -> %s)",
			target.ID, target.MasterPageID, masterHashBefore, masterHashAfter)
	}

	if len(siblingsBefore) == 0 {
		return // no siblings to check; master hash check above is sufficient
	}
	siblingsAfter := make(map[string]shapeSnapshot, len(siblingsBefore))
	for _, p := range v.Pages {
		for _, s := range p.AllShapes() {
			if s.MasterPageID == target.MasterPageID && s.ID != target.ID {
				siblingsAfter[s.ID] = snapshotShape(s)
			}
		}
	}
	for _, b := range siblingsBefore {
		a, ok := siblingsAfter[b.id]
		if !ok {
			t.Errorf("sibling %s disappeared after mutation", b.id)
			continue
		}
		if diffs := diffShapeSnapshots(b.snap, a); len(diffs) > 0 {
			t.Errorf("master isolation violated: mutation on %s leaked into sibling %s: %v",
				target.ID, b.id, diffs)
		}
	}
}

// masterXMLHash returns a stable hash of the master page's XML, or "" when
// no master matches the given ID. Used by AssertMasterIsolation.
func masterXMLHash(v *VisioFile, masterPageID string) string {
	for _, m := range v.MasterPages {
		if m.PageID() == masterPageID && m.xml != nil {
			b, err := m.xml.WriteToBytes()
			if err != nil {
				return ""
			}
			sum := sha256.Sum256(b)
			return hex.EncodeToString(sum[:8])
		}
	}
	return ""
}

// AssertLocalGeometryCreated checks that after the given mutation, the
// instance shape owns its own <Section N="Geometry"> child in its XML.
// Without this, the mutation only edited an inherited section — meaning the
// instance has nothing distinguishing it from the master.
func AssertLocalGeometryCreated(t *testing.T, s *Shape, mutate func(*Shape)) {
	t.Helper()
	beforeSections := len(s.XML().FindElements("Section[@N='Geometry']"))
	mutate(s)
	afterSections := len(s.XML().FindElements("Section[@N='Geometry']"))
	if afterSections <= beforeSections {
		t.Errorf("mutation did not create a local Geometry section: had %d, now %d",
			beforeSections, afterSections)
	}
}
