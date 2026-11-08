package tools

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/link"
	"github.com/d3uceY/spriteware/internal/protocol"
)

// doctorProbeTimeout is how long doctor waits for its ping. An attached but
// unresponsive extension would otherwise stall the report for the whole call
// timeout.
const doctorProbeTimeout = 3 * time.Second

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
			probeCtx, cancel := context.WithTimeout(ctx, doctorProbeTimeout)
			err := l.Call(probeCtx, "ping", nil, &pong)
			cancel()
			report.Connected = err == nil && pong.Pong
			if hello := l.Hello(); hello != nil {
				report.ExtensionVersion = hello.ExtensionVersion
				report.AsepriteVersion = hello.AsepriteVersion
				report.Features = hello.Features
			}
			// Live mode has no Hello on the link; the script version arrives over
			// the bridge instead, so the summary can name the script in use.
			if report.ExtensionVersion == "" && report.Bridge.Connected {
				report.ExtensionVersion = report.Bridge.Version
				report.AsepriteVersion = report.Bridge.Aseprite
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
		if r.Mode == string(config.ModeLive) {
			return "A window is attached to the bridge but did not answer. Aseprite may be busy; try again."
		}
		return "An Aseprite window is attached to the bridge; switch to live mode to use it."
	}
	if r.Remediation != "" {
		return r.Remediation
	}
	return "Call doctor again after changing the settings."
}

// doctorRemediation writes the next step for the mode the caller is in. Headless
// advice must not say "open Aseprite with the extension": that cannot work in
// that mode.
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
			mcp.Description("list | info | new | open | activate | save | save_as | duplicate | close | close_all | resize"),
			mcp.Required(),
			mcp.Enum("list", "info", "new", "open", "activate", "save", "save_as", "duplicate", "close", "close_all", "resize"),
		),
		mcp.WithString("path", mcp.Description("Absolute file path, for open/save_as/duplicate.")),
		mcp.WithString("name", mcp.Description("For new: the name an unsaved document answers to. Without one every new document is called Untitled.")),
		mcp.WithNumber("width", mcp.Description("Canvas width for op=new or resize.")),
		mcp.WithNumber("height", mcp.Description("Canvas height for op=new or resize.")),
		mcp.WithNumber("frames", mcp.Description("For new: how many animation frames to create, default 1.")),
		mcp.WithBoolean("force", mcp.Description("For close_all: discard documents that were never saved. Without it, close_all refuses rather than losing work.")),
		mcp.WithString("sprite", mcp.Description(spriteNote)),
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
			if name := strings.TrimSpace(req.GetString("name", "")); name != "" {
				args["name"] = name
			}
			if frames := req.GetInt("frames", 1); frames > 1 {
				args["frames"] = frames
			}
			var out map[string]any
			if err := s.call(ctx, "doc.new", args, &out); err != nil {
				return fail(err), nil
			}
			return ok(out, "created a new document"), nil

		case "open", "activate":
			// activate also accepts an id or name, which resolveOutput would turn
			// into a path no document answers to. The resolved path goes as `path`,
			// what the caller typed as `sprite`, and the Lua tries both.
			typed := strings.TrimSpace(req.GetString("path", ""))
			if typed == "" {
				typed = strings.TrimSpace(req.GetString("sprite", ""))
			}
			path, err := resolveOutput(typed)
			if err != nil {
				return fail(err), nil
			}
			cmd := "doc.open"
			args := map[string]any{"path": path}
			if opName == "activate" {
				cmd = "doc.activate"
				args["sprite"] = typed
			}
			var out map[string]any
			if err := s.call(ctx, cmd, args, &out); err != nil {
				return fail(err), nil
			}
			if already, _ := out["alreadyOpen"].(bool); already {
				return ok(out, "activated "+typed+", which was already open"), nil
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

		case "close_all":
			args := map[string]any{}
			if req.GetBool("force", false) {
				args["force"] = true
			}
			var out map[string]any
			if err := s.call(ctx, "doc.closeAll", args, &out); err != nil {
				return fail(err), nil
			}
			return ok(out, "closed "+strconv.Itoa(toInt(out["closed"]))+" document(s)"), nil

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
			"list", "info", "new", "open", "activate", "save", "save_as", "duplicate", "close", "close_all", "resize")), nil
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
		"slices":      d.Slices,
		"palette":     d.Palette,
		"activeFrame": d.ActiveFrame,
		"activeLayer": d.ActiveLayer,
		"summary":     summary,
	}
}
