---
PLAN: "feat: OPFS as files.Reader, files.Writer and files.Appender"
TAG: v0.1.0
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 1930191296648010563
PR: https://github.com/webtyp/opfs/pull/1
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `webtyp/opfs` v0.1.0

## Read first

- [docs/ARCHITECTURE.md](ARCHITECTURE.md) is the specification (how it reads and writes, paths,
  blocking). It is already written and correct; this plan implements it.
- Do **not** ask questions. If something in this plan cannot be done, write what and why in a
  section `## Executor notes` at the end of this file, do everything else, and open the PR.
- Do **not** edit this file's frontmatter.

## Development rules

- The implementation is browser-only: every file that imports `syscall/js` starts with
  `//go:build wasm`. Add one file **without** a build tag, `doc.go`, holding only the package
  comment, so `go vet ./...` and `go build ./...` work natively.
- Do not import `fmt` (stdlib), `errors`, `strings`, `strconv`. Use `webtyp.com/fmt`
  (`fmt.Err`, `fmt.Errf`, `fmt.HasPrefix`, `fmt.Split`… — `fmt.Split` has legacy quirks, so split
  paths by scanning bytes yourself, see Stage 2).
- No `map[K]V`.
- Every error text is a named unexported constant (listed below).
- Tests live in `tests/` (`package tests`), all with `//go:build wasm`, and use only the exported
  API. They run in a real browser with `wasmbrowsertest`:
  `GOOS=js GOARCH=wasm go test -exec wasmbrowsertest ./...` (install with
  `go install webtyp.com/wasmbrowsertest@latest`; `gotest` does this automatically).

## Facts this plan relies on (verified 2026-10-01 in `wasmbrowsertest`, headless Chrome)

- In the page (where `wasmbrowsertest` runs tests): `navigator.storage.getDirectory()`,
  `getFileHandle(name, {create: true})`, `createWritable()`, `write(Uint8Array)`, `close()`,
  `getFile()`, `file.slice(a, b).arrayBuffer()` all work. `getFileHandle` of a missing name
  rejects with a `DOMException` named `NotFoundError`. `createSyncAccessHandle` is **undefined**
  in the page: do not use it.
- `webtyp.com/await` v0.1.2: `await.Promise(p js.Value) (js.Value, error)` blocks the goroutine
  until `p` settles. On rejection the error text is the rejection's `toString()`, which for a
  `DOMException` is `"<name>: <message>"`, e.g. `"NotFoundError: A requested file or directory could not be found…"`.
- `webtyp.com/files` v0.0.2: `files.Reader`, `files.Writer`, `files.Appender`,
  `files.ReadWriter`, `files.ErrNotExist` (returned **unwrapped**, callers compare with `==`),
  and the suite `webtyp.com/files/conformance`: `conformance.Run(t, conformance.Factory{Name, New: func(t *testing.T) files.ReadWriter})`.
  `New` must return a fresh, empty file system for every call.

## Design gate

1. **Prior art.** Go's `os.ReadFile`/`os.WriteFile` (whole-file, path-based, `fs.ErrNotExist`);
   the browser's own File System Access API (handles, writables); `afero`'s pluggable file
   systems (`MemMapFs`, `OsFs`). This repo is the browser implementation of the ecosystem's
   `files` contract, like `files/mem` is the in-memory one: same methods, same `ErrNotExist`.
2. **Novice-name test.** `opfs.Open("models")` → `*opfs.FS`; `fs.ReadFile`, `fs.WriteFile`,
   `fs.AppendFile` are the contract's names.
