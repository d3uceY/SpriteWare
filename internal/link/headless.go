package link

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/luaext"
	"github.com/d3uceY/spriteware/internal/protocol"
)

// Defaults from measurement: batch mode starts in ~0.4s warm, seconds on a cold
// disk.
const (
	defaultStartupTimeout = 25 * time.Second
	defaultCallTimeout    = 25 * time.Second
	// diagnosticLines is how much stderr a crash report keeps.
	diagnosticLines = 24
)

// HeadlessOptions configures the background-Aseprite link.
type HeadlessOptions struct {
	// Exe is the Aseprite executable. Empty means find it later, and every call
	// then fails with a clear message.
	Exe string
	// Root holds extension/ and headless/. Empty unpacks the embedded copies.
	Root string
	// Cwd is the batch process's working directory; relative tool paths resolve
	// here.
	Cwd string
	// UserFolder overrides Aseprite's settings directory. Default is a scratch
	// folder, so a background run never loads the user's extensions.
	UserFolder string
	// AllowLua passes the escape hatch through to the script.
	AllowLua bool
	// Timeout bounds one command. Zero uses the default.
	Timeout time.Duration
	// StartupTimeout bounds the wait for the first hello. Zero uses the default.
	StartupTimeout time.Duration
	// Args overrides the process arguments: tests run a fake Aseprite with it.
	Args []string
	// Log receives non-fatal diagnostics.
	Log func(format string, args ...any)
}

// Headless is one long-lived `aseprite -b` process.
//
// Long-lived on purpose: a process per call would reload the document and lose
// the active layer, frame and undo history. One process keeps documents in
// memory like the editor does.
type Headless struct {
	opts HeadlessOptions

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	pending map[string]chan *protocol.Reply
	hello   *protocol.Hello
	// lostSession is set when the process died between calls. The next call
	// reports it, because every open document is gone.
	lostSession *protocol.Error
	closing     bool
	stderr      []string
	started     chan struct{}
	startErr    error

	// luaCleanup removes the unpacked Lua folder, or does nothing when the caller
	// brought its own.
	luaCleanup func()
	// luaRoot is where those scripts were placed, so a startup failure can name
	// the folder.
	luaRoot string

	seq atomic.Uint64
}

// NewHeadless builds a headless link without starting Aseprite; the first call
// does, so constructing one is always safe.
func NewHeadless(opts HeadlessOptions) (*Headless, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = defaultCallTimeout
	}
	if opts.StartupTimeout <= 0 {
		opts.StartupTimeout = defaultStartupTimeout
	}
	if opts.Log == nil {
		opts.Log = func(string, ...any) {}
	}
	return &Headless{opts: opts, pending: map[string]chan *protocol.Reply{}}, nil
}

// Mode reports headless.
func (h *Headless) Mode() Mode { return ModeHeadless }

// Hello returns the greeting, or nil before the first call.
func (h *Headless) Hello() *protocol.Hello {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hello
}

// Features lists what the installed script supports.
func (h *Headless) Features() []string {
	if hello := h.Hello(); hello != nil {
		return hello.Features
	}
	return nil
}

// Call runs one command.
func (h *Headless) Call(ctx context.Context, cmd string, args any, out any) error {
	if err := h.ensureStarted(ctx); err != nil {
		return err
	}

	raw, err := encodeArgs(args)
	if err != nil {
		return protocol.New(protocol.CodeInvalidArgs, "encode arguments: %v", err).WithRemediation("check the tool arguments")
	}

	id := fmt.Sprintf("r%d", h.seq.Add(1))
	line, err := protocol.EncodeCommand(protocol.Command{ID: id, Cmd: cmd, Args: raw})
	if err != nil {
		return protocol.AsError(err)
	}

	ch := make(chan *protocol.Reply, 1)
	h.mu.Lock()
	if h.stdin == nil {
		h.mu.Unlock()
		return h.lostError()
	}
	h.pending[id] = ch
	stdin := h.stdin
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
	}()

	if _, err := stdin.Write(line); err != nil {
		return protocol.New(protocol.CodeHeadlessExited, "could not send %q to Aseprite: %v", cmd, err).
			WithRemediation("run the doctor tool; the background Aseprite is not accepting commands")
	}

	timeout := h.opts.Timeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case reply := <-ch:
		if reply == nil {
			return h.lostError()
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
		return protocol.New(protocol.CodeTimeout, "%s did not answer within %s", cmd, timeout).
			WithRemediation("the document may be very large; try a filter such as a single layer or frame").
			WithDetail("cmd", cmd)
	}
}

