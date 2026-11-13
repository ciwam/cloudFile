package main

import (
	"path/filepath"
	"strings"
	"sync"

	"file-manager/vfs"

	"github.com/egoist/mygo/ui"
)

// iconSVGs parses the embedded Material Icon Theme SVGs once.
var (
	iconSVGs   = map[string]*ui.SVG{}
	iconSVGsMu sync.Mutex
)

// iconFor parses and returns the SVG of name, nil when unknown.
func iconFor(name string) *ui.SVG {
	iconSVGsMu.Lock()
	defer iconSVGsMu.Unlock()
	if s, ok := iconSVGs[name]; ok {
		return s
	}
	data, err := iconFS.ReadFile("assets/icons/" + name + ".svg")
	if err != nil {
		iconSVGs[name] = nil
		return nil
	}
	s := ui.MustParseSVG(data)
	iconSVGs[name] = s
	return s
}

// entryIcon returns the Material Icon Theme SVG for an entry: a folder
// icon for directories, else the icon of its extension, of its file name
// (Makefile, Dockerfile), or the plain file icon.
func entryIcon(e entry) *ui.SVG {
	if e.Kind == vfs.KindDir {
		return iconFor("folder")
	}
	lower := strings.ToLower(e.Name)
	if name, ok := fileNameIcon[lower]; ok {
		return iconFor(name)
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(lower)), ".")
	if name, ok := extIcon[ext]; ok {
		return iconFor(name)
	}
	return iconFor("file")
}
