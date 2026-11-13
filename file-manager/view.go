package main

import (
	"os"
	"strings"
	"time"

	"file-manager/locales"
	"file-manager/vfs"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// cols names the columns; their titles are localized at build time by
// columnTitles.
var cols = []ui.TableColumn{
	{Title: "Name", ID: "Name", Sortable: true},
	{Title: "Size", ID: "Size", Width: 90, Align: ui.End, Sortable: true},
	{Title: "Modified", ID: "Modified", Width: 140, Sortable: true},
}

// columnTitles returns the columns with localized titles. The IDs stay
// fixed so the user's column layout and sort survive a language switch.
func columnTitles() []ui.TableColumn {
	out := make([]ui.TableColumn, len(cols))
	copy(out, cols)
	out[0].Title = locales.ColumnName()
	out[1].Title = locales.ColumnSize()
	out[2].Title = locales.ColumnModified()
	return out
}

func (a *model) view(c *ui.Context) {
	// Window-level shortcuts.
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) { // New Folder
		a.newFolderOpen, a.newFolderName = true, ""
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		a.searchFocus.Focus(c)
	}
	// Quick Look, as Finder's space bar does: Space opens the preview of
	// the chosen entry, closes the panel when it shows, and the arrows
	// move the preview through the directory while it is up. The key's
	// auto-repeat is ignored, so holding Space does not toggle.
	if !c.FocusTakesText() && qlPanelVisible() {
		switch {
		case c.Shortcut(0, ui.KeySpace) && !c.ShortcutRepeated():
			qlHidePanel()
		case c.Shortcut(0, ui.KeyDown) && !c.ShortcutRepeated():
			a.previewNeighbor(1)
		case c.Shortcut(0, ui.KeyUp) && !c.ShortcutRepeated():
			a.previewNeighbor(-1)
		}
	} else if c.Shortcut(0, ui.KeySpace) && !c.ShortcutRepeated() && !c.FocusTakesText() {
		a.previewChosen()
	}
	if c.Shortcut(ui.Cmd, ui.KeyC) && !c.FocusTakesText() {
		if e := a.selectedEntries(); len(e) > 0 {
			a.clipPaths, a.cutMode = []string{e[0].path}, false
			a.setStatus(locales.Copied())
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyX) && !c.FocusTakesText() {
		if e := a.selectedEntries(); len(e) > 0 {
			a.clipPaths, a.cutMode = []string{e[0].path}, true
			a.setStatus(locales.CutStatus())
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyV) && !c.FocusTakesText() {
		a.paste()
	}

	ui.Split(c, &a.sidebarWidth, func() { a.sidebar(c) }, func() { a.mainPane(c) }).
		Fill().SplitReset(200)
	a.settingsDialog(c)
}

func (a *model) sidebar(c *ui.Context) {
	type place struct{ dir, label string }
	var places []place
	add := func(name mygo.PathName, label string) {
		if d, err := mygo.App.Path(name); err == nil {
			places = append(places, place{d, label})
		}
	}
	add(mygo.PathHome, locales.Home())
	add(mygo.PathDesktop, locales.Desktop())
	add(mygo.PathDocuments, locales.Documents())
	add(mygo.PathDownloads, locales.Downloads())

	sel := a.sidebarSel
	if ui.Sidebar(c, &sel, func() {
		ui.SidebarSection(c, locales.Favorites(), nil, func() {
			for _, p := range places {
				ui.SidebarItem(c, p.dir, nil, p.label)
			}
		})
	}).Width(a.sidebarWidth).Changed() {
		a.navigate(sel)
	}
}

func (a *model) mainPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		a.toolbar(c)
		ui.Divider(c)
		if a.loading && len(a.entries) == 0 {
			ui.Row(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Spinner(c).Label(locales.Loading())
				ui.Text(c, locales.Loading()).TextColor(t.TextMuted)
			})
		} else if a.loadErr != "" {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Text(c, a.loadErr).TextColor(t.Danger)
				if ui.Button(c, locales.Retry()).Clicked() {
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
		if ui.Button(c, "‹").Label(locales.Back()).Disabled(a.historyPos == 0).Clicked() {
			a.back()
		}
		if ui.Button(c, "›").Label(locales.Forward()).Disabled(a.historyPos >= len(a.history)-1).Clicked() {
			a.forward()
		}
		ui.Text(c, a.cwd).SingleLine().TextColor(t.TextMuted).Grow(1)
		ui.SearchField(c, &a.query).Label(locales.Search()).Placeholder(locales.SearchPlaceholder()).Width(180).
			Bind(&a.searchFocus).Changed()
		if ui.Checkbox(c, &a.showDots, locales.HiddenFiles()).Changed() {
			a.refresh()
		}
		if ui.Button(c, locales.NewFolder()).Clicked() {
			a.newFolderOpen, a.newFolderName = true, ""
		}
		if ui.Button(c, "").Label(locales.Settings()).Tooltip(locales.Settings()).
			Children(func() { ui.Text(c, "⚙") }).Clicked() {
			a.settingsOpen = true
		}
	})
}

