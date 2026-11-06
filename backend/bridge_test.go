package backend

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/d3uceY/spriteware/internal/aseprite"
	"github.com/d3uceY/spriteware/internal/config"
	"github.com/d3uceY/spriteware/internal/link"
)

// freePort asks the OS for a port and gives it back: the bridge needs a real
// number.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("no free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func serviceAt(t *testing.T, port int) *MCPService {
	t.Helper()
	store, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := store.Save(config.Config{
		Mode:       config.ModeLive,
		BridgePort: port,
		Port:       config.DefaultPort,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	links := link.NewSelector(link.Options{
		Mode:       config.ModeLive,
		BridgePort: port,
	})
	t.Cleanup(func() { _ = links.Close() })
	svc := NewMCPService(store, links)
	t.Cleanup(func() { _ = svc.StopBridge() })
	return svc
}

// Starting the bridge must leave a loadable extension naming the listening port.
func TestStartBridgeInstallsTheExtensionOnThatPort(t *testing.T) {
	ext := t.TempDir()
	t.Setenv(aseprite.EnvOverride, ext)

	port := freePort(t)
	svc := serviceAt(t, port)

	if err := svc.StartBridge(); err != nil {
		t.Fatalf("start bridge: %v", err)
	}

	dir, installedPort, ok := aseprite.Installed()
	if !ok {
		t.Fatalf("no extension was written into %s", ext)
	}
	if installedPort != port {
		t.Fatalf("the extension dials %d while the bridge listens on %d", installedPort, port)
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
		t.Errorf("the manifest is missing, so Aseprite would ignore the folder: %v", err)
	}
}

// A port change moves the listener and the extension together.
func TestChangingTheBridgePortMovesIt(t *testing.T) {
	ext := t.TempDir()
	t.Setenv(aseprite.EnvOverride, ext)

	first := freePort(t)
	svc := serviceAt(t, first)
	if err := svc.StartBridge(); err != nil {
		t.Fatalf("start bridge: %v", err)
	}

	second := freePort(t)
	if _, err := svc.SaveSettings(config.Config{
		Mode:       config.ModeLive,
		BridgePort: second,
		Port:       config.DefaultPort,
	}); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	svc.mu.Lock()
	bridge := svc.bridge
	svc.mu.Unlock()
	if bridge == nil {
		t.Fatal("the bridge is gone after the port change")
	}
	if got := bridge.Port(); got != second {
		t.Errorf("the bridge listens on %d, want %d", got, second)
	}
	if _, installedPort, ok := aseprite.Installed(); !ok || installedPort != second {
		t.Errorf("the extension dials %d (installed=%v), want %d", installedPort, ok, second)
	}
}

// Switching to live with no server must bring the bridge up.
func TestSwitchingToLiveStartsTheBridge(t *testing.T) {
	ext := t.TempDir()
	t.Setenv(aseprite.EnvOverride, ext)

	port := freePort(t)
	store, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(config.Config{Mode: config.ModeHeadless, BridgePort: port}); err != nil {
		t.Fatal(err)
	}
	links := link.NewSelector(link.Options{Mode: config.ModeHeadless, BridgePort: port})
	t.Cleanup(func() { _ = links.Close() })
	svc := NewMCPService(store, links)
	t.Cleanup(func() { _ = svc.StopBridge() })

	if _, err := svc.SaveSettings(config.Config{Mode: config.ModeLive, BridgePort: port}); err != nil {
		t.Fatalf("save: %v", err)
	}

	svc.mu.Lock()
	running := svc.bridge != nil
	svc.mu.Unlock()
	if !running {
		t.Fatal("live mode was selected but no bridge is listening")
	}
	if _, _, ok := aseprite.Installed(); !ok {
		t.Error("live mode was selected but the extension was not installed")
	}
}
