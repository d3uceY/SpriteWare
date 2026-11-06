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
shift, desaturate), cels (copy, tween, oscillate), transforms (flip, rotate,
scale, crop), tag updates, and the strictness around all of them (a 45° rotate
is refused, a tween past the last frame is refused).

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
.\bin\spriteware-dev.exe

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

## Watching the app

Build and run `bin\spriteware-dev.exe` yourself. It uses the real settings file
unless you set `SPRITEWARE_CONFIG`, so keep the two runs apart.

For UI work, `wails3 dev` gives a window that reloads when the Elm or CSS
changes.
