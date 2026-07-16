package workflow

import "testing"

func TestState_IsTerminal(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		terminal bool
	}{
		{"done is terminal", StateDone, true},
		{"failed is terminal", StateFailed, true},
		{"received is not terminal", StateReceived, false},
		{"executing is not terminal", StateExecuting, false},
		{"clarifying is not terminal", StateClarifying, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsTerminal(); got != tt.terminal {
				t.Errorf("State(%s).IsTerminal() = %v, want %v", tt.state, got, tt.terminal)
			}
		})
	}
}

func TestState_IsActive(t *testing.T) {
	if StateDone.IsActive() {
		t.Error("StateDone.IsActive() should be false")
	}
	if !StateExecuting.IsActive() {
		t.Error("StateExecuting.IsActive() should be true")
	}
}

func TestState_CanTransition_Valid(t *testing.T) {
	tests := []struct {
		from   State
		to     State
		expect bool
	}{
		// Happy path transitions
		{StateReceived, StateIngesting, true},
		{StateIngesting, StateInspecting, true},
		{StateInspecting, StatePlanning, true},
		{StateInspecting, StateClarifying, true},
		{StateClarifying, StatePlanning, true},
		{StatePlanning, StateExecuting, true},
		{StatePlanning, StateWaitingApproval, true},
		{StateWaitingApproval, StateExecuting, true},
		{StateExecuting, StateVerifying, true},
		{StateVerifying, StateReporting, true},
		{StateReporting, StateDone, true},

		// Error transitions (all active states can go to FAILED)
		{StateReceived, StateFailed, true},
		{StateIngesting, StateFailed, true},
		{StateExecuting, StateFailed, true},
		{StateVerifying, StateFailed, true},

		// Rollback transitions
		{StateExecuting, StateRollback, true},
		{StateVerifying, StateRollback, true},
		{StateWaitingApproval, StateRollback, true},

		// Invalid transitions
		{StateDone, StateReporting, false},
		{StateFailed, StateExecuting, false},
		{StateReceived, StateDone, false},
		{StateExecuting, StateReceived, false},
		{StatePlanning, StateReceived, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			got := tt.from.CanTransition(tt.to)
			if got != tt.expect {
				t.Errorf("%s.CanTransition(%s) = %v, want %v", tt.from, tt.to, got, tt.expect)
			}
		})
	}
}

func TestState_ValidTransitions(t *testing.T) {
	// StateDone has no valid transitions
	if len(StateDone.ValidTransitions()) != 0 {
		t.Error("StateDone should have no valid transitions")
	}

	// StateReceived should have at least 2 valid transitions
	transitions := StateReceived.ValidTransitions()
	if len(transitions) < 2 {
		t.Errorf("StateReceived should have at least 2 valid transitions, got %d", len(transitions))
	}
}

func TestState_String(t *testing.T) {
	if StateExecuting.String() != "EXECUTING" {
		t.Errorf("StateExecuting.String() = %q, want %q", StateExecuting.String(), "EXECUTING")
	}
}

func TestTransitionTable_TerminalsHaveNoOutgoing(t *testing.T) {
	terminals := []State{StateDone, StateFailed}
	for _, terminal := range terminals {
		transitions, exists := transitionTable[terminal]
		if !exists || len(transitions) != 0 {
			t.Errorf("terminal state %s should have empty transition table, got %v", terminal, transitions)
		}
	}
}

func TestTransitionTable_AllActiveStatesReachFailed(t *testing.T) {
	activeStates := []State{
		StateReceived, StateIngesting, StateInspecting,
		StateClarifying, StatePlanning, StateWaitingApproval,
		StateExecuting, StateVerifying, StateReporting,
	}
	for _, s := range activeStates {
		if !s.CanTransition(StateFailed) {
			t.Errorf("state %s should be able to transition to FAILED", s)
		}
	}
}
