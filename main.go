// SpriteWare - Wails v3 + Elm.
//
// Layers, one job each:
//
//	backend/       Go services - every exported method becomes a JS binding
//	internal/      the MCP server, the checker and the colour maths
//	main.go        window and wiring (this file)
//	src/main.js    ports <-> bindings, the only file that knows both worlds
//	elm/src        state, view, subscriptions
package main

import (
	"embed"
	"log"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/d3uceY/spriteware/backend"
	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
)

// frontend/dist is embedded here. `all:` lets the pattern match files starting
// with "." or "_", which Go skips. Resolved at compile time, so build the
// frontend first.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// A config problem must never stop the app starting: missing means defaults.
	cfg, err := config.Load()
	if err != nil {
		slog.Warn("could not load the configuration; using defaults", "err", err)
	}
	current := cfg.Get()

	links := link.NewSelector(link.Options{
		AsepritePath: current.AsepritePath,
		Mode:         current.Mode,
		BridgePort:   current.BridgePort,
		AllowLua:     current.AllowLua,
	})
	defer links.Close()

	mcpService := backend.NewMCPService(cfg, links)

	// Registering the event type lets the frontend import its name.
	application.RegisterEvent[backend.MCPStatus]("mcp:status")

	app := application.New(application.Options{
		Name:        "SpriteWare",
		Description: "Pixel-art tooling an AI agent can actually use",
		Services: []application.Service{
			application.NewService(mcpService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// 900x620 is about 884px of client width on Windows, which is what media
	// queries see.
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "SpriteWare",
		Width:            900,
		Height:           620,
		MinWidth:         680,
		MinHeight:        460,
		BackgroundColour: application.NewRGB(11, 13, 18),
		URL:              "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	})

	// Starting at launch makes the answer to "how do I point my agent at this" a
	// URL that already works.
	if current.StartMCPOnLaunch {
		if _, err := mcpService.StartServer(current.Port, string(current.Transport)); err != nil {
			slog.Warn("MCP server did not start", "err", err)
		}
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