func (a *model) fileTable(c *ui.Context) {
	t := c.Theme()
	a.applyFilterAndSort()
	n := len(a.entries)
	titles := columnTitles()

	// menu outcomes, noted here and applied after the table build
	var menuRename, menuDelete, menuCopy, menuCut, menuPaste, menuNewFolder, menuQuickLook bool
	var menuRow = -1

	a.table.CellMenu = func(m *ui.Menu, row, col int) {
		if row >= 0 && row < n {
			menuRow = row
			if m.Item(locales.Open()).Chosen() {
				a.open(a.entries[row])
			}
			if m.Item(locales.Rename()).Chosen() {
				menuRename = true
			}
			if m.Item(locales.Copy()).Chosen() {
				menuCopy = true
			}
			if m.Item(locales.Cut()).Chosen() {
				menuCut = true
			}
			if m.Item(locales.QuickLook()).Chosen() {
				menuQuickLook = true
			}
			m.Separator()
			if m.Item(locales.MoveToTrash()).Chosen() {
				menuDelete = true
			}
		}
		m.Separator()
		if m.Item(locales.Paste()).Disabled(len(a.clipPaths) == 0).Chosen() {
			menuPaste = true
		}
		if m.Item(locales.NewFolderItem()).Chosen() {
			menuNewFolder = true
		}
	}

	tbl := ui.Table(c, &a.table, titles, n, func(row, col int) {
		e := &a.entries[row]
		switch col {
		case 0:
			ui.Row(c).Gap(6).Children(func() {
				if ic := entryIcon(*e); ic != nil {
					ui.Icon(c, ic).Size(16, 16)
				}
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
	summary := locales.ItemsCount(len(a.entries))
	if a.selected >= 0 && a.selected < len(a.entries) {
		summary = locales.ItemsSelected(len(a.entries), a.entries[a.selected].Name)
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
	switch ui.AlertDialog(c, &a.confirmDelOpen, locales.TrashTitle(), strings.Join(names, ", "), locales.Cancel(), locales.TrashButton()) {
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
			ui.Text(c, locales.NewFolderTitle()).Bold()
			if ui.TextInput(c, &a.newFolderName).Placeholder(locales.UntitledFolder()).AutoFocus().Submitted() {
				name := a.newFolderName
				a.newFolderOpen = false
				a.mkdir(name)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, locales.Cancel()).Clicked() {
					a.newFolderOpen = false
				}
				if ui.PrimaryButton(c, locales.Create()).Clicked() {
					name := a.newFolderName
					a.newFolderOpen = false
					a.mkdir(name)
				}
			})
		})
	})
}

// settingsDialog is the settings page: the interface language, following
// the system or one of the app's languages, and the diagnostic log.
func (a *model) settingsDialog(c *ui.Context) {
	if !a.settingsOpen {
		return
	}
	ui.Modal(c, &a.settingsOpen, func() {
		t := c.Theme()
		ui.Column(c).Gap(12).Width(520).Height(480).Children(func() {
			ui.Text(c, locales.SettingsTitle()).FontSize(16).Bold()
			ui.Text(c, locales.Language()).Bold()
			ui.Text(c, locales.SettingsLanguageHint()).FontSize(12).TextColor(t.TextMuted)
			lang := a.langChoice // the radio group's value
			ui.RadioGroup(c, func() {
				if ui.Radio(c, &lang, "", locales.LanguageSystem()).Changed() {
					a.langChoice, a.langPicked = "", true
				}
				if ui.Radio(c, &lang, "en", locales.LanguageEnglish()).Changed() {
					a.langChoice, a.langPicked = "en", true
				}
				if ui.Radio(c, &lang, "zh-Hans", locales.LanguageChinese()).Changed() {
					a.langChoice, a.langPicked = "zh-Hans", true
				}
				if a.langPicked {
					lang = a.langChoice
				}
			})
			// Apply the choice: SetLocale persists it for next launches.
			if a.langPicked {
				a.langPicked = false
				if a.applyLocale != nil {
					a.applyLocale(a.langChoice)
				}
				a.langApplied = true
			}
			ui.Divider(c)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.LogTitle()).Bold().Grow(1)
				if ui.Button(c, locales.LogCopy()).Clicked() {
					logMu.Lock()
					text := strings.Join(a.logs, "\n")
					logMu.Unlock()
					mygo.Clipboard.WriteText(text)
					a.logf("log copied to clipboard (%d lines)", len(strings.Split(text, "\n")))
				}
				if ui.Button(c, locales.LogClear()).Clicked() {
					logMu.Lock()
					a.logs = nil
					logMu.Unlock()
				}
			})
			logMu.Lock()
			lines := append([]string(nil), a.logs...)
			logMu.Unlock()
			ui.Scroll(c).Height(220).Grow(1).Background(t.Surface).Radius(6).Padding(6, 8).Children(func() {
				if len(lines) == 0 {
					ui.Text(c, locales.LogEmpty()).FontSize(12).TextColor(t.TextMuted)
				}
				for _, l := range lines {
					ui.Text(c, l).FontSize(11).Font("monospace").SingleLine().Selectable()
				}
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.PrimaryButton(c, locales.Done()).Clicked() {
					a.settingsOpen = false
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

var _ = os.Getenv
