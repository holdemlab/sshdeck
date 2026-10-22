package ui

// The key reference lives in docs/help.md (shown with prefix + ?), so the
// help screen and the documentation never disagree.

// helpSize is the outer size of the help box.
func (m Model) helpSize() (w, h int) {
	w = min(m.width-2, 104)
	h = m.height - 2
	return max(w, 20), max(h, 6)
}
