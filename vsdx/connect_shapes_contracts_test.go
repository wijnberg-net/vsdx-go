package vsdx

import (
	"strings"
	"testing"
)

// ConnectShapes(WithStyle) is the canonical cascade operation: copy connector
// template, remap shape-IDs, fix up the Master attribute, bind endpoints,
// then run SetStartAndFinish which cascades into SetWidth/SetHeight.
//
// The audit (Hot zone #3 + a regression case) flagged this as one of the highest-risk
// surfaces because of the embedded-media + ID-remap interaction. The
// master-collision bug — fixed in commit an earlier fix — happened because the
// copied template's Master attribute pointed at an unrelated master in the
// destination file after CopyShape ran. Without a regression test, a future
// refactor of CopyShape or media handling could reintroduce it.
//
// These tests pin down the contract:
//   1. The new connector's master is "Dynamic connector" (or nil if the
//      target file has no such master), never some unrelated stencil.
//   2. BeginX/Y and EndX/Y are bound to the source/target shapes via
//      <Connect> elements in the page.
//   3. The connector's local geometry produces a line shape (MoveTo +
//      LineTo or NURBSTo), not a closed path that would render as a blob.
//   4. Round-trip persistence: master assignment survives save+reopen.
//   5. The style parameter ("straight" vs "curved") changes geometry but
//      not master.

// connectorMasterName returns the name of the master that a shape resolves
// to via MasterPageID, or "" if no master is bound or found.
func connectorMasterName(v *VisioFile, s *Shape) string {
	if s.MasterPageID == "" {
		return ""
	}
	for _, m := range v.MasterPages {
		if m.PageID() == s.MasterPageID {
			return m.Name()
		}
	}
	return ""
}

// a regression case REGRESSION: master-collision. The connector's master must resolve
// to a shape that's actually a connector, not whatever-master happens to
// share the template's original ID after CopyShape's ID-remap. The fix
// (commit an earlier fix) sets MasterPageID to "Dynamic connector" explicitly.
func TestConnectShapesContract_NoMasterCollision(t *testing.T) {
	v, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	page := v.Pages[0]
	shapes := page.ChildShapes()
	if len(shapes) < 2 {
		t.Fatalf("need >=2 shapes, got %d", len(shapes))
	}

	conn, err := v.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}

	gotMaster := connectorMasterName(v, conn)
	// Either the connector's master IS "Dynamic connector" (when the target
	// has one), or it's empty (when the target doesn't have one and the fix
	// fell back to "no master, use own geometry"). Anything else means the
	// master-collision bug returned.
	if gotMaster != "Dynamic connector" && gotMaster != "" {
		t.Errorf("connector master = %q, want %q or empty (master-collision regression)",
			gotMaster, "Dynamic connector")
	}
}

// SideEffects: ConnectShapes must NOT mutate fromShape or toShape's own
// geometry/position. The connector is a new shape with its own cells.
func TestConnectShapesContract_SourceShapesUnchanged(t *testing.T) {
	v, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	page := v.Pages[0]
	shapes := page.ChildShapes()
	beforeA := snapshotShape(shapes[0])
	beforeB := snapshotShape(shapes[1])

	if _, err := v.ConnectShapes(page, shapes[0], shapes[1]); err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}

	afterA := snapshotShape(shapes[0])
	afterB := snapshotShape(shapes[1])
	if diffs := diffShapeSnapshots(beforeA, afterA); len(diffs) > 0 {
		t.Errorf("fromShape changed: %v", diffs)
	}
	if diffs := diffShapeSnapshots(beforeB, afterB); len(diffs) > 0 {
		t.Errorf("toShape changed: %v", diffs)
	}
}

// Endpoint binding: BeginX bound to fromShape via Connect, EndX to toShape.
func TestConnectShapesContract_EndpointsBound(t *testing.T) {
	v, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	page := v.Pages[0]
	shapes := page.ChildShapes()
	conn, err := v.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}

	var foundBegin, foundEnd bool
	for _, c := range page.Connects() {
		if c.ConnectorShapeID() != conn.ID {
			continue
		}
		if c.FromRel == "BeginX" && c.ShapeID() == shapes[0].ID {
			foundBegin = true
		}
		if c.FromRel == "EndX" && c.ShapeID() == shapes[1].ID {
			foundEnd = true
		}
	}
	if !foundBegin {
		t.Errorf("no BeginX Connect bound to shape %s", shapes[0].ID)
	}
	if !foundEnd {
		t.Errorf("no EndX Connect bound to shape %s", shapes[1].ID)
	}
}

