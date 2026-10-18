package daemon

import (
	"net"
	"testing"
	"time"
)

// TestWebUIURL_StripsTheAddrColon: getPort()/tm.apiPort carry the http.Server Addr
// (":8091"), not the bare port. Interpolating that straight produces "http://localhost::8091"
// — invalid host, and the browser opens an error page instead of the app. That is exactly
// what the first-run boot did.
func TestWebUIURL_StripsTheAddrColon(t *testing.T) {
	cases := []struct {
		port string
		want string
	}{
		{":8091", "http://localhost:8091/#/status"},
		{"8091", "http://localhost:8091/#/status"},
		{"", "http://localhost:8091/#/status"},
		{":", "http://localhost:8091/#/status"},
	}

	for _, tc := range cases {
		if got := WebUIURL(tc.port, "/#/status"); got != tc.want {
			t.Errorf("WebUIURL(%q) = %q, want %q", tc.port, got, tc.want)
		}
	}
}

func TestWebUIURL_EmptyRouteIsTheRoot(t *testing.T) {
	if got := WebUIURL(":8091", ""); got != "http://localhost:8091" {
		t.Fatalf("want the root with no suffix, got %q", got)
	}
}

// TestWaitForListener_ReturnsOnceBound: the API server starts in a goroutine, so opening the
// browser right after races ListenAndServe — losing the race is ERR_CONNECTION_REFUSED, the
// same visible symptom as "opened without the app".
func TestWaitForListener_ReturnsOnceBound(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	if err := WaitForListener(ln.Addr().String(), time.Second); err != nil {
		t.Fatalf("a port already listening must return with no error: %v", err)
	}
}

func TestWaitForListener_TimesOut(t *testing.T) {
	// Closed port: bind one and release it, to get a number nobody is listening on.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	if err := WaitForListener(addr, 150*time.Millisecond); err == nil {
		t.Fatal("want an error when nobody is bound on the port")
	}
}
