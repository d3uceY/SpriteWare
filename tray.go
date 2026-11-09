//go:build !server

package main

import (
	_ "embed"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/d3uceY/spriteware/backend"
	"github.com/d3uceY/spriteware/internal/config"
)

// The tray mark is the app's hat, drawn twice: black for light taskbars and
// white for dark ones. macOS wants the black one as a template image, which the
// system tints to match the menu bar.
//
//go:embed assets/tray/tray-black.png
var trayBlack []byte

//go:embed assets/tray/tray-white.png
var trayWhite []byte

// tray keeps the window out of the way without taking the MCP endpoint with it.
// Closing the window hides it; the process, the server and the bridge stay up
// until Quit is chosen from this menu.
type tray struct {
	app        *application.App
	window     application.Window
	systemTray *application.SystemTray
	statusLine *application.MenuItem
	toggleItem *application.MenuItem
	mcp        *backend.MCPService
	cfg        *config.Store
}

func installTray(app *application.App, window application.Window, mcp *backend.MCPService, cfg *config.Store) *tray {
	t := &tray{app: app, window: window, mcp: mcp, cfg: cfg}
	t.systemTray = app.SystemTray.New()

	// Windows picks between the light and dark icons by the taskbar theme.
	// macOS gets a template image and does the tinting itself. Linux panels do
	// not expose a matching per-icon theme switch, so the black mark is the
	// safe default there.
	switch runtime.GOOS {
	case "windows":
		t.systemTray.SetIcon(trayBlack)
		t.systemTray.SetDarkModeIcon(trayWhite)
	case "darwin":
		t.systemTray.SetTemplateIcon(trayBlack)
	default:
		t.systemTray.SetIcon(trayBlack)
	}
	t.systemTray.SetTooltip("SpriteWare")

	menu := app.NewMenu()
	menu.Add("Open SpriteWare").OnClick(func(*application.Context) {
		t.showWindow()
	})
	t.statusLine = menu.Add("MCP server: stopped").SetEnabled(false)
	t.toggleItem = menu.Add("Start MCP server").OnClick(func(*application.Context) {
		t.toggleServer()
	})
	menu.AddSeparator()
	menu.Add("Quit SpriteWare").OnClick(func(*application.Context) {
		// A real quit runs ServiceShutdown, which closes the MCP socket and the
		// Aseprite bridge. Closing the window no longer does that.
		t.app.Quit()
	})
	t.systemTray.SetMenu(menu)

	// Windows and Linux: one click brings the window back. macOS left-click
	// opens the menu instead, which is its convention; the menu has the same
	// "Open SpriteWare" row.
	if runtime.GOOS != "darwin" {
		t.systemTray.OnClick(t.showWindow)
	}

	// The service already emits this event for the window. Reuse it so the tray
	// never has to poll the server.
	app.Event.On("mcp:status", func(event *application.CustomEvent) {
		if status, ok := event.Data.(backend.MCPStatus); ok {
			t.showStatus(status)
		}
	})
	t.showStatus(mcp.GetMCPStatus())
	return t
}

func (t *tray) showWindow() {
	t.window.Show()
	t.window.UnMinimise()
	t.window.Focus()
}

func (t *tray) toggleServer() {
	status := t.mcp.GetMCPStatus()
	if status.Running {
		if err := t.mcp.StopServer(); err != nil {
			slog.Warn("tray could not stop the MCP server", "err", err)
		}
		return
	}

	current := t.cfg.Get()
	if _, err := t.mcp.StartServer(current.Port, string(current.Transport)); err != nil {
		slog.Warn("tray could not start the MCP server", "err", err)
	}
}

// showStatus keeps the menu and the hover text honest without rebuilding the
// menu while it is open.
func (t *tray) showStatus(status backend.MCPStatus) {
	if status.Running {
		t.statusLine.SetLabel(fmt.Sprintf("MCP server: listening on %d", status.Port))
		t.toggleItem.SetLabel("Stop MCP server")
		if status.URL != "" {
			t.systemTray.SetTooltip("SpriteWare — " + status.URL)
		} else {
			t.systemTray.SetTooltip("SpriteWare — MCP server listening")
		}
		return
	}

	if status.LastError != "" {
		t.statusLine.SetLabel("MCP server: " + status.LastError)
	} else {
		t.statusLine.SetLabel("MCP server: stopped")
	}
	t.toggleItem.SetLabel("Start MCP server")
	t.systemTray.SetTooltip("SpriteWare — MCP server stopped")
}