// Dump reads the sprite into memory.
func (h *Headless) Dump(ctx context.Context, filter *doc.Filter) (*doc.Dump, error) {
	// One directory per call, the same rule as the live link: a shared folder let
	// two dumps overwrite each other's manifest and cel PNGs.
	dir, err := os.MkdirTemp("", "spriteware-dump-")
	if err != nil {
		return nil, protocol.AsError(err)
	}
	defer os.RemoveAll(dir)

	var out dumpResult
	if err := h.Call(ctx, "doc.dump", dumpArgs{Filter: filter, Sprite: spriteOf(filter), Dir: dir}, &out); err != nil {
		return nil, err
	}
	return out.toDump()
}

// Close stops the process, discarding open documents. Only for shutdown.
func (h *Headless) Close() error {
	h.mu.Lock()
	h.closing = true
	cmd := h.cmd
	stdin := h.stdin
	h.stdin = nil
	luaCleanup := h.luaCleanup
	h.luaCleanup = nil
	pending := h.pending
	h.pending = map[string]chan *protocol.Reply{}
	h.mu.Unlock()

	if luaCleanup != nil {
		defer luaCleanup()
	}

	for _, ch := range pending {
		select {
		case ch <- nil:
		default:
		}
	}

	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Process lifecycle
// ---------------------------------------------------------------------------

func (h *Headless) ensureStarted(ctx context.Context) error {
	h.mu.Lock()
	switch {
	case h.closing:
		h.mu.Unlock()
		return protocol.New(protocol.CodeHeadlessExited, "the link is closed")
	case h.cmd != nil:
		h.mu.Unlock()
		return nil
	case h.lostSession != nil:
		err := h.lostSession
		h.mu.Unlock()
		return err
	}
	started := h.started
	h.mu.Unlock()

	if started != nil {
		select {
		case <-started:
			return h.startErr
		case <-ctx.Done():
			return protocol.New(protocol.CodeTimeout, "waiting for Aseprite to start")
		case <-time.After(h.opts.StartupTimeout):
			return protocol.New(protocol.CodeHeadlessExited, "Aseprite did not start within %s%s",
				h.opts.StartupTimeout, h.luaNote()).
				WithRemediation("run the doctor tool to check the asepritePath setting")
		}
	}

	h.mu.Lock()
	if h.started == nil {
		h.started = make(chan struct{})
	}
	started = h.started
	h.mu.Unlock()

	h.startErr = h.start()
	close(started)
	return h.startErr
}

// luaNote names the script folder when known, so a broken unpack reads
// differently from a slow launch.
func (h *Headless) luaNote() string {
	h.mu.Lock()
	root := h.luaRoot
	h.mu.Unlock()
	if root == "" {
		return ""
	}
	return " (the scripts are in " + root + ")"
}

func (h *Headless) start() error {
	if strings.TrimSpace(h.opts.Exe) == "" {
		return protocol.New(protocol.CodeNotConnected, "no Aseprite executable is configured").
			WithRemediation("run the doctor tool, then set asepritePath in the settings")
	}

	root, cleanup, err := luaext.Prepare(h.opts.Root)
	if err != nil {
		return protocol.AsError(err)
	}
	h.mu.Lock()
	h.luaCleanup = cleanup
	h.luaRoot = root
	h.mu.Unlock()

	args := h.opts.Args
	if args == nil {
		args = []string{
			"-b",
			"--script-param", "root=" + root,
			"--script", root + string(os.PathSeparator) + "headless" + string(os.PathSeparator) + "runner.lua",
		}
	}

	cmd := exec.Command(h.opts.Exe, args...)
	cmd.Dir = h.opts.Cwd

	env := os.Environ()
	env = append(env, "ASEPRITE_USER_FOLDER="+h.userFolder())
	if h.opts.AllowLua {
		env = append(env, "SPRITEWARE_ALLOW_LUA=1")
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return protocol.New(protocol.CodeHeadlessExited, "open stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return protocol.New(protocol.CodeHeadlessExited, "open stdout: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return protocol.New(protocol.CodeHeadlessExited, "open stderr: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return protocol.New(protocol.CodeNotConnected, "could not start Aseprite at %q: %v (the scripts it was given are in %s)", h.opts.Exe, err, root).
			WithRemediation("check asepritePath in the settings; the Lua folder is printed above so a missing or unreadable copy can be spotted").
			WithDetail("asepritePath", h.opts.Exe).
			WithDetail("luaRoot", root)
	}

	h.mu.Lock()
	h.cmd = cmd
	h.stdin = stdin
	h.mu.Unlock()

	go h.readStdout(stdout)
	go h.readStderr(stderr)
	go h.wait(cmd)
	return nil
}

func (h *Headless) userFolder() string {
	if h.opts.UserFolder != "" {
		return h.opts.UserFolder
	}
	dir := os.TempDir() + string(os.PathSeparator) + "spriteware-headless"
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func (h *Headless) readStdout(r io.Reader) {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			h.handleLine(line)
		}
		if err != nil {
			return
		}
	}
}

func (h *Headless) handleLine(line []byte) {
	if hello, ok := protocol.ParseHello(line); ok {
		h.mu.Lock()
		h.hello = hello
		h.mu.Unlock()
		return
	}
	reply, isFrame, err := protocol.ParseReply(line)
	if !isFrame {
		// Something else wrote to stdout: a diagnostic, never an answer.
		h.appendStderr("stdout: " + strings.TrimSpace(string(line)))
		return
	}
	if err != nil {
		h.appendStderr("malformed frame: " + err.Error())
		return
	}

	h.mu.Lock()
	ch, ok := h.pending[reply.ID]
	h.mu.Unlock()
	if ok {
		select {
		case ch <- reply:
		default:
		}
	}
}

func (h *Headless) readStderr(r io.Reader) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			h.appendStderr(strings.TrimRight(line, "\r\n"))
		}
		if err != nil {
			return
		}
	}
}

