package link

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/protocol"
)

// The bridge speaks newline-delimited JSON over plain TCP on 127.0.0.1. Both
// ends are ours, so no WebSocket: no framing layer to get wrong, luasocket is
// enough, and a test can drive it.
//
// The first line names the caller:
//
//	{"role":"extension"}   Aseprite dialling in
//	{"role":"client"}      an MCP server dialling in
//
// Everything after that is the same framed envelope the headless runner uses.

// BridgeState is what the bridge knows about the Aseprite attached to it.
type BridgeState struct {
	Connected bool   `json:"connected"`
	Version   string `json:"version,omitempty"`
	Aseprite  string `json:"aseprite,omitempty"`
	Clients   int    `json:"clients"`
}

// Bridge holds the line open between the tools and the user's Aseprite window.
//
// Separate from the MCP server on purpose: an agent host may restart the server,
// and a socket there would drop the window every time.
type Bridge struct {
	mu      sync.Mutex
	ln      net.Listener
	ext     *peer
	clients map[*peer]bool
	next    int
	state   BridgeState
}

// NewBridge builds a stopped bridge.
func NewBridge() *Bridge {
	return &Bridge{clients: map[*peer]bool{}}
}

// Start listens on 127.0.0.1:port.
func (b *Bridge) Start(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("bridge could not listen on port %d: %w", port, err)
	}
	b.mu.Lock()
	b.ln = ln
	b.mu.Unlock()

	go b.acceptLoop(ln)
	return nil
}

// Port reports the listening port, or 0 before Start.
func (b *Bridge) Port() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ln == nil {
		return 0
	}
	if addr, ok := b.ln.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

// State reports the current attachment.
func (b *Bridge) State() BridgeState {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.state
	st.Connected = b.ext != nil
	st.Clients = len(b.clients)
	return st
}

// Close stops listening and drops every connection.
func (b *Bridge) Close() error {
	b.mu.Lock()
	ln := b.ln
	ext := b.ext
	clients := b.clients
	b.ln = nil
	b.ext = nil
	b.clients = map[*peer]bool{}
	b.mu.Unlock()

	if ext != nil {
		ext.close()
	}
	for c := range clients {
		c.close()
	}
	if ln != nil {
		return ln.Close()
	}
	return nil
}

func (b *Bridge) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go b.serveConn(conn)
	}
}

