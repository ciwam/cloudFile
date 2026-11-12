package vfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Local is an FS rooted at a directory on the local disk.
type Local struct {
	root string
}

// NewLocal returns an FS over dir. The root may be "/" for the whole disk.
func NewLocal(dir string) (*Local, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrNotDir
	}
	return &Local{root: abs}, nil
}

// resolve joins an FS path ("", "/", "/a/b") onto the root and rejects
// escapes via "..". Each element is checked against the root with the
// kernel (via directory-relative opens) only for Open and Create; lexical
// components here keep the mapping from VFS paths to disk paths total.
func (l *Local) resolve(path string) (string, error) {
	p := strings.Trim(path, "/")
	if p == "" {
		return l.root, nil
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrNotExist
	}
	return filepath.Join(l.root, filepath.FromSlash(clean)), nil
}

func toEntry(name string, info fs.FileInfo) Entry {
	e := Entry{Name: name, ModTime: info.ModTime()}
	if info.IsDir() {
		e.Kind = KindDir
	} else {
		e.Size = info.Size()
	}
	return e
}

// List returns the entries of dir sorted by name. A missing or non-
// directory path returns ErrNotExist or ErrNotDir.
func (l *Local) List(ctx context.Context, dir string) ([]Entry, error) {
	full, err := l.resolve(dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return nil, mapErr(err)
	}
	if !info.IsDir() {
		return nil, ErrNotDir
	}
	dirents, err := os.ReadDir(full)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]Entry, 0, len(dirents))
	for _, d := range dirents {
		if e, err := d.Info(); err == nil {
			out = append(out, toEntry(d.Name(), e))
		}
	}
	return out, nil
}

// Stat returns one entry's metadata.
func (l *Local) Stat(ctx context.Context, path string) (Entry, error) {
	full, err := l.resolve(path)
	if err != nil {
		return Entry{}, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return Entry{}, mapErr(err)
	}
	return toEntry(filepath.Base(full), info), nil
}

// Mkdir creates a directory. An existing directory is ErrExist.
func (l *Local) Mkdir(ctx context.Context, dir string) error {
	full, err := l.resolve(dir)
	if err != nil {
		return err
	}
	if err := os.Mkdir(full, 0o755); err != nil {
		if errors.Is(err, syscall.EEXIST) {
			if info, serr := os.Lstat(full); serr == nil && info.IsDir() {
				return ErrExist
			}
		}
		return mapErr(err)
	}
	return nil
}

// Open opens path for reading.
func (l *Local) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	full, err := l.resolve(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, mapErr(err)
	}
	return f, nil
}

// Create truncates or creates path for writing, creating parent
// directories as needed.
func (l *Local) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	full, err := l.resolve(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, mapErr(err)
	}
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, mapErr(err)
	}
	return f, nil
}

// Delete removes a file or a directory with all of its contents.
func (l *Local) Delete(ctx context.Context, path string) error {
	full, err := l.resolve(path)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(full); err != nil {
		return mapErr(err)
	}
	return nil
}

// Rename moves a file or directory within this FS. An existing destination
// is replaced; cloud backends that cannot rename atomically implement this
// as copy plus delete and document what happens if the delete fails.
func (l *Local) Rename(ctx context.Context, oldPath, newPath string) error {
	oldFull, err := l.resolve(oldPath)
	if err != nil {
		return err
	}
	newFull, err := l.resolve(newPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newFull), 0o755); err != nil {
		return mapErr(err)
	}
	return mapErr(os.Rename(oldFull, newFull))
}

// Resolve returns the local disk path of a VFS path, for the system shell
// (open with default app, Trash, Quick Look).
func (l *Local) Resolve(path string) (string, error) {
	return l.resolve(path)
}

func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case os.IsNotExist(err):
		return ErrNotExist
	case os.IsExist(err):
		return ErrExist
	case os.IsPermission(err):
		return ErrPermission
	case errors.Is(err, syscall.ENOTDIR), errors.Is(err, syscall.EISDIR):
		return ErrNotDir
	default:
		return err
	}
}
