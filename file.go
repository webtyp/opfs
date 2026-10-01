//go:build wasm

package opfs

import (
	"syscall/js"

	"webtyp.com/await"
	"webtyp.com/files"
	"webtyp.com/fmt"
)

// parent walks all segments but the last with getDirectoryHandle(name, {create}).
func (f *FS) parent(segments []string, create bool) (js.Value, error) {
	curr := f.dir
	if len(segments) <= 1 {
		return curr, nil
	}
	opt := js.Global().Get("Object").New()
	opt.Set("create", create)
	for _, seg := range segments[:len(segments)-1] {
		next, err := await.Promise(curr.Call("getDirectoryHandle", seg, opt))
		if err != nil {
			return js.Undefined(), browserError(err)
		}
		curr = next
	}
	return curr, nil
}

// handle validates the path, finds the parent directory, and calls getFileHandle(last, {create}).
func (f *FS) handle(path string, create bool) (js.Value, error) {
	segments, ok := splitPath(path)
	if !ok {
		return js.Undefined(), fmt.Errf(errInvalidPath, path)
	}
	parentDir, err := f.parent(segments, create)
	if err != nil {
		return js.Undefined(), err
	}
	opt := js.Global().Get("Object").New()
	opt.Set("create", create)
	h, err := await.Promise(parentDir.Call("getFileHandle", segments[len(segments)-1], opt))
	if err != nil {
		return js.Undefined(), browserError(err)
	}
	return h, nil
}

// browserError maps a rejection to files.ErrNotExist (unwrapped, as the contract requires) when
// the browser says the file or a directory is missing, and wraps anything else.
func browserError(err error) error {
	if fmt.HasPrefix(err.Error(), notFoundName) {
		return files.ErrNotExist
	}
	return fmt.Errf(errBrowser, err)
}

// ReadFile reads the entire file named by path.
func (f *FS) ReadFile(path string) ([]byte, error) {
	h, err := f.handle(path, false)
	if err != nil {
		return nil, err
	}
	file, err := await.Promise(h.Call("getFile"))
	if err != nil {
		return nil, browserError(err)
	}
	size := file.Get("size").Int()
	out := make([]byte, size)
	uint8Array := js.Global().Get("Uint8Array")
	for off := 0; off < size; off += ChunkSize {
		end := min(off+ChunkSize, size)
		ab, err := await.Promise(file.Call("slice", off, end).Call("arrayBuffer"))
		if err != nil {
			return nil, browserError(err)
		}
		js.CopyBytesToGo(out[off:end], uint8Array.New(ab))
	}
	return out, nil
}

// WriteFile writes data to the file named by path, creating it or truncating it if it exists.
func (f *FS) WriteFile(path string, data []byte) error {
	h, err := f.handle(path, true)
	if err != nil {
		return err
	}
	opt := js.Global().Get("Object").New()
	opt.Set("keepExistingData", false)
	w, err := await.Promise(h.Call("createWritable", opt))
	if err != nil {
		return browserError(err)
	}

	if len(data) > 0 {
		bufLen := min(ChunkSize, len(data))
		chunk := js.Global().Get("Uint8Array").New(bufLen)
		for off := 0; off < len(data); off += ChunkSize {
			end := min(off+ChunkSize, len(data))
			n := end - off
			view := chunk
			if n < bufLen {
				view = chunk.Call("subarray", 0, n)
			}
			js.CopyBytesToJS(view, data[off:end])
			_, writeErr := await.Promise(w.Call("write", view))
			if writeErr != nil {
				w.Call("abort")
				return browserError(writeErr)
			}
		}
	}

	_, closeErr := await.Promise(w.Call("close"))
	if closeErr != nil {
		w.Call("abort")
		return browserError(closeErr)
	}
	return nil
}

// AppendFile appends data to the file named by path, creating it if it does not exist.
func (f *FS) AppendFile(path string, data []byte) error {
	h, err := f.handle(path, true)
	if err != nil {
		return err
	}
	opt := js.Global().Get("Object").New()
	opt.Set("keepExistingData", true)
	w, err := await.Promise(h.Call("createWritable", opt))
	if err != nil {
		return browserError(err)
	}

	file, err := await.Promise(h.Call("getFile"))
	if err != nil {
		w.Call("abort")
		return browserError(err)
	}
	size := file.Get("size").Int()
	_, seekErr := await.Promise(w.Call("seek", size))
	if seekErr != nil {
		w.Call("abort")
		return browserError(seekErr)
	}

	if len(data) > 0 {
		bufLen := min(ChunkSize, len(data))
		chunk := js.Global().Get("Uint8Array").New(bufLen)
		for off := 0; off < len(data); off += ChunkSize {
			end := min(off+ChunkSize, len(data))
			n := end - off
			view := chunk
			if n < bufLen {
				view = chunk.Call("subarray", 0, n)
			}
			js.CopyBytesToJS(view, data[off:end])
			_, writeErr := await.Promise(w.Call("write", view))
			if writeErr != nil {
				w.Call("abort")
				return browserError(writeErr)
			}
		}
	}

	_, closeErr := await.Promise(w.Call("close"))
	if closeErr != nil {
		w.Call("abort")
		return browserError(closeErr)
	}
	return nil
}
