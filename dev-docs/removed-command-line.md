# The command line that is not here

SpriteWare ships one binary: the window, and the MCP server it starts. It used
to have a terminal front door as well. This records what that was, the
measurements that made it untenable, and the one piece deliberately left
standing.

## What was removed

| Gone | What it was |
|---|---|
| `cmd/spriteware` | the console program: ran the commands, or spawned the window |
| `internal/cli/` | the cobra tree — `root`, `mcp`, `config`, `doctor`, `path`, `version`, `output`, `console_windows`, `console_other`, the `eval` wrapper, and two test files |
| `internal/onpath/` | `Add`, `Remove`, `Status`, one file per OS, three test files |
| `internal/version/` | the single version string both exes shared; `backend.Version` is a plain const again |
| `backend/path.go` | `GetPathStatus`, `AddToPath`, `RemoveFromPath` |
| the **Command line** card | `pathCard` in `View.elm`, `PathInfo` and its messages in `Types.elm`, the port and decoder in `Main.elm`, the fetch and two actions in `src/main.js`, and the regenerated bindings (24 methods to 21, 10 models to 9) |
| the installer's command-line half | the `$INSTDIR\bin` folder, its `File` line, and the `sw.path.add` / `sw.path.remove` macros with their HKLM/HKCU write and `WM_SETTINGCHANGE` broadcast |
| `dev-docs/cli.md` | the page describing all of the above |

`cmd/spriteware-mcp`, an earlier stdio-only server binary, was already gone
before this.

Not removed, but rewritten: `cmd/spriteforge` used to spawn the CLI over stdio
with `client.NewStdioMCPClient`. It now builds the server in this process with
`client.NewInProcessClient(mcpserver.New(tools.New(store, links), …))`, so the
harness needs no second binary either.

## Why

On Windows the shell decides whether to wait for a child by reading the
subsystem field in its PE header, at launch. Every arrangement below was built
and run, not reasoned about:

| Arrangement | Double-click | A command from a shell |
|---|---|---|
| one exe, GUI subsystem | no console, no flash | shell never waits: the prompt returns mid-output, `a && b` races, `$LASTEXITCODE` is stale, Ctrl+C has nowhere to land. `AttachConsole(ATTACH_PARENT_PROCESS)` fixes none of it |
| one exe, console subsystem | Windows creates the console before `main` runs, so every launch flashes one; `FreeConsole` can only shorten it | correct: waits, prints, chains, returns the exit code |
| two exes: console launcher, GUI app | no flash | worked from `cmd`, but the GUI child could not write to a ConPTY console at all — empty output, exit 1 — while a redirect to a file or `cmd /c` worked |

The flag costs nothing in size: `-H windowsgui` and its absence produce
byte-identical exes (17,757,184 each, measured). The whole cost is behaviour,
and the behaviour has no single-binary answer.

The two-exe split did work when the CLI *was* the console process. It was
removed because a desktop app should not need a second binary and a PATH entry
to be scriptable, which is what the split had grown into.

So the product is a desktop app whose MCP server is started from the window. A
terminal front door is possible, and its shape is known — a console exe linking
`internal/cli` and `internal/onpath` — but adding it back means accepting one of
the two costs above, and that trade belongs in a decision, not in a commit.

## Why the eval harness stayed

`internal/eval` and `eval/` are a measuring instrument, not a command surface.

- Its only caller was `internal/cli/eval.go`. That file was the harness
  implementation, moved there from `cmd/spriteware/main.go` and wrapped in a
  thirty-line cobra command. Deleting it removed the harness's only entry point,
  not its logic: `internal/eval` was never edited to make the CLI work, so it
  came through the whole episode untouched.
- It still compiles and `go test ./...` runs its tests, so the package did not
  rot when its caller left.
- `eval/benchmarks/` and `eval/runs/` hold recorded attempts — the ring-knight
  runs, their review forms, their verdicts. None of that has anything to do with
  the CLI decision, and deleting the wrapper was not a reason to lose it.
- Reviving it is a small wrapper, not a rewrite: one command with
  `DisableFlagParsing` handing its arguments to the same functions that still
  live in `internal/eval`.

Recover the wrapper itself from history when a command line returns:

```powershell
git show cea395d^:internal/cli/eval.go
```

## Where this lives in history

| Commit | Change |
|---|---|
| `cea395d` | remove the console program and its packaging |
| `ebe69ec` | remove the PATH bindings from the service |
| `2f78ee9` | build the MCP server in-process in `spriteforge` |
| `8307a66` | point the code map at one binary |
