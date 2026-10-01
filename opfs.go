//go:build wasm

package opfs

import (
	"syscall/js"

	"webtyp.com/await"
	"webtyp.com/files"
	"webtyp.com/fmt"
)

// FS is a directory of the origin's private file system, read and written as whole files.
type FS struct {
	dir js.Value // FileSystemDirectoryHandle
}

var (
	_ files.ReadWriter = (*FS)(nil)
	_ files.Appender   = (*FS)(nil)
)

// ChunkSize is how many bytes cross between JavaScript and Go at a time, so reading or writing
// a file needs only the file plus one chunk of memory.
const ChunkSize = 8 << 20

const (
	errUnavailable = "opfs: this browser has no Origin Private File System"
	errInvalidPath = "opfs: invalid path %q"
	errBrowser     = "opfs: %v"
	notFoundName   = "NotFoundError" // the DOMException name of a missing file or directory
)

// Open returns the directory dir under the origin's OPFS root, creating it (and its parents)
// when needed; "" is the root itself. Call it, and every method, from a goroutine: they block
// on the browser's promises (see docs/ARCHITECTURE.md, "Blocking").
func Open(dir string) (*FS, error) {
	storage := js.Global().Get("navigator").Get("storage")
	if storage.IsUndefined() || storage.Get("getDirectory").Type() != js.TypeFunction {
		return nil, fmt.Err(errUnavailable)
	}
	root, err := await.Promise(storage.Call("getDirectory"))
	if err != nil {
		return nil, fmt.Errf(errBrowser, err)
	}
	if dir == "" {
		return &FS{dir: root}, nil
	}
	segments, ok := splitPath(dir)
	if !ok {
		return nil, fmt.Errf(errInvalidPath, dir)
	}
	curr := root
	opt := js.Global().Get("Object").New()
	opt.Set("create", true)
	for _, seg := range segments {
		next, err := await.Promise(curr.Call("getDirectoryHandle", seg, opt))
		if err != nil {
			return nil, fmt.Errf(errBrowser, err)
		}
		curr = next
	}
	return &FS{dir: curr}, nil
}
