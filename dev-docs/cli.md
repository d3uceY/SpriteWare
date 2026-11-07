# The CLI

A terminal front door to the same machinery the window uses. The MCP server is
the reason it exists: an agent, a CI job or a headless box needs a server it can
start, watch and stop without a desktop.

## One binary, dispatch first

`main.go` looks at `os.Args` before it builds anything. No arguments → the
window. Anything else → the CLI, and no Wails code runs at all.

```go
func main() {
	if len(os.Args) > 1 {
		os.Exit(cli.Run(backend.Version, os.Args[1:]))
	}
	runGUI()
}
```

One artifact, already signed, already installed, already the thing on PATH. The
window persists settings; commands read them and never write back unless asked.

### What it cost on Windows

The production build is `-H windowsgui`, so the exe owns no console. Standard
handles are inherited and output prints, but Ctrl+C only reaches a
*console-attached* process, so `spriteware mcp` would have been unstoppable.
`internal/cli/console_windows.go` calls `AttachConsole(ATTACH_PARENT_PROCESS)`
before anything else. It does **not** rebind stdout to `CONOUT$`: the inherited
handles already point at the console, the pipe or the redirect, and rebinding
would break `spriteware doctor > out.txt`.

`cmd.exe` does not wait for a GUI-subsystem child, so the prompt returns while
the server runs. Closing the terminal still stops it. Dev builds have no
`-H windowsgui`, so none of this is felt while developing.

## Commands

```
spriteware                        the window
spriteware mcp                    serve MCP in the foreground until Ctrl+C
spriteware doctor                 check the install, name what is broken
spriteware config [get|set|path|reset]
spriteware path   [status|add|remove]
spriteware eval   list|new|add|brief|check|score|table
spriteware version
```

`help`, `-h`, `help <cmd>`, `--version` and shell completion come from cobra.
Exit code is 0 or 1; an unknown command is an error and does not fall through to
the window, because a typo in a script must not open a GUI on a headless box.

### `spriteware mcp`

```
spriteware mcp [--port N] [--transport streamable-http|sse] [--mode headless|live]
               [--bridge-port N] [--stdio] [--allow-lua]
```

- HTTP on the configured port, loopback only, in the foreground.
- Banner to **stderr**, then blocks:

  ```
  SpriteWare MCP  http://127.0.0.1:9933/mcp
  mode headless · 14 tools · Ctrl+C to stop
  point a client at it:  {"mcpServers":{"spriteware":{"url":"http://…/mcp"}}}
  ```
- Shutdown: `signal.NotifyContext` on interrupt and SIGTERM → `Host.Stop()`
  (transport shutdown with a 5s deadline), bridge closed, links closed, exit 0.
  Closing the terminal ends the process the same way.
- `--stdio`: `ServeStdio`, logs to stderr, stdout belongs to the protocol. This
  is what replaced `cmd/spriteware-mcp`.
- `--mode live` starts the bridge and installs the Aseprite extension, because
  the extension has the port baked into it. Headless does not touch the bridge,
  so it cannot collide with a running window.
- A flag overrides the config for that run only; nothing is saved.
- Port taken → one line naming the port, exit 1.

### `spriteware doctor`

`ok | warn | fail` per check, plus a fix line that can be pasted. Exit 1 when any
check failed, so a script can gate on it.

| check | fails when |
|---|---|
| config | the file cannot be opened for writing |
| aseprite | the configured path is not a file, or nothing was found |
| extension | the Aseprite extension is missing (warn) |
| port / bridgePort | the port is taken |
| path | `spriteware` does not resolve to this build (warn) |

### `spriteware config`

Keys: `asepritePath`, `asepritePathConfirmed`, `mode`, `port`, `transport`,
`bridgePort`, `projectPath`, `startMcpOnLaunch`, `allowLua`. A
`map[string]func(*config.Config, string) error` — one place to add a key, with a
written error message instead of a generated one.

