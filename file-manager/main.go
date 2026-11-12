package main

import (
	"log"
	"os"
	"path/filepath"

	"file-manager/vfs"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	local, err := vfs.NewLocal("/")
	if err != nil {
		log.Fatal(err)
	}

	a := newModel(local, filepath.ToSlash(home))
	// Seed the sidebar entry list without navigation churn.
	a.history = []string{a.cwd}

	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "Files",
			Width:     1000,
			Height:    640,
			MinWidth:  640,
			MinHeight: 400,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			a.importFiles(e.Paths)
		})
		a.refresh()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
