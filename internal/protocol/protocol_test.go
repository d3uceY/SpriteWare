package protocol

import (
	"strings"
	"testing"
)

func TestEncodeCommandIsOneLineOfJSON(t *testing.T) {
	line, err := EncodeCommand(Command{ID: "r1", Cmd: "ping"})
	if err != nil {
		t.Fatalf("EncodeCommand: %v", err)
	}
	if strings.HasPrefix(string(line), FrameMarker) {
		t.Fatalf("commands must not carry the reply marker: %q", line)
	}
	if !strings.HasSuffix(string(line), "\n") {
		t.Fatal("command is not newline terminated; the reader would block")
	}
	if strings.Count(string(line), "\n") != 1 {
		t.Fatalf("command spans more than one line: %q", line)
	}
}

func TestEncodeCommandRejectsAnEmptyID(t *testing.T) {
	if _, err := EncodeCommand(Command{Cmd: "ping"}); err == nil {
		t.Error("a command with no id was accepted; its reply could never be routed")
	}
}

// Anything unframed must be ignored: a warning read as an answer answers the
// wrong call.
func TestUnframedOutputIsNotAReply(t *testing.T) {
	reply, isFrame, err := ParseReply([]byte("Aseprite: something happened\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isFrame || reply != nil {
		t.Fatal("an unframed line was treated as a reply")
	}
}

func TestParseReplyRoundTrip(t *testing.T) {
	line := []byte(FrameMarker + `{"id":"r7","ok":true,"data":{"pong":true}}` + "\n")
	reply, isFrame, err := ParseReply(line)
	if err != nil || !isFrame {
		t.Fatalf("ParseReply: isFrame=%v err=%v", isFrame, err)
	}
	if reply.ID != "r7" || !reply.OK {
		t.Errorf("reply = %+v", reply)
	}
	if !strings.Contains(string(reply.Data), `"pong":true`) {
		t.Errorf("data = %s", reply.Data)
	}
}

func TestParseHello(t *testing.T) {
	line := []byte(FrameMarker + `{"type":"hello","protocol":1,"extensionVersion":"0.1.0","features":["dump"]}` + "\n")
	hello, ok := ParseHello(line)
	if !ok {
		t.Fatal("hello frame was not recognised")
	}
	if hello.ExtensionVersion != "0.1.0" || len(hello.Features) != 1 {
		t.Errorf("hello = %+v", hello)
	}
}

// The rendered error is what the model reads, so code and next step must both be
// there.
func TestErrorRenderIncludesCodeAndRemediation(t *testing.T) {
	e := New(CodeNotConnected, "no Aseprite is attached").
		WithRemediation("open Aseprite, or switch to headless mode")
	rendered := e.Render()

	if !strings.HasPrefix(rendered, CodeNotConnected+": ") {
		t.Errorf("rendered error does not start with its code: %q", rendered)
	}
	if !strings.Contains(rendered, "remediation:") {
		t.Errorf("rendered error has no next step: %q", rendered)
	}
}

func TestAsErrorWrapsAPlainError(t *testing.T) {
	e := AsError(errString("boom"))
	if e.Code != CodeInternal {
		t.Errorf("code = %q, want %q", e.Code, CodeInternal)
	}
	if !strings.Contains(e.Message, "boom") {
		t.Errorf("message = %q", e.Message)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
