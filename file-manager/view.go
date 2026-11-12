package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"file-manager/vfs"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/sfsymbols"
	"github.com/egoist/mygo/ui"
)

var cols = []ui.TableColumn{
	{Title: "Name", Sortable: true},
	{Title: "Size", Width: 90, Align: ui.End, Sortable: true},
	{Title: "Modified", Width: 140, Sortable: true},
}

func (a *model) view(c *ui.Context) {
	// Window-level shortcuts.
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) { // New Folder
		a.newFolderOpen, a.newFolderName = true, ""
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		a.searchFocus.Focus(c)
	}
	if c.Shortcut(ui.Cmd, ui.KeyC) && !c.FocusTakesText() {
		if e := a.selectedEntries(); len(e) > 0 {
			a.clipPaths, a.cutMode = []string{e[0].path}, false
			a.setStatus("Copied")
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyX) && !c.FocusTakesText() {
		if e := a.selectedEntries(); len(e) > 0 {
			a.clipPaths, a.cutMode = []string{e[0].path}, true
			a.setStatus("Cut")
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyV) && !c.FocusTakesText() {
		a.paste()
	}

	ui.Split(c, &a.sidebarWidth, func() { a.sidebar(c) }, func() { a.mainPane(c) }).
		Fill().SplitReset(200)
}

func (a *model) sidebar(c *ui.Context) {
	type place struct{ dir, label, sym string }
	var places []place
	add := func(name mygo.PathName, label, sym string) {
		if d, err := mygo.App.Path(name); err == nil {
			places = append(places, place{d, label, sym})
		}
	}
	add(mygo.PathHome, "Home", "house")
	add(mygo.PathDesktop, "Desktop", "menubar.dock.rectangle")
	add(mygo.PathDocuments, "Documents", "doc.text")
	add(mygo.PathDownloads, "Downloads", "arrow.down.circle")

	sel := a.sidebarSel
	if ui.Sidebar(c, &sel, func() {
		ui.SidebarSection(c, "Favorites", nil, func() {
			for _, p := range places {
				ui.SidebarItem(c, p.dir, nil, p.label)
			}
		})
	}).Width(a.sidebarWidth).Changed() {
		a.navigate(sel)
	}
	_ = sfsymbols.Symbol
}

func (a *model) mainPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		a.toolbar(c)
		ui.Divider(c)
		if a.loading && len(a.entries) == 0 {
			ui.Row(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Spinner(c).Label("Loading")
				ui.Text(c, "Loading…").TextColor(t.TextMuted)
			})
		} else if a.loadErr != "" {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Text(c, a.loadErr).TextColor(t.Danger)
				if ui.Button(c, "Retry").Clicked() {
					a.refresh()
				}
			})
		} else {
			a.fileTable(c)
		}
		a.statusBar(c)
	})
}

func (a *model) toolbar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(8).Padding(8, 12).Children(func() {
		if ui.Button(c, "‹").Label("Back").Disabled(a.historyPos == 0).Clicked() {
			a.back()
		}
		if ui.Button(c, "›").Label("Forward").Disabled(a.historyPos >= len(a.history)-1).Clicked() {
			a.forward()
		}
		ui.Text(c, a.cwd).SingleLine().TextColor(t.TextMuted).Grow(1)
		ui.SearchField(c, &a.query).Label("Search").Placeholder("Search").Width(180).
			Bind(&a.searchFocus).Changed()
		if ui.Checkbox(c, &a.showDots, "Hidden").Changed() {
			a.refresh()
		}
		if ui.Button(c, "New Folder").Clicked() {
			a.newFolderOpen, a.newFolderName = true, ""
		}
	})
}

