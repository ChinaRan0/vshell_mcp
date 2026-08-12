// Package vshell implements a client for the v_shell management platform's
// Web API. It wraps the HTTP endpoints used by the v_shell web console so the
// MCP server can drive command control and file management on managed hosts.
package vshell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Config holds the settings needed to talk to a v_shell instance.
// Users provide these via command-line flags; see main.go.
type Config struct {
	BaseURL string // e.g. "http://your-vshell-server:8082"
	Prefix  string // API path prefix, usually "/api"
	Username string
	Password string
	Token    string // optional pre-supplied JWT; if empty we log in
	Timeout  time.Duration
}

// Client is a thin wrapper around the v_shell HTTP API with automatic
// login / token refresh.
type Client struct {
	cfg      Config
	http     *http.Client
	mu       sync.Mutex
	jwt      string
	loginURL string
}

// NewClient builds a Client from the given configuration.
func NewClient(cfg Config) *Client {
	if cfg.Prefix == "" {
		cfg.Prefix = "/api"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	prefix := strings.Trim(cfg.Prefix, "/")
	api := base
	if prefix != "" {
		api = base + "/" + prefix
	}
	return &Client{
		cfg:      cfg,
		http:     &http.Client{Timeout: cfg.Timeout},
		jwt:      cfg.Token,
		loginURL: api + "/login",
	}
}

// apiURL joins the API prefix to a path, e.g. "/client/list" -> "http://h:8082/api/client/list".
func (c *Client) apiURL(path string) string {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	prefix := strings.Trim(c.cfg.Prefix, "/")
	path = strings.TrimLeft(path, "/")
	if prefix == "" {
		return base + "/" + path
	}
	return base + "/" + prefix + "/" + path
}

// newGET builds a GET request carrying the v_shell token header.
func newGET(ctx context.Context, url, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Token", token)
	return req, nil
}

// apiError is the error type surfaced for non-zero response codes.
type apiError struct {
	Code    int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("v_shell api error (code=%d): %s", e.Code, e.Message)
}

// login authenticates and caches a fresh JWT.
func (c *Client) login(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{
		"username": c.cfg.Username,
		"password": c.cfg.Password,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.loginURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Result  struct {
			Token string `json:"token"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("login: bad response: %w", err)
	}
	if envelope.Code != 0 || envelope.Result.Token == "" {
		return fmt.Errorf("login failed: %s", envelope.Message)
	}
	c.mu.Lock()
	c.jwt = envelope.Result.Token
	c.mu.Unlock()
	return nil
}

// token returns the current JWT, logging in first if none is cached.
func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	tok := c.jwt
	c.mu.Unlock()
	if tok != "" {
		return tok, nil
	}
	if err := c.login(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.jwt, nil
}

// APIResponse is the common v_shell response envelope.
type APIResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
	Type    string          `json:"type"`
}

// Call performs a POST to the given path with a JSON body, returning the
// raw result payload. It transparently logs in / refreshes the token when
// the cached token is missing or rejected.
func (c *Client) Call(ctx context.Context, path string, body any) (json.RawMessage, error) {
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := c.token(ctx)
		if err != nil {
			return nil, err
		}
		var payload []byte
		if body != nil {
			payload, err = json.Marshal(body)
			if err != nil {
				return nil, err
			}
		}
		url := c.apiURL(path)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Token", tok)
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		// A stale token triggers a 401; refresh once and retry.
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			c.mu.Lock()
			c.jwt = ""
			c.mu.Unlock()
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("http %d from %s: %s", resp.StatusCode, path, strings.TrimSpace(string(data)))
		}
		var envelope APIResponse
		if err := json.Unmarshal(data, &envelope); err != nil {
			return nil, fmt.Errorf("bad response from %s: %w", path, err)
		}
		if envelope.Code != 0 {
			return nil, &apiError{Code: envelope.Code, Message: envelope.Message}
		}
		return envelope.Result, nil
	}
	return nil, fmt.Errorf("authentication failed for %s", path)
}
