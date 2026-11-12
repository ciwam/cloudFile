// Package vfs defines a unified filesystem interface so that local disks,
// WebDAV and S3 backends are interchangeable. The UI only sees this
// interface.
//
// Paths are slash-separated and rooted at "/": "/" is the root, other
// paths have no trailing slash. All methods must be safe for concurrent
// use.
//
// Semantics every backend must honor:
//
//   - List: the entries of one directory, sorted by name, without "." or
//     "..". A missing path is ErrNotExist; a path that is not a directory
//     is ErrNotDir. A backend whose store paginates (S3) fetches pages
//     internally; the caller sees one complete listing.
//   - Stat: one entry's metadata; a missing path is ErrNotExist.
//   - Mkdir: creates one directory; an existing directory (or file) is
//     ErrExist. Parent directories are not created implicitly.
//   - Create: truncates or creates the file for writing. Parent
//     directories are created as needed (S3 keys have no real hierarchy;
//     WebDAV backends may MKCOL parents first).
//   - Delete: removes a file, or a directory with all of its contents.
//   - Rename: moves within this FS. An existing destination is replaced
//     when the backend can (Local), or rejected with ErrExist when it
//     cannot do so atomically — a backend must document which. Backends
//     without atomic rename (S3) implement it as copy plus delete and
//     report a delete failure as an error; the source may remain.
//
// Methods likely to need extending for cloud backends (Settle these before
// writing a second backend, since they change every caller):
//
//   - Copy, for server-side duplication (S3 CopyObject, WebDAV COPY)
//     instead of read+write through the client.
//   - A paginated List (cursor + limit) for very large directories.
//   - ETag or a content version on Entry, for optimistic concurrency
//     (If-Match) and cache validation.
//   - Content type and metadata on Create, which S3 needs at PUT time.
package vfs

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrNotExist   = errors.New("not found")
	ErrExist      = errors.New("already exists")
	ErrNotDir     = errors.New("not a directory")
	ErrPermission = errors.New("permission denied")
)

// EntryKind distinguishes files from directories.
type EntryKind int

const (
	KindFile EntryKind = iota
	KindDir
)

// Entry is one item in a listing.
type Entry struct {
	Name    string // base name, no separators
	Kind    EntryKind
	Size    int64
	ModTime time.Time
}

// FS is one filesystem root.
type FS interface {
	List(ctx context.Context, dir string) ([]Entry, error)
	Stat(ctx context.Context, path string) (Entry, error)
	Mkdir(ctx context.Context, dir string) error
	Open(ctx context.Context, path string) (io.ReadCloser, error)
	Create(ctx context.Context, path string) (io.WriteCloser, error)
	Delete(ctx context.Context, path string) error
	Rename(ctx context.Context, oldPath, newPath string) error
}
