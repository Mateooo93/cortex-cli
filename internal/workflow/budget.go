package workflow

import (
	"sync/atomic"
)

// Budget tracks token usage across a workflow run. The runtime
// reads from it to enforce the user's token target; agents
// write to it after each LLM call.
//
// A zero-value Budget has no limit (remaining is infinite).
// Call SetTotal with the user's "+500k"-style directive to
// enable capping.
type Budget struct {
	total atomic.Int64   // token target (0 = unlimited)
	spent atomic.Int64   // tokens consumed so far
}

// SetTotal configures the budget ceiling. A value of 0 means
// no limit.
func (b *Budget) SetTotal(n int64) {
	b.total.Store(n)
}

// Total returns the budget ceiling, or 0 if unlimited.
func (b *Budget) Total() int64 {
	return b.total.Load()
}

// Spend records token consumption. Safe for concurrent use.
func (b *Budget) Spend(n int64) {
	b.spent.Add(n)
}

// Spent returns tokens consumed so far.
func (b *Budget) Spent() int64 {
	return b.spent.Load()
}

// Remaining returns tokens left before the cap is hit.
// Returns MaxInt64 when there is no cap (Total() == 0).
func (b *Budget) Remaining() int64 {
	total := b.Total()
	if total == 0 {
		return 1<<63 - 1 // effectively infinite
	}
	spent := b.Spent()
	if spent >= total {
		return 0
	}
	return total - spent
}

// Exhausted returns true when a cap is set and spent >= total.
func (b *Budget) Exhausted() bool {
	total := b.Total()
	return total > 0 && b.Spent() >= total
}

// Reset zeroes the spent counter (keeps the total).
func (b *Budget) Reset() {
	b.spent.Store(0)
}

// BudgetSnapshot is a point-in-time view for the UI.
type BudgetSnapshot struct {
	Total     int64
	Spent     int64
	Remaining int64
}

// Snapshot returns a copy of the current budget state.
func (b *Budget) Snapshot() BudgetSnapshot {
	return BudgetSnapshot{
		Total:     b.Total(),
		Spent:     b.Spent(),
		Remaining: b.Remaining(),
	}
}
