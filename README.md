<div align="center">

<img src="build/appicon.png" width="120" alt="SpriteWare's pixel-art hat icon">

# SpriteWare

Pixel-art tooling an AI agent can actually use

[![Latest release](https://img.shields.io/github/v/release/d3uceY/SpriteWare?style=flat-square&label=release&sort=semver)](https://github.com/d3uceY/SpriteWare/releases/latest)
[![License](https://img.shields.io/github/license/d3uceY/SpriteWare?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/d3uceY/SpriteWare?style=flat-square&label=go)](go.mod)
[![Aseprite](https://img.shields.io/badge/aseprite-1.3%2B-7d929e?style=flat-square)](https://www.aseprite.org/)

</div>

SpriteWare runs an MCP server that lets an agent open a sprite, draw pixels, read
the result back and export a PNG. It drives Aseprite for the agent: the window
you have open, or a hidden Aseprite when none is.

## Download

Latest release.

|    | Platform | File |
|:-:|---|---|
| <img src="https://api.iconify.design/mdi/microsoft-windows.svg?color=%230078D4" width="18" height="18" alt="Windows"> | Windows installer | [SpriteWare-windows-amd64-installer.exe](https://github.com/d3uceY/SpriteWare/releases/latest/download/SpriteWare-windows-amd64-installer.exe) |
| <img src="https://api.iconify.design/mdi/microsoft-windows.svg?color=%230078D4" width="18" height="18" alt="Windows"> | Windows portable | [SpriteWare-windows-amd64.exe](https://github.com/d3uceY/SpriteWare/releases/latest/download/SpriteWare-windows-amd64.exe) |
| <img src="https://api.iconify.design/mdi/apple.svg?color=%23A2AAAD" width="18" height="18" alt="macOS"> | macOS (Apple Silicon and Intel) | [SpriteWare-macos-universal.dmg](https://github.com/d3uceY/SpriteWare/releases/latest/download/SpriteWare-macos-universal.dmg) |
| <img src="https://api.iconify.design/mdi/linux.svg?color=%23FCC624" width="18" height="18" alt="Linux"> | Linux binary | [SpriteWare-linux-amd64](https://github.com/d3uceY/SpriteWare/releases/latest/download/SpriteWare-linux-amd64) |
| <img src="https://api.iconify.design/mdi/debian.svg?color=%23A80030" width="18" height="18" alt="Debian"> | Linux package | [SpriteWare-linux-amd64.deb](https://github.com/d3uceY/SpriteWare/releases/latest/download/SpriteWare-linux-amd64.deb) |

The builds are unsigned. Windows: SmartScreen → **More info** → **Run anyway**.
macOS: right-click the app click **Open**. Linux binary: `chmod +x` it first. For Mac, you may need to allow apps from unidentified developers in System Preferences then Security & Privacy.

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
