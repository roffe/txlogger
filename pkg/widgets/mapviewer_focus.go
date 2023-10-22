package widgets

func (mw *MapViewer) FocusGained() {
	mw.focused = true
}
func (mw *MapViewer) FocusLost() {
	mw.focused = false
}
func (mw *MapViewer) Focused() bool {
	return mw.focused
}

/*
func (mw *MapViewer) TypedRune(r rune) {
	log.Printf("TypedRune %c", r)
}
func (mw *MapViewer) TypedKey(event *fyne.KeyEvent) {
	log.Printf("TypedKey %v", event)
}
*/
