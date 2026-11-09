//go:build server

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/d3uceY/spriteware/backend"
	"github.com/d3uceY/spriteware/internal/config"
)

// Server mode is the headless HTTP build: there is no window and no tray. The
// HTTP server itself is the thing that must stay up.
func installTray(_ *application.App, _ application.Window, _ *backend.MCPService, _ *config.Store) {}
