package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"file-manager/vfs"

	"github.com/egoist/mygo/ui"
)

// testModel builds a model over a temp directory with a window-less view
// tester, as a user would click and type. The model's own refresh is
// asynchronous (Window.Update), so tests set entries directly and call
// loadEntries for the synchronous part of the load.
func testModel(t *testing.T) (*model, *ui.Tester, string) {
	t.Helper()
	root := t.TempDir()
	local, err := vfs.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	a := newModel(local, "/")
	a.history = []string{a.cwd}
	tt := ui.NewTester(a.view, 1000, 640)
	a.testNow = func() time.Time { return tt.Now() }
	return a, tt, root
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestShowsEntries(t *testing.T) {
	a, tt, root := testModel(t)
	write(t, root, "hello.txt", "hello world")
	write(t, root, "notes.md", "# notes")
	a.refresh()
	tt.Frame()
	if !tt.HasText("hello.txt") || !tt.HasText("notes.md") {
		t.Fatalf("entries missing, texts: %v", tt.Texts())
	}
}

func TestHiddenFilesToggle(t *testing.T) {
	a, tt, root := testModel(t)
	write(t, root, ".hidden", "x")
	write(t, root, "visible.txt", "y")
	a.refresh()
	tt.Frame()
	if tt.HasText(".hidden") {
		t.Fatal("hidden file shown by default")
	}
	a.showDots = true
	a.refresh()
	tt.Frame()
	if !tt.HasText(".hidden") {
		t.Fatal("hidden file not shown after toggle")
	}
}

func TestSearchFilters(t *testing.T) {
	a, tt, root := testModel(t)
	write(t, root, "alpha.txt", "1")
	write(t, root, "beta.txt", "2")
	a.refresh()
	tt.Frame()
	a.query = "beta"
	a.refresh()
	tt.Frame()
	if tt.HasText("alpha.txt") {
		t.Fatal("search did not filter alpha.txt")
	}
	if !tt.HasText("beta.txt") {
		t.Fatal("search filtered out beta.txt")
	}
}

func TestDotsFirst(t *testing.T) {
	a, _, root := testModel(t)
	write(t, root, "z.txt", "z")
	if err := os.Mkdir(filepath.Join(root, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	if len(a.entries) != 2 || a.entries[0].Name != "adir" {
		t.Fatalf("entries = %+v, want dir first", a.entries)
	}
}

func TestCopyTree(t *testing.T) {
	a, _, root := testModel(t)
	write(t, root, "src/file.txt", "data")
	write(t, root, "src/sub/deep.txt", "deep")
	if err := copyTree(a.fs, "/src", "/dst"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "dst", "file.txt"))
	if err != nil || string(b) != "data" {
		t.Fatalf("copied file = %q, %v", b, err)
	}
	b, err = os.ReadFile(filepath.Join(root, "dst", "sub", "deep.txt"))
	if err != nil || string(b) != "deep" {
		t.Fatalf("copied nested file = %q, %v", b, err)
	}
}

func TestCutPaste(t *testing.T) {
	a, _, root := testModel(t)
	write(t, root, "src/moved.txt", "m")
	if err := os.Mkdir(filepath.Join(root, "dst"), 0o755); err != nil {
		t.Fatal(err)
	}
	a.clipPaths, a.cutMode = []string{"/src/moved.txt"}, true
	a.cwd = "/dst"
	a.pasteSync()
	if _, err := os.Stat(filepath.Join(root, "dst", "moved.txt")); err != nil {
		t.Fatalf("cut+paste failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "moved.txt")); !os.IsNotExist(err) {
		t.Fatal("source still exists after cut+paste")
	}
}

func TestCutPasteRefusesClobber(t *testing.T) {
	a, _, root := testModel(t)
	write(t, root, "src/moved.txt", "new")
	write(t, root, "dst/moved.txt", "old")
	a.clipPaths, a.cutMode = []string{"/src/moved.txt"}, true
	a.cwd = "/dst"
	a.pasteSync()
	b, err := os.ReadFile(filepath.Join(root, "dst", "moved.txt"))
	if err != nil || string(b) != "old" {
		t.Fatalf("paste clobbered the destination: %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "moved.txt")); err != nil {
		t.Fatal("source removed despite failed paste")
	}
}

func TestRenameRejectsBadNames(t *testing.T) {
	a, _, root := testModel(t)
	write(t, root, "old.txt", "x")
	write(t, root, "taken.txt", "y")
	a.refresh()

	a.rename(0, "../escape.txt")
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("rename escaped the directory")
	}
	a.rename(0, "taken.txt")
	b, err := os.ReadFile(filepath.Join(root, "taken.txt"))
	if err != nil || string(b) != "y" {
		t.Fatalf("rename clobbered an existing file: %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(root, "old.txt")); err != nil {
		t.Fatal("source removed despite failed rename")
	}
}

func TestSearchClearRestores(t *testing.T) {
	a, tt, root := testModel(t)
	write(t, root, "alpha.txt", "1")
	write(t, root, "beta.txt", "2")
	a.refresh()
	tt.Frame()
	a.query = "beta"
	tt.Frame()
	if tt.HasText("alpha.txt") {
		t.Fatal("filter did not apply")
	}
	a.query = ""
	tt.Frame()
	if !tt.HasText("alpha.txt") || !tt.HasText("beta.txt") {
		t.Fatal("clearing the search did not restore the list")
	}
}

func TestRename(t *testing.T) {
	a, _, root := testModel(t)
	write(t, root, "old.txt", "x")
	a.refresh()
	a.rename(0, "new.txt")
	if _, err := os.Stat(filepath.Join(root, "new.txt")); err != nil {
		t.Fatalf("rename failed: %v", err)
	}
	if a.renamingPath != "" {
		t.Fatal("renamingPath not cleared")
	}
}

func TestFmtSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
	}
	for _, c := range cases {
		if got := fmtSize(c.n); got != c.want {
			t.Errorf("fmtSize(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestToolbarButtons(t *testing.T) {
	a, tt, root := testModel(t)
	write(t, root, "f.txt", "1")
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	a.refresh()
	tt.Frame()
	// navigate into a subdirectory through the model, then Back
	a.navigate("/sub")
	a.refresh()
	tt.Frame()
	if a.cwd != "/sub" {
		t.Fatalf("cwd = %q", a.cwd)
	}
	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	if a.cwd != "/" {
		t.Fatalf("after Back cwd = %q, want /", a.cwd)
	}
	if err := tt.Click("Forward"); err != nil {
		t.Fatal(err)
	}
	if a.cwd != "/sub" {
		t.Fatalf("after Forward cwd = %q, want /sub", a.cwd)
	}
}

func TestStatusBarShowsCount(t *testing.T) {
	a, tt, root := testModel(t)
	write(t, root, "one.txt", "1")
	write(t, root, "two.txt", "2")
	a.refresh()
	tt.Frame()
	if !tt.HasText("2 items") {
		t.Fatalf("status bar text missing, texts: %v", tt.Texts())
	}
}

func TestStatusExpires(t *testing.T) {
	a, tt, root := testModel(t)
	write(t, root, "f.txt", "1")
	a.refresh()
	tt.Frame()
	a.setStatus("hello")
	tt.Frame()
	if !tt.HasText("hello") {
		t.Fatalf("status not shown, texts: %v", tt.Texts())
	}
	tt.Advance(6 * time.Second)
	if tt.HasText("hello") {
		t.Fatal("status did not expire")
	}
}

func TestSettingsLanguageDialog(t *testing.T) {
	a, tt, root := testModel(t)
	write(t, root, "f.txt", "1")
	a.refresh()
	tt.Frame()

	// Open settings from the toolbar.
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Language") {
		t.Fatalf("settings dialog not shown, texts: %v", tt.Texts())
	}

	// The radios carry the language names in themselves.
	if err := tt.Click("简体中文"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.langChoice != "zh-Hans" || !a.langApplied {
		t.Fatalf("langChoice = %q applied=%v, want zh-Hans applied", a.langChoice, a.langApplied)
	}

	// Back to system.
	if err := tt.Click("System"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.langChoice != "" || !a.langApplied {
		t.Fatalf("langChoice = %q applied=%v, want empty (system) applied", a.langChoice, a.langApplied)
	}
}

func TestNavigateAfterPressDoesNotCrash(t *testing.T) {
	// Double-clicking a folder navigates while the engine may still hold
	// the press's state; the ListState must not be zeroed between frames
	// (regression: SIGSEGV in ui.scrollDragSelect).
	a, tt, root := testModel(t)
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, root, "f.txt", "1")
	a.refresh()
	tt.Frame()

	// Select a row (a press), then navigate as a double click would.
	a.selected = 0
	a.navigate("/sub")
	a.refresh()
	tt.Frame()
	tt.Frame()
	tt.Frame()
	if a.cwd != "/sub" {
		t.Fatalf("cwd = %q", a.cwd)
	}

	// And back again, the other direction of the same gesture.
	if err := tt.Click("Back"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.cwd != "/" {
		t.Fatalf("cwd = %q after Back", a.cwd)
	}
}
