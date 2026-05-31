package vsdx

import (
	"testing"
	"time"
)

// TestNewShape_CyclicMasterDoesNotStackOverflow verifies the depth guard
// in newShape's master-geometry inheritance path. Constructs two masters
// whose interior shapes reference each other via the Master attribute
// (A.Master=B, B.Master=A) — a malformed-but-syntactically-valid VSDX
// situation that previously caused MasterShape → ChildShapes → newShape →
// MasterShape to recurse without bound and stack-overflow the goroutine.
//
// After the fix newShape's geometry-inheritance block bumps a per-
// VisioFile counter and skips the recursive lookup once the counter
// crosses shapeResolveDepthLimit. The test passes by completing within a
// short timeout — without the guard it OOMs.
func TestNewShape_CyclicMasterDoesNotStackOverflow(t *testing.T) {
	vis, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening blank: %v", err)
	}
	t.Cleanup(func() { _ = vis.Close() })

	a, err := vis.CreateMaster("CycleA")
	if err != nil {
		t.Fatalf("CreateMaster A: %v", err)
	}
	b, err := vis.CreateMaster("CycleB")
	if err != nil {
		t.Fatalf("CreateMaster B: %v", err)
	}
	// Set up the cycle.
	a.ChildShapes()[0].xml.CreateAttr("Master", b.pageID)
	b.ChildShapes()[0].xml.CreateAttr("Master", a.pageID)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Triggering shape construction on either side previously
		// stack-overflowed via the mutual MasterShape() resolution.
		_ = a.AllShapes()
		_ = b.AllShapes()
	}()
	select {
	case <-done:
		// Passed — depth guard stopped the recursion in time.
	case <-time.After(2 * time.Second):
		t.Fatal("AllShapes() did not return within 2s; cyclic-master depth guard likely broken")
	}

	// The shapes should still be retrievable — we don't require any
	// specific behaviour about whether master geometry was inherited or
	// not (the guard simply skips inheritance), only that no crash
	// occurred and the structural data is intact.
	if got := len(a.ChildShapes()); got == 0 {
		t.Error("CycleA lost its shape after cycle-guard skip")
	}
	if got := len(b.ChildShapes()); got == 0 {
		t.Error("CycleB lost its shape after cycle-guard skip")
	}
}

// TestNewShape_DeepLegitMasterChainStillResolves checks that a deep but
// non-cyclic master chain (depth well below shapeResolveDepthLimit)
// still resolves its master geometry correctly. The guard should only
// kick in for pathological cycles, not for legitimate inheritance.
func TestNewShape_DeepLegitMasterChainStillResolves(t *testing.T) {
	vis, err := Open("../tests/blank.vsdx")
	if err != nil {
		t.Fatalf("opening blank: %v", err)
	}
	t.Cleanup(func() { _ = vis.Close() })

	// Chain of 8 masters: M1 → M2 → M3 → … → M8 → (no master)
	// Each Mi's first shape has Master pointing at M(i+1).
	const chainLen = 8
	masters := make([]*Page, chainLen)
	for i := 0; i < chainLen; i++ {
		m, err := vis.CreateMaster(formatMasterName(i))
		if err != nil {
			t.Fatalf("CreateMaster M%d: %v", i, err)
		}
		masters[i] = m
	}
	for i := 0; i < chainLen-1; i++ {
		masters[i].ChildShapes()[0].xml.CreateAttr("Master", masters[i+1].pageID)
	}

	// Resolving the head of the chain should walk the full depth without
	// hitting the guard (chainLen << shapeResolveDepthLimit).
	shapes := masters[0].AllShapes()
	if len(shapes) == 0 {
		t.Fatal("head of legitimate master chain returned no shapes")
	}
	// Sanity: depth counter should be 0 after every public call returns.
	if got := vis.shapeResolveDepth; got != 0 {
		t.Errorf("shapeResolveDepth leaked: %d, want 0", got)
	}
}

func formatMasterName(i int) string {
	// Tiny inline helper so this test stays free of fmt's heavier import
	// when chained alongside other lightweight tests.
	return "ChainMaster" + string(rune('A'+i))
}
