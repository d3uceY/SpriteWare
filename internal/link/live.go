package link

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/protocol"
)

// The bridge speaks WebSocket on 127.0.0.1. Aseprite's Lua has no LuaSocket
// (`require("socket")` fails in every build), but it does expose a WebSocket
// class, so the extension dials in over that and the tools use the same
// transport.
//
// Two routes, one per role:
//
//	/extension   Aseprite dialling in
//	/client      an MCP server dialling in
//
// One WebSocket message is one envelope, the same shape the headless runner
// uses on stdin and stdout.

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
	srv     *http.Server
	ext     *peer
	clients map[*peer]bool
	next    int
	state   BridgeState
	// onState runs after any attachment change, so a UI can repaint without
	// polling. Called outside the lock; it must not call back into the bridge
	// from the same goroutine's critical section.
	onState func(BridgeState)
}

// OnStateChange registers the callback run whenever the attached window comes
// or goes.
func (b *Bridge) OnStateChange(fn func(BridgeState)) {
	b.mu.Lock()
	b.onState = fn
	b.mu.Unlock()
}

// NewBridge builds a stopped bridge.
func NewBridge() *Bridge {
	return &Bridge{clients: map[*peer]bool{}}
}

// acceptOptions upgrades every request.
//
// The origin check is off because the listener is already bound to 127.0.0.1,
// which is the boundary that matters, and the extension sends no Origin header
// at all. Aseprite's WebSocket client is not a browser and cannot be made to
// add one.
var acceptOptions = &websocket.AcceptOptions{InsecureSkipVerify: true}

// Start listens on 127.0.0.1:port and upgrades both routes to WebSocket.
func (b *Bridge) Start(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("bridge could not listen on port %d: %w", port, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/extension", b.serveExtension)
	mux.HandleFunc("/client", b.serveClient)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	b.mu.Lock()
	b.ln = ln
	b.srv = srv
	b.mu.Unlock()

	go func() { _ = srv.Serve(ln) }()
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
	srv := b.srv
	ext := b.ext
	clients := b.clients
	b.ln = nil
	b.srv = nil
	b.ext = nil
	b.clients = map[*peer]bool{}
	b.mu.Unlock()

	if ext != nil {
		ext.close()
	}
	for c := range clients {
		c.close()
	}
	if srv != nil {
		return srv.Close()
	}
	return nil
}

// serveExtension holds the user's window open for as long as Aseprite stays up.
func (b *Bridge) serveExtension(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, acceptOptions)
	if err != nil {
		return
	}

	p := newPeer("ext", ws)
	b.mu.Lock()
	previous := b.ext
	b.ext = p
	b.mu.Unlock()
	if previous != nil {
		previous.close()
	}
	b.setState(BridgeState{})

	go b.keepAlive(p)
	b.readPeer(p, b.fromExtension)

	// A replaced window's cleanup used to report the new one as gone, so only
	// the peer that is still current clears the state.
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
}

// serveClient accepts one MCP server's connection.
func (b *Bridge) serveClient(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, acceptOptions)
	if err != nil {
		return
	}

	b.mu.Lock()
	b.next++
	p := newPeer("c"+strconv.Itoa(b.next), ws)
	b.clients[p] = true
	b.mu.Unlock()

	if st := b.State(); st.Connected {
		p.write(statePayload(st))
	}

	b.readPeer(p, b.fromClient)

	b.mu.Lock()
	delete(b.clients, p)
	b.mu.Unlock()
	p.close()
}

// readPeer feeds every message to handle until the socket dies.
func (b *Bridge) readPeer(p *peer, handle func(*peer, []byte)) {
	for {
		_, data, err := p.ws.Read(p.ctx)
		if err != nil {
			return
		}
		handle(p, data)
	}
}

// keepAlive pings the window and drops it when the pong stops.
//
// A killed app leaves the socket looking open on the far side, so without this
// the plate says "connected" for minutes after Aseprite or the app is gone.
func (b *Bridge) keepAlive(p *peer) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(p.ctx, 2*time.Second)
			err := p.ws.Ping(ctx)
			cancel()
			if err != nil {
				p.close()
				return
			}
		}
	}
}

// fromClient prefixes the id with the client's slot, so the reply routes back.
func (b *Bridge) fromClient(p *peer, line []byte) {
	b.mu.Lock()
	ext := b.ext
	b.mu.Unlock()

	var cmd protocol.Command
	if err := json.Unmarshal(line, &cmd); err != nil {
		p.write(replyFrameJSON("", protocol.New(protocol.CodeInvalidArgs, "malformed command")))
		return
	}

	if ext == nil {
		p.write(replyFrameJSON(cmd.ID, protocol.New(protocol.CodeNotConnected,
			"Aseprite is not connected to the bridge").
			WithRemediation("open Aseprite with the SpriteWare script installed, or switch to headless mode")))
		return
	}

	cmd.ID = p.id + "::" + cmd.ID
	out, err := json.Marshal(cmd)
	if err != nil {
		p.write(replyFrameJSON(cmd.ID, protocol.New(protocol.CodeInternal, "encode command: %v", err)))
		return
	}
	ext.write(out)
}

