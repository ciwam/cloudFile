package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"file-manager/locales"
	"file-manager/vfs"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// entry couples a VFS entry with its VFS path.
type entry struct {
	vfs.Entry
	path string
}

// model is the app state the window shows.
type model struct {
	win *mygo.Window

	fs vfs.FS
	// history of visited directories for back/forward.
	history    []string
	historyPos int

	cwd        string
	entries    []entry // filtered and sorted for display
	unfiltered []entry // the directory's entries before the search filter
	loading    bool
	loadErr    string
	status     string
	statusAt   time.Time
	showDots   bool
	query      string // search filter
	table      ui.ListState
	selected   int // table Selected

	// pending file ops
	renamingPath   string // entry being renamed in place
	confirmDel     []string
	confirmDelOpen bool
	newFolderOpen  bool
	newFolderName  string

	// settings
	settingsOpen bool
	langChoice   string
	langPicked   bool
	langApplied  bool // test hook: an applyLocale ran

	// applyLocale applies a language choice; nil in tests, which set
	// langChoice directly.
	applyLocale func(tag string)

	// clipboard of VFS paths, cut if cutMode
	clipPaths []string
	cutMode   bool

	sidebarWidth float32
	sidebarSel   string
	searchFocus  ui.Handle

	// testNow overrides the clock in tests.
	testNow func() time.Time
}

func newModel(fs vfs.FS, start string) *model {
	return &model{fs: fs, cwd: start, sidebarSel: start, selected: -1, sidebarWidth: 200}
}

// refresh reloads the current directory. With a window it loads
// asynchronously and applies the result on the main thread; without one
// (tests) it loads synchronously.
func (a *model) refresh() {
	dir := a.cwd
	a.loading = true
	a.loadErr = ""
	if a.win == nil {
		list, err := a.fs.List(context.Background(), dir)
		a.applyLoaded(dir, list, err)
		return
	}
	fs := a.fs
	win := a.win
	go func() {
		list, err := fs.List(context.Background(), dir)
		win.Update(func() {
			a.applyLoaded(dir, list, err)
		})
	}()
}

func (a *model) applyLoaded(dir string, list []vfs.Entry, err error) {
	if dir != a.cwd {
		return // stale: another load is under way
	}
	a.loading = false
	if err != nil {
		a.entries = nil
		a.loadErr = err.Error()
		return
	}
	out := make([]entry, 0, len(list))
	for _, e := range list {
		if a.showDots || !strings.HasPrefix(e.Name, ".") {
			out = append(out, entry{Entry: e, path: path.Join("/", dir, e.Name)})
		}
	}
	a.entries = out
	a.unfiltered = out
	a.applyFilterAndSort()
	a.selected = -1
	// Reset the view of the rows without zeroing the ListState: the
	// engine may still hold a press on one of its rows for this event
	// cycle, and a zeroed frame crashes its drag-select scroll.
	a.table.ScrollTo(0, ui.Start)
	a.table.Key = func(i int) any { return a.entries[i].path }
	a.table.Label = func(i int) string { return a.entries[i].Name }
	a.table.Selected = &a.selected
}

// applyFilterAndSort filters the current directory by the search query and
// orders the result: dirs first, then by the table's Sort when the user
// clicked a header.
func (a *model) applyFilterAndSort() {
	a.entries = a.filterSort(a.unfiltered)
}