`config` prints the effective values and the file path, `get <key>` prints one
raw value, `set <key> <value>` validates on a copy before saving (a rejected
value leaves the file untouched), `path` prints the location, `reset` writes the
defaults. `--json` switches the listing and `get` to JSON; the three commands
that report an action stay in prose, because a person reads them.

Precedence: flag > config file > default. `SPRITEWARE_CONFIG` moves the file.

## PATH

| OS | Install time | `path add` / the button |
|---|---|---|
| Windows | NSIS appends `$INSTDIR` to the machine `Path` when elevated | `HKCU\Environment`, `REG_EXPAND_SZ`, then `WM_SETTINGCHANGE` |
| macOS | `/usr/local/bin/spriteware` → `…/SpriteWare.app/Contents/MacOS/SpriteWare` | symlink, `/usr/local/bin` then `~/.local/bin` |
| Linux | `.deb` ships `/usr/bin/spriteware` | symlink, same two folders |

`internal/onpath` holds it: `Add`, `Remove`, `Status`, one file per OS. An entry
that is already there is not added twice, so the button can be pressed without
thinking. Windows path lookup is case-insensitive, so `spriteware` finds
`SpriteWare.exe` — no rename, no shim, no second binary.

Two honest caveats, both shown in the UI rather than hidden: a terminal that is
already open keeps its old PATH, and `/usr/local/bin` may need sudo, in which
case the button shows the command to run.

`backend/path.go` exposes `GetPathStatus`, `AddToPath`, `RemoveFromPath` over the
same package with `os.Executable()` as the target.

## Layout

```
main.go                         dispatch + runGUI
internal/cli/
  root.go                       cli.Run, the command tree, --json
  mcp.go                        flags, banner, signal-to-shutdown
  config.go                     the key registry and four subcommands
  doctor.go                     one func per check
  path.go                       status | add | remove
  eval.go                       the old harness, unchanged behind cobra
  version.go, output.go         version; one JSON printer
  console_windows.go            AttachConsole
internal/mcpserver/host.go      Host{Start, URL, Stop, State}
internal/onpath/                Add, Remove, Status; one file per OS
backend/path.go                 the settings-panel bindings
```

The eval tree is wrapped, not rewritten: `spriteware eval <cmd> …` is handed to
the same `evalCmd` as before with `DisableFlagParsing`, so its flags and output
are byte-identical.

`mcpserver.Host` was extracted out of `backend/mcp.go`: the window and the CLI
now share one HTTP lifecycle. The Wails service kept its ports, config
persistence and events; `backend/mcp_test.go` and `bridge_test.go` pass without
edits.

## Done, and what is left

| Phase | State |
|---|---|
| 0 — cobra skeleton, dispatch, eval move, `cmd/spriteware` deleted | done |
| 1 — `AttachConsole`, `Host`, `mcp`, `cmd/spriteware-mcp` deleted | done, MCP initialize answered over HTTP from a real process |
| 2 — `config`, `doctor` | done |
| 3 — `onpath`, `path`, backend bindings | done in Go |
| 3 — the settings-panel button, NSIS/deb/mac packaging | **not started** |
| 4 — `spriteware install` (agent configs) | not started, optional |

The button needs `wails3 generate bindings` plus a card in `View.elm`'s server
page; the packaging work is `project.nsi` for Windows and the `.deb` recipe for
Linux.

## Tests

- `internal/cli`: an out-of-range port and an unknown key are refused and leave
  the file alone; a valid value persists.
- `internal/onpath`: link → status → idempotent add → remove → status false
  (Unix); the Windows test covers the entry matching, not the registry.
- `internal/mcpserver`: a real `initialize` over HTTP on an ephemeral port, then
  `Stop` leaves nothing running; a busy port is refused.

## Not building

No daemon or `--detach` (closing the terminal already stops the server), no
plugin commands, no config TUI, no shell-rc editing, no verbose/quiet matrix
beyond `--json`, no second config format.
