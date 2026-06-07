package ui

// DaemonStatusMsg carries the daemon connection status.
type DaemonStatusMsg struct{ Connected bool }

// SignalQuitMsg is sent when SIGINT/SIGTERM arrives so the running model can
// persist state and quit through Bubble Tea (which restores the terminal).
type SignalQuitMsg struct{}
