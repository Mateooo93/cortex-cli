package ui

// FocusState tracks which UI component currently has keyboard focus.
type FocusState int

const (
	FocusEditor     FocusState = iota // textarea input has focus
	FocusChat                         // chat viewport has focus (scrollable)
	FocusRightPanel                   // right panel has focus (when open)
)

// restoreEditorFocus returns the session's chat input to editor focus.
func (sess *SessionState) restoreEditorFocus() {
	if sess == nil {
		return
	}
	sess.focus = FocusEditor
	sess.input.Focus()
}
