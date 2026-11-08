# Testing

There are three layers. Two of them are opt-in, because they need something
outside the repo.

## 1. Unit tests

```powershell
go test ./...
```

No Aseprite, no network, no window. Fast, and safe to run on every save.

What this covers: tool schemas, grid and text compilation, export formats, the
validate rules, the protocol parser, the skill tree, the config file, the
install paths.

## 2. Integration tests, against your Aseprite

These start a real batch Aseprite and check what actually lands on the canvas.

```powershell
$env:SPRITEWARE_ASEPRITE = "C:\path\to\aseprite.exe"
go test ./internal/link -run Integration -v
```

Each test gets its own Aseprite in its own temp user folder, so your settings,
brushes and extensions are never touched. The whole set takes about five
seconds.

What this covers: drawing (outline, shade, fill, blit, dither, gradient, hue
shift, desaturate, remap, frame, shift), cels (copy, tween, oscillate),
transforms (flip, rotate, scale, crop), tag updates, and the strictness around
all of them (a 45° rotate is refused, a tween past the last frame is refused).

`real_bugs_test.go` is the regression set for `dev-docs/mcp-improvements.md`: a
dump by path reads that document, opening an open file activates it instead of
copying it, a replace region is honoured, an identical redraw reports zero
changes, a new document takes frames and a name, and `closeAll` refuses unsaved
work. Each one was red before its fix.

Add a test here for anything that runs inside Aseprite. A Lua handler that only
runs in Aseprite is untested until it runs in Aseprite.

## 3. End to end, against the running app

Point the app at a throwaway settings file, start it, then drive the running
binary over HTTP.

```powershell
$cfg = "$env:TEMP\spriteware-e2e\config.json"
[System.IO.File]::WriteAllText($cfg,
  '{"mode":"headless","port":9977,"bridgePort":9978,"startMcpOnLaunch":true}',
  (New-Object System.Text.UTF8Encoding($false)))

$env:SPRITEWARE_CONFIG = $cfg
$env:SPRITEWARE_EXTENSIONS_DIR = "$env:TEMP\spriteware-e2e\extensions"
.\bin\spriteware.exe

$env:SPRITEWARE_E2E_URL = "http://127.0.0.1:9977/mcp"
$env:SPRITEWARE_E2E_BRIDGE = "9978"
go test ./backend -run TestE2E -v
```

Written with `New-Object UTF8Encoding($false)` on purpose: PowerShell's
`Set-Content -Encoding utf8` adds a BOM. The config loader now ignores one, but
writing it clean keeps the test honest about what it is checking.

What this covers: the tool list a real client sees, the served resources, that
`doctor` always answers, and that the bridge accepts an extension and reports it
connected. This is the layer that found `doctor` reporting an always-zero bridge,
and the BOM problem above.

## 4. A throwaway instance for drawing art

The same recipe, kept running while the app under test is rebuilt: a second
build on its own ports, its own settings file and its own extension folder.

```powershell
$dir = "$env:TEMP\spriteware-assets"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$body = '{"mode":"headless","port":9953,"bridgePort":9954,"startMcpOnLaunch":true,' +
        '"asepritePathConfirmed":true,"asepritePath":"<path to aseprite.exe>"}'
[System.IO.File]::WriteAllText("$dir\config.json", $body,
  (New-Object System.Text.UTF8Encoding($false)))

go build -o bin\spriteware-assets.exe .
$env:SPRITEWARE_CONFIG = "$dir\config.json"
$env:SPRITEWARE_EXTENSIONS_DIR = "$dir\extensions"
.\bin\spriteware-assets.exe
```

Its MCP endpoint is `http://127.0.0.1:9953/mcp`. Register that URL in
`.vscode/mcp.json` (gitignored, per-machine) and the sprite tools act on the
long-lived instance: open documents and undo history survive every rebuild of
the app you are working on.

**A test that starts the bridge rewrites the extension** with the bridge port it
was given. `newTestService` therefore sets `SPRITEWARE_EXTENSIONS_DIR` to a temp
folder; without it a suite run leaves the user's real
`%APPDATA%\Aseprite\extensions\spriteware\spriteware.lua` dialling a test port.

## Watching the app

Build and run `bin\spriteware.exe` yourself (`task build`, then the exe). It
uses the real settings file unless you set `SPRITEWARE_CONFIG`, so keep the two
runs apart.

For UI work, `wails3 dev` gives a window that reloads when the Elm or CSS
changes.
