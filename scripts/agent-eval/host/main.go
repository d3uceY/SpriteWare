// Command evalhost starts the SpriteWare MCP server headlessly, outside the
// window, so an agent eval can drive the same tool surface the app serves.
//
// This is a measuring instrument, not a shipped command: it lives under tmp/.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
	"github.com/d3uceY/spriteware/internal/mcpserver"
	"github.com/d3uceY/spriteware/internal/tools"
)

func main() {
	store, err := config.Load()
	if err != nil {
		fail("load config: %v", err)
	}
	c := store.Get()
	links := link.NewSelector(link.Options{
		AsepritePath: c.AsepritePath,
		Mode:         c.Mode,
		BridgePort:   c.BridgePort,
		AllowLua:     c.AllowLua,
	})
	defer links.Close()

	host := &mcpserver.Host{}
	url, err := host.Start(mcpserver.Options{
		Port:      c.Port,
		Transport: c.Transport,
		Tools:     tools.New(store, links),
		Version:   "eval",
	})
	if err != nil {
		fail("start: %v", err)
	}
	fmt.Printf("ready %s tools=%d mode=%s aseprite=%s\n", url, mcpserver.ToolCount(), c.Mode, c.AsepritePath)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	_ = host.Stop()
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "evalhost: "+format+"\n", args...)
	os.Exit(1)
}
