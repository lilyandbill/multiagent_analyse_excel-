package skills

import (
	"fmt"
	"sort"
	"sync"
)

// Registry stores and retrieves skills by name. Each skill name maps to
// a version history, with the latest version being the active one.
// A skill must not be promoted to active without passing static validation,
// unit tests, golden dataset replay, cost regression, and human approval.
type Registry struct {
	mu     sync.RWMutex
	skills map[string][]*Skill // skill name -> ordered version history
	active map[string]*Skill   // skill name -> currently active version
}

// NewRegistry creates an empty skill registry.
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string][]*Skill),
		active: make(map[string]*Skill),
	}
}

// Register adds a skill to the registry. If a version of this skill already exists,
// the new version is appended to the history. The skill does NOT automatically
// become active — it must be explicitly activated via Activate.
func (r *Registry) Register(skill *Skill) error {
	if skill == nil {
		return fmt.Errorf("skill must not be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	name := skill.Manifest.Name
	version := skill.Manifest.Version

	// Check for version collision.
	for _, existing := range r.skills[name] {
		if existing.Manifest.Version.Compare(version) == 0 {
			return fmt.Errorf("skill %q version %s is already registered", name, version)
		}
	}

	r.skills[name] = append(r.skills[name], skill)

	// Keep version history sorted (newest first).
	sort.SliceStable(r.skills[name], func(i, j int) bool {
		return r.skills[name][i].Manifest.Version.Compare(
			r.skills[name][j].Manifest.Version) > 0
	})

	return nil
}

// Activate promotes a specific version of a skill to be the active one.
// The skill and version must already be registered.
func (r *Registry) Activate(name string, version Version) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	versions, ok := r.skills[name]
	if !ok {
		return fmt.Errorf("skill %q not found", name)
	}

	for _, s := range versions {
		if s.Manifest.Version.Compare(version) == 0 {
			r.active[name] = s
			return nil
		}
	}

	return fmt.Errorf("skill %q version %s not found", name, version)
}

// ActivateLatest promotes the most recent version of a skill to active.
func (r *Registry) ActivateLatest(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	versions, ok := r.skills[name]
	if !ok || len(versions) == 0 {
		return fmt.Errorf("skill %q not found", name)
	}

	// Versions are sorted newest-first.
	r.active[name] = versions[0]
	return nil
}

// GetActive returns the currently active version of a skill.
// Returns nil if the skill is not found or not activated.
func (r *Registry) GetActive(name string) *Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.active[name]
}

// GetVersion returns a specific version of a skill.
// Returns nil if the skill or version is not found.
func (r *Registry) GetVersion(name string, version Version) *Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.skills[name]
	if !ok {
		return nil
	}
	for _, s := range versions {
		if s.Manifest.Version.Compare(version) == 0 {
			return s
		}
	}
	return nil
}

// ListVersions returns all registered versions of a skill, newest first.
func (r *Registry) ListVersions(name string) []Version {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.skills[name]
	if !ok {
		return nil
	}
	result := make([]Version, len(versions))
	for i, s := range versions {
		result[i] = s.Manifest.Version
	}
	return result
}

// ListSkills returns the names of all registered skills.
func (r *Registry) ListSkills() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Count returns the total number of registered skill versions across all skills.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, versions := range r.skills {
		count += len(versions)
	}
	return count
}

// ActiveCount returns the number of activated skills.
func (r *Registry) ActiveCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.active)
}

// Deactivate removes a skill from the active set without deleting it.
func (r *Registry) Deactivate(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, name)
}
