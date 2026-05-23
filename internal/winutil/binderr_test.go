package winutil

import (
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
)

func TestExplainListenError_addrInUse(t *testing.T) {
	inner := &net.OpError{Op: "listen", Err: syscall.EADDRINUSE}
	got := ExplainListenError("127.0.0.1:443", inner)
	if got == nil {
		t.Fatal("expected error")
	}
	msg := got.Error()
	if !strings.Contains(msg, "port already in use") {
		t.Fatalf("want port-in-use hint, got %q", msg)
	}
	if !strings.Contains(msg, "127.0.0.1:443") {
		t.Fatalf("want bind addr in message, got %q", msg)
	}
}

func TestExplainListenError_accessDenied(t *testing.T) {
	inner := &net.OpError{Op: "listen", Err: syscall.EACCES}
	got := ExplainListenError("127.0.0.1:443", inner)
	if got == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(got.Error(), "administrator") {
		t.Fatalf("want admin hint, got %q", got)
	}
}

func TestListenErrorHint_windowsText(t *testing.T) {
	err := errors.New(`listen tcp 127.0.0.1:443: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted.`)
	hint := listenErrorHint("127.0.0.1:443", err)
	if hint == "" {
		t.Fatal("expected hint for WSAEADDRINUSE text")
	}
}
