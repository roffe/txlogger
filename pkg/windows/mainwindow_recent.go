package windows

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

const (
	prefsRecentFiles = "recentFiles"
	maxRecentFiles   = 10
)

// newRecentMenu builds the File > Recent submenu. The item is kept on mw so
// addRecent can repopulate it in place instead of rebuilding the main menu.
func (mw *MainWindow) newRecentMenu() *fyne.MenuItem {
	mw.recentItem = fyne.NewMenuItemWithIcon("Recent", theme.HistoryIcon(), nil)
	mw.recentItem.ChildMenu = fyne.NewMenu("Recent")
	mw.refreshRecentMenu()
	return mw.recentItem
}

// refreshRecentMenu repopulates the Recent submenu from preferences. Must run
// on the UI goroutine.
func (mw *MainWindow) refreshRecentMenu() {
	recent := mw.app.Preferences().StringList(prefsRecentFiles)
	items := make([]*fyne.MenuItem, 0, len(recent)+2)
	for _, filename := range recent {
		items = append(items, fyne.NewMenuItemWithIcon(filepath.Base(filename), theme.DocumentIcon(), func() {
			mw.openRecent(filename)
		}))
	}
	if len(items) == 0 {
		empty := fyne.NewMenuItem("(none)", nil)
		empty.Disabled = true
		items = append(items, empty)
	} else {
		items = append(items,
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItemWithIcon("Clear", theme.ContentClearIcon(), func() {
				mw.app.Preferences().SetStringList(prefsRecentFiles, nil)
				mw.refreshRecentMenu()
			}),
		)
	}
	mw.recentItem.ChildMenu.Items = items
	if m := mw.Window.MainMenu(); m != nil {
		m.Refresh()
	}
}

// addRecent moves filename to the front of the rolling recent list. Every load
// path funnels through here, so entries that aren't a real file on disk (the
// in-memory loads from the mobile file picker carry a bare name) are dropped
// rather than remembered as something we can't reopen.
func (mw *MainWindow) addRecent(filename string) {
	abs, err := filepath.Abs(filename)
	if err != nil {
		return
	}
	if st, err := os.Stat(abs); err != nil || st.IsDir() {
		return
	}
	mw.app.Preferences().SetStringList(prefsRecentFiles,
		pushRecent(mw.app.Preferences().StringList(prefsRecentFiles), abs))
	// Safe from either goroutine: before the event loop runs, fyne.Do on the
	// main goroutine executes inline; after, it queues.
	fyne.Do(mw.refreshRecentMenu)
}

// pushRecent moves abs to the front of list, dropping any earlier occurrence so
// reopening a file promotes it instead of duplicating it, and trims the tail to
// maxRecentFiles.
func pushRecent(list []string, abs string) []string {
	list = slices.DeleteFunc(list, func(s string) bool { return s == abs })
	list = append([]string{abs}, list...)
	if len(list) > maxRecentFiles {
		list = list[:maxRecentFiles]
	}
	return list
}

// removeRecent forgets a path that can no longer be opened.
func (mw *MainWindow) removeRecent(filename string) {
	recent := slices.DeleteFunc(mw.app.Preferences().StringList(prefsRecentFiles), func(s string) bool {
		return s == filename
	})
	mw.app.Preferences().SetStringList(prefsRecentFiles, recent)
	mw.refreshRecentMenu()
}

// openRecent reopens a remembered path, routing on extension the same way the
// drop handler does. A file that has since moved or been deleted is forgotten.
func (mw *MainWindow) openRecent(filename string) {
	if _, err := os.Stat(filename); err != nil {
		mw.Error(fmt.Errorf("%s is no longer available", filename))
		mw.removeRecent(filename)
		return
	}
	if strings.EqualFold(filepath.Ext(filename), ".bin") {
		if err := mw.LoadSymbolsFromFile(filename); err != nil {
			mw.Error(err)
		}
		return
	}
	f, err := os.Open(filename)
	if err != nil {
		mw.Error(err)
		return
	}
	defer f.Close()
	sz := mw.Window.Content().Size()
	mw.LoadLogfile(filename, f, fyne.NewPos(sz.Width/2, sz.Height/2))
}
