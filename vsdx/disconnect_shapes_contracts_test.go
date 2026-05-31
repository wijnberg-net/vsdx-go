package vsdx

import (
	"testing"
)

// a regression case (closed): DisconnectShapes strips <Connect> bindings between a
// connector and a terminal shape WITHOUT removing the connector shape
// itself. This is the complement to Shape.Remove which deletes the shape
// and cascades through removeOrphanConnects.
//
// Three behaviours are pinned:
//   1. Both shapes survive Disconnect — only the bindings disappear.
//   2. The returned count matches the number of <Connect> elements stripped.
//   3. nil arguments act as wildcards on that side.

func TestDisconnectShapesContract_RemovesBindingsKeepsShapes(t *testing.T) {
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
	connID := conn.ID

	// Two Connect elements are created (BeginX→from, EndX→to).
	bindingsBefore := 0
	for _, c := range page.Connects() {
		if c.FromID == connID {
			bindingsBefore++
		}
	}
	if bindingsBefore < 1 {
		t.Fatalf("no bindings created by ConnectShapes")
	}

	// Disconnect the connector from the second shape only — should remove
	// just the EndX binding (1), leaving the BeginX binding intact.
	removed := page.DisconnectShapes(conn, shapes[1])
	if removed != 1 {
		t.Errorf("DisconnectShapes removed %d, want 1", removed)
	}

	// Connector shape itself still present.
	if page.FindShapeByID(connID) == nil {
		t.Error("connector shape was deleted by DisconnectShapes (must survive)")
	}
	// Terminal shape still present.
	if page.FindShapeByID(shapes[1].ID) == nil {
		t.Error("terminal shape was deleted by DisconnectShapes")
	}

	// Only the BeginX→shapes[0] binding remains.
	remaining := 0
	for _, c := range page.Connects() {
		if c.FromID == connID {
			remaining++
		}
	}
	if remaining != bindingsBefore-1 {
		t.Errorf("connector has %d bindings after Disconnect, want %d", remaining, bindingsBefore-1)
	}
}

// nil terminal acts as wildcard: drops EVERY binding owned by the connector.
func TestDisconnectShapesContract_NilTerminalIsWildcard(t *testing.T) {
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

	bindings := 0
	for _, c := range page.Connects() {
		if c.FromID == conn.ID {
			bindings++
		}
	}
	removed := page.DisconnectShapes(conn, nil)
	if removed != bindings {
		t.Errorf("DisconnectShapes(conn, nil) removed %d, want %d (all bindings)", removed, bindings)
	}
	for _, c := range page.Connects() {
		if c.FromID == conn.ID {
			t.Errorf("binding %s still present after wildcard disconnect", c.String())
		}
	}
}

// Connect.Remove detaches a single binding directly.
func TestConnectRemove_DetachesSingleBinding(t *testing.T) {
	v, err := Open(testFile("test1.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	page := v.Pages[0]
	shapes := page.ChildShapes()
	if _, err := v.ConnectShapes(page, shapes[0], shapes[1]); err != nil {
		t.Fatalf("ConnectShapes: %v", err)
	}

	before := page.Connects()
	if len(before) == 0 {
		t.Fatal("no connects to remove")
	}
	target := before[0]
	target.Remove()

	for _, c := range page.Connects() {
		if c.xml == target.xml {
			t.Error("removed Connect still appears in page.Connects()")
		}
	}
	// Idempotent: second Remove is a no-op.
	target.Remove()
}

// Round-trip: removed bindings stay gone after save+reopen.
func TestDisconnectShapesContract_RoundTrip(t *testing.T) {
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
	connID := conn.ID
	page.DisconnectShapes(conn, nil)

	out, err := v.SaveVsdxBytes()
	if err != nil {
		v.Close()
		t.Fatalf("SaveVsdxBytes: %v", err)
	}
	v.Close()

	v2 := openFromBytes(t, out)
	defer v2.Close()
	page2 := v2.Pages[0]
	for _, c := range page2.Connects() {
		if c.FromID == connID {
			t.Errorf("binding %s reappeared after round-trip", c.String())
		}
	}
	if page2.FindShapeByID(connID) == nil {
		t.Error("connector shape disappeared after round-trip")
	}
}
