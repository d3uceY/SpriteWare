package mcpserver_test

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
	"github.com/d3uceY/spriteware/internal/mcpserver"
	"github.com/d3uceY/spriteware/internal/tools"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func initialize(t *testing.T, url string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "host-test", "version": "0"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Start returns before the goroutine is listening, so the first try can lose
	// a race with the bind.
	deadline := time.Now().Add(3 * time.Second)
	for {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			return resp
		}
		if time.Now().After(deadline) {
			t.Fatalf("the server never answered on %s: %v", url, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestHostServesAndStops(t *testing.T) {
	store, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	links := link.NewSelector(link.Options{})
	defer links.Close()

	host := &mcpserver.Host{}
	url, err := host.Start(mcpserver.Options{
		Port:      freePort(t),
		Transport: config.TransportStreamableHTTP,
		Tools:     tools.New(store, links),
		Version:   "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if url == "" {
		t.Fatal("Start returned no URL")
	}

	resp := initialize(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initialize returned %s, want 200", resp.Status)
	}

	if err := host.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if state := host.State(); state.Running {
		t.Fatal("still running after Stop")
	}
	if url := host.URL(); url != "" {
		t.Fatalf("URL = %q after Stop, want empty", url)
	}
}

func TestHostRefusesATakenPort(t *testing.T) {
	store, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	links := link.NewSelector(link.Options{})
	defer links.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	host := &mcpserver.Host{}
	_, err = host.Start(mcpserver.Options{
		Port:      ln.Addr().(*net.TCPAddr).Port,
		Transport: config.TransportStreamableHTTP,
		Tools:     tools.New(store, links),
		Version:   "test",
	})
	if err == nil {
		t.Fatal("a busy port must be refused")
	}
}