func (a *model) filterSort(all []entry) []entry {
	q := strings.ToLower(a.query)
	var shown []entry
	if q == "" {
		shown = all
	} else {
		shown = make([]entry, 0, len(all))
		for _, e := range all {
			if strings.Contains(strings.ToLower(e.Name), q) {
				shown = append(shown, e)
			}
		}
	}
	sortCol, desc := "", false
	if a.table.Sort != nil {
		sortCol, desc = a.table.Sort.Column, a.table.Sort.Descending
	}
	sort.SliceStable(shown, func(i, j int) bool {
		x, y := shown[i], shown[j]
		if sortCol == "Name" { // dirs always first regardless of direction
			if x.Kind != y.Kind {
				return x.Kind == vfs.KindDir
			}
		}
		cmp := 0
		switch sortCol {
		case "Name":
			cmp = strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		case "Size":
			cmp = cmpInt64(x.Size, y.Size)
		case "Modified":
			cmp = x.ModTime.Compare(y.ModTime)
		}
		if desc {
			cmp = -cmp
		}
		return cmp < 0
	})
	return shown
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// navigate goes to dir, recording it in the history.
func (a *model) navigate(dir string) {
	if dir == a.cwd {
		return
	}
	a.cwd = dir
	a.sidebarSel = dir
	a.history = append(a.history[:a.historyPos+1], dir)
	a.historyPos = len(a.history) - 1
	a.query = ""
	a.clipPaths = nil
	a.refresh()
}

func (a *model) back() {
	if a.historyPos > 0 {
		a.historyPos--
		a.cwd = a.history[a.historyPos]
		a.sidebarSel = a.cwd
		a.query = ""
		a.refresh()
	}
}

func (a *model) forward() {
	if a.historyPos < len(a.history)-1 {
		a.historyPos++
		a.cwd = a.history[a.historyPos]
		a.sidebarSel = a.cwd
		a.query = ""
		a.refresh()
	}
}

func (a *model) selectedEntries() []entry {
	if a.selected < 0 || a.selected >= len(a.entries) {
		return nil
	}
	return []entry{a.entries[a.selected]}
}

// chosenEntry returns the entry the keys or click chose, nil for none.
func (a *model) chosenEntry() *entry {
	if e := a.selectedEntries(); len(e) > 0 {
		return &e[0]
	}
	return nil
}

// open does what double click on an entry should: enter a directory, open
// a file with its default app.
func (a *model) open(e entry) {
	if e.Kind == vfs.KindDir {
		a.navigate(e.path)
		return
	}
	full, ok := a.localPath(e.path)
	if !ok {
		return
	}
	go mygo.Shell.OpenPath(full)
}

// localPath resolves a VFS path on the local disk; backends without local
// paths have none to give the system shell.
func (a *model) localPath(p string) (string, bool) {
	local, ok := a.fs.(*vfs.Local)
	if !ok {
		return "", false
	}
	full, err := local.Resolve(p)
	if err != nil {
		a.setStatus(err.Error())
		return "", false
	}
	return full, true
}

func (a *model) setStatus(s string) {
	a.status = s
	a.statusAt = a.now()
	c := a.win
	if c != nil {
		c.Invalidate()
	}
}

// now returns the app's clock: the window's in tests, wall time otherwise.
func (a *model) now() time.Time {
	if a.testNow != nil {
		return a.testNow()
	}
	return time.Now()
}

// mkdir creates a folder in the current directory. The work runs off the
// main thread; the result comes back through the window.
func (a *model) mkdir(name string) {
	p := path.Join(a.cwd, name)
	fs := a.fs
	win := a.win
	work := func() error {
		ctx := context.Background()
		if _, err := fs.Stat(ctx, p); err == nil {
			return vfs.ErrExist
		}
		return fs.Mkdir(ctx, p)
	}
	if win == nil {
		if err := work(); err != nil {
			a.mkdirDone(name, err)
		}
		return
	}
	go func() {
		err := work()
		win.Update(func() { a.mkdirDone(name, err) })
	}()
}

func (a *model) mkdirDone(name string, err error) {
	switch {
	case err == nil:
	case errors.Is(err, vfs.ErrExist):
		a.setStatus(locales.AlreadyExists(name))
	default:
		a.setStatus(err.Error())
	}
	a.refresh()
}

// rename renames entry at index i to newName (same directory only). It
// refuses names that leave the directory or clobber an existing entry.
func (a *model) rename(i int, newName string) {
	a.renamingPath = ""
	if i < 0 || i >= len(a.entries) || newName == "" || newName == a.entries[i].Name {
		return
	}
	if err := validName(newName); err != nil {
		a.setStatus(err.Error())
		return
	}
	old := a.entries[i].path
	to := path.Join(a.cwd, newName)
	fs := a.fs
	win := a.win
	work := func() error {
		ctx := context.Background()
		if _, err := fs.Stat(ctx, to); err == nil {
			return vfs.ErrExist
		}
		return fs.Rename(ctx, old, to)
	}
	if win == nil {
		a.renameDone(a.entries[i].Name, newName, work())
		return
	}
	oldName := a.entries[i].Name
	go func() {
		err := work()
		win.Update(func() { a.renameDone(oldName, newName, err) })
	}()
}

func (a *model) renameDone(oldName, newName string, err error) {
	switch {
	case err == nil:
		a.setStatus(locales.Renamed(oldName, newName))
	case errors.Is(err, vfs.ErrExist):
		a.setStatus(locales.AlreadyExists(newName))
	default:
		a.setStatus(err.Error())
	}
	a.refresh()
}

// validName rejects names that are not a single path element.
func validName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || strings.ContainsRune(name, 0) {
		return errors.New(locales.InvalidName(name))
	}
	return nil
}

