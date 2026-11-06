// Package protocol defines the envelope between the MCP server and the Lua
// script.
//
// One message is one JSON line behind a marker character. Aseprite's warnings
// share our stdout, and without the marker a warning would answer whichever
// request was in flight.
package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ProtocolVersion is bumped whenever the envelope changes shape. The server
// refuses a mismatch instead of guessing.
const ProtocolVersion = 1

// FrameMarker prefixes every reply line. 0x1e is the ASCII record separator,
// which no Aseprite output begins with.
const FrameMarker = "\x1eSW1 "

// Command is a request to the Lua side.
type Command struct {
	ID   string          `json:"id"`
	Cmd  string          `json:"cmd"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Reply is the Lua side's answer.
type Reply struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error *WireError      `json:"error,omitempty"`
}

// WireError is the machine-readable failure the Lua side sends back.
type WireError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Hello is sent unprompted when the script starts answering.
type Hello struct {
	Type             string   `json:"type"`
	Protocol         int      `json:"protocol"`
	ExtensionVersion string   `json:"extensionVersion"`
	AsepriteVersion  string   `json:"asepriteVersion"`
	Headless         bool     `json:"headless"`
	Features         []string `json:"features"`
}

// IsHello reports whether a reply line is the startup greeting.
func (r *Reply) IsHello(raw []byte) bool {
	var probe struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(raw, &probe) == nil && probe.Type == "hello"
}

// EncodeCommand renders one command as a single newline-terminated line.
//
// Commands are not framed: they only travel outward, on a channel nothing else
// writes to.
func EncodeCommand(c Command) ([]byte, error) {
	if c.ID == "" {
		return nil, errors.New("command has no id")
	}
	if c.Cmd == "" {
		return nil, errors.New("command has no name")
	}
	body, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("encode command: %w", err)
	}
	line := make([]byte, 0, len(body)+len(FrameMarker)+1)
	line = append(line, body...)
	line = append(line, '\n')
	return line, nil
}

// ParseReply turns one stdout line into a Reply. Lines without the marker come
// back as not-a-frame, which the caller logs and drops.
func ParseReply(line []byte) (*Reply, bool, error) {
	if !bytes.HasPrefix(line, []byte(FrameMarker)) {
		return nil, false, nil
	}
	body := bytes.TrimSpace(line[len(FrameMarker):])
	if len(body) == 0 {
		return nil, true, errors.New("empty frame")
	}
	var r Reply
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, true, fmt.Errorf("decode frame: %w", err)
	}
	return &r, true, nil
}

// ParseHello decodes a greeting frame.
func ParseHello(line []byte) (*Hello, bool) {
	if !bytes.HasPrefix(line, []byte(FrameMarker)) {
		return nil, false
	}
	body := bytes.TrimSpace(line[len(FrameMarker):])
	var h Hello
	if err := json.Unmarshal(body, &h); err != nil || h.Type != "hello" {
		return nil, false
	}
	return &h, true
}

// Error codes. Every failure maps to one, so a client can branch on the code.
const (
	CodeNotConnected       = "not_connected"
	CodeBridgeUnavailable  = "bridge_unavailable"
	CodeHeadlessExited     = "headless_exited"
	CodeUnsupportedCommand = "unsupported_command"
	CodeNotFound           = "not_found"
	CodeOpenFailed         = "open_failed"
	CodePermissionDenied   = "permission_denied"
	CodeInvalidArgs        = "invalid_args"
	CodeFrameOutOfBounds   = "frame_out_of_bounds"
	CodeLayerNotFound      = "layer_not_found"
	CodeAmbiguousLayer     = "ambiguous_layer"
	CodeTimeout            = "timeout"
	CodeConflict           = "conflict"
	CodeLuaError           = "lua_error"
	CodeInternal           = "internal"
	CodeBadConfig          = "bad_config"
)

// Error is a tool failure carrying a code, a sentence and a next step.
//
// Returned as a tool *result*, never a protocol error: the model has to read the
// message to recover.
type Error struct {
	Code        string
	Message     string
	Remediation string
	Details     map[string]any
}

// New builds an Error with the given code and message.
func New(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func (e *Error) Error() string { return e.Message }

// WithRemediation returns e with a suggested next step.
func (e *Error) WithRemediation(format string, args ...any) *Error {
	e.Remediation = fmt.Sprintf(format, args...)
	return e
}

// WithDetail attaches one machine-readable fact.
func (e *Error) WithDetail(key string, value any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[key] = value
	return e
}

// AsError unwraps any error into an *Error, defaulting to CodeInternal.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return New(CodeInternal, "%v", err)
}

// Render formats the error for a tool result: what happened, then what to do.
func (e *Error) Render() string {
	var b strings.Builder
	b.WriteString(e.Code)
	b.WriteString(": ")
	b.WriteString(e.Message)
	if e.Remediation != "" {
		b.WriteString("\nremediation: ")
		b.WriteString(e.Remediation)
	}
	return b.String()
}
