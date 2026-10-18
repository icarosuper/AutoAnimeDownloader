package daemon

import (
	"fmt"
	"net"
	"strings"
	"time"
)

const defaultAPIPort = "8091"

// WebUIURL builds the Web UI URL from what the process calls a "port", which in practice is
// the http.Server Addr: ":8091", with a leading colon. Interpolating that value straight into
// "http://localhost:%s" produces "http://localhost::8091" — invalid host, and the browser
// opens an error page instead of the app.
//
// It exists because that conversion was already written in three places (the verification
// pass, the tray and the first-run boot) and the newest one forgot TrimPrefix. One function.
//
// route comes in as-is ("/#/status", "/api/v1/check", "" for the root).
func WebUIURL(port, route string) string {
	port = strings.TrimPrefix(port, ":")
	if port == "" {
		port = defaultAPIPort
	}
	return fmt.Sprintf("http://localhost:%s%s", port, route)
}

// WaitForListener waits for the port to accept a connection, until timeout.
//
// The API server starts in a goroutine and the "API server started successfully" log fires
// before the socket is bound. Whoever opens the browser right after races ListenAndServe:
// losing it is ERR_CONNECTION_REFUSED, which to the user is the same "opened without the
// app" as a broken URL.
func WaitForListener(addr string, timeout time.Duration) error {
	// The http.Server Addr usually comes without a host (":8091"); net.Dial needs one.
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}

	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s to accept connections: %w", addr, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