// trash moves the given VFS paths to the macOS Trash, off the main thread.
func (a *model) trash(paths []string) {
	win := a.win
	resolved := make([]string, 0, len(paths))
	for _, p := range paths {
		if full, ok := a.localPath(p); ok {
			resolved = append(resolved, full)
		}
	}
	work := func() []error {
		errs := make([]error, len(resolved))
		for i, full := range resolved {
			errs[i] = mygo.Shell.TrashItem(full)
		}
		return errs
	}
	if win == nil {
		a.trashDone(len(resolved), work())
		return
	}
	go func() {
		errs := work()
		win.Update(func() { a.trashDone(len(resolved), errs) })
	}()
}

func (a *model) trashDone(n int, errs []error) {
	failed := 0
	for _, err := range errs {
		if err != nil {
			failed++
			a.setStatus(err.Error())
		}
	}
	if n > 0 && failed == 0 {
		a.setStatus(locales.MovedToTrash(n))
	}
	a.selected = -1
	a.refresh()
}

// paste copies or moves the clipboard paths into the current directory.
func (a *model) paste() {
	if len(a.clipPaths) == 0 {
		return
	}
	paths := a.clipPaths
	cut := a.cutMode
	a.clipPaths = nil
	win := a.win
	dir := a.cwd
	go func() {
		firstErr := a.pasteInto(dir, paths, cut)
		if win == nil {
			a.pasteDone(firstErr)
			return
		}
		win.Update(func() { a.pasteDone(firstErr) })
	}()
}

// pasteSync runs the paste on the caller's goroutine (tests only).
func (a *model) pasteSync() {
	paths := a.clipPaths
	cut := a.cutMode
	a.clipPaths = nil
	a.pasteDone(a.pasteInto(a.cwd, paths, cut))
}

func (a *model) pasteInto(dir string, paths []string, cut bool) error {
	ctx := context.Background()
	var firstErr error
	for _, src := range paths {
		name := path.Base(src)
		dst := path.Join(dir, name)
		if dst == src {
			continue
		}
		if dst == src || strings.HasPrefix(dst, src+"/") {
			firstErr = errors.New(locales.CannotPasteIntoItself(name))
			continue
		}
		var err error
		if cut {
			if _, err = a.fs.Stat(ctx, dst); err == nil {
				err = vfs.ErrExist // refuse to clobber
			} else {
				err = a.fs.Rename(ctx, src, dst)
			}
		} else {
			err = copyTree(a.fs, src, dst)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (a *model) pasteDone(err error) {
	if err != nil {
		a.setStatus(err.Error())
	} else {
		a.setStatus(locales.Done())
	}
	a.refresh()
}

// copyTree copies src to dst recursively within one FS.
func copyTree(fs vfs.FS, src, dst string) error {
	ctx := context.Background()
	e, err := fs.Stat(ctx, src)
	if err != nil {
		return err
	}
	if e.Kind == vfs.KindDir {
		if err := fs.Mkdir(ctx, dst); err != nil {
			return err
		}
		children, err := fs.List(ctx, src)
		if err != nil {
			return err
		}
		for _, c := range children {
			if err := copyTree(fs, path.Join(src, c.Name), path.Join(dst, c.Name)); err != nil {
				return err
			}
		}
		return nil
	}
	r, err := fs.Open(ctx, src)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := fs.Create(ctx, dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// importFiles copies dropped/picked local files into the current directory.
func (a *model) importFiles(local []string) {
	dstDir := a.cwd
	go func() {
		ctx := context.Background()
		var firstErr error
		for _, src := range local {
			name := path.Base(src)
			dst := path.Join(dstDir, name)
			r, err := os.Open(src)
			if err == nil {
				var w io.WriteCloser
				w, err = a.fs.Create(ctx, dst)
				if err == nil {
					_, err = io.Copy(w, r)
					r.Close()
					if cerr := w.Close(); err == nil {
						err = cerr
					}
				} else {
					r.Close()
				}
			}
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
		a.win.Update(func() {
			if firstErr != nil {
				a.setStatus(firstErr.Error())
			} else {
				a.setStatus(locales.Imported(len(local)))
			}
			a.refresh()
		})
	}()
}

func fmtSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
