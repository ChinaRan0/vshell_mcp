package vshell

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"
)

// ListHosts returns the managed hosts. status filters to online hosts when
// true; set to false or omit to include all.
func (c *Client) ListHosts(ctx context.Context, status *bool) ([]Host, error) {
	body := map[string]any{"page": 1, "pageSize": 9999}
	if status != nil {
		body["status"] = *status
	}
	raw, err := c.Call(ctx, "/client/list", body)
	if err != nil {
		return nil, err
	}
	var res HostListResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("parse client/list: %w", err)
	}
	return res.Items, nil
}

// ListDirectory returns the entries in path on the given host.
func (c *Client) ListDirectory(ctx context.Context, hostID int, path string) ([]FileEntry, error) {
	if path == "" {
		path = "/"
	}
	raw, err := c.Call(ctx, "/file/ls", map[string]any{"id": hostID, "path": path})
	if err != nil {
		return nil, err
	}
	var res FileListResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("parse file/ls: %w", err)
	}
	return res.Items, nil
}

// GetDisks returns the disk / mount info for a host.
func (c *Client) GetDisks(ctx context.Context, hostID int) ([]DiskEntry, error) {
	raw, err := c.Call(ctx, "/file/getdisk", map[string]any{"id": hostID})
	if err != nil {
		return nil, err
	}
	var res []DiskEntry
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("parse file/getdisk: %w", err)
	}
	return res, nil
}

// ReadFile returns the content of a file on the host. Binary content is
// base64-encoded and flagged.
func (c *Client) ReadFile(ctx context.Context, hostID int, path, filename string) (content string, base64Encoded bool, err error) {
	raw, err := c.Call(ctx, "/file/cat", map[string]any{"id": hostID, "path": path, "target": filename})
	if err != nil {
		return "", false, err
	}
	var res CatResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", false, fmt.Errorf("parse file/cat: %w", err)
	}
	if !utf8.ValidString(res.Content) {
		return base64.StdEncoding.EncodeToString([]byte(res.Content)), true, nil
	}
	return res.Content, false, nil
}

// WriteFile writes content to a file on the host.
func (c *Client) WriteFile(ctx context.Context, hostID int, path, filename, content string) error {
	_, err := c.Call(ctx, "/file/edit", map[string]any{"id": hostID, "path": path, "target": filename, "content": content})
	return err
}

// CreateDirectory makes a directory on the host.
func (c *Client) CreateDirectory(ctx context.Context, hostID int, path, dirname string) error {
	_, err := c.Call(ctx, "/file/mkdir", map[string]any{"id": hostID, "path": path, "target": dirname})
	return err
}

// CreateFile creates an empty file on the host.
func (c *Client) CreateFile(ctx context.Context, hostID int, path, filename string) error {
	_, err := c.Call(ctx, "/file/touch", map[string]any{"id": hostID, "path": path, "target": filename})
	return err
}

// DeleteFile removes a single file/dir on the host.
func (c *Client) DeleteFile(ctx context.Context, hostID int, path, filename string) error {
	_, err := c.Call(ctx, "/file/rm", map[string]any{"id": hostID, "path": path, "target": filename})
	return err
}

// DeleteFiles removes multiple files/dirs on the host.
func (c *Client) DeleteFiles(ctx context.Context, hostID int, path string, names []string) error {
	_, err := c.Call(ctx, "/file/rmlist", map[string]any{"id": hostID, "path": path, "name": names})
	return err
}

// RenameFile moves/re-names a file on the host.
func (c *Client) RenameFile(ctx context.Context, hostID int, path, target, newName string) error {
	_, err := c.Call(ctx, "/file/mv", map[string]any{"id": hostID, "path": path, "target": target, "to": newName})
	return err
}

// WgetFile downloads a URL into the host's directory.
func (c *Client) WgetFile(ctx context.Context, hostID int, path, urlStr string) error {
	_, err := c.Call(ctx, "/file/wget", map[string]any{"id": hostID, "path": path, "target": urlStr})
	return err
}

// UploadFile uploads a local file (localPath) to the host directory.
func (c *Client) UploadFile(ctx context.Context, hostID int, path, localPath string, remoteName string) (remote string, err error) {
	if remoteName == "" {
		remoteName = filepath.Base(localPath)
	}
	f, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	tok, err := c.token(ctx)
	if err != nil {
		return "", err
	}
	uploadURL := c.apiURL("/file/upload")

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		mw.WriteField("id", fmt.Sprintf("%d", hostID))
		mw.WriteField("path", path)
		fw, err := mw.CreateFormFile("file", remoteName)
		if err == nil {
			_, err = io.Copy(fw, f)
		}
		mw.Close()
		pw.CloseWithError(err)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, pr)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Token", tok)
	// The upload pipe can take a while for large files.
	client := &http.Client{Timeout: 24 * time.Hour}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var envelope APIResponse
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", fmt.Errorf("parse file/upload response: %w", err)
	}
	if envelope.Code != 0 {
		return "", &apiError{Code: envelope.Code, Message: envelope.Message}
	}
	// The upload response puts "url" at the top level of the JSON object.
	var result struct {
		URL string `json:"url"`
	}
	json.Unmarshal(data, &result)
	return result.URL, nil
}

// DownloadFile downloads a host file to localPath, staging it through the
// v_shell server first. Returns the bytes written and the staging percent.
func (c *Client) DownloadFile(ctx context.Context, hostID int, path, filename, localPath string) (int64, error) {
	if localPath == "" {
		localPath = filename
	}
	// 1. Ask the server to pull the file from the host.
	if _, err := c.Call(ctx, "/file/downloadtoserver", map[string]any{
		"id": hostID, "path": path, "target": filename,
	}); err != nil {
		return 0, err
	}
	// 2. Poll until the staging completes.
	for i := 0; i < 600; i++ {
		raw, err := c.Call(ctx, "/file/getdownloadper", map[string]any{
			"id": hostID, "path": path, "target": filename, "size": 0,
		})
		if err != nil {
			return 0, err
		}
		var per struct {
			Pre int `json:"pre"`
		}
		json.Unmarshal(raw, &per)
		if per.Pre >= 100 {
			break
		}
		time.Sleep(time.Second)
	}
	// 3. Stream it down to the local file.
	tok, err := c.token(ctx)
	if err != nil {
		return 0, err
	}
	dlURL := c.apiURL(fmt.Sprintf("/file/downloadtobrowser?id=%d&target=%s", hostID, url.QueryEscape(filename)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Token", tok)
	client := &http.Client{Timeout: 24 * time.Hour}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download returned http %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return 0, err
	}
	out, err := os.Create(localPath)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	return io.Copy(out, resp.Body)
}

func utf8Valid(s string) bool {
	return utf8.ValidString(s)
}