func (b *Bridge) fromExtension(_ *peer, line []byte) {
	if hello, ok := protocol.ParseHello(append([]byte(protocol.FrameMarker), line...)); ok {
		b.setState(BridgeState{Version: hello.ExtensionVersion, Aseprite: hello.AsepriteVersion})
		b.broadcast(statePayload(b.State()))
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
		target.write(out)
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
	hook := b.onState
	b.mu.Unlock()

	if hook != nil {
		hook(st)
	}
}

// ---------------------------------------------------------------------------
// Live link
// ---------------------------------------------------------------------------

// Live talks to the bridge, which talks to the user's window.
type Live struct {
	port int
	url  string

	mu      sync.Mutex
	conn    *websocket.Conn
	hello   *protocol.Hello
	pending map[string]chan *protocol.Reply
	seq     uint64
	state   BridgeState
	// scratchDir holds the PNGs Aseprite writes during a dump; removed on close.
	scratchDir string
}

// NewLive builds a live link for a bridge port. It connects on the first call.
func NewLive(port int) (*Live, error) {
	if port < 1 || port > 65535 {
		return nil, protocol.New(protocol.CodeBadConfig, "bridge port %d is not a usable port", port)
	}
	return &Live{
		port:    port,
		url:     fmt.Sprintf("ws://127.0.0.1:%d/client", port),
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
	connected := l.conn != nil
	l.mu.Unlock()
	if connected {
		return nil
	}

	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, l.url, nil)
	if err != nil {
		return protocol.New(protocol.CodeBridgeUnavailable,
			"no bridge is listening on 127.0.0.1:%d", l.port).
			WithRemediation("start the MCP server from the app, which runs the bridge, or switch to headless mode").
			WithDetail("address", l.url)
	}

	l.mu.Lock()
	l.conn = conn
	l.mu.Unlock()

	go l.readLoop(conn)
	return nil
}

func (l *Live) readLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.Read(context.Background())
		if err != nil {
			l.mu.Lock()
			if l.conn == conn {
				l.conn = nil
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

		if st, ok := parseStateFrame(data); ok {
			l.mu.Lock()
			l.state = st
			l.mu.Unlock()
			continue
		}
		withMarker := append([]byte(protocol.FrameMarker), data...)
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
	conn := l.conn
	ch := make(chan *protocol.Reply, 1)
	l.pending[id] = ch
	l.mu.Unlock()

	defer func() {
		l.mu.Lock()
		delete(l.pending, id)
		l.mu.Unlock()
	}()

	if conn == nil {
		return protocol.New(protocol.CodeNotConnected, "the bridge connection closed").
			WithRemediation("try again; the app restarts the bridge automatically")
	}

	line, err := protocol.EncodeCommand(protocol.Command{ID: id, Cmd: cmd, Args: raw})
	if err != nil {
		return protocol.AsError(err)
	}

	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	writeErr := conn.Write(writeCtx, websocket.MessageText, line)
	cancel()
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
	// The script has no writable idea of "here", so it is handed a folder, the
	// same way the headless runner is.
	dir, err := l.scratch()
	if err != nil {
		return nil, protocol.AsError(err)
	}
	var out dumpResult
	if err := l.Call(ctx, "doc.dump", dumpArgs{Filter: filter, Sprite: spriteOf(filter), Dir: dir}, &out); err != nil {
		return nil, err
	}
	return out.toDump()
}

// scratch returns the folder the script writes cel PNGs into, created on first
// use.
func (l *Live) scratch() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.scratchDir != "" {
		return l.scratchDir, nil
	}
	dir, err := os.MkdirTemp("", "spriteware-live-")
	if err != nil {
		return "", err
	}
	l.scratchDir = dir
	return dir, nil
}

// Close drops the bridge connection.
func (l *Live) Close() error {
	l.mu.Lock()
	conn := l.conn
	l.conn = nil
	scratch := l.scratchDir
	l.scratchDir = ""
	l.mu.Unlock()
	if scratch != "" {
		defer os.RemoveAll(scratch)
	}
	if conn != nil {
		return conn.CloseNow()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Peer and framing helpers
// ---------------------------------------------------------------------------

// peer is one WebSocket on the bridge: the user's window, or one MCP server.
type peer struct {
	id   string
	ws   *websocket.Conn
	ctx  context.Context
	stop context.CancelFunc
	wmu  sync.Mutex
}

func newPeer(id string, ws *websocket.Conn) *peer {
	ctx, stop := context.WithCancel(context.Background())
	return &peer{id: id, ws: ws, ctx: ctx, stop: stop}
}

// write sends one envelope. A dead socket is dropped silently: the reader loop
// notices and unregisters the peer.
func (p *peer) write(body []byte) {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	ctx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
	defer cancel()
	_ = p.ws.Write(ctx, websocket.MessageText, body)
}

func (p *peer) close() {
	p.stop()
	_ = p.ws.CloseNow()
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

func statePayload(st BridgeState) []byte {
	payload := map[string]any{"type": "bridge_state", "state": st}
	out, err := json.Marshal(payload)
	if err != nil {
		return []byte(`{"type":"bridge_state"}`)
	}
	return out
}

func parseStateFrame(line []byte) (BridgeState, bool) {
	var probe struct {
		Type  string      `json:"type"`
		State BridgeState `json:"state"`
	}
	if err := json.Unmarshal(line, &probe); err != nil || probe.Type != "bridge_state" {
		return BridgeState{}, false
	}
	return probe.State, true
}