func (b *Bridge) serveConn(conn net.Conn) {
	reader := bufio.NewReaderSize(conn, 1<<20)

	// The role line, then everything else is frames.
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	roleLine, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	var hello struct {
		Role string `json:"role"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(roleLine)), &hello) != nil {
		_ = conn.Close()
		return
	}

	p := &peer{conn: conn, writer: bufio.NewWriter(conn)}

	switch hello.Role {
	case "extension":
		p.id = "ext"
		b.mu.Lock()
		if b.ext != nil {
			b.ext.close()
		}
		b.ext = p
		b.mu.Unlock()
		b.setState(BridgeState{})
		b.readExtension(p, reader)

	case "client":
		b.mu.Lock()
		b.next++
		p.id = "c" + strconv.Itoa(b.next)
		b.clients[p] = true
		b.mu.Unlock()

		if st := b.State(); st.Connected {
			p.write(stateFrame(st))
		}
		b.readClient(p, reader)

	default:
		_ = conn.Close()
	}
}

func (b *Bridge) readExtension(p *peer, reader *bufio.Reader) {
	defer func() {
		b.mu.Lock()
		wasCurrent := b.ext == p
		if wasCurrent {
			b.ext = nil
		}
		b.mu.Unlock()
		if wasCurrent {
			b.setState(BridgeState{})
		}
		p.close()
	}()

	for {
		line, err := readFrame(reader)
		if err != nil {
			return
		}
		b.fromExtension(p, line)
	}
}

func (b *Bridge) readClient(p *peer, reader *bufio.Reader) {
	defer func() {
		b.mu.Lock()
		delete(b.clients, p)
		b.mu.Unlock()
		p.close()
	}()

	for {
		line, err := readFrame(reader)
		if err != nil {
			return
		}
		b.fromClient(p, line)
	}
}

// readFrame reads one line and strips the marker.
func readFrame(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	return stripMarker(line), nil
}

// fromClient prefixes the id with the client's slot, so the reply routes back.
func (b *Bridge) fromClient(p *peer, line []byte) {
	b.mu.Lock()
	ext := b.ext
	b.mu.Unlock()

	var cmd protocol.Command
	if err := json.Unmarshal(line, &cmd); err != nil {
		p.write(frame(replyFrameJSON("", protocol.New(protocol.CodeInvalidArgs, "malformed command"))))
		return
	}

	if ext == nil {
		p.write(frame(replyFrameJSON(cmd.ID, protocol.New(protocol.CodeNotConnected,
			"Aseprite is not connected to the bridge").
			WithRemediation("open Aseprite with the SpriteWare script installed, or switch to headless mode"))))
		return
	}

	cmd.ID = p.id + "::" + cmd.ID
	out, err := json.Marshal(cmd)
	if err != nil {
		p.write(frame(replyFrameJSON(cmd.ID, protocol.New(protocol.CodeInternal, "encode command: %v", err))))
		return
	}
	ext.write(frame(out))
}

func (b *Bridge) fromExtension(_ *peer, line []byte) {
	if hello, ok := protocol.ParseHello(append([]byte(protocol.FrameMarker), line...)); ok {
		b.setState(BridgeState{Version: hello.ExtensionVersion, Aseprite: hello.AsepriteVersion})
		b.broadcast(stateFrame(b.State()))
		return
	}

	var reply protocol.Reply
	if err := json.Unmarshal(line, &reply); err != nil {
		return
	}

	clientID, localID, ok := strings.Cut(reply.ID, "::")
	if !ok {
		return
	}
	reply.ID = localID
	out, err := json.Marshal(reply)
	if err != nil {
		return
	}

	b.mu.Lock()
	var target *peer
	for c := range b.clients {
		if c.id == clientID {
			target = c
			break
		}
	}
	b.mu.Unlock()

	if target != nil {
		target.write(frame(out))
	}
}

func (b *Bridge) broadcast(f []byte) {
	b.mu.Lock()
	clients := make([]*peer, 0, len(b.clients))
	for c := range b.clients {
		clients = append(clients, c)
	}
	b.mu.Unlock()
	for _, c := range clients {
		c.write(f)
	}
}

// setState records what the attached window reports. Connected is read from
// b.ext under the same lock: a replaced window's cleanup used to report the new
// one as gone.
func (b *Bridge) setState(st BridgeState) {
	b.mu.Lock()
	st.Connected = b.ext != nil
	if !st.Connected {
		st.Version, st.Aseprite = "", ""
	}
	b.state = st
	b.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Live link
// ---------------------------------------------------------------------------

// Live talks to the bridge, which talks to the user's window.
type Live struct {
	port int
	addr string

	mu      sync.Mutex
	conn    net.Conn
	writer  *bufio.Writer
	hello   *protocol.Hello
	pending map[string]chan *protocol.Reply
	seq     uint64
	state   BridgeState
}

// NewLive builds a live link for a bridge port. It connects on the first call.
func NewLive(port int) (*Live, error) {
	if port < 1 || port > 65535 {
		return nil, protocol.New(protocol.CodeBadConfig, "bridge port %d is not a usable port", port)
	}
	return &Live{
		port:    port,
		addr:    fmt.Sprintf("127.0.0.1:%d", port),
		pending: map[string]chan *protocol.Reply{},
	}, nil
}

// Mode reports live.
func (l *Live) Mode() Mode { return ModeLive }

// Hello returns the extension greeting once it has arrived.
func (l *Live) Hello() *protocol.Hello {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.hello
}

// Features lists what the installed script supports.
func (l *Live) Features() []string {
	if h := l.Hello(); h != nil {
		return h.Features
	}
	return nil
}

// State reports whether a window is attached.
func (l *Live) State() BridgeState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state
}

// Connect dials the bridge if it is not already connected.
func (l *Live) Connect(ctx context.Context) error {
	l.mu.Lock()
	if l.conn != nil {
		l.mu.Unlock()
		return nil
	}
	l.mu.Unlock()

	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", l.addr)
	if err != nil {
		return protocol.New(protocol.CodeBridgeUnavailable,
			"no bridge is listening on 127.0.0.1:%d", l.port).
			WithRemediation("start the MCP server from the app, which runs the bridge, or switch to headless mode").
			WithDetail("address", l.addr)
	}

	writer := bufio.NewWriter(conn)
	if _, err := writer.WriteString("{\"role\":\"client\"}\n"); err != nil {
		_ = conn.Close()
		return protocol.New(protocol.CodeBridgeUnavailable, "bridge handshake failed: %v", err)
	}
	if err := writer.Flush(); err != nil {
		_ = conn.Close()
		return protocol.New(protocol.CodeBridgeUnavailable, "bridge handshake failed: %v", err)
	}

	l.mu.Lock()
	l.conn = conn
	l.writer = writer
	l.mu.Unlock()

	go l.readLoop(conn)
	return nil
}

func (l *Live) readLoop(conn net.Conn) {
	reader := bufio.NewReaderSize(conn, 1<<20)
	for {
		line, err := readFrame(reader)
		if err != nil {
			l.mu.Lock()
			if l.conn == conn {
				l.conn = nil
				l.writer = nil
				l.state = BridgeState{}
			}
			pending := l.pending
			l.pending = map[string]chan *protocol.Reply{}
			l.mu.Unlock()
			for _, ch := range pending {
				select {
				case ch <- nil:
				default:
				}
			}
			return
		}

		if st, ok := parseStateFrame(line); ok {
			l.mu.Lock()
			l.state = st
			l.mu.Unlock()
			continue
		}
		withMarker := append([]byte(protocol.FrameMarker), line...)
		if hello, ok := protocol.ParseHello(withMarker); ok {
			l.mu.Lock()
			l.hello = hello
			l.mu.Unlock()
			continue
		}
		reply, isFrame, perr := protocol.ParseReply(withMarker)
		if !isFrame || perr != nil {
			continue
		}
		l.mu.Lock()
		ch, ok := l.pending[reply.ID]
		l.mu.Unlock()
		if ok {
			select {
			case ch <- reply:
			default:
			}
		}
	}
}

// Call runs one command over the bridge.
func (l *Live) Call(ctx context.Context, cmd string, args any, out any) error {
	if err := l.Connect(ctx); err != nil {
		return err
	}

	raw, err := encodeArgs(args)
	if err != nil {
		return protocol.New(protocol.CodeInvalidArgs, "encode arguments: %v", err)
	}

	l.mu.Lock()
	l.seq++
	id := fmt.Sprintf("r%d", l.seq)
	conn, writer, ok := l.conn, l.writer, l.conn != nil
	ch := make(chan *protocol.Reply, 1)
	l.pending[id] = ch
	l.mu.Unlock()

	defer func() {
		l.mu.Lock()
		delete(l.pending, id)
		l.mu.Unlock()
	}()

	if !ok || conn == nil || writer == nil {
		return protocol.New(protocol.CodeNotConnected, "the bridge connection closed").
			WithRemediation("try again; the app restarts the bridge automatically")
	}

	line, err := protocol.EncodeCommand(protocol.Command{ID: id, Cmd: cmd, Args: raw})
	if err != nil {
		return protocol.AsError(err)
	}

	l.mu.Lock()
	_, writeErr := writer.Write(line)
	if writeErr == nil {
		writeErr = writer.Flush()
	}
	l.mu.Unlock()
	if writeErr != nil {
		return protocol.New(protocol.CodeBridgeUnavailable, "could not send %q: %v", cmd, writeErr)
	}

	timeout := defaultCallTimeout
	if deadline, ctxOK := ctx.Deadline(); ctxOK {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case reply := <-ch:
		if reply == nil {
			return protocol.New(protocol.CodeNotConnected, "Aseprite closed the connection during %q", cmd)
		}
		if !reply.OK {
			return wireToError(reply)
		}
		if out != nil && len(reply.Data) > 0 {
			if err := json.Unmarshal(reply.Data, out); err != nil {
				return protocol.New(protocol.CodeInternal, "decode %q result: %v", cmd, err)
			}
		}
		return nil
	case <-ctx.Done():
		return protocol.New(protocol.CodeTimeout, "%s was cancelled", cmd)
	case <-timer.C:
		return protocol.New(protocol.CodeTimeout, "%s did not answer within %s", cmd, timeout)
	}
}

// Dump reads the sprite through the window.
func (l *Live) Dump(ctx context.Context, filter *doc.Filter) (*doc.Dump, error) {
	var out dumpResult
	if err := l.Call(ctx, "doc.dump", dumpArgs{Filter: filter}, &out); err != nil {
		return nil, err
	}
	return out.toDump()
}

// Close drops the bridge connection.
func (l *Live) Close() error {
	l.mu.Lock()
	conn := l.conn
	l.conn = nil
	l.writer = nil
	l.mu.Unlock()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Peer and framing helpers
// ---------------------------------------------------------------------------

type peer struct {
	id     string
	conn   net.Conn
	writer *bufio.Writer
	wmu    sync.Mutex
}

func (p *peer) write(data []byte) {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	if _, err := p.writer.Write(data); err != nil {
		return
	}
	_ = p.writer.Flush()
}

func (p *peer) close() {
	_ = p.conn.Close()
}

// frame prefixes a payload with the protocol marker and a newline.
func frame(body []byte) []byte {
	out := make([]byte, 0, len(body)+len(protocol.FrameMarker)+1)
	out = append(out, protocol.FrameMarker...)
	out = append(out, body...)
	out = append(out, '\n')
	return out
}

// stripMarker removes the marker and any trailing newline.
func stripMarker(line []byte) []byte {
	s := strings.TrimPrefix(string(line), protocol.FrameMarker)
	return []byte(strings.TrimRight(s, "\r\n"))
}

func replyFrameJSON(id string, err *protocol.Error) []byte {
	reply := protocol.Reply{ID: id, OK: false, Error: &protocol.WireError{
		Code:    err.Code,
		Message: err.Message,
		Details: err.Details,
	}}
	out, marshalErr := json.Marshal(reply)
	if marshalErr != nil {
		return []byte(`{"ok":false,"error":{"code":"internal","message":"encode failed"}}`)
	}
	return out
}

func stateFrame(st BridgeState) []byte {
	payload := map[string]any{"type": "bridge_state", "state": st}
	out, err := json.Marshal(payload)
	if err != nil {
		return frame([]byte(`{"type":"bridge_state"}`))
	}
	return frame(out)
}

func parseStateFrame(line []byte) (BridgeState, bool) {
	var probe struct {
		Type  string      `json:"type"`
		State BridgeState `json:"state"`
	}
	if err := json.Unmarshal(stripMarker(line), &probe); err != nil || probe.Type != "bridge_state" {
		return BridgeState{}, false
	}
	return probe.State, true
}
