package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/d3uceY/spriteware/internal/aseprite"
	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
	"github.com/d3uceY/spriteware/internal/mcpserver"
	"github.com/d3uceY/spriteware/internal/tools"
)

// mcpFlags is what one run may override. A flag never rewrites the settings
// file: the window persists what worked, a one-off run must not.
type mcpFlags struct {
	port       int
	transport  string
	mode       string
	bridgePort int
	stdio      bool
	allowLua   bool
}

func newMCPCmd(version string) *cobra.Command {
	f := mcpFlags{}
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve MCP in the foreground until Ctrl+C",
		Long: `Serve the tools over MCP and stay in the foreground: the terminal is the
server's life. Ctrl+C, closing the window, or EOF on stdin stops it.

The default is HTTP on 127.0.0.1, at the port in the settings file. --stdio is
the shape an agent host wants when it launches a child process instead.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runMCP(cmd, version, f) },
	}

	fs := cmd.Flags()
	fs.IntVar(&f.port, "port", 0, "HTTP port (default: the settings file)")
	fs.StringVar(&f.transport, "transport", "", "streamable-http or sse (default: the settings file)")
	fs.StringVar(&f.mode, "mode", "", "headless or live (default: the settings file)")
	fs.IntVar(&f.bridgePort, "bridge-port", 0, "where the Aseprite extension dials in (default: the settings file)")
	fs.BoolVar(&f.stdio, "stdio", false, "serve over stdin/stdout instead of HTTP")
	fs.BoolVar(&f.allowLua, "allow-lua", false, "unlock the run-any-code escape hatch")
	return cmd
}

func runMCP(cmd *cobra.Command, version string, f mcpFlags) error {
	errOut := cmd.ErrOrStderr()

	// A config problem never stops the server: an unreadable file means defaults.
	store, err := config.Load()
	if err != nil {
		fmt.Fprintln(errOut, "config: "+err.Error())
		if store == nil {
			return err
		}
	}
	cfg := store.Get()

	mode := cfg.Mode
	if cmd.Flags().Changed("mode") {
		mode = config.Mode(f.mode)
		if mode != config.ModeLive && mode != config.ModeHeadless {
			return fmt.Errorf("mode %q is not headless or live", f.mode)
		}
	}
	transport := cfg.Transport
	if cmd.Flags().Changed("transport") {
		transport = config.Transport(f.transport)
		if transport != config.TransportSSE && transport != config.TransportStreamableHTTP {
			return fmt.Errorf("transport %q is not streamable-http or sse", f.transport)
		}
	}
	port := cfg.Port
	if cmd.Flags().Changed("port") {
		port = f.port
	}
	bridgePort := cfg.BridgePort
	if cmd.Flags().Changed("bridge-port") {
		bridgePort = f.bridgePort
	}
	// The config is normalised on load, but a flag is not, and the extension has
	// this number baked into it: a nonsense port would install a script that
	// dials nowhere.
	if bridgePort < 1 || bridgePort > 65535 {
		return fmt.Errorf("invalid bridge port %d: use a value in 1..65535", bridgePort)
	}

	// Only live mode needs the bridge; starting it in headless mode would take a
	// port away from a window that is already running one.
	if mode == config.ModeLive {
		bridge := link.NewBridge()
		if err := bridge.Start(bridgePort); err != nil {
			fmt.Fprintln(errOut, "bridge: "+err.Error())
		} else {
			defer bridge.Close()
			// The extension has the port baked into it, so both change together.
			if _, err := aseprite.Install(bridgePort); err != nil {
				fmt.Fprintln(errOut, "extension: "+err.Error())
			}
		}
	}

	links := link.NewSelector(link.Options{
		AsepritePath: cfg.AsepritePath,
		Mode:         mode,
		BridgePort:   bridgePort,
		AllowLua:     cfg.AllowLua || f.allowLua,
	})
	defer links.Close()

	svc := tools.New(store, links)

	if f.stdio {
		// stderr is for logging; stdout belongs to the protocol.
		slog.SetDefault(slog.New(slog.NewTextHandler(errOut, &slog.HandlerOptions{Level: slog.LevelInfo})))
		return server.ServeStdio(mcpserver.New(svc, version))
	}

	host := &mcpserver.Host{}
	url, err := host.Start(mcpserver.Options{
		Port:      port,
		Transport: transport,
		Tools:     svc,
		Version:   version,
	})
	if err != nil {
		return err
	}
	defer host.Stop()

	fmt.Fprintf(errOut, "SpriteWare MCP  %s\n", url)
	fmt.Fprintf(errOut, "mode %s · %d tools · Ctrl+C to stop\n", mode, mcpserver.ToolCount())
	fmt.Fprintf(errOut, "point a client at it:  {\"mcpServers\":{\"spriteware\":{\"url\":%q}}}\n", url)

	// Closing a terminal, a service manager or a parent process all arrive as
	// one of these two; either way the listener goes down before the process.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	fmt.Fprintln(errOut, "stopping")
	return nil
}
