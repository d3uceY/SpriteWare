package link

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// The bridge keeps the user's window attached while an agent host restarts the
// server. These tests drive it over real sockets, as production does.
// ---------------------------------------------------------------------------

func startBridge(t *testing.T) *Bridge {
	t.Helper()
	b := NewBridge()
	if err := b.Start(0); err != nil {
		t.Fatalf("bridge did not start: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func dialRole(t *testing.T, b *Bridge, role string) (net.Conn, *bufio.Reader) {
	t.Helper()
	addr := fmt.Sprintf("127.0.0.1:%d", b.Port())
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := fmt.Fprintf(conn, "{\"role\":%q}\n", role); err != nil {
		t.Fatalf("send role: %v", err)
	}
	return conn, bufio.NewReaderSize(conn, 1<<20)
}

func writeCommand(t *testing.T, conn net.Conn, id, cmd string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"id": id, "cmd": cmd})
	if _, err := conn.Write(frame(body)); err != nil {
		t.Fatalf("write command: %v", err)
	}
}

func readLine(t *testing.T, conn net.Conn, r *bufio.Reader) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(stripMarker(line), &out); err != nil {
		t.Fatalf("decode %q: %v", line, err)
	}
	return out
}

// waitForID skips bridge_state notices and returns the reply with this id.
func waitForID(t *testing.T, conn net.Conn, r *bufio.Reader, id string) map[string]any {
	t.Helper()
	for i := 0; i < 10; i++ {
		msg := readLine(t, conn, r)
		if msg["type"] == "bridge_state" {
			continue
		}
		if got, _ := msg["id"].(string); got == id {
			return msg
		}
		t.Fatalf("got a reply for %v, want %q", msg["id"], id)
	}
	t.Fatalf("no reply for %q", id)
	return nil
}

// Two clients, one Aseprite: each gets its own answer. Without id prefixing one
// client's reply lands in the other's channel.
func TestBridgeRoutesRepliesToTheRightClient(t *testing.T) {
	b := startBridge(t)
	addr := fmt.Sprintf("127.0.0.1:%d", b.Port())

	ext, extReader := dialRole(t, b, "extension")
	_ = ext

	c1, c1Reader := dialRole(t, b, "client")
	c2, c2Reader := dialRole(t, b, "client")

	writeCommand(t, c1, "a1", "doc.info")
	writeCommand(t, c2, "a2", "doc.info")

	// Which connection is numbered first depends on the scheduler, so the prefix
	// is read off the wire.
	prefix := map[string]string{}
	for _, msg := range []map[string]any{
		waitForExtensionCommand(t, ext, extReader),
		waitForExtensionCommand(t, ext, extReader),
	} {
		id, _ := msg["id"].(string)
		_, local, ok := strings.Cut(id, "::")
		if !ok {
			t.Fatalf("id %q carries no client prefix", id)
		}
		prefix[local] = id
	}
	if len(prefix) != 2 || prefix["a1"] == "" || prefix["a2"] == "" || prefix["a1"] == prefix["a2"] {
		t.Fatalf("requests were not separated per client: %v", prefix)
	}

	// Reply to the second client first, out of order, and check delivery.
	if _, err := ext.Write(frame(mustJSON(t, map[string]any{
		"id": prefix["a2"], "ok": true, "data": map[string]any{"who": "two"},
	}))); err != nil {
		t.Fatalf("extension write: %v", err)
	}
	got := waitForID(t, c2, c2Reader, "a2")
	data, _ := got["data"].(map[string]any)
	if data["who"] != "two" {
		t.Fatalf("client 2 got %v", got)
	}

	if _, err := ext.Write(frame(mustJSON(t, map[string]any{
		"id": prefix["a1"], "ok": true, "data": map[string]any{"who": "one"},
	}))); err != nil {
		t.Fatalf("extension write: %v", err)
	}
	got = waitForID(t, c1, c1Reader, "a1")
	data, _ = got["data"].(map[string]any)
	if data["who"] != "one" {
		t.Fatalf("client 1 got %v", got)
	}

	_ = addr
}

// A request with no window must fail immediately and say why: a hang reads as
// "slow", and the obvious retry edits the file behind the user's editor.
func TestBridgeAnswersImmediatelyWhenAsepriteIsMissing(t *testing.T) {
	b := startBridge(t)
	c1, reader := dialRole(t, b, "client")

	writeCommand(t, c1, "r1", "doc.info")
	msg := waitForID(t, c1, reader, "r1")

	if okValue, _ := msg["ok"].(bool); okValue {
		t.Fatal("the bridge reported success with no Aseprite attached")
	}
	errObj, _ := msg["error"].(map[string]any)
	if errObj["code"] != "not_connected" {
		t.Fatalf("error code = %v, want not_connected", errObj["code"])
	}
}

// Restarting Aseprite must not be locked out by the previous socket.
func TestBridgeAcceptsAReconnect(t *testing.T) {
	b := startBridge(t)

	first, firstReader := dialRole(t, b, "extension")
	_ = first
	if _, err := first.Write(frame(mustJSON(t, map[string]any{
		"type": "hello", "protocol": 1, "extensionVersion": "0.1.0", "features": []any{},
	}))); err != nil {
		t.Fatalf("hello: %v", err)
	}
	_ = firstReader

	waitConnected(t, b, true)

	second, _ := dialRole(t, b, "extension") // a second window takes over
	if _, err := second.Write(frame(mustJSON(t, map[string]any{
		"type": "hello", "protocol": 1, "extensionVersion": "0.2.0", "features": []any{},
	}))); err != nil {
		t.Fatalf("second hello: %v", err)
	}

	// Wait for the takeover to be recorded before killing the old socket, or the
	// test races the bridge.
	waitVersion(t, b, "0.2.0")

	// The old socket's cleanup must not report the replacement as gone.
	_ = first.Close()
	for i := 0; i < 20; i++ {
		if !b.State().Connected {
			t.Fatal("the old socket's cleanup cleared the new window")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitVersion blocks until the bridge reports this version: a replacement has
// taken over.
func waitVersion(t *testing.T, b *Bridge, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if b.State().Version == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the bridge still reports version %q, want %q", b.State().Version, want)
}

func waitConnected(t *testing.T, b *Bridge, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if b.State().Connected == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("bridge state connected = %v, want %v", b.State().Connected, want)
}

func waitForExtensionCommand(t *testing.T, conn net.Conn, r *bufio.Reader) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("extension read: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(stripMarker(line), &out); err != nil {
		t.Fatalf("decode %q: %v", line, err)
	}
	return out
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return body
}
