package vfs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func testLocal(t *testing.T) *Local {
	t.Helper()
	return &Local{root: t.TempDir()}
}

func TestLocalListStat(t *testing.T) {
	l := testLocal(t)
	ctx := context.Background()
	if err := os.MkdirAll(filepath.Join(l.root, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.root, "a", "f.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := l.List(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "a" || entries[0].Kind != KindDir {
		t.Fatalf("root entries = %+v", entries)
	}

	entries, err = l.List(ctx, "/a")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "b" || entries[1].Name != "f.txt" {
		t.Fatalf("a entries = %+v", entries)
	}

	e, err := l.Stat(ctx, "/a/f.txt")
	if err != nil {
		t.Fatal(err)
	}
	if e.Size != 5 || e.Kind != KindFile {
		t.Fatalf("stat = %+v", e)
	}

	if _, err := l.Stat(ctx, "/nope"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("stat missing = %v", err)
	}
}

func TestLocalWriteReadDelete(t *testing.T) {
	l := testLocal(t)
	ctx := context.Background()

	w, err := l.Create(ctx, "/dir/data.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	r, err := l.Open(ctx, "/dir/data.bin")
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if _, err := io.Copy(&got, r); err != nil {
		t.Fatal(err)
	}
	r.Close()
	if got.String() != "payload" {
		t.Fatalf("read = %q", got.String())
	}

	if err := l.Rename(ctx, "/dir/data.bin", "/dir/renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Stat(ctx, "/dir/renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if err := l.Delete(ctx, "/dir"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Stat(ctx, "/dir"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("deleted dir stat = %v", err)
	}
}

func TestLocalRejectsEscape(t *testing.T) {
	l := testLocal(t)
	if _, err := l.List(context.Background(), "/../etc"); err == nil {
		t.Fatal("path escape was allowed")
	}
}