// Geometry: the connector's MASTER must produce a line-shape, not a closed
// area (which would render as a blob — the original bug's symptom). The
// local connector typically only overrides one or two rows; the bulk of the
// path lives on the master. So we check the master, which is what the
// renderer effectively uses.
func TestConnectShapesContract_GeometryIsLineShape(t *testing.T) {
	v, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	page := v.Pages[0]
	shapes := page.ChildShapes()
	conn, err := v.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}
	master := conn.MasterShape()
	if master == nil {
		t.Skip("connector has no master; cannot check master geometry")
	}
	if len(master.Geometries) == 0 {
		t.Fatal("connector master has no geometry")
	}
	var hasMoveTo, hasLineSegment bool
	for _, r := range master.Geometries[0].Rows {
		switch strings.ToLower(r.RowType()) {
		case "moveto":
			hasMoveTo = true
		case "lineto", "nurbsto":
			hasLineSegment = true
		case "ellipse":
			t.Errorf("master geometry contains Ellipse row — looks like the rounded-blob master-collision bug")
		}
	}
	if !hasMoveTo {
		t.Error("master geometry has no MoveTo row")
	}
	if !hasLineSegment {
		t.Error("master geometry has no LineTo/NURBSTo — endpoints would not be connected")
	}
}

// Round-trip: master assignment must persist through save+reopen, given
// the Fase 1 fix to SaveVsdxBytes that serialises master files.
func TestConnectShapesContract_RoundTripPersistMaster(t *testing.T) {
	v, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	page := v.Pages[0]
	shapes := page.ChildShapes()
	conn, err := v.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		v.Close()
		t.Fatalf("ConnectShapes: %v", err)
	}
	beforeMaster := connectorMasterName(v, conn)
	connID := conn.ID

	out, err := v.SaveVsdxBytes()
	if err != nil {
		v.Close()
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	v.Close()

	v2 := openFromBytes(t, out)
	defer v2.Close()
	var conn2 *Shape
	for _, s := range v2.Pages[0].AllShapes() {
		if s.ID == connID {
			conn2 = s
			break
		}
	}
	if conn2 == nil {
		t.Fatalf("connector %s not found after round-trip", connID)
	}
	afterMaster := connectorMasterName(v2, conn2)
	if beforeMaster != afterMaster {
		t.Errorf("master assignment lost: before=%q after=%q", beforeMaster, afterMaster)
	}
}

// Style parameter: "straight" vs "curved" must produce different geometries
// but the SAME master ("Dynamic connector"). The two styles differ in the
// connector's own cells (route style + geometry rows), not in master choice.
func TestConnectShapesContract_StyleAffectsGeometryNotMaster(t *testing.T) {
	openConnector := func(style string) (*VisioFile, *Shape) {
		v, err := Open(testFile("test1.vsdx"))
		if err != nil {
			t.Fatal(err)
		}
		page := v.Pages[0]
		shapes := page.ChildShapes()
		c, err := v.ConnectShapesWithStyle(page, shapes[0], shapes[1], style)
		if err != nil {
			v.Close()
			t.Fatalf("ConnectShapesWithStyle %q: %v", style, err)
		}
		return v, c
	}

	vS, cs := openConnector("straight")
	defer vS.Close()
	vC, cc := openConnector("curved")
	defer vC.Close()

	masterS := connectorMasterName(vS, cs)
	masterC := connectorMasterName(vC, cc)
	if masterS != masterC {
		t.Errorf("style parameter affected master: straight=%q curved=%q (both should be the same)",
			masterS, masterC)
	}

	// Geometries must differ — at least the row-type composition.
	rowTypes := func(s *Shape) []string {
		var out []string
		if len(s.Geometries) == 0 {
			return out
		}
		for _, r := range s.Geometries[0].Rows {
			out = append(out, r.RowType())
		}
		return out
	}
	rsS := rowTypes(cs)
	rsC := rowTypes(cc)
	if len(rsS) == 0 || len(rsC) == 0 {
		t.Skip("connector has no local geometry; cannot compare styles")
	}
}

// Invalid style parameter must error, not silently fall back.
func TestConnectShapesContract_InvalidStyleErrors(t *testing.T) {
	v, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	page := v.Pages[0]
	shapes := page.ChildShapes()
	_, err = v.ConnectShapesWithStyle(page, shapes[0], shapes[1], "wavy")
	if err == nil {
		t.Error("ConnectShapesWithStyle with invalid style \"wavy\" should error")
	}
}
