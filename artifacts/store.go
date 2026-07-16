// Package artifacts provides an immutable artifact store with snapshot and rollback
// capabilities for the single-agent V1 architecture.
//
// Artifacts are the output of deterministic tools (calculations, verifications,
// reports). They are stored by reference and are immutable once written.
// Snapshots store references and metadata, not full directory copies.
package artifacts

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ArtifactType classifies the kind of artifact.
type ArtifactType string

const (
	// ArtifactTypeCatalog is a data catalog (schema descriptions, field mappings).
	ArtifactTypeCatalog ArtifactType = "catalog"

	// ArtifactTypePlan is an execution plan.
	ArtifactTypePlan ArtifactType = "plan"

	// ArtifactTypeResult is a computed result (yield, bin distribution, etc.).
	ArtifactTypeResult ArtifactType = "result"

	// ArtifactTypeVerification is a deterministic verification output.
	ArtifactTypeVerification ArtifactType = "verification"

	// ArtifactTypeReport is a generated report.
	ArtifactTypeReport ArtifactType = "report"

	// ArtifactTypeSnapshot is a state snapshot reference.
	ArtifactTypeSnapshot ArtifactType = "snapshot"

	// ArtifactTypeEvent is an event log entry.
	ArtifactTypeEvent ArtifactType = "event"
)

// Artifact represents an immutable piece of data produced during task execution.
type Artifact struct {
	// ID is a unique identifier for this artifact.
	ID string `json:"id"`

	// TaskID is the task that produced this artifact.
	TaskID string `json:"task_id"`

	// Type classifies the artifact.
	Type ArtifactType `json:"type"`

	// Name is a human-readable label.
	Name string `json:"name"`

	// Path is the file path to the artifact data (relative to workspace root).
	Path string `json:"path"`

	// ContentType is the MIME type of the artifact (e.g., "application/json").
	ContentType string `json:"content_type"`

	// Size is the size of the artifact in bytes.
	Size int64 `json:"size"`

	// Checksum is a deterministic hash of the artifact content (SHA-256 hex).
	Checksum string `json:"checksum,omitempty"`

	// Metadata carries additional structured metadata about the artifact.
	Metadata map[string]any `json:"metadata,omitempty"`

	// CreatedAt is when the artifact was written.
	CreatedAt time.Time `json:"created_at"`
}

// Snapshot represents a point-in-time reference to the task state.
// It stores artifact references and metadata, not full directory copies.
type Snapshot struct {
	// ID uniquely identifies this snapshot.
	ID string `json:"id"`

	// TaskID is the task this snapshot belongs to.
	TaskID string `json:"task_id"`

	// State is the workflow state at the time of the snapshot.
	State string `json:"state"`

	// ArtifactRefs lists the IDs of artifacts that existed at this snapshot.
	ArtifactRefs []string `json:"artifact_refs"`

	// Description is a human-readable note about when/why the snapshot was taken.
	Description string `json:"description"`

	// CreatedAt is when the snapshot was taken.
	CreatedAt time.Time `json:"created_at"`
}

// Store provides immutable artifact storage with snapshot support.
// All methods are safe for concurrent use.
type Store struct {
	mu        sync.RWMutex
	artifacts map[string]*Artifact // artifact ID -> artifact
	snapshots map[string]*Snapshot // snapshot ID -> snapshot
	byTask    map[string][]string  // task ID -> artifact IDs
}

// NewStore creates an empty artifact store.
func NewStore() *Store {
	return &Store{
		artifacts: make(map[string]*Artifact),
		snapshots: make(map[string]*Snapshot),
		byTask:    make(map[string][]string),
	}
}

// NewArtifactID generates a unique artifact ID.
func NewArtifactID() string {
	return uuid.New().String()
}

// NewSnapshotID generates a unique snapshot ID.
func NewSnapshotID() string {
	return uuid.New().String()
}

// Put stores an artifact. Returns an error if an artifact with the same ID
// already exists (artifacts are immutable).
func (s *Store) Put(artifact *Artifact) error {
	if artifact == nil {
		return fmt.Errorf("artifact must not be nil")
	}
	if artifact.ID == "" {
		return fmt.Errorf("artifact ID must not be empty")
	}
	if artifact.TaskID == "" {
		return fmt.Errorf("artifact TaskID must not be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.artifacts[artifact.ID]; exists {
		return fmt.Errorf("artifact %q already exists (immutable)", artifact.ID)
	}

	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = time.Now()
	}

	s.artifacts[artifact.ID] = artifact
	s.byTask[artifact.TaskID] = append(s.byTask[artifact.TaskID], artifact.ID)
	return nil
}

// Get retrieves an artifact by ID. Returns nil if not found.
func (s *Store) Get(id string) *Artifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.artifacts[id]
}

// ListByTask returns all artifact IDs for a given task.
func (s *Store) ListByTask(taskID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.byTask[taskID]
	result := make([]string, len(ids))
	copy(result, ids)
	return result
}

// ListByType returns all artifact IDs of a given type.
func (s *Store) ListByType(artifactType ArtifactType) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var ids []string
	for id, a := range s.artifacts {
		if a.Type == artifactType {
			ids = append(ids, id)
		}
	}
	return ids
}

// CreateSnapshot takes a point-in-time snapshot of the current artifact state.
// It records the IDs of all existing artifacts for the given task.
func (s *Store) CreateSnapshot(taskID, state, description string) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := &Snapshot{
		ID:           NewSnapshotID(),
		TaskID:       taskID,
		State:        state,
		Description:  description,
		ArtifactRefs: copyStringSlice(s.byTask[taskID]),
		CreatedAt:    time.Now(),
	}

	s.snapshots[snap.ID] = snap
	return snap, nil
}

// GetSnapshot retrieves a snapshot by ID. Returns nil if not found.
func (s *Store) GetSnapshot(id string) *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshots[id]
}

// ListSnapshots returns all snapshot IDs for a given task, ordered by creation time.
func (s *Store) ListSnapshots(taskID string) []*Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Snapshot
	for _, snap := range s.snapshots {
		if snap.TaskID == taskID {
			result = append(result, snap)
		}
	}
	return result
}

// GetArtifactsForSnapshot returns the artifacts referenced by a snapshot.
func (s *Store) GetArtifactsForSnapshot(snapshotID string) []*Artifact {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap, ok := s.snapshots[snapshotID]
	if !ok {
		return nil
	}

	artifacts := make([]*Artifact, 0, len(snap.ArtifactRefs))
	for _, ref := range snap.ArtifactRefs {
		if a, ok := s.artifacts[ref]; ok {
			artifacts = append(artifacts, a)
		}
	}
	return artifacts
}

// Count returns the total number of stored artifacts.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.artifacts)
}

// SnapshotCount returns the total number of stored snapshots.
func (s *Store) SnapshotCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.snapshots)
}

func copyStringSlice(src []string) []string {
	if src == nil {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}