func (a *model) fileTable(c *ui.Context) {
	t := c.Theme()
	a.applyFilterAndSort()
	n := len(a.entries)

	// menu outcomes, noted here and applied after the table build
	var menuRename, menuDelete, menuCopy, menuCut, menuPaste, menuNewFolder, menuQuickLook bool
	var menuRow = -1

	a.table.CellMenu = func(m *ui.Menu, row, col int) {
		if row >= 0 && row < n {
			menuRow = row
			if m.Item("Open").Chosen() {
				a.open(a.entries[row])
			}
			if m.Item("Rename").Chosen() {
				menuRename = true
			}
			if m.Item("Copy").Chosen() {
				menuCopy = true
			}
			if m.Item("Cut").Chosen() {
				menuCut = true
			}
			if m.Item("Quick Look").Chosen() {
				menuQuickLook = true
			}
			m.Separator()
			if m.Item("Move to Trash").Chosen() {
				menuDelete = true
			}
		}
		m.Separator()
		if m.Item("Paste").Disabled(len(a.clipPaths) == 0).Chosen() {
			menuPaste = true
		}
		if m.Item("New Folder").Chosen() {
			menuNewFolder = true
		}
	}

	tbl := ui.Table(c, &a.table, cols, n, func(row, col int) {
		e := &a.entries[row]
		switch col {
		case 0:
			ui.Row(c).Gap(6).Children(func() {
				ui.Text(c, iconGlyph(*e)).Width(20).TextColor(t.TextMuted)
				if a.renamingPath == e.path {
					if ui.EditableText(c, &e.Name).Changed() {
						a.rename(row, e.Name)
					}
				} else {
					ui.Text(c, e.Name).SingleLine()
				}
			})
		case 1:
			if e.Kind == vfs.KindFile {
				ui.Textf(c, "%s", fmtSize(e.Size)).FontFeatures("tnum").TextColor(t.TextMuted)
			}
		case 2:
			ui.Text(c, e.ModTime.Format("2006-01-02 15:04")).FontFeatures("tnum").TextColor(t.TextMuted)
		}
	}).Grow(1)

	// Drag selection out to Finder (copy semantic; MyGo never deletes).
	a.table.DragRows = func(rows []int) any {
		if len(rows) == 0 {
			return nil
		}
		paths := make([]string, 0, len(rows))
		for _, r := range rows {
			if r >= 0 && r < n {
				if full, ok := a.localPath(a.entries[r].path); ok {
					paths = append(paths, full)
				}
			}
		}
		if len(paths) == 0 {
			return nil
		}
		return paths
	}

	if tbl.Submitted() && a.selected >= 0 && a.selected < n {
		a.open(a.entries[a.selected])
	}
	if tbl.Changed() {
		// column layout / sort changed by the user; sorting applies next frame
	}

	if menuRename && menuRow >= 0 && menuRow < n {
		a.renamingPath = a.entries[menuRow].path
	}
	if menuCopy || menuCut {
		if menuRow >= 0 && menuRow < n {
			a.clipPaths, a.cutMode = []string{a.entries[menuRow].path}, menuCut
		}
	}
	if menuPaste {
		a.paste()
	}
	if menuNewFolder {
		a.newFolderOpen, a.newFolderName = true, ""
	}
	if menuDelete && menuRow >= 0 && menuRow < n {
		p := a.entries[menuRow].path
		a.trash([]string{p})
	}
	if menuQuickLook && menuRow >= 0 && menuRow < n {
		if full, ok := a.localPath(a.entries[menuRow].path); ok {
			go mygo.Shell.PreviewItem(full)
		}
	}

	a.deleteDialog(c)
	a.newFolderDialog(c)
}

func (a *model) statusBar(c *ui.Context) {
	t := c.Theme()
	summary := fmt.Sprintf("%d items", len(a.entries))
	if a.selected >= 0 && a.selected < len(a.entries) {
		summary = fmt.Sprintf("%d items — %s selected", len(a.entries), a.entries[a.selected].Name)
	}
	if a.status != "" && a.now().Sub(a.statusAt) < 5*time.Second {
		summary = a.status
		c.After(1 * time.Second)
	}
	ui.Row(c).Padding(4, 12).Children(func() {
		ui.Text(c, summary).FontSize(12).TextColor(t.TextMuted).SingleLine()
	})
}

func (a *model) deleteDialog(c *ui.Context) {
	if !a.confirmDelOpen || len(a.confirmDel) == 0 {
		return
	}
	names := make([]string, len(a.confirmDel))
	for i, p := range a.confirmDel {
		names[i] = baseName(p)
	}
	switch ui.AlertDialog(c, &a.confirmDelOpen, "Move to Trash?", strings.Join(names, ", "), "Cancel", "Move to Trash") {
	case 1:
		paths := a.confirmDel
		a.confirmDel = nil
		a.trash(paths)
	case ui.AlertDismissed:
		a.confirmDel = nil
	}
}

func (a *model) newFolderDialog(c *ui.Context) {
	if !a.newFolderOpen {
		return
	}
	ui.Modal(c, &a.newFolderOpen, func() {
		ui.Column(c).Gap(12).Children(func() {
			ui.Text(c, "New Folder").Bold()
			if ui.TextInput(c, &a.newFolderName).Placeholder("untitled folder").AutoFocus().Submitted() {
				name := a.newFolderName
				a.newFolderOpen = false
				a.mkdir(name)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					a.newFolderOpen = false
				}
				if ui.PrimaryButton(c, "Create").Clicked() {
					name := a.newFolderName
					a.newFolderOpen = false
					a.mkdir(name)
				}
			})
		})
	})
}

func baseName(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// iconGlyph returns a simple text glyph per entry kind. Real file-type
// icons use mygo.Shell.FileIcon, drawn per visible row.
func iconGlyph(e entry) string {
	if e.Kind == vfs.KindDir {
		return "📁"
	}
	return extGlyph(strings.ToLower(filepath.Ext(e.Name)))
}

func extGlyph(ext string) string {
	switch ext {
	case ".txt", ".md", ".log":
		return "📄"
	case ".png", ".jpg", ".jpeg", ".gif", ".heic", ".webp", ".tiff":
		return "🖼"
	case ".go", ".py", ".js", ".ts", ".rs", ".c", ".h", ".sh":
		return "📜"
	case ".zip", ".tar", ".gz", ".bz2", ".xz", ".dmg":
		return "📦"
	case ".app":
		return "🚀"
	case ".pdf":
		return "📕"
	case ".mp3", ".m4a", ".flac", ".wav":
		return "🎵"
	case ".mp4", ".mov", ".mkv":
		return "🎬"
	default:
		return "📄"
	}
}

var _ = os.Getenv
