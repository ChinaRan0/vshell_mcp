package vshell

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[>=]|\x1b[()][0-9A-Z]`)

// Terminal executes commands on a managed host through v_shell's interactive
// WebSocket shell (/api/terminal/ws). Each Exec opens a fresh shell session,
// disables terminal echo, runs the command between unique begin/end markers,
// and returns only the command's output.
type Terminal struct {
	c  *Client
	ws *websocket.Conn
}

func randToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

// wsURL builds the terminal websocket URL, downgrading http/https to ws/wss.
func (c *Client) wsURL(hostID int, token string) string {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	scheme := "ws"
	if strings.HasPrefix(base, "https://") {
		scheme = "wss"
		base = strings.Replace(base, "https://", "", 1)
	} else {
		base = strings.Replace(base, "http://", "", 1)
	}
	prefix := strings.Trim(c.cfg.Prefix, "/")
	if prefix == "" {
		return fmt.Sprintf("%s://%s/terminal/ws?id=%d&token=%s", scheme, base, hostID, token)
	}
	return fmt.Sprintf("%s://%s/%s/terminal/ws?id=%d&token=%s", scheme, base, prefix, hostID, token)
}

// readUntilPrompt reads from the socket until the buffer ends in a shell
// prompt character ('$', '#', '>'), or wait elapses. It returns the buffered
// bytes and whether a prompt was seen.
func (t *Terminal) readUntilPrompt(wait time.Duration) ([]byte, bool) {
	var buf []byte
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		t.ws.SetReadDeadline(time.Now().Add(time.Second))
		_, data, err := t.ws.ReadMessage()
		if err != nil {
			return buf, hasPromptSuffix(buf)
		}
		buf = append(buf, data...)
		if hasPromptSuffix(buf) {
			return buf, true
		}
	}
	return buf, hasPromptSuffix(buf)
}

func hasPromptSuffix(buf []byte) bool {
	s := strings.TrimRight(string(buf), " \t\r\n")
	return strings.HasSuffix(s, "$") || strings.HasSuffix(s, "#") || strings.HasSuffix(s, ">")
}

// readUntilMarker reads until the needle bytes appear in the stream or wait
// elapses, returning everything read.
func (t *Terminal) readUntilMarker(needle []byte, wait time.Duration) []byte {
	var buf []byte
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		t.ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, err := t.ws.ReadMessage()
		if err != nil {
			return buf
		}
		buf = append(buf, data...)
		if strings.Contains(string(buf), string(needle)) {
			return buf
		}
	}
	return buf
}

// Exec runs command on the host. If workdir is non-empty the command runs
// after a cd into that directory. timeout bounds the whole call.
func (c *Client) Exec(ctx context.Context, hostID int, command, workdir string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	tok, err := c.token(ctx)
	if err != nil {
		return "", err
	}
	url := c.wsURL(hostID, tok)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
		ws, _, err := dialer.DialContext(ctx, url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		t := &Terminal{c: c, ws: ws}
		out, err := t.execOnce(command, workdir, timeout)
		ws.Close()
		if err == nil {
			return out, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("terminal exec failed after 3 attempts: %v", lastErr)
}

func (t *Terminal) execOnce(command, workdir string, timeout time.Duration) (string, error) {
	// 1. Wait for the initial shell prompt.
	if _, ok := t.readUntilPrompt(10 * time.Second); !ok {
		return "", fmt.Errorf("shell prompt not detected on connect")
	}
	// 2. Disable echo and pin the prompt so markers are unambiguous.
	t.ws.WriteMessage(websocket.TextMessage, []byte("stty -echo 2>/dev/null; PS1='> '; clear\r"))
	promptBuf, posix := t.readUntilPrompt(5 * time.Second)
	_ = promptBuf

	begin := "MCPB_" + randToken(8)
	end := "MCPD_" + randToken(8)

	var sent string
	if posix {
		if workdir != "" {
			sent = fmt.Sprintf("echo %s; cd %s 2>/dev/null; %s; echo %s", begin, workdir, command, end)
		} else {
			sent = fmt.Sprintf("echo %s; %s; echo %s", begin, command, end)
		}
	} else {
		// Non-POSIX fallback (e.g. Windows cmd): echo stays on, so we must
		// strip the echoed command line afterwards. '&' chains commands.
		if workdir != "" {
			sent = fmt.Sprintf("cd /d %s & echo %s & %s & echo %s", workdir, begin, command, end)
		} else {
			sent = fmt.Sprintf("echo %s & %s & echo %s", begin, command, end)
		}
	}
	t.ws.WriteMessage(websocket.TextMessage, []byte(sent+"\r"))

	buf := t.readUntilMarker([]byte(end), timeout)
	beginIdx := strings.Index(string(buf), begin)
	endIdx := strings.Index(string(buf), end)
	var result string
	if beginIdx >= 0 && endIdx > beginIdx {
		result = string(buf[beginIdx+len(begin) : endIdx])
	} else if endIdx >= 0 {
		result = string(buf[:endIdx])
	} else {
		result = string(buf)
	}
	result = ansiEscape.ReplaceAllString(result, "")
	if !posix {
		// strip the echoed input line that leads the output
		if i := strings.Index(result, sent); i >= 0 {
			result = result[i+len(sent):]
		}
	}
	return strings.Trim(strings.Trim(result, "\r\n\x00 "), " "), nil
}
