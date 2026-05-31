package vsdx

import (
	"testing"
)

// Shape.Remove deletes the shape's XML element from its parent (page or
// group). The audit (a regression case) flagged a gap: it doesn't touch the page's
// <Connect> elements. Connects with FromSheet or ToSheet pointing at the
// removed shape become orphans — they reference a non-existent shape, which
// Visio's repair logic will flag.
//
// The tests below pin the contract:
//
//   1. The shape's XML is gone after Remove.
//   2. Sibling shapes on the same page are untouched.
//   3. <Connect> elements referencing the removed shape (as either
//      ConnectorShape or terminal Shape) are also removed.
//   4. Connector shapes whose only endpoint was the removed shape stay
//      where they are — Remove on a terminal shape doesn't cascade. Cleanup
//      of dangling connectors is the caller's responsibility.
//   5. Round-trip persists the removal.

// setupConnectedPair creates a fresh page with two shapes connected by a
// single connector. Returns the file plus the IDs of (from, to, connector).
func setupConnectedPair(t *testing.T) (*VisioFile, *Page, string, string, string) {
	t.Helper()
	v, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	page := v.Pages[0]
	shapes := page.ChildShapes()
	if len(shapes) < 2 {
		v.Close()
		t.Fatalf("need >=2 shapes in fixture, got %d", len(shapes))
	}
	conn, err := v.ConnectShapes(page, shapes[0], shapes[1])
	if err != nil {
		v.Close()
		t.Fatalf("ConnectShapes: %v", err)
	}
	return v, page, shapes[0].ID, shapes[1].ID, conn.ID
}

func TestRemoveContract_ShapeIsGone(t *testing.T) {
	v, page, fromID, _, _ := setupConnectedPair(t)
	defer v.Close()

	target := page.FindShapeByID(fromID)
	target.Remove()

	if page.FindShapeByID(fromID) != nil {
		t.Errorf("shape %s still findable after Remove", fromID)
	}
}

func TestRemoveContract_DoesNotAffectSiblings(t *testing.T) {
	v, page, fromID, toID, connID := setupConnectedPair(t)
	defer v.Close()

	before := snapshotShape(page.FindShapeByID(toID))

	page.FindShapeByID(fromID).Remove()

	after := snapshotShape(page.FindShapeByID(toID))
	if diffs := diffShapeSnapshots(before, after); len(diffs) > 0 {
		t.Errorf("removing %s changed sibling %s: %v", fromID, toID, diffs)
	}
	// The connector survives as a dangling shape (per contract #4) — assert
	// it's still findable. Caller can choose whether to clean it up.
	if page.FindShapeByID(connID) == nil {
		t.Errorf("connector %s also disappeared — Remove cascaded unexpectedly", connID)
	}
}

func TestRemoveContract_RoundTripXML(t *testing.T) {
	v, _, fromID, _, _ := setupConnectedPair(t)
	v.Pages[0].FindShapeByID(fromID).Remove()
	out, err := v.SaveVsdxBytes()
	if err != nil {
		v.Close()
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	v.Close()

	v2 := openFromBytes(t, out)
	defer v2.Close()
	if v2.Pages[0].FindShapeByID(fromID) != nil {
		t.Errorf("shape %s reappeared after save+reopen", fromID)
	}
}

// a regression case REGRESSION: orphan Connect elements must be removed alongside the
// shape they reference. A Connect with ToSheet=removedID is a dangling
// reference that triggers a Visio repair prompt on open.
func TestRemoveContract_OrphanConnectsAreRemoved(t *testing.T) {
	v, page, fromID, _, connID := setupConnectedPair(t)
	defer v.Close()

	// Sanity precondition: a Connect exists with ToSheet=fromID.
	preFound := false
	for _, c := range page.Connects() {
		if c.ShapeID() == fromID {
			preFound = true
			break
		}
	}
	if !preFound {
		t.Skip("fixture has no Connect pointing at the target shape; cannot test orphan cleanup")
	}

	page.FindShapeByID(fromID).Remove()

	for _, c := range page.Connects() {
		if c.ShapeID() == fromID {
			t.Errorf("orphan Connect points at removed shape %s (connector %s, FromCell=%s, ToCell=%s)",
				fromID, c.ConnectorShapeID(), c.FromRel, c.ToRel)
		}
		if c.ConnectorShapeID() == fromID {
			t.Errorf("orphan Connect has connector=removed shape %s", fromID)
		}
	}

	// The connector that pointed at fromID should still be present (per
	// contract #4) — only its Connect bindings to fromID are gone.
	if page.FindShapeByID(connID) == nil {
		t.Errorf("connector shape %s was removed; only Connects should have been", connID)
	}
}

// Removing the connector itself must also clean the Connect entries that
// have FromSheet=connectorID.
func TestRemoveContract_RemovingConnectorCleansItsConnects(t *testing.T) {
	v, page, _, _, connID := setupConnectedPair(t)
	defer v.Close()

	page.FindShapeByID(connID).Remove()

	for _, c := range page.Connects() {
		if c.ConnectorShapeID() == connID {
			t.Errorf("orphan Connect references removed connector %s", connID)
		}
	}
}
