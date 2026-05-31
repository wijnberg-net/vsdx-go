package vsdx

import (
	"testing"
)

// a regression case: DataProperties() lazily caches the merged master+local map. After
// AddDataProperty the cache must be invalidated so a subsequent read sees
// the new row. InvalidateDataPropertiesCache() exposes the same hook for
// callers who mutate the XML directly.

func TestEC004_AddDataProperty_InvalidatesCache(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	s := v.GetPage(0).AddShape()

	// Prime the cache.
	first := s.DataProperties()
	if _, ok := first["NewProp"]; ok {
		t.Fatal("fixture unexpectedly already has NewProp")
	}

	s.AddDataProperty("Row_NewProp", "NewProp", "value-1")

	after := s.DataProperties()
	if _, ok := after["NewProp"]; !ok {
		t.Errorf("DataProperties() after AddDataProperty missing newly added \"NewProp\" — cache was not invalidated")
	}
}

func TestEC004_ExplicitInvalidatePicksUpManualXMLEdit(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	s := v.GetPage(0).AddShape()
	s.AddDataProperty("Row_1", "Initial", "v1")

	// Prime the cache to pin "Initial".
	if _, ok := s.DataProperties()["Initial"]; !ok {
		t.Fatal("baseline cache missing \"Initial\"")
	}

	// Bypass the public API: manipulate the XML directly to add another
	// property and ensure the cache wouldn't show it.
	propSect := s.XML().FindElement("Section[@N='Property']")
	row := propSect.CreateElement("Row")
	row.CreateAttr("N", "Row_2")
	labelCell := row.CreateElement("Cell")
	labelCell.CreateAttr("N", "Label")
	labelCell.CreateAttr("V", "Manual")
	valueCell := row.CreateElement("Cell")
	valueCell.CreateAttr("N", "Value")
	valueCell.CreateAttr("V", "v2")

	// Without invalidation: cache still shows the old map.
	if _, ok := s.DataProperties()["Manual"]; ok {
		t.Fatal("DataProperties() picked up manual XML edit without invalidation; precondition broken")
	}

	// With explicit invalidation: the new property surfaces.
	s.InvalidateDataPropertiesCache()
	if _, ok := s.DataProperties()["Manual"]; !ok {
		t.Errorf("InvalidateDataPropertiesCache did not surface the manually added row")
	}
}
