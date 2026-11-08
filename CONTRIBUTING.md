# Contributing

## Setup

Go 1.26, Node, and Wails v3 (`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`).

```powershell
task dev
task build   # into bin/
```

## Before a pull request

```powershell
gofmt -l .     
go test ./...   
```

For testing and stuff:

```powershell
$env:SPRITEWARE_ASEPRITE = "C:\path\to\aseprite.exe"
go test ./internal/link -run Integration
```



## Docs

- `dev-docs/README.md` — the design record, indexed.
- `dev-docs/code-map.md` — where to change things.
- `dev-docs/testing.md` — the three test layers.
- `DESIGN.md` — palette, layout and art rules.
