package vshell

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// wsShellServer emulates a v_shell terminal WebSocket: after receiving a command
// it writes beginMsg, goes silent for gap (simulating a slow command like
// `sleep 3`), then writes finalMsg.
func wsShellServer(t *testing.T, beginMsg, finalMsg string, gap time.Duration) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, _, _ = conn.ReadMessage() // wait for the command line
		if beginMsg != "" {
			conn.WriteMessage(websocket.TextMessage, []byte(beginMsg+"\n"))
		}
		time.Sleep(gap) // no output for `gap`
		conn.WriteMessage(websocket.TextMessage, []byte(finalMsg))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dialTestShell(t *testing.T, srv *httptest.Server) *Terminal {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return &Terminal{ws: conn}
}

// TestReadUntilMarkerSlowCommand ensures readUntilMarker keeps waiting while a
// slow command (e.g. `sleep 3`) produces no output, until the end marker lands.
func TestReadUntilMarkerSlowCommand(t *testing.T) {
	begin := "MCPB_test0000"
	end := "MCPD_test0000"
	srv := wsShellServer(t, begin, end+"\n", 3*time.Second)
	term := dialTestShell(t, srv)

	term.ws.WriteMessage(websocket.TextMessage, []byte("echo "+begin+"; sleep 3; echo "+end+"\r"))
	start := time.Now()
	buf := term.readUntilMarker([]byte(end), 60*time.Second)
	elapsed := time.Since(start)

	t.Logf("elapsed=%v  gotEnd=%v  buf=%q", elapsed, strings.Contains(string(buf), end), string(buf))
	if elapsed < 3*time.Second {
		t.Errorf("readUntilMarker returned after %v (< gap), end marker not waited for", elapsed)
	}
	if !strings.Contains(string(buf), end) {
		t.Errorf("end marker %q missing; returned buffer=%q", end, string(buf))
	}
}

// TestReadUntilPromptSlowCommand is the same check for readUntilPrompt.
func TestReadUntilPromptSlowCommand(t *testing.T) {
	srv := wsShellServer(t, "", "> ", 3*time.Second)
	term := dialTestShell(t, srv)

	term.ws.WriteMessage(websocket.TextMessage, []byte("sleep 3\r"))
	start := time.Now()
	buf, ok := term.readUntilPrompt(60 * time.Second)
	elapsed := time.Since(start)

	t.Logf("elapsed=%v ok=%v buf=%q", elapsed, ok, string(buf))
	if elapsed < 3*time.Second {
		t.Errorf("readUntilPrompt returned after %v (< gap)", elapsed)
	}
	if !ok {
		t.Errorf("prompt not detected, buf=%q", string(buf))
	}
}

// TestReadUntilMarkerDeadlineBounds ensures the overall wait still caps the
// read: when the command outlives the deadline, readUntilMarker returns at the
// deadline with a partial buffer, without panicking.
func TestReadUntilMarkerDeadlineBounds(t *testing.T) {
	begin := "MCPB_test0000"
	end := "MCPD_test0000"
	srv := wsShellServer(t, begin, end+"\n", 10*time.Second)
	term := dialTestShell(t, srv)

	term.ws.WriteMessage(websocket.TextMessage, []byte("echo "+begin+"; sleep 10; echo "+end+"\r"))
	start := time.Now()
	buf := term.readUntilMarker([]byte(end), 3*time.Second)
	elapsed := time.Since(start)

	t.Logf("elapsed=%v gotEnd=%v buf=%q", elapsed, strings.Contains(string(buf), end), string(buf))
	if elapsed < 3*time.Second || elapsed > 6*time.Second {
		t.Errorf("readUntilMarker should return at ~3s deadline, took %v", elapsed)
	}
	if !strings.Contains(string(buf), begin) {
		t.Errorf("expected partial buffer to contain begin marker %q, got %q", begin, string(buf))
	}
}

// TestReadUntilMarkerConnectionCloseReturnsFast ensures a real connection error
// (server closing) ends the read promptly instead of waiting out the deadline.
func TestReadUntilMarkerConnectionCloseReturnsFast(t *testing.T) {
	begin := "MCPB_test0000"
	end := "MCPD_test0000"
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, _, _ = conn.ReadMessage()
		conn.WriteMessage(websocket.TextMessage, []byte(begin+"\n"))
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		conn.Close() // close frame then TCP close
	}))
	t.Cleanup(srv.Close)
	term := dialTestShell(t, srv)

	term.ws.WriteMessage(websocket.TextMessage, []byte("echo "+begin+"; crash\r"))
	start := time.Now()
	buf := term.readUntilMarker([]byte(end), 60*time.Second)
	elapsed := time.Since(start)

	t.Logf("elapsed=%v buf=%q", elapsed, string(buf))
	if elapsed > 5*time.Second {
		t.Errorf("connection close should end read promptly, took %v", elapsed)
	}
	if !strings.Contains(string(buf), begin) {
		t.Errorf("expected buffer to contain begin marker %q, got %q", begin, string(buf))
	}
}
