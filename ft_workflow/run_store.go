package ft_workflow

import (
	"encoding/json"
	"fmt"
	"sync"

	"excel-agent/artifacts"
	"excel-agent/workflow"
)

// RunStore persists and retrieves RunState across HTTP requests using
// the artifact store's snapshot mechanism.
type RunStore struct {
	mu    sync.Mutex
	store *artifacts.Store
	runs  map[string]RunState // in-memory cache backed by artifacts
}

// NewRunStore creates a new run store.
func NewRunStore() *RunStore {
	return &RunStore{
		store: artifacts.NewStore(),
		runs:  make(map[string]RunState),
	}
}

// Save persists a run state as an artifact snapshot.
func (rs *RunStore) Save(runID string, state RunState) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	// Serialize state to JSON and store as artifact.
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal run state: %w", err)
	}

	artID := "runstate_" + runID
	_ = rs.store.Put(&artifacts.Artifact{
		ID:          artID,
		TaskID:      runID,
		Type:        artifacts.ArtifactTypeSnapshot,
		Name:        "run_state",
		Path:        "artifacts/run_state.json",
		ContentType: "application/json",
		Size:        int64(len(data)),
		Metadata:    map[string]any{"state": state},
	})

	// Create a snapshot.
	_, err = rs.store.CreateSnapshot(runID, string(state.State), "run persisted")
	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	rs.runs[runID] = state
	return nil
}

// Load retrieves a run state by ID.
func (rs *RunStore) Load(runID string) (RunState, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	state, ok := rs.runs[runID]
	if !ok {
		return RunState{}, fmt.Errorf("run %q not found", runID)
	}
	return state, nil
}

// LoadAndLock loads a run state and locks it to prevent concurrent confirmation.
// Returns an error if the run is not in WAITING_APPROVAL state.
// The state is not modified — the orchestrator handles the transition via Confirm().
func (rs *RunStore) LoadAndLock(runID string) (RunState, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	state, ok := rs.runs[runID]
	if !ok {
		return RunState{}, fmt.Errorf("run %q not found", runID)
	}
	if state.State != workflow.StateWaitingApproval {
		return RunState{}, fmt.Errorf("run %q is in state %s, expected WAITING_APPROVAL", runID, state.State)
	}
	// Mark as EXECUTING to prevent concurrent/duplicate confirmation.
	state.State = workflow.StateExecuting
	rs.runs[runID] = state
	return state, nil
}

// Delete removes a run.
func (rs *RunStore) Delete(runID string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	delete(rs.runs, runID)
}

// List returns all run IDs.
func (rs *RunStore) List() []string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	ids := make([]string, 0, len(rs.runs))
	for id := range rs.runs {
		ids = append(ids, id)
	}
	return ids
}
