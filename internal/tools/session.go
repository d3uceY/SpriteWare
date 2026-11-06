package tools

import (
	"context"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/link"
	"github.com/d3uceY/spriteware/internal/protocol"
)

// DoctorReport answers "why is this not working" without leaving the tools.
type DoctorReport struct {
	OK               bool             `json:"ok"`
	Mode             string           `json:"mode"`
	AsepritePath     string           `json:"asepritePath,omitempty"`
	AsepriteFound    bool             `json:"asepriteFound"`
	Connected        bool             `json:"connected"`
	ExtensionVersion string           `json:"extensionVersion,omitempty"`
	AsepriteVersion  string           `json:"asepriteVersion,omitempty"`
	Features         []string         `json:"features,omitempty"`
	Bridge           link.BridgeState `json:"bridge"`
	// BridgeListening is false when there is no bridge at all, which differs from
	// a bridge with nothing attached.
	BridgeListening bool          `json:"bridgeListening"`
	ConfigFile      string        `json:"configFile"`
	Config          config.Config `json:"config"`
	Remediation     string        `json:"remediation,omitempty"`
	Directive       string        `json:"directive"`
	Summary         string        `json:"summary"`
}

func (s *Service) registerDoctor(srv *server.MCPServer) {
	tool := mcp.NewTool("doctor",
		mcp.WithDescription(
			"Report the state of the connection to Aseprite. Always answers, never blocks anything.\n"+
				"This is a report, not a gate: every other tool connects itself when it needs to, so a broken "+
				"connection can never make the tools that would fix it unreachable.\n"+
				"Returns the mode, whether the Aseprite executable exists, whether the script answered, its "+
				"version and features, and a remediation written for the mode you are in.\n"+
				"Pass mode=\"headless\" or mode=\"live\" to switch. Both connections stay alive, so a switch "+
				"loses no documents on either side."),
		mcp.WithString("mode",
			mcp.Description("Switch the connection. Omit to just report."),
			mcp.Enum(string(config.ModeHeadless), string(config.ModeLive)),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cfg := s.cfg.Get()

		if want := strings.TrimSpace(req.GetString("mode", "")); want != "" {
			s.links.Use(config.Mode(want))
			cfg = s.cfg.Get()
		}

		report := DoctorReport{
			Mode:       string(s.links.Mode()),
			ConfigFile: s.cfg.FilePath(),
			Config:     cfg,
		}

		// The bridge is separate from the mode: live can have nothing attached, and
		// headless can still have a window dialled in.
		if state, running := s.bridgeState(); running {
			report.Bridge = state
			report.BridgeListening = true
		}

		if path, err := config.FindAseprite(cfg.AsepritePath); err == nil {
			report.AsepritePath = path
			report.AsepriteFound = true
		} else {
			report.AsepritePath = cfg.AsepritePath
			report.Remediation = "Set asepritePath in the settings to the full path of aseprite.exe."
		}

		if l, err := s.linkFor(); err != nil {
			report.Summary = "Could not start the " + report.Mode + " connection: " + err.Error()
			if report.Remediation == "" {
				report.Remediation = protocol.AsError(err).Remediation
			}
		} else {
			var pong struct {
				Pong bool `json:"pong"`
			}
			err := l.Call(ctx, "ping", nil, &pong)
			report.Connected = err == nil && pong.Pong
			if hello := l.Hello(); hello != nil {
				report.ExtensionVersion = hello.ExtensionVersion
				report.AsepriteVersion = hello.AsepriteVersion
				report.Features = hello.Features
			}
			if err != nil {
				report.Remediation = doctorRemediation(report.Mode, err, report.AsepriteFound)
			}
			report.Summary = doctorSummary(report)
		}

		report.OK = report.Connected
		report.Directive = doctorDirective(report)
		return ok(report, report.Summary+". "+report.Directive), nil
	})
}

// doctorDirective is the single next action, not four booleans to infer from.
func doctorDirective(r DoctorReport) string {
	if r.Connected {
		return "Ready: draw something, then look at it."
	}
	// A live window is worth naming even when the mode's link is down: it is
	// usually what the user meant.
	if r.BridgeListening && r.Bridge.Connected {
		return "An Aseprite window is attached to the bridge; switch to live mode to use it."
	}
	if r.Remediation != "" {
		return r.Remediation
	}
	return "Call doctor again after changing the settings."
}

// doctorRemediation writes the next step for the mode the caller is in.
//
// The reference project told headless users to "open Aseprite with the
// extension installed", advice that cannot work in that mode.
func doctorRemediation(mode string, err error, found bool) string {
	e := protocol.AsError(err)
	switch e.Code {
	case protocol.CodeNotConnected:
		if !found {
			return "Aseprite was not found. Set asepritePath in the settings, then call doctor again."
		}
		if mode == string(config.ModeLive) {
			return "Open Aseprite and make sure the SpriteWare script is installed, or switch to headless mode."
		}
		return "Aseprite was found but did not start. Check asepritePath points at the executable, not a folder."
	case protocol.CodeBridgeUnavailable:
		return "The bridge is not listening. Start the MCP server from the SpriteWare app, or switch to headless mode."
	case protocol.CodeUnsupportedCommand:
		return "The installed Aseprite script is older than this build. Reinstall the extension from the app."
	case protocol.CodeTimeout:
		return "The command timed out. The document may be very large; try reading one layer or one frame."
	}
	if e.Remediation != "" {
		return e.Remediation
	}
	return "Call doctor again after changing the settings; nothing is cached."
}

