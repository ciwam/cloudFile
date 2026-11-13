package main

import (
	"log"
	"os"
	"path/filepath"

	"file-manager/locales"
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
		// Main thread: safe to read the app's language choice.
		a.langChoice = mygo.App.ChosenLocale()
		a.applyLocale = func(tag string) {
			mygo.App.SetLocale(tag)
			a.langChoice = mygo.App.ChosenLocale()
			a.logf("language set to %q", tag)
		}

		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     locales.AppName(),
			Width:     1000,
			Height:    640,
			MinWidth:  640,
			MinHeight: 400,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			a.logf("dropped %d item(s)", len(e.Paths))
			a.importFiles(e.Paths)
		})
		a.logf("app ready, locale %q, start %s", mygo.App.Locale(), a.cwd)
		a.refresh()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
