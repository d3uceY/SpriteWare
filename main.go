// SpriteWare - Wails v3 + Elm starter.
//
// Layers, one job each:
//
//	backend/   Go services - every exported method becomes a JS binding
//	main.go    window, embedded assets, packaging (this file)
//	src/main.js  ports <-> bindings, the only file that knows both worlds
//	elm/src    state, view, subscriptions
package main

import (
	"embed"
	"log"
	"time"

	"github.com/d3uceY/spriteware/backend"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// frontend/dist is compiled by `npm run build` and embedded here. The `all:`
// prefix is what lets the pattern match files starting with "." or "_", which
// Go skips by default. This is resolved at compile time, so the frontend must
// be built before `go build`.
//
//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Registering the event type gives the binding generator something to work
	// with, so the frontend can import the event name instead of a string.
	application.RegisterEvent[string]("tick")
}

func main() {
	app := application.New(application.Options{
		Name:        "SpriteWare",
		Description: "Wails v3 + Elm starter template",
		Services: []application.Service{
			application.NewService(&backend.App{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// A 900x620 window gives roughly 884px of client width on Windows, which is
	// what media queries actually see.
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

	// Demo of the push direction: Go -> event -> main.js -> Elm port -> update.
	// Nothing needs this; delete it once you have real events.
	go func() {
		for {
			app.Event.Emit("tick", time.Now().Format(time.TimeOnly))
			time.Sleep(time.Second)
		}
	}()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
