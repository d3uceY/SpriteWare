// Command spriteware-mcp runs the MCP server over stdio.
//
// The app serves the same tools over HTTP; this entry point is for agent hosts
// that launch a child process, and for CI.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
	"github.com/d3uceY/spriteware/internal/mcpserver"
	"github.com/d3uceY/spriteware/internal/tools"
)

// version is stamped at build time with -ldflags "-X main.version=…".
var version = "0.1.0"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

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

	srv := mcpserver.New(tools.New(cfg, links), version)

	// stdio is what agents expect from a child process. stderr is for logging;
	// stdout belongs to the protocol.
	if err := server.ServeStdio(srv); err != nil {
		fmt.Fprintln(os.Stderr, "spriteware-mcp:", err)
		os.Exit(1)
	}
}
