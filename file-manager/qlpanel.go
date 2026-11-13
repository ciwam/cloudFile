package main

import (
	"runtime"
	"sync"

	"github.com/ebitengine/purego/objc"
)

// The app's own Quick Look data source. mygo's Shell.PreviewItem drives
// the shared QLPreviewPanel with a data source of one item, so the
// panel's native arrow-key navigation has nothing to move to. This class
// holds the directory's entries instead; setting it on the panel (the
// panel is a singleton, the last setter wins) turns the arrows and Space
// into the panel's own prev/next/close.
//
// The UI pushes the entries before showing the panel; the ObjC callbacks
// read them.

var (
	qlSrcOnce sync.Once
	qlSrc     objc.ID

	qlMu          sync.Mutex
	qlPaths       []string // full local paths, in panel order
	qlPreviewPath string   // what the panel shows now
)

// qlSourceReady reports whether the data source class registered.
func qlSourceReady() bool {
	qlSrcOnce.Do(func() {
		if runtime.GOOS != "darwin" {
			return
		}
		cls, err := objc.RegisterClass(
			"FileManagerPreviewDataSource",
			objc.GetClass("NSObject"),
			[]*objc.Protocol{objc.GetProtocol("QLPreviewPanelDataSource")},
			nil,
			[]objc.MethodDef{
				{
					Cmd: objc.RegisterName("numberOfPreviewItemsInPreviewPanel:"),
					Fn: func(self objc.ID, cmd objc.SEL, panel objc.ID) int {
						return qlCount()
					},
				},
				{
					Cmd: objc.RegisterName("previewPanel:previewItemAtIndex:"),
					Fn: func(self objc.ID, cmd objc.SEL, panel objc.ID, index int) objc.ID {
						return qlFileURL(qlItemAt(index))
					},
				},
				{
					// The panel asks this when it appears, to know which
					// item to start at.
					Cmd: objc.RegisterName("previewPanel:currentPreviewItemIndex:"),
					Fn: func(self objc.ID, cmd objc.SEL, panel objc.ID) int {
						return qlCurrentIndex()
					},
				},
			},
		)
		if err != nil {
			return
		}
		qlSrc = objc.ID(cls).Send(objc.RegisterName("new"))
	})
	return qlSrc != 0
}

// qlSetItems tells the data source what to preview: the directory's
// entries as full local paths, and which one is current.
func qlSetItems(paths []string, current string) {
	qlMu.Lock()
	qlPaths = paths
	qlPreviewPath = current
	qlMu.Unlock()
}

func qlCount() int {
	qlMu.Lock()
	defer qlMu.Unlock()
	return len(qlPaths)
}

func qlCurrentIndex() int {
	qlMu.Lock()
	defer qlMu.Unlock()
	for i, p := range qlPaths {
		if p == qlPreviewPath {
			return i
		}
	}
	return 0
}

func qlItemAt(i int) string {
	qlMu.Lock()
	defer qlMu.Unlock()
	if i < 0 || i >= len(qlPaths) {
		return ""
	}
	return qlPaths[i]
}

func qlCurrentPath() string {
	qlMu.Lock()
	defer qlMu.Unlock()
	return qlPreviewPath
}

// qlFileURL makes a retained NSURL for path, 0 on failure.
func qlFileURL(path string) objc.ID {
	if path == "" {
		return 0
	}
	cls := objc.GetClass("NSURL")
	if cls == 0 {
		return 0
	}
	u := objc.ID(cls).Send(objc.RegisterName("fileURLWithPath:"), path)
	// Retain so the panel holds a valid URL after this frame; balanced by
	// replacing the item set (leaked by one release per item is negligible
	// for a listing's lifetime).
	if u == 0 {
		return 0
	}
	u.Send(objc.RegisterName("retain"))
	return u
}

// qlPanel returns the shared QLPreviewPanel as an ID, 0 when the class
// is not visible.
func qlPanel() objc.ID {
	cls := objc.GetClass("QLPreviewPanel")
	if cls == 0 {
		return 0
	}
	return objc.ID(cls).Send(objc.RegisterName("sharedPreviewPanel"))
}

// qlShowPanel sets our data source on the panel and shows it at the
// current item. Falling back to mygo's PreviewItem is up to the caller
// when this returns false.
func qlShowPanel() bool {
	if !qlSourceReady() {
		return false
	}
	panel := qlPanel()
	if panel == 0 {
		return false
	}
	panel.Send(objc.RegisterName("setDataSource:"), qlSrc)
	panel.Send(objc.RegisterName("reloadData"))
	panel.Send(objc.RegisterName("makeKeyAndOrderFront:"), 0)
	return true
}

// qlHidePanel closes the Quick Look panel, as Space does again in Finder.
func qlHidePanel() {
	if panel := qlPanel(); panel != 0 {
		panel.Send(objc.RegisterName("orderOut:"), 0)
	}
}

// qlPanelVisible reports whether the panel is on screen.
func qlPanelVisible() bool {
	panel := qlPanel()
	if panel == 0 {
		return false
	}
	return objc.Send[bool](panel, objc.RegisterName("isVisible"))
}
