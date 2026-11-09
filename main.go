// SpriteWare - Wails v3 + Elm.
//
//	main.go      the window
//	backend/     Go services - every exported method becomes a JS binding
//	internal/    MCP server, checker, colour maths
//	src/main.js  ports <-> bindings
//	elm/src      state, view, subscriptions
package main

import (
	"embed"
	"log"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/d3uceY/spriteware/backend"
	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
)

// frontend/dist is embedded. The all: prefix matches files starting with "."
// or "_", which Go otherwise skips. Build the frontend first.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// GUI-subsystem binary: a double-click never flashes a console.
	runGUI()
}

func runGUI() {
	// Bad config must not stop startup; missing means defaults.
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
	appService := backend.NewAppService()

	// Registering the event type lets the frontend import its name.
	application.RegisterEvent[backend.MCPStatus]("mcp:status")

	app := application.New(application.Options{
		Name:        "SpriteWare",
		Description: "Pixel-art tooling an AI agent can actually use",
		Services: []application.Service{
			application.NewService(mcpService),
			application.NewService(appService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// Closing the window parks the app in the tray; only Quit from the
			// tray menu (or the platform's quit command) ends the process.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	// 900x620 is ~884px of client width on Windows, which media queries see.
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "SpriteWare",
		Width:            900,
		Height:           620,
		MinWidth:         680,
		MinHeight:        460,
		BackgroundColour: application.NewRGB(211, 203, 190),
		URL:              "/",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	})

	// Closing is hidden-to-tray, not quit: the MCP endpoint is the whole point
	// and must outlive the window. ServiceShutdown still runs on a real quit.
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		window.Hide()
		event.Cancel()
	})

	// Start at launch so the agent URL works immediately.
	if current.StartMCPOnLaunch {
		if _, err := mcpService.StartServer(current.Port, string(current.Transport)); err != nil {
			slog.Warn("MCP server did not start", "err", err)
		}
	}

	// The tray is created after the first start so its menu opens with the
	// real state; ServiceStartup has not run yet, so no mcp:status event has
	// reached the listener at this point.
	installTray(app, window, mcpService, cfg)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