3. **Complexity ledger.** +1 constructor, +1 type, 0 new method names (they are the contract's).
   Ways to do the same thing: 0 new (the only other browser storage for files would be IndexedDB,
   which `kvdb` uses for small records).
4. **Where it belongs.** One concern: OPFS. Waiting for promises is `webtyp/await`'s; the
   contract and its conformance suite are `webtyp/files`'s.
5. **What it deletes.** Nothing exists yet (the repository only has docs). Remove the old
   `opfs.go` stub if it is still in git (`git rm opfs.go`).

## Stage 1 — `doc.go`, `opfs.go`: the type and Open

`doc.go` (no build tag):

```go
// Package opfs is the browser's Origin Private File System as the ecosystem's whole-file
// contract (webtyp.com/files). It is browser-only: the implementation builds with GOOS=js.
package opfs
```

`opfs.go` (`//go:build wasm`):

```go
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

// Open returns the directory dir under the origin's OPFS root, creating it (and its parents)
// when needed; "" is the root itself. Call it, and every method, from a goroutine: they block
// on the browser's promises (see docs/ARCHITECTURE.md, "Blocking").
func Open(dir string) (*FS, error)
```

`Open`:

1. `storage := js.Global().Get("navigator").Get("storage")`; if `storage` is undefined or
   `storage.Get("getDirectory").Type() != js.TypeFunction` → `fmt.Err(errUnavailable)`.
2. `root, err := await.Promise(storage.Call("getDirectory"))`; error → `fmt.Errf(errBrowser, err)`.
3. `""` → `&FS{dir: root}`. Otherwise split `dir` (Stage 2 rules) and walk it with
   `getDirectoryHandle(name, {create: true})`.

Error constants (all of them, exactly):

```go
const (
	errUnavailable = "opfs: this browser has no Origin Private File System"
	errInvalidPath = "opfs: invalid path %q"
	errBrowser     = "opfs: %v"
	notFoundName   = "NotFoundError" // the DOMException name of a missing file or directory
)
```

## Stage 2 — `path.go` (`//go:build wasm` is not needed here; it has no `syscall/js`)

```go
// splitPath splits a relative path on "/". It refuses "", a leading or trailing "/", an empty
// segment ("a//b"), "." and "..".
func splitPath(path string) ([]string, bool)
```

Scan the bytes; no `strings`. `"a/b/c.bin"` → `["a", "b", "c.bin"]`; `"a.bin"` → `["a.bin"]`.

## Stage 3 — `file.go` (`//go:build wasm`): ReadFile, WriteFile, AppendFile

Two unexported helpers:

- `(f *FS) parent(segments []string, create bool) (js.Value, error)`: walks all segments but
  the last with `getDirectoryHandle(name, {create})`.
- `(f *FS) handle(path string, create bool) (js.Value, error)`: `splitPath` (invalid →
  `fmt.Errf(errInvalidPath, path)`), `parent`, then `getFileHandle(last, {create})`.

Every rejected promise goes through:

```go
// browserError maps a rejection to files.ErrNotExist (unwrapped, as the contract requires) when
// the browser says the file or a directory is missing, and wraps anything else.
func browserError(err error) error {
	if fmt.HasPrefix(err.Error(), notFoundName) {
		return files.ErrNotExist
	}
	return fmt.Errf(errBrowser, err)
}
```

The `{create: bool}` option object: `o := js.Global().Get("Object").New(); o.Set("create", create)`.

**ReadFile(path)**

1. `h, err := f.handle(path, false)` (missing → `files.ErrNotExist`).
2. `file := await.Promise(h.Call("getFile"))`; `size := file.Get("size").Int()`.
3. `out := make([]byte, size)`; for `off := 0; off < size; off += ChunkSize`:
   `end := min(off+ChunkSize, size)`; `ab := await.Promise(file.Call("slice", off, end).Call("arrayBuffer"))`;
   `js.CopyBytesToGo(out[off:end], js.Global().Get("Uint8Array").New(ab))`.
4. Return `out` (a new slice each call, so the caller may change it: the conformance suite
   checks this).

**WriteFile(path, data)**

1. `h, err := f.handle(path, true)`.
2. `w := await.Promise(h.Call("createWritable", {keepExistingData: false}))`.
3. One reusable `chunk := js.Global().Get("Uint8Array").New(min(ChunkSize, len(data)))` (skip if
   `len(data) == 0`); for each chunk: if shorter than the buffer use
   `chunk.Call("subarray", 0, n)`; `js.CopyBytesToJS(view, data[off:end])`;
   `await.Promise(w.Call("write", view))`.
4. `await.Promise(w.Call("close"))`. On any error after `createWritable`, call
   `w.Call("abort")` (ignore its result) before returning, so no half-written file is published.

**AppendFile(path, data)**: like WriteFile, with `{keepExistingData: true}`, and before writing:
`size := await.Promise(h.Call("getFile")).Get("size").Int()`; `await.Promise(w.Call("seek", size))`.

## Stage 4 — tests (`tests/`, `package tests`, `//go:build wasm`)

### `tests/conformance_test.go`

```go
// opfs passes the files contract's suite: any code written against files works on it.
func TestConformance(t *testing.T) {
	n := 0
	conformance.Run(t, conformance.Factory{
		Name: "opfs",
		New: func(t *testing.T) files.ReadWriter {
			n++
			fs, err := opfs.Open(fmt.Sprintf("conformance/%d-%d", time.Now().UnixNano(), n))
			if err != nil {
				t.Fatal(err)
			}
			return fs
		},
	})
}
```

(Tests may use the stdlib `fmt` and `time`.)

### `tests/opfs_test.go`

One comment line per test with its use case:

- `TestLargeFileCrossesChunks`: a 20 MiB file (bytes `i % 251`) written and read back equal
  (three chunks); appending 1 MiB more gives 21 MiB with the appended bytes at the end.
- `TestNestedPaths`: `WriteFile("a/b/c.bin", …)` then `ReadFile` returns it; `ReadFile("a/x/y.bin")`
  (missing directory) and `ReadFile("a/b/none.bin")` (missing file) both return exactly
  `files.ErrNotExist` (`err == files.ErrNotExist`).
- `TestInvalidPaths`: `""`, `"/a"`, `"a/"`, `"a//b"`, `"./a"`, `"a/../b"` each return the error
  text `opfs: invalid path "<path>"` from `ReadFile` and `WriteFile`.
- `TestOpenSameDirectoryTwice`: a file written through one `Open("shared")` is read through a
  second `Open("shared")`.

## Stage 5 — docs

- `README.md`, `docs/ARCHITECTURE.md`: verify against the code; change nothing unless they
  disagree (then the docs are right and the code is fixed).
- Add `AGENTS.md`: "Browser-only implementation of `webtyp/files`. Tests run in a browser
  (`GOOS=js GOARCH=wasm go test -exec wasmbrowsertest ./...`). Do not use synchronous access
  handles: they do not exist in the page, where the tests run (docs/ARCHITECTURE.md)." Index it
  in `README.md`.

## Acceptance criteria

```bash
go vet ./... && go build ./...                                     # native: doc.go only
GOOS=js GOARCH=wasm go vet ./...
GOOS=js GOARCH=wasm go test -exec wasmbrowsertest ./...           # all green in the browser
grep -rn '"strings"\|"errors"\|"strconv"\|createSyncAccessHandle\|map\[' --include=*.go . | grep -v "^./tests/"   # empty
test ! -e opfs.go || grep -q "type FS struct" opfs.go
```

## Stages

| # | Files | Done when |
|---|---|---|
| 1 | `doc.go`, `opfs.go` | builds for `GOOS=js GOARCH=wasm` |
| 2 | `path.go` | builds |
| 3 | `file.go` | builds |
| 4 | `tests/conformance_test.go`, `tests/opfs_test.go` | all green in the browser |
| 5 | `README.md`, `docs/ARCHITECTURE.md`, `AGENTS.md` | acceptance criteria pass |
