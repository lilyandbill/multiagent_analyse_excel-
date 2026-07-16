package skills

import (
	"context"
	"testing"
)

func makeTestSkill(name string, major, minor, patch int) *Skill {
	s, _ := NewSkill(Manifest{
		Name:        name,
		Version:     Version{major, minor, patch},
		Description: "test skill",
	}, func(ctx context.Context, input map[string]any) (any, error) {
		return nil, nil
	})
	return s
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	err := r.Register(makeTestSkill("s1", 1, 0, 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Count() != 1 {
		t.Errorf("Count = %d, want 1", r.Count())
	}
}

func TestRegistry_Register_Nil(t *testing.T) {
	r := NewRegistry()
	err := r.Register(nil)
	if err == nil {
		t.Error("expected error for nil skill")
	}
}

func TestRegistry_Register_DuplicateVersion(t *testing.T) {
	r := NewRegistry()
	r.Register(makeTestSkill("s1", 1, 0, 0))
	err := r.Register(makeTestSkill("s1", 1, 0, 0))
	if err == nil {
		t.Error("expected error for duplicate version")
	}
}

func TestRegistry_Register_MultipleVersions_Sorted(t *testing.T) {
	r := NewRegistry()
	r.Register(makeTestSkill("s1", 1, 0, 0))
	r.Register(makeTestSkill("s1", 2, 0, 0))
	r.Register(makeTestSkill("s1", 1, 5, 0))

	versions := r.ListVersions("s1")
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	// Should be sorted newest first.
	if versions[0].String() != "v2.0.0" {
		t.Errorf("versions[0] = %s, want v2.0.0", versions[0])
	}
}

func TestRegistry_Activate(t *testing.T) {
	r := NewRegistry()
	r.Register(makeTestSkill("s1", 1, 0, 0))
	r.Register(makeTestSkill("s1", 2, 0, 0))

	err := r.Activate("s1", Version{1, 0, 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	active := r.GetActive("s1")
	if active == nil {
		t.Fatal("expected active skill, got nil")
	}
	if active.Manifest.Version.String() != "v1.0.0" {
		t.Errorf("active version = %s, want v1.0.0", active.Manifest.Version)
	}
}

func TestRegistry_Activate_NotFound(t *testing.T) {
	r := NewRegistry()
	err := r.Activate("nonexistent", Version{1, 0, 0})
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestRegistry_Activate_VersionNotFound(t *testing.T) {
	r := NewRegistry()
	r.Register(makeTestSkill("s1", 1, 0, 0))
	err := r.Activate("s1", Version{99, 0, 0})
	if err == nil {
		t.Error("expected error for nonexistent version")
	}
}

func TestRegistry_ActivateLatest(t *testing.T) {
	r := NewRegistry()
	r.Register(makeTestSkill("s1", 1, 0, 0))
	r.Register(makeTestSkill("s1", 3, 2, 1))
	r.Register(makeTestSkill("s1", 2, 0, 0))

	err := r.ActivateLatest("s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	active := r.GetActive("s1")
	if active.Manifest.Version.String() != "v3.2.1" {
		t.Errorf("active version = %s, want v3.2.1", active.Manifest.Version)
	}
}

func TestRegistry_GetVersion(t *testing.T) {
	r := NewRegistry()
	r.Register(makeTestSkill("s1", 1, 0, 0))
	r.Register(makeTestSkill("s1", 2, 0, 0))

	s := r.GetVersion("s1", Version{1, 0, 0})
	if s == nil {
		t.Fatal("expected skill, got nil")
	}
	if s.Manifest.Version.String() != "v1.0.0" {
		t.Errorf("version = %s, want v1.0.0", s.Manifest.Version)
	}
}

func TestRegistry_GetVersion_NotFound(t *testing.T) {
	r := NewRegistry()
	s := r.GetVersion("nonexistent", Version{1, 0, 0})
	if s != nil {
		t.Error("expected nil for nonexistent skill")
	}
}

func TestRegistry_ListSkills(t *testing.T) {
	r := NewRegistry()
	r.Register(makeTestSkill("zzz", 1, 0, 0))
	r.Register(makeTestSkill("aaa", 1, 0, 0))
	r.Register(makeTestSkill("mmm", 1, 0, 0))

	names := r.ListSkills()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	if names[0] != "aaa" || names[1] != "mmm" || names[2] != "zzz" {
		t.Errorf("names not sorted: %v", names)
	}
}

func TestRegistry_Deactivate(t *testing.T) {
	r := NewRegistry()
	r.Register(makeTestSkill("s1", 1, 0, 0))
	r.ActivateLatest("s1")

	if r.ActiveCount() != 1 {
		t.Errorf("ActiveCount = %d, want 1", r.ActiveCount())
	}

	r.Deactivate("s1")
	if r.ActiveCount() != 0 {
		t.Errorf("ActiveCount = %d, want 0 after deactivate", r.ActiveCount())
	}
	if r.GetActive("s1") != nil {
		t.Error("GetActive should return nil after deactivate")
	}
	// Deactivating should not remove the skill from the registry.
	if r.Count() != 1 {
		t.Errorf("Count = %d, want 1 (skill should remain registered)", r.Count())
	}
}

func TestRegistry_ActivateLatest_Empty(t *testing.T) {
	r := NewRegistry()
	err := r.ActivateLatest("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestRegistry_Count(t *testing.T) {
	r := NewRegistry()
	r.Register(makeTestSkill("s1", 1, 0, 0))
	r.Register(makeTestSkill("s1", 2, 0, 0))
	r.Register(makeTestSkill("s2", 1, 0, 0))

	if r.Count() != 3 {
		t.Errorf("Count = %d, want 3", r.Count())
	}
}

func TestRegistry_Register_InvalidManifest(t *testing.T) {
	r := NewRegistry()
	// Construct a Skill directly with an invalid manifest (empty name),
	// bypassing NewSkill's validation to exercise the registry's defense-in-depth.
	invalidSkill := &Skill{
		Manifest: Manifest{},
		Handler: func(ctx context.Context, input map[string]any) (any, error) {
			return nil, nil
		},
	}
	err := r.Register(invalidSkill)
	if err == nil {
		t.Error("expected error for invalid manifest")
	}
}

func TestRegistry_Register_InvalidManifest_ZeroVersion(t *testing.T) {
	r := NewRegistry()
	invalidSkill := &Skill{
		Manifest: Manifest{
			Name:        "test",
			Version:     Version{0, 0, 0},
			Description: "desc",
		},
		Handler: func(ctx context.Context, input map[string]any) (any, error) {
			return nil, nil
		},
	}
	err := r.Register(invalidSkill)
	if err == nil {
		t.Error("expected error for zero version in manifest")
	}
}

func TestRegistry_Register_InvalidManifest_NoDescription(t *testing.T) {
	r := NewRegistry()
	invalidSkill := &Skill{
		Manifest: Manifest{
			Name:    "test",
			Version: Version{1, 0, 0},
		},
		Handler: func(ctx context.Context, input map[string]any) (any, error) {
			return nil, nil
		},
	}
	err := r.Register(invalidSkill)
	if err == nil {
		t.Error("expected error for missing description")
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	done := make(chan bool)

	// Concurrent registrations of different skills.
	for i := 0; i < 20; i++ {
		go func(n int) {
			name := "skill_" + string(rune('a'+n))
			_ = r.Register(makeTestSkill(name, 1, 0, 0))
			done <- true
		}(i)
	}

	// Concurrent readers.
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 20; j++ {
				r.Count()
				r.ActiveCount()
				r.ListSkills()
			}
			done <- true
		}()
	}

	// Wait for all goroutines.
	for i := 0; i < 30; i++ {
		<-done
	}

	// All 20 skills should be registered.
	if r.Count() != 20 {
		t.Errorf("Count = %d, want 20", r.Count())
	}
	names := r.ListSkills()
	if len(names) != 20 {
		t.Errorf("ListSkills = %d names, want 20", len(names))
	}
}

func TestRegistry_ListSkills_EmptyRegistry(t *testing.T) {
	r := NewRegistry()
	names := r.ListSkills()
	if len(names) != 0 {
		t.Errorf("expected empty list, got %v", names)
	}
}

func TestRegistry_ListVersions_NotFound(t *testing.T) {
	r := NewRegistry()
	versions := r.ListVersions("nonexistent")
	if versions != nil {
		t.Errorf("expected nil for nonexistent skill, got %v", versions)
	}
}

func TestRegistry_GetActive_NotActivated(t *testing.T) {
	r := NewRegistry()
	r.Register(makeTestSkill("s1", 1, 0, 0))
	// Skill is registered but not activated.
	active := r.GetActive("s1")
	if active != nil {
		t.Error("expected nil for registered but not activated skill")
	}
}

func TestRegistry_Register_SameNameDifferentVersion(t *testing.T) {
	r := NewRegistry()
	err := r.Register(makeTestSkill("s1", 1, 0, 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = r.Register(makeTestSkill("s1", 1, 1, 0))
	if err != nil {
		t.Fatalf("different versions should be allowed: %v", err)
	}
	if r.Count() != 2 {
		t.Errorf("Count = %d, want 2", r.Count())
	}
}
