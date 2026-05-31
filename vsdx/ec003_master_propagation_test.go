package vsdx

import (
	"testing"
)

// a regression case: master-side mutations don't auto-propagate to instances'
// cached state during the same session. Provide explicit invalidation
// hooks so callers who edit a master can force instances to re-read.

func TestEC003_InvalidateInstanceCachesForMaster_RefreshesDataProperties(t *testing.T) {
	v, err := Open(testFile("test_master.vsdx"))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	// Find an instance with a master.
	inst := findShape(t, v, func(s *Shape) bool { return s.MasterPageID != "" })
	masterID := inst.MasterPageID
	masterShape := inst.MasterShape()
	if masterShape == nil {
		t.Skip("instance has no master shape; cannot run")
	}

	// Prime the instance's cache.
	_ = inst.DataProperties()

	// Edit the master directly: add a new data property.
	masterShape.AddDataProperty("Row_NewMasterProp", "MasterAdded", "from-master")

	// Without invalidation the instance's cached map can be stale (because
	// it was primed before the master mutation). Explicit per-pointer
	// invalidation: callers who hold their own *Shape MUST call the
	// pointer-level helper. V-level InvalidateInstanceCachesForMaster only
	// reaches shapes returned by AllShapes() at call time.
	inst.InvalidateInheritanceCaches()
	got := inst.DataProperties()
	if _, ok := got["MasterAdded"]; !ok {
		t.Errorf("instance DataProperties does not contain master-added prop after InvalidateInheritanceCaches")
	}

	// And as a separate validation, AllShapes()-returned freshly-built
	// instances must also see it after the V-level helper fires.
	v.InvalidateInstanceCachesForMaster(masterID)
	for _, s := range v.Pages[0].AllShapes() {
		if s.MasterPageID == masterID {
			if _, ok := s.DataProperties()["MasterAdded"]; !ok {
				t.Errorf("freshly-fetched instance %s did not see master-added prop", s.ID)
			}
		}
	}
}

func TestEC003_InvalidateInheritanceCaches_IsSafeOnFreshShape(t *testing.T) {
	v := newBlankFile(t)
	defer v.Close()
	s := v.GetPage(0).AddShape()
	// Brand-new shape, no master, no caches populated. Must not panic.
	s.InvalidateInheritanceCaches()
}
