// Package workflow provides the workflow state machine for the single-agent V1 architecture.
//
// The state machine defines the lifecycle of a task from receipt to completion,
// with explicit transitions controlled by code, not by LLM output parsing.
package workflow

// State represents a discrete stage in the task processing lifecycle.
type State string

const (
	// StateReceived is the initial state when a task request is received.
	StateReceived State = "RECEIVED"

	// StateIngesting means the system is loading and validating uploaded files.
	StateIngesting State = "INGESTING"

	// StateInspecting means the system is examining schemas and building a data catalog.
	StateInspecting State = "INSPECTING"

	// StateClarifying means the Main Agent has detected material ambiguity
	// and is waiting for user input.
	StateClarifying State = "CLARIFYING"

	// StatePlanning means the system is generating or selecting an execution plan.
	StatePlanning State = "PLANNING"

	// StateWaitingApproval means the plan requires explicit user approval before execution.
	StateWaitingApproval State = "WAITING_APPROVAL"

	// StateExecuting means a skill is actively running.
	StateExecuting State = "EXECUTING"

	// StateVerifying means deterministic verification checks are running on results.
	StateVerifying State = "VERIFYING"

	// StateReporting means the final report is being generated.
	StateReporting State = "REPORTING"

	// StateDone is the terminal success state.
	StateDone State = "DONE"

	// StateFailed is the terminal failure state.
	StateFailed State = "FAILED"

	// StateRollback means the system is reverting to a previous snapshot.
	StateRollback State = "ROLLBACK"
)

// IsTerminal returns true if the state is a terminal state (no further transitions).
func (s State) IsTerminal() bool {
	return s == StateDone || s == StateFailed
}

// IsActive returns true if the task is still being processed.
func (s State) IsActive() bool {
	return !s.IsTerminal()
}

// CanTransition checks whether a transition from the current state to the target state
// is valid according to the state machine rules.
func (s State) CanTransition(target State) bool {
	allowed, ok := transitionTable[s]
	if !ok {
		return false
	}
	return allowed[target]
}

// ValidTransitions returns all valid target states from the current state.
func (s State) ValidTransitions() []State {
	allowed, ok := transitionTable[s]
	if !ok {
		return nil
	}
	states := make([]State, 0, len(allowed))
	for target, valid := range allowed {
		if valid {
			states = append(states, target)
		}
	}
	return states
}

// transitionTable defines all valid state transitions.
// A true value means the transition from row state to column state is allowed.
var transitionTable = map[State]map[State]bool{
	StateReceived: {
		StateIngesting: true,
		StateFailed:    true,
	},
	StateIngesting: {
		StateInspecting: true,
		StateFailed:     true,
	},
	StateInspecting: {
		StateClarifying: true,
		StatePlanning:   true,
		StateFailed:     true,
	},
	StateClarifying: {
		StateInspecting: true,
		StatePlanning:   true,
		StateFailed:     true,
	},
	StatePlanning: {
		StateWaitingApproval: true,
		StateExecuting:       true,
		StateFailed:          true,
	},
	StateWaitingApproval: {
		StateExecuting: true,
		StateRollback:  true,
		StateFailed:    true,
	},
	StateExecuting: {
		StateVerifying: true,
		StateRollback:  true,
		StateFailed:    true,
	},
	StateVerifying: {
		StateReporting: true,
		StateRollback:  true,
		StateFailed:    true,
	},
	StateReporting: {
		StateDone:   true,
		StateFailed: true,
	},
	StateRollback: {
		StateReceived:   true, // restart from beginning
		StateClarifying: true,
		StatePlanning:   true,
		StateFailed:     true,
	},
	// Terminal states have no outgoing transitions.
	StateDone:   {},
	StateFailed: {},
}

// String returns the string representation of the state.
func (s State) String() string {
	return string(s)
}
