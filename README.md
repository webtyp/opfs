# opfs
<img src="docs/img/badges.svg">

The browser's **Origin Private File System** (OPFS) from Go/TinyGo, as the ecosystem's whole-file
contract ([`webtyp/files`](https://github.com/webtyp/files)): read, write and append whole files,
in the page or in a Web Worker. Used for model weights (hundreds of MB) and other large files.

```go
fs, err := opfs.Open("models")
data, err := fs.ReadFile("decider-0.8b.wtypw") // files.ErrNotExist when missing
```

## Documentation

- [Architecture](docs/ARCHITECTURE.md) — why OPFS, how it reads and writes in chunks, blocking.
- [Agent Instructions](AGENTS.md) — instructions and notes for automated agents.
