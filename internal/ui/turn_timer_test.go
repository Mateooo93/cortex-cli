package ui

import (
	"testing"
	"time"
)

// TestTurnElapsed_OnlyCountsWhileActive verifies the per-turn
// timer the user asked for: the elapsed time should grow
// only while the agent is working (StartTurn → FinishTurn),
// not wall-clock-since-session-open.
func TestTurnElapsed_OnlyCountsWhileActive(t *testing.T) {
	s := &SessionState{}
	s.StartTurn()
	time.Sleep(50 * time.Millisecond)
	during := s.TurnElapsed()
	if during < 50*time.Millisecond {
		t.Errorf("during work, elapsed should be >= 50ms, got %s", during)
	}
	// Mark the turn as finished and verify the timer
	// stops accumulating.
	s.FinishTurn()
	final := s.TurnElapsed()
	time.Sleep(50 * time.Millisecond)
	after := s.TurnElapsed()
	if after != final {
		t.Errorf("after FinishTurn, timer should not advance: final=%s, after=%s", final, after)
	}
}
