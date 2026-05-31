package vsdx

import (
	"os"
	"testing"
)

// TestMasterIsolation_GeometryRowMutationDoesNotLeakAcrossInstances exercises
// the bug catalogued in the contract tests phase 1: when a shape has no local
// Geometry section, vsdx-go used to alias the master's Geometries slice
// (shape.go:131), so a row mutation via instance.Geometries[0].Rows[ix]
// modified the master's XML element — which is shared with every other
// instance of that master.
//
// The repro fixture is logical-architecture.vsdx (in the the reference project repo), which
// has three Can instances ("Can", "Can.15", "Can.17") sharing one Can master.
// We mutate Can.15's first Ellipse row and assert the other two instances'
// view of that row is unchanged.
func TestMasterIsolation_GeometryRowMutationDoesNotLeakAcrossInstances(t *testing.T) {
	const fixture = "testdata/architecture-sample.vsdx"
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("fixture %s not available: %v", fixture, err)
	}
	v, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer v.Close()

	var canA, canB, canC *Shape
	for _, s := range v.Pages[0].AllShapes() {
		switch s.ShapeName {
		case "Can":
			canA = s
		case "Can.15":
			canB = s
		case "Can.17":
			canC = s
		}
	}
	if canA == nil || canB == nil || canC == nil {
		t.Fatalf("expected three Can instances, got Can=%v Can.15=%v Can.17=%v", canA, canB, canC)
	}

	// All three should share the same master and start with the same X value
	// for the Ellipse row.
	ellipseX := func(s *Shape) float64 {
		if len(s.Geometries) == 0 {
			return -1
		}
		for _, r := range s.Geometries[0].Rows {
			if r.RowType() == "Ellipse" {
				return r.X()
			}
		}
		return -1
	}
	beforeA, beforeB, beforeC := ellipseX(canA), ellipseX(canB), ellipseX(canC)
	if beforeA != beforeB || beforeB != beforeC {
		t.Fatalf("baseline mismatch: A=%v B=%v C=%v", beforeA, beforeB, beforeC)
	}

	// Mutate ONLY canB.
	var mutated bool
	for _, r := range canB.Geometries[0].Rows {
		if r.RowType() == "Ellipse" {
			r.SetX(999.0)
			mutated = true
			break
		}
	}
	if !mutated {
		t.Fatal("could not find Ellipse row on Can.15")
	}

	// canB should see the new value; canA + canC must NOT.
	afterA, afterB, afterC := ellipseX(canA), ellipseX(canB), ellipseX(canC)
	if afterB != 999.0 {
		t.Errorf("Can.15 (mutated) X = %v, want 999", afterB)
	}
	if afterA != beforeA {
		t.Errorf("Can (other instance) X changed: %v -> %v, expected unchanged", beforeA, afterA)
	}
	if afterC != beforeC {
		t.Errorf("Can.17 (other instance) X changed: %v -> %v, expected unchanged", beforeC, afterC)
	}
}

// TestMasterIsolation_MutationCreatesLocalGeometrySection checks the
// structural side of the localize-on-mutation contract: after any mutation
// on an aliased Geometry, the instance shape's XML must own its own
// <Section N="Geometry"> child. Without this, the mutation lives only on
// the master XML and the instance has nothing to distinguish itself.
func TestMasterIsolation_MutationCreatesLocalGeometrySection(t *testing.T) {
	const fixture = "testdata/architecture-sample.vsdx"
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("fixture %s not available: %v", fixture, err)
	}
	v, err := OpenBytes(data)
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer v.Close()

	var canB *Shape
	for _, s := range v.Pages[0].AllShapes() {
		if s.ShapeName == "Can.15" {
			canB = s
			break
		}
	}
	if canB == nil {
		t.Fatal("Can.15 not found in fixture")
	}

	// Before mutation: Can.15 has no local Geometry section.
	if got := len(canB.XML().FindElements("Section[@N='Geometry']")); got != 0 {
		t.Fatalf("expected Can.15 to start with 0 local Geometry sections, got %d", got)
	}

	// Trigger localize via Move (which only matters for MoveTo/LineTo rows
	// but the localize hook fires regardless).
	canB.Geometries[0].Move(0.0, 0.0)

	// After mutation: Can.15 should now own its Geometry XML.
	if got := len(canB.XML().FindElements("Section[@N='Geometry']")); got != 1 {
		t.Errorf("expected Can.15 to have 1 local Geometry section after Move, got %d", got)
	}
}
