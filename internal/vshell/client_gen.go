package vshell

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Client generation kinds supported by the v_shell /download endpoints.
const (
	GenStage     = "stage"     // Stage 反向客户端: 需要监听器
	GenListen    = "listen"    // 正向客户端: 直连地址
	GenListendll = "listendll" // DLL 正向客户端
	GenShellcode = "shellcode" // shellcode, 需要监听器
	GenDll       = "dll"       // DLL(监听器), arch 可为 loader
	GenStageless = "stageless" // 无阶段客户端, 需要监听器
	GenEbpf      = "ebpf"      // eBPF 客户端(监听器方式, 固定 0.0.0.0:49319)
)

// GenerateClientRequest carries the parameters for building a client.
// Which fields are required depends on Kind; see the MCP tool description.
type GenerateClientRequest struct {
	Kind       string // stage | listen | listendll | shellcode | dll | stageless | ebpf
	SavePath   string // local path to write the binary (required)
	Arch       string // e.g. linux_amd64, windows_amd64.exe, windows_amd64.dll, loader
	ListenerID int    // required for stage / stageless / shellcode / dll
	TP         string // tcp | kcp | ws (forward clients)
	Host       string // server host for forward clients
	Port       int    // server port for forward clients
	Vkey       string // communication credential
	Salt       string // traffic encryption salt
	UPX        string // "true" / "false"
	Proxy      string // proxy for the client (dll/stageless)
	Format     string // shellcode format: .bin | .c | .raw.txt
}

// buildURL assembles the download URL for the requested kind.
func (r GenerateClientRequest) buildURL() string {
	q := url.Values{}
	arch := r.Arch
	// shellcode appends the format to the arch value (windows_amd64 + .bin)
	if r.Kind == GenShellcode {
		format := r.Format
		if format != "" && !strings.HasPrefix(format, ".") {
			format = "." + format
		}
		arch += format
	}
	q.Set("arch", arch)
	switch r.Kind {
	case GenStage:
		q.Set("id", strconv.Itoa(r.ListenerID))
	case GenListen, GenListendll:
		q.Set("tp", r.TP)
		q.Set("host", r.Host)
		q.Set("port", strconv.Itoa(r.Port))
		q.Set("vkey", r.Vkey)
		q.Set("salt", r.Salt)
		q.Set("upx", r.UPX)
	case GenShellcode:
		q.Set("id", strconv.Itoa(r.ListenerID))
	case GenDll:
		q.Set("id", strconv.Itoa(r.ListenerID))
		q.Set("upx", r.UPX)
		q.Set("proxy", r.Proxy)
	case GenStageless:
		q.Set("id", strconv.Itoa(r.ListenerID))
		q.Set("upx", r.UPX)
		q.Set("proxy", r.Proxy)
	case GenEbpf:
		// eBPF 客户端: 固定监听地址与端口, 开启 listen + ebpf
		q.Set("tp", r.TP)
		q.Set("host", "0.0.0.0")
		q.Set("port", "49319")
		q.Set("upx", r.UPX)
		q.Set("vkey", r.Vkey)
		q.Set("salt", r.Salt)
		q.Set("listen", "true")
		q.Set("ebpf", "true")
	}
	return "/download/" + r.Kind + "?" + q.Encode()
}

// defaultName returns a sensible local filename for the generated client.
func (r GenerateClientRequest) defaultName() string {
	switch r.Kind {
	case GenStage:
		return "stage_" + r.Arch
	case GenListen, GenListendll, GenStageless, GenDll:
		return r.Kind + "_" + r.Arch
	case GenShellcode:
		return "shellcode_" + r.Arch
	case GenEbpf:
		return "ebpf_" + r.Arch
	}
	return r.Kind + "_" + r.Arch
}

// GenerateClient builds a client via the v_shell /download endpoint and saves
// the binary to req.SavePath. It returns the filename the server assigned.
func (c *Client) GenerateClient(ctx context.Context, req GenerateClientRequest) (string, error) {
	if req.UPX == "" {
		req.UPX = "false"
	}
	tok, err := c.token(ctx)
	if err != nil {
		return "", err
	}
	dlURL := c.apiURL(req.buildURL())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Token", tok)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("generate client: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Server-assigned filename from Content-Disposition, if present.
	serverName := ""
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		for _, part := range strings.Split(cd, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "filename=") {
				serverName = strings.Trim(strings.TrimPrefix(part, "filename="), `"`)
			}
		}
	}

	path := req.SavePath
	if path == "" {
		return "", fmt.Errorf("缺少 save_path 参数")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", err
	}
	return serverName, nil
}
