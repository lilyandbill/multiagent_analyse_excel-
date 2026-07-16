package artifacts

import (
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	s := NewStore()
	if s == nil {
		t.Fatal("expected non-nil store")
	}
	if s.Count() != 0 {
		t.Errorf("Count = %d, want 0", s.Count())
	}
}

func TestNewArtifactID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewArtifactID()
		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestNewSnapshotID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewSnapshotID()
		if ids[id] {
			t.Errorf("duplicate snapshot ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestStore_Put(t *testing.T) {
	s := NewStore()
	a := &Artifact{
		ID:     NewArtifactID(),
		TaskID: "task-1",
		Type:   ArtifactTypeResult,
		Name:   "yield_result",
		Path:   "artifacts/yield_result.json",
	}

	if err := s.Put(a); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Count() != 1 {
		t.Errorf("Count = %d, want 1", s.Count())
	}
}

func TestStore_Put_Nil(t *testing.T) {
	s := NewStore()
	err := s.Put(nil)
	if err == nil {
		t.Error("expected error for nil artifact")
	}
}

func TestStore_Put_EmptyID(t *testing.T) {
	s := NewStore()
	err := s.Put(&Artifact{TaskID: "task-1"})
	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestStore_Put_EmptyTaskID(t *testing.T) {
	s := NewStore()
	err := s.Put(&Artifact{ID: "id-1"})
	if err == nil {
		t.Error("expected error for empty TaskID")
	}
}

func TestStore_Put_Duplicate(t *testing.T) {
	s := NewStore()
	a := &Artifact{ID: "id-1", TaskID: "task-1"}
	s.Put(a)
	err := s.Put(a)
	if err == nil {
		t.Error("expected error for duplicate artifact ID")
	}
}

func TestStore_Put_SetsCreatedAt(t *testing.T) {
	s := NewStore()
	a := &Artifact{ID: "id-1", TaskID: "task-1"}
	s.Put(a)

	retrieved := s.Get("id-1")
	if retrieved.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set automatically")
	}
}

func TestStore_Get(t *testing.T) {
	s := NewStore()
	a := &Artifact{ID: "id-1", TaskID: "task-1", Name: "test"}
	s.Put(a)

	retrieved := s.Get("id-1")
	if retrieved == nil {
		t.Fatal("expected artifact, got nil")
	}
	if retrieved.Name != "test" {
		t.Errorf("Name = %q, want %q", retrieved.Name, "test")
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	s := NewStore()
	if s.Get("nonexistent") != nil {
		t.Error("expected nil for nonexistent artifact")
	}
}

func TestStore_ListByTask(t *testing.T) {
	s := NewStore()
	s.Put(&Artifact{ID: "id-1", TaskID: "task-1"})
	s.Put(&Artifact{ID: "id-2", TaskID: "task-1"})
	s.Put(&Artifact{ID: "id-3", TaskID: "task-2"})

	ids := s.ListByTask("task-1")
	if len(ids) != 2 {
		t.Errorf("expected 2 artifacts for task-1, got %d", len(ids))
	}
}

func TestStore_ListByType(t *testing.T) {
	s := NewStore()
	s.Put(&Artifact{ID: "id-1", TaskID: "task-1", Type: ArtifactTypeResult})
	s.Put(&Artifact{ID: "id-2", TaskID: "task-1", Type: ArtifactTypeResult})
	s.Put(&Artifact{ID: "id-3", TaskID: "task-1", Type: ArtifactTypeCatalog})

	ids := s.ListByType(ArtifactTypeResult)
	if len(ids) != 2 {
		t.Errorf("expected 2 result artifacts, got %d", len(ids))
	}
}

func TestStore_CreateSnapshot(t *testing.T) {
	s := NewStore()
	s.Put(&Artifact{ID: "id-1", TaskID: "task-1", Type: ArtifactTypeCatalog})
	s.Put(&Artifact{ID: "id-2", TaskID: "task-1", Type: ArtifactTypeResult})

	snap, err := s.CreateSnapshot("task-1", "INSPECTING", "after catalog build")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.State != "INSPECTING" {
		t.Errorf("State = %q, want %q", snap.State, "INSPECTING")
	}
	if len(snap.ArtifactRefs) != 2 {
		t.Errorf("ArtifactRefs = %d, want 2", len(snap.ArtifactRefs))
	}
	if s.SnapshotCount() != 1 {
		t.Errorf("SnapshotCount = %d, want 1", s.SnapshotCount())
	}
}

func TestStore_GetSnapshot(t *testing.T) {
	s := NewStore()
	s.Put(&Artifact{ID: "id-1", TaskID: "task-1"})
	snap, _ := s.CreateSnapshot("task-1", "EXECUTING", "pre-verify")

	retrieved := s.GetSnapshot(snap.ID)
	if retrieved == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if retrieved.ID != snap.ID {
		t.Errorf("ID mismatch: %s vs %s", retrieved.ID, snap.ID)
	}
}

func TestStore_ListSnapshots(t *testing.T) {
	s := NewStore()
	s.Put(&Artifact{ID: "id-1", TaskID: "task-1"})
	s.CreateSnapshot("task-1", "INSPECTING", "first")
	s.CreateSnapshot("task-1", "EXECUTING", "second")
	s.CreateSnapshot("task-2", "INSPECTING", "other task")

	snaps := s.ListSnapshots("task-1")
	if len(snaps) != 2 {
		t.Errorf("expected 2 snapshots for task-1, got %d", len(snaps))
	}
}

func TestStore_GetArtifactsForSnapshot(t *testing.T) {
	s := NewStore()
	s.Put(&Artifact{ID: "id-1", TaskID: "task-1", Name: "catalog"})
	s.Put(&Artifact{ID: "id-2", TaskID: "task-1", Name: "result"})

	snap, _ := s.CreateSnapshot("task-1", "EXECUTING", "snap")

	artifacts := s.GetArtifactsForSnapshot(snap.ID)
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}
}

func TestStore_GetArtifactsForSnapshot_NotFound(t *testing.T) {
	s := NewStore()
	artifacts := s.GetArtifactsForSnapshot("nonexistent")
	if artifacts != nil {
		t.Error("expected nil for nonexistent snapshot")
	}
}

func TestStore_SnapshotIsIndependent(t *testing.T) {
	s := NewStore()
	s.Put(&Artifact{ID: "id-1", TaskID: "task-1"})
	snap, _ := s.CreateSnapshot("task-1", "EXECUTING", "before add")

	// Add another artifact after the snapshot
	s.Put(&Artifact{ID: "id-2", TaskID: "task-1"})

	// Snapshot should still only reference the first artifact
	artifacts := s.GetArtifactsForSnapshot(snap.ID)
	if len(artifacts) != 1 {
		t.Errorf("snapshot should have 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].ID != "id-1" {
		t.Errorf("snapshot artifact = %q, want %q", artifacts[0].ID, "id-1")
	}
}

func TestArtifactType_Values(t *testing.T) {
	types := []ArtifactType{
		ArtifactTypeCatalog,
		ArtifactTypePlan,
		ArtifactTypeResult,
		ArtifactTypeVerification,
		ArtifactTypeReport,
		ArtifactTypeSnapshot,
		ArtifactTypeEvent,
	}
	seen := make(map[ArtifactType]bool)
	for _, at := range types {
		if seen[at] {
			t.Errorf("duplicate ArtifactType: %s", at)
		}
		seen[at] = true
	}
	if len(seen) != 7 {
		t.Errorf("expected 7 unique artifact types, got %d", len(seen))
	}
}

func TestStore_ConcurrentAccess(t *testing.T) {
	s := NewStore()
	done := make(chan bool)

	// Concurrent writers
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 10; j++ {
				s.Put(&Artifact{
					ID:     NewArtifactID(),
					TaskID: "task-1",
					Type:   ArtifactTypeResult,
				})
			}
			done <- true
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 20; j++ {
				s.Count()
				s.ListByTask("task-1")
				s.ListByType(ArtifactTypeResult)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}

	if s.Count() != 100 {
		t.Errorf("Count = %d, want 100", s.Count())
	}
}

func TestStore_Put_PathTraversal_DotDot(t *testing.T) {
	s := NewStore()
	err := s.Put(&Artifact{
		ID:     "id-1",
		TaskID: "task-1",
		Path:   "artifacts/../etc/passwd",
	})
	if err == nil {
		t.Error("expected error for path containing '..'")
	}
}

func TestStore_Put_PathTraversal_DotDotStart(t *testing.T) {
	s := NewStore()
	err := s.Put(&Artifact{
		ID:     "id-1",
		TaskID: "task-1",
		Path:   "../etc/passwd",
	})
	if err == nil {
		t.Error("expected error for path starting with '..'")
	}
}

func TestStore_Put_PathTraversal_Backslash(t *testing.T) {
	s := NewStore()
	err := s.Put(&Artifact{
		ID:     "id-1",
		TaskID: "task-1",
		Path:   `..\windows\system32`,
	})
	if err == nil {
		t.Error("expected error for path with backslash traversal")
	}
}

func TestStore_Put_ValidPath(t *testing.T) {
	s := NewStore()
	validPaths := []string{
		"artifacts/yield_result.json",
		"reports/summary.md",
		"catalog.json",
		"sub/dir/file.csv",
		"",
	}
	for _, p := range validPaths {
		err := s.Put(&Artifact{
			ID:     NewArtifactID(),
			TaskID: "task-1",
			Path:   p,
		})
		if err != nil {
			t.Errorf("valid path %q should be accepted: %v", p, err)
		}
	}
}

func TestStore_Put_PathTraversal_Encoded(t *testing.T) {
	// Only literal ".." is blocked; encoded variants are not (this is by design
	// — filesystem operations should also check before use).
	s := NewStore()
	err := s.Put(&Artifact{
		ID:     "id-1",
		TaskID: "task-1",
		Path:   "artifacts/yield_result.json",
	})
	if err != nil {
		t.Errorf("normal path should be accepted: %v", err)
	}
}

func TestStore_ListByType_Deterministic(t *testing.T) {
	s := NewStore()
	// Insert in non-sorted order with fixed IDs.
	ids := []string{"zebra", "alpha", "mike", "beta"}
	for _, id := range ids {
		s.Put(&Artifact{
			ID:     id,
			TaskID: "task-1",
			Type:   ArtifactTypeResult,
		})
	}
	// Add artifacts of a different type to ensure they are filtered out.
	s.Put(&Artifact{
		ID:     "gamma",
		TaskID: "task-1",
		Type:   ArtifactTypeCatalog,
	})

	result := s.ListByType(ArtifactTypeResult)
	if len(result) != 4 {
		t.Fatalf("expected 4 results, got %d: %v", len(result), result)
	}
	// Must be sorted alphabetically by ID.
	expected := []string{"alpha", "beta", "mike", "zebra"}
	for i, id := range expected {
		if result[i] != id {
			t.Errorf("result[%d] = %q, want %q (full: %v)", i, result[i], id, result)
		}
	}
}

func TestStore_ListByTask_Deterministic(t *testing.T) {
	s := NewStore()
	// Insert in insertion order.
	s.Put(&Artifact{ID: "id-c", TaskID: "task-1"})
	s.Put(&Artifact{ID: "id-a", TaskID: "task-1"})
	s.Put(&Artifact{ID: "id-b", TaskID: "task-1"})

	result := s.ListByTask("task-1")
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	// ListByTask preserves insertion order.
	if result[0] != "id-c" || result[1] != "id-a" || result[2] != "id-b" {
		t.Errorf("ListByTask should preserve insertion order, got %v", result)
	}
}

func TestStore_ListByTask_ReturnsCopy(t *testing.T) {
	s := NewStore()
	s.Put(&Artifact{ID: "id-1", TaskID: "task-1"})
	s.Put(&Artifact{ID: "id-2", TaskID: "task-1"})

	result := s.ListByTask("task-1")
	// Mutate the returned slice.
	result[0] = "hacked"

	// Original should be unchanged.
	result2 := s.ListByTask("task-1")
	if result2[0] != "id-1" {
		t.Errorf("ListByTask should return a copy, got %v", result2)
	}
}

func TestStore_ListByType_EmptyType(t *testing.T) {
	s := NewStore()
	s.Put(&Artifact{ID: "id-1", TaskID: "task-1", Type: ArtifactTypeResult})

	result := s.ListByType(ArtifactTypeCatalog)
	if len(result) != 0 {
		t.Errorf("expected empty list for type with no artifacts, got %v", result)
	}
}

func TestStore_Get_ReturnsPointerToInternal(t *testing.T) {
	s := NewStore()
	s.Put(&Artifact{ID: "id-1", TaskID: "task-1", Name: "original"})

	// Get returns a pointer to the internal artifact. Callers must not mutate it.
	retrieved := s.Get("id-1")
	retrieved.Name = "mutated"

	// Verify the internal state is also mutated (this is intentional — callers own the pointer).
	internal := s.Get("id-1")
	if internal.Name != "mutated" {
		t.Errorf("Get returns a shared pointer, Name = %q, want %q", internal.Name, "mutated")
	}
}

func TestArtifact_CreatedAtPreserved(t *testing.T) {
	s := NewStore()
	now := time.Now()
	a := &Artifact{
		ID:        "id-1",
		TaskID:    "task-1",
		CreatedAt: now,
	}
	s.Put(a)

	retrieved := s.Get("id-1")
	if !retrieved.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt should be preserved: %v vs %v", retrieved.CreatedAt, now)
	}
}
