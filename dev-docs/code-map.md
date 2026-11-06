# Code map

Start at `main.go`. It is about a hundred lines and it shows the whole shape:
build the config, build the link selector, build the service, open the window.

```
main.go                     wires the app together
backend/                    the Go side the UI talks to
  mcp.go                    start/stop the MCP server and the bridge
  install.go                the eleven agent configs and the skill installer
  project.go                the project folder and its picker
  setup.go                  finding Aseprite, remembering the answer
internal/
  tools/                    the fourteen tools and their JSON schemas
  link/                     talking to Aseprite, live or headless
  luaext/                   the Lua that runs inside Aseprite
  protocol/                 the line format both sides speak
  analyze/                  validate: the rules engine
  export/                   PNG, frames, SVG, GIF, sheets, tilesets
  render/                   previews: ascii, filmstrip, onion, diff
  doc/                      the in-memory copy of a sprite
  craft/                    colour maths and palette presets
  font/                     the 5x7 font that draw kind=text uses
  skill/                    the SKILL.md tree, embedded
  mcpserver/                server setup, resources, instructions
  config/                   settings file
frontend/                   Elm UI, built with Vite
build/                      packaging assets for Windows, macOS, Linux
cmd/                        small dev tools, not part of the app
```

## Where to change things

| You want to | Edit |
|---|---|
| add a tool | a new file in `internal/tools`, register it in `tools.Register`, and add a line to `tools.Catalog` |
| add a draw behaviour | a `kind` in `internal/tools/drawing.go` (schema) and `luaext/extension/spriteware.lua` |
| change what a tool returns | the tool's `AddTool` handler |
| change a validate rule | `internal/analyze/analyze.go` |
| add an export format | `internal/export/`, then the `export` tool |
| change the generated palette resource | `internal/mcpserver/knowledge.go` |
| teach an agent a new trick | the matching file under `internal/skill/` |
| change the agent's instructions | `internal/skill/SKILL.md` |
| fix an agent config path | `backend/install.go`, function `entryFor` |
| change a skill install path | `internal/skill/skill.go`, function `DirFor` |
| change the window | `frontend/elm/src/View.elm` |

## The split that matters

Most tools are thin. They check arguments, call one Aseprite command, and format
the answer. The work happens in one of two places:

- **In Lua**, when it needs the document: drawing, layers, frames, tags, cels.
- **In Go**, when it needs the whole picture: validate, export, previews.

That is why `look` and `validate` read a dump (`internal/doc`) instead of asking
Aseprite per pixel: one round trip, then all the analysis is local and testable.

## Reading a tool

Take `draw` in `internal/tools/drawing.go`.

- `drawSchema` is the JSON Schema an agent sees. Hand-written, because the ops
  are an object array.
- `compileGrid` and `compileText` turn readable input (character rows, a string)
  into plain `pixels` ops before anything reaches Aseprite. All the strictness
  lives here: a ragged row or an unknown glyph fails before a pixel is written.
- The handler sends one `draw.apply` with all the compiled ops, so the user's
  Ctrl+Z undoes the whole batch.

`transform` (`transform.go`), `export` (`inspect.go`) and the cel/ layer/ frame
tools (`structure.go`) follow the same shape.
