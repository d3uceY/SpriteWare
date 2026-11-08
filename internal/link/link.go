// Package link is the connection between the tools and Aseprite.
//
// Two implementations: one drives a background Aseprite, the other the user's
// open window. The tools cannot tell which, which keeps the modes honest.
package link

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/doc"
	"github.com/d3uceY/spriteware/internal/protocol"
)

// Mode re-exports the config type so callers need only one import.
type Mode = config.Mode

const (
	ModeLive     Mode = config.ModeLive
	ModeHeadless Mode = config.ModeHeadless
)

// Link is one conversation with one Aseprite.
type Link interface {
	// Mode reports which connection this is.
	Mode() Mode
	// Hello reports the script's version and capabilities, or nil before it
	// answers.
	Hello() *protocol.Hello
	// Features lists what the installed script can do.
	Features() []string
	// Call runs one command and decodes its data into out. out may be nil.
	Call(ctx context.Context, cmd string, args any, out any) error
	// Dump reads the sprite into memory. filter may be nil for everything.
	Dump(ctx context.Context, filter *doc.Filter) (*doc.Dump, error)
	// Close ends the conversation.
	Close() error
}

// Selector owns both links and points the tools at whichever is current.
//
// Both are created lazily and live for the process, so switching modes loses no
// work on either side.
type Selector struct {
	mu    sync.RWMutex
	mode  Mode
	live  *Live
	head  *Headless
	opts  Options
	nextL func() (Link, error)
}

// Options configures a Selector.
type Options struct {
	AsepritePath string
	Mode         Mode
	BridgePort   int
	AllowLua     bool
	// Root holds extension/ and headless/. Empty unpacks the embedded copies to
	// a temp directory.
	Root string
}

// NewSelector builds a selector that starts in opts.Mode.
func NewSelector(opts Options) *Selector {
	if opts.Mode != ModeLive {
		opts.Mode = ModeHeadless
	}
	return &Selector{mode: opts.Mode, opts: opts}
}

// Mode reports the current connection.
func (s *Selector) Mode() Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// Use switches the connection. Links are created on demand, so a switch is
// cheap and lossless. It never happens automatically.
func (s *Selector) Use(mode Mode) Mode {
	if mode != ModeLive {
		mode = ModeHeadless
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
	return s.mode
}

// SetAsepritePath repoints the selector and drops the headless link, which was
// started with the old executable.
func (s *Selector) SetAsepritePath(path string) {
	s.mu.Lock()
	s.opts.AsepritePath = path
	head := s.head
	s.head = nil
	s.mu.Unlock()

	if head != nil {
		_ = head.Close()
	}
}

// SetBridgePort repoints the live link and drops the one built for the old port,
// which would otherwise keep dialling a listener that has moved.
func (s *Selector) SetBridgePort(port int) {
	s.mu.Lock()
	s.opts.BridgePort = port
	live := s.live
	s.live = nil
	s.mu.Unlock()

	if live != nil {
		_ = live.Close()
	}
}

// Current returns the link for the current mode, starting it if needed.
func (s *Selector) Current() (Link, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.mode == ModeLive {
		if s.live == nil {
			l, err := NewLive(s.opts.BridgePort)
			if err != nil {
				return nil, err
			}
			s.live = l
		}
		return s.live, nil
	}
	if s.head == nil {
		h, err := NewHeadless(HeadlessOptions{
			Exe:      s.opts.AsepritePath,
			Root:     s.opts.Root,
			AllowLua: s.opts.AllowLua,
		})
		if err != nil {
			return nil, err
		}
		s.head = h
	}
	return s.head, nil
}

// Close shuts down whichever links were created.
func (s *Selector) Close() error {
	s.mu.Lock()
	live, head := s.live, s.head
	s.live, s.head = nil, nil
	s.mu.Unlock()

	var firstErr error
	if live != nil {
		if err := live.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if head != nil {
		if err := head.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// encodeArgs turns any Go value into the raw JSON the protocol carries.
func encodeArgs(args any) (json.RawMessage, error) {
	if args == nil {
		return nil, nil
	}
	if raw, ok := args.(json.RawMessage); ok {
		return raw, nil
	}
	return json.Marshal(args)
}

// asProtocolError converts any error into the coded form.
func asProtocolError(err error) *protocol.Error {
	if err == nil {
		return nil
	}
	return protocol.AsError(err)
}
