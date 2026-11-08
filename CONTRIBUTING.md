# Contributing

## Setup

Go 1.26, Node, and Wails v3 (`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`).

```powershell
task dev
task build   # into bin/
```

## Before a pull request

```powershell
gofmt -l .      # nothing new should appear (internal/skill/skill.go is grandfathered)
go test ./...   # no Aseprite, no window, no network
```

Anything that runs inside Aseprite needs a test that runs inside Aseprite:

```powershell
$env:SPRITEWARE_ASEPRITE = "C:\path\to\aseprite.exe"
go test ./internal/link -run Integration
```

## Conventions

- One change per pull request, described in the title.
- Comments say why, not what. Match the surrounding style.
- UI changes follow `DESIGN.md`. Sprites are drawn with SpriteWare's own MCP
  tools.

## Docs

- `dev-docs/README.md` — the design record, indexed.
- `dev-docs/code-map.md` — where to change things.
- `dev-docs/testing.md` — the three test layers.
- `DESIGN.md` — palette, layout and art rules.