func doctorSummary(r DoctorReport) string {
	if r.Connected {
		return r.Mode + " connection is live (script " + r.ExtensionVersion + ")"
	}
	return r.Mode + " connection is not available"
}

// ---------------------------------------------------------------------------
// sprite
// ---------------------------------------------------------------------------

func (s *Service) registerSprite(srv *server.MCPServer) {
	tool := mcp.NewTool("sprite",
		mcp.WithDescription(
			"List, create, open, save, duplicate and inspect documents.\n"+indexNote+"\n"+
				"`info` returns the document as data — dimensions, layers with nesting, frames with "+
				"durations, tags, palette, and a one-line `summary` for humans. There is no prose-only path."),
		mcp.WithString("op",
			mcp.Description("list | info | new | open | activate | save | save_as | duplicate | close | resize"),
			mcp.Required(),
			mcp.Enum("list", "info", "new", "open", "activate", "save", "save_as", "duplicate", "close", "resize"),
		),
		mcp.WithString("path", mcp.Description("Absolute file path, for open/save_as/duplicate.")),
		mcp.WithNumber("width", mcp.Description("Canvas width for op=new or resize.")),
		mcp.WithNumber("height", mcp.Description("Canvas height for op=new or resize.")),
		mcp.WithString("sprite", mcp.Description("Target sprite by absolute path. Omit to use the active one.")),
	)

	srv.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		opName := op(req, "info")
		switch opName {
		case "list":
			var out []doc.Sprite
			if err := s.call(ctx, "doc.list", nil, &out); err != nil {
				return fail(err), nil
			}
			return ok(map[string]any{"sprites": out, "count": len(out)}, "documents"), nil

		case "info":
			var d doc.Dump
			if err := s.call(ctx, "doc.info", spriteArgs(req), &d); err != nil {
				return fail(err), nil
			}
			summary := describeDump(&d)
			return ok(structuredDump(&d, summary), summary), nil

		case "new":
			args := map[string]any{"width": req.GetInt("width", 32), "height": req.GetInt("height", 32)}
			var out map[string]any
			if err := s.call(ctx, "doc.new", args, &out); err != nil {
				return fail(err), nil
			}
			return ok(out, "created a new document"), nil

		case "open", "activate":
			path, err := resolveOutput(req.GetString("path", ""))
			if err != nil {
				return fail(err), nil
			}
			cmd := "doc.open"
			if opName == "activate" {
				cmd = "doc.activate"
			}
			var out map[string]any
			if err := s.call(ctx, cmd, map[string]any{"path": path}, &out); err != nil {
				return fail(err), nil
			}
			return ok(out, "opened "+path), nil

		case "save":
			var out map[string]any
			if err := s.call(ctx, "doc.save", spriteArgs(req), &out); err != nil {
				return fail(err), nil
			}
			return ok(out, "saved"), nil

		case "save_as", "duplicate":
			path, err := resolveOutput(req.GetString("path", ""))
			if err != nil {
				return fail(err), nil
			}
			cmd := "doc.saveAs"
			if opName == "duplicate" {
				cmd = "doc.duplicate"
			}
			args := spriteArgs(req)
			args["path"] = path
			var out map[string]any
			if err := s.call(ctx, cmd, args, &out); err != nil {
				return fail(err), nil
			}
			return ok(out, "wrote "+path), nil

		case "close":
			var out map[string]any
			if err := s.call(ctx, "doc.close", spriteArgs(req), &out); err != nil {
				return fail(err), nil
			}
			return ok(out, "closed"), nil

		case "resize":
			args := spriteArgs(req)
			args["width"] = req.GetInt("width", 0)
			args["height"] = req.GetInt("height", 0)
			var out map[string]any
			if err := s.call(ctx, "doc.resize", args, &out); err != nil {
				return fail(err), nil
			}
			return ok(out, "resized"), nil
		}
		return fail(unknownOp("sprite", opName,
			"list", "info", "new", "open", "activate", "save", "save_as", "duplicate", "close", "resize")), nil
	})
}

func spriteArgs(req mcp.CallToolRequest) map[string]any {
	args := map[string]any{}
	if v := strings.TrimSpace(req.GetString("sprite", "")); v != "" {
		args["sprite"] = v
	}
	return args
}

// describeDump writes the one-line human summary.
func describeDump(d *doc.Dump) string {
	name := d.Sprite.Name
	if name == "" {
		name = "Untitled"
	}
	parts := []string{
		name,
		strconv.Itoa(d.Sprite.Width) + "x" + strconv.Itoa(d.Sprite.Height),
		d.Sprite.ColorMode,
		strconv.Itoa(len(d.Frames)) + " frame(s)",
		strconv.Itoa(len(d.Layers)) + " layer(s)",
		strconv.Itoa(len(d.Palette)) + " colour(s)",
	}
	if len(d.Tags) > 0 {
		names := make([]string, 0, len(d.Tags))
		for _, t := range d.Tags {
			names = append(names, t.Name)
		}
		parts = append(parts, "tags: "+strings.Join(names, ", "))
	}
	return strings.Join(parts, " · ")
}

// structuredDump adds the summary to the structured result.
func structuredDump(d *doc.Dump, summary string) map[string]any {
	return map[string]any{
		"sprite":      d.Sprite,
		"layers":      d.Layers,
		"frames":      d.Frames,
		"tags":        d.Tags,
		"palette":     d.Palette,
		"activeFrame": d.ActiveFrame,
		"activeLayer": d.ActiveLayer,
		"summary":     summary,
	}
}
