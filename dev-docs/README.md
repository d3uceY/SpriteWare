# SpriteWare developer docs

SpriteWare is a desktop app that lets an AI agent draw pixel art in Aseprite.

An agent cannot click in a window. It can call tools. So the app gives it tools
(open a sprite, draw pixels, read the result back, export a PNG) and talks to
Aseprite on its behalf.

Read these in order:

| File | Answers |
|---|---|
| [architecture.md](./architecture.md) | what the pieces are and how they talk |
| [code-map.md](./code-map.md) | which folder does what, and where to start |
| [protocol.md](./protocol.md) | the exact bytes between the app and Aseprite |
| [skills.md](./skills.md) | the text that teaches an agent to use the tools |
| [testing.md](./testing.md) | how to run the tests, including against a real Aseprite |
| [evals.md](./evals.md) | benchmarks, runs, and how an attempt is scored |
| [mcp-improvements.md](./mcp-improvements.md) | what drawing the app's own art broke, and its status |
| [removed-command-line.md](./removed-command-line.md) | what the CLI was, why it is gone, and why the eval harness stayed |

## The short version

```
agent  --HTTP-->  MCP server  -->  tools  -->  Aseprite
                  (in the app)              (window, or a hidden process)
```

The app runs the MCP server and a small bridge. Aseprite talks to the bridge
when it is your own editor window. When there is no window, the app starts a
hidden Aseprite and talks to it over a pipe.

Both paths run the same Lua file, so a drawing command behaves the same in
either mode.

## Run it

```powershell
npm --prefix frontend run build          # Elm + Vite -> frontend/dist
go build -o bin\spriteware.exe .
.\bin\spriteware.exe
```

`task build` does the same; `task dev` is `wails3 dev`, which reloads the window
as the Elm or CSS changes.

The window opens on the Server page. Start the server, copy the URL into your
agent's MCP config, or press Install to write that config for you.
