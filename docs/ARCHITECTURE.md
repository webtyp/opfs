# Architecture — `webtyp/opfs`

## What this is

**OPFS** (Origin Private File System) is a file system the browser gives each website, private
to it and invisible to the user. `opfs` exposes it to Go/TinyGo as the ecosystem's whole-file
contract, [`webtyp/files`](https://github.com/webtyp/files): `files.Reader`, `files.Writer` and
`files.Appender`.

You meet it when a model runs in the browser. Its weights (400–850 MB) are downloaded once,
stored here, and read back on every later visit; the decision model's tool-list cache
(`qwen.SaveDecisionCache`, ≈ 19 MB) is kept here between sessions too.

```go
fs, err := opfs.Open("models")                 // a directory under the origin's OPFS root
data, err := fs.ReadFile("decider-0.8b.wtypw") // files.ErrNotExist when it was never stored
err = fs.WriteFile("decider-0.8b.wtypw", data)
```

## Why OPFS and not IndexedDB for large files

- **Memory.** IndexedDB hands back a `Blob` or an `ArrayBuffer` of the whole file, which lives in
  JavaScript memory while Go copies it again into WebAssembly memory: for 750 MB, a 1.5 GB peak.
  `opfs` reads and writes in **chunks of 8 MiB**, so the peak is the file plus one chunk.
- **Same rules.** Quota and eviction are the same as IndexedDB, and `navigator.storage.persist()`
  protects both.

IndexedDB remains the right place for records with indexes (vector documents, agent memory).

## How it reads and writes

- **Read:** `FileSystemFileHandle.getFile()`, then `file.slice(start, end).arrayBuffer()` one
  chunk at a time, each copied into the Go buffer (`js.CopyBytesToGo`).
- **Write:** `createWritable({keepExistingData: false})`, one `write()` per chunk, `close()`.
  Nothing is visible until `close()`: a reader never sees half a file.
- **Append:** `createWritable({keepExistingData: true})`, `seek(size)`, then the same writes.
- **Paths** are relative and use `/`: `"cache/decisions.bin"` creates the directory `cache` on
  write. An empty segment, `.` or `..` is refused.

These are the asynchronous OPFS methods, which exist in both the page and a Worker. The faster
*synchronous access handles* exist only inside a dedicated Worker, so they could not be tested by
the ecosystem's browser test runner (`wasmbrowsertest` runs tests in the page); they are an
optimization to measure later, not a second code path now.

## Blocking

Each method waits for the browser's promises by blocking its goroutine (Go and TinyGo both let
the JavaScript event loop run meanwhile). Call it from a goroutine, never directly inside a
`js.FuncOf` callback, where blocking deadlocks.
