<div align="center">

<img src="build/appicon.png" width="120" alt="SpriteWare's pixel-art mascot on a slate plate">

# SpriteWare

Pixel-art tooling an AI agent can actually use

</div>

SpriteWare runs an MCP server that lets an agent open a sprite, draw pixels, read
the result back and export a PNG. It drives Aseprite for the agent: the window
you have open, or a hidden Aseprite when none is.

## Download

Latest release.

| Platform | File |
|---|---|
| Windows installer | [SpriteWare-windows-amd64-installer.exe](https://github.com/d3uceY/SpriteWare/releases/latest/download/SpriteWare-windows-amd64-installer.exe) |
| Windows portable | [SpriteWare-windows-amd64.exe](https://github.com/d3uceY/SpriteWare/releases/latest/download/SpriteWare-windows-amd64.exe) |
| macOS (Apple Silicon and Intel) | [SpriteWare-macos-universal.dmg](https://github.com/d3uceY/SpriteWare/releases/latest/download/SpriteWare-macos-universal.dmg) |
| Linux binary | [SpriteWare-linux-amd64](https://github.com/d3uceY/SpriteWare/releases/latest/download/SpriteWare-linux-amd64) |
| Linux package | [SpriteWare-linux-amd64.deb](https://github.com/d3uceY/SpriteWare/releases/latest/download/SpriteWare-linux-amd64.deb) |

The builds are unsigned. Windows: SmartScreen → **More info** → **Run anyway**.
macOS: right-click the app → **Open**. Linux binary: `chmod +x` it first.

## Run it

Aseprite 1.3+ is required. First launch asks for its executable.
The Server screen starts the MCP server and writes the entry into your assistant's
config (Claude Desktop and Claude Code, VS Code, Zed, Cursor, Windsurf, Cline,
Gemini CLI, Codex, OpenCode, Kiro). I have not tested them all to check if the intalls work correctly but if there are any issues, please make an issue on GitHub.

## Build

```powershell
npm --prefix frontend run build
go build -o bin\spriteware.exe .
```

`task build` runs both. Go 1.26+ and Node 24.

## Docs

[dev-docs](./dev-docs/README.md) covers the architecture, the Aseprite protocol,
tests, and [releasing](./dev-docs/release.md).

## License

MIT