func (h *Headless) appendStderr(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	h.mu.Lock()
	h.stderr = append(h.stderr, line)
	if len(h.stderr) > diagnosticLines {
		h.stderr = h.stderr[len(h.stderr)-diagnosticLines:]
	}
	h.mu.Unlock()
	h.opts.Log("%s", line)
}

func (h *Headless) wait(cmd *exec.Cmd) {
	err := cmd.Wait()

	h.mu.Lock()
	stderr := append([]string(nil), h.stderr...)
	closing := h.closing
	h.cmd = nil
	h.stdin = nil
	pending := h.pending
	h.pending = map[string]chan *protocol.Reply{}
	if !closing {
		h.lostSession = protocol.New(protocol.CodeHeadlessExited, "Aseprite stopped: %v", exitReason(err)).
			WithRemediation("documents open in the background Aseprite are gone; open the sprite again").
			WithDetail("stderr", stderr)
	}
	h.mu.Unlock()

	for _, ch := range pending {
		select {
		case ch <- nil:
		default:
		}
	}
}

func exitReason(err error) string {
	if err == nil {
		return "it exited"
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Sprintf("exit code %d", exitErr.ExitCode())
	}
	return err.Error()
}

func (h *Headless) lostError() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lostSession != nil {
		return h.lostSession
	}
	return protocol.New(protocol.CodeHeadlessExited, "the background Aseprite is not running").
		WithRemediation("run the doctor tool to see why it could not start")
}

func wireToError(reply *protocol.Reply) error {
	if reply.Error == nil {
		return protocol.New(protocol.CodeInternal, "Aseprite refused the command without saying why")
	}
	e := protocol.New(reply.Error.Code, "%s", reply.Error.Message)
	if len(reply.Error.Details) > 0 {
		for k, v := range reply.Error.Details {
			e.WithDetail(k, v)
		}
	}
	return e
}

// ---------------------------------------------------------------------------
// Dump transfer
// ---------------------------------------------------------------------------

type dumpArgs struct {
	Filter *doc.Filter `json:"filter,omitempty"`
	Sprite string      `json:"sprite,omitempty"`
	Dir    string      `json:"dir"`
}

// spriteOf reads the target document out of a filter, which is the one place a
// caller names it.
func spriteOf(filter *doc.Filter) string {
	if filter == nil {
		return ""
	}
	return filter.Sprite
}

// dumpResult mirrors what Lua sends: metadata plus a scratch directory of cel
// PNGs.
type dumpResult struct {
	doc.Dump
	Dir string `json:"dir"`
}

func (r dumpResult) toDump() (*doc.Dump, error) {
	if r.Dir == "" {
		d := r.Dump
		return &d, nil
	}
	d, err := doc.Load(r.Dir)
	if err != nil {
		return nil, protocol.AsError(err)
	}
	return d, nil
}
