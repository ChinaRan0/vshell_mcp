// vshell-mcp exposes the v_shell management platform — command control,
// file management, tunnel/listener management and client generation on
// managed hosts — as a Model Context Protocol server over an SSE transport.
//
// Command-line configuration (no environment variables needed):
//
//	-h <url>    v_shell web console base URL (required), e.g. http://host:8082
//	-u <user>   login username (default: admin)
//	-p <pass>   login password (required unless -token is given)
//	-token      optional pre-supplied JWT; skips username/password login
//	-port       MCP SSE server listen port (default: 19080)
//	-mcpurl     optional externally-reachable MCP base URL (behind a proxy)
//	-prefix     v_shell API path prefix (default: /api)
//	-timeout    per-request timeout in seconds (default: 60)
//
// Example:
//
//	./vshell-mcp -h http://your-vshell-server:8082 -u admin -p 'secret'
//
// Clients configure it as an SSE MCP server; the URL users must reach is
// the --port of this process (the /sse endpoint).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chinaran404/vshell-mcp/internal/mcp"
	"github.com/chinaran404/vshell-mcp/internal/vshell"
)

// version is injected at build time via
// `go build -ldflags "-X main.version=<tag>"`. Defaults to "dev".
var version = "dev"

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `vshell-mcp — v_shell 管理平台 MCP 服务(SSE 传输)

把 v_shell 的命令控制、文件管理、隧道/监听管理、客户端生成、服务器信息等能力开放给 AI。

用法:
  vshell-mcp -h <v_shell地址> -u <账号> -p <密码> [选项]

必填:
  -h <url>   v_shell Web 控制台地址,如 http://your-vshell-server:8082
  -u <user>  登录账号(默认 admin)
  -p <pass>  登录密码(设了 -token 则无需)

选项:
  -token     预置 JWT,跳过账号密码登录
  -port      MCP SSE 服务监听端口(默认 19080)
  -mcpurl    MCP 服务器对外可达地址(置于反向代理后时设置)
  -prefix    v_shell API 路径前缀(默认 /api)
  -timeout   单次请求超时秒数(默认 60)
  -version   显示版本号并退出
  -help      显示本帮助

接入(Claude Code):
  claude mcp add vshell --type sse --url http://127.0.0.1:19080/sse

注:-h 已绑定为 v_shell 地址参数(而非帮助);查看帮助请用 -help。
`)
	}
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		flag.Usage()
		os.Exit(1)
	}

	vs := vshell.NewClient(cfg.vshell)
	// Fail fast if credentials are wrong.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	if _, err := vs.ListHosts(ctx, nil); err != nil {
		cancel()
		log.Fatalf("无法连接 v_shell(检查 -h 地址与 -u/-p 账号密码): %v", err)
	}
	cancel()

	addr := fmt.Sprintf(":%d", cfg.port)
	srv := mcp.New(vs, cfg.vshell.BaseURL, version)
	fmt.Printf("v_shell MCP server listening on %s  (SSE endpoint: %s/sse)\n", addr, "http://127.0.0.1:"+cfg.portStr())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		os.Exit(0)
	}()

	if err := srv.StartSSE(addr, cfg.mcpURL); err != nil {
		log.Fatalf("MCP SSE server failed: %v", err)
	}
}

type config struct {
	vshell vshell.Config
	port   int
	mcpURL string
}

// parseFlags parses command-line flags. Note: -h is intentionally bound to
// the v_shell URL (not help); use -help to print usage.
func parseFlags() (*config, error) {
	var (
		baseURL = flag.String("h", "", "v_shell Web 控制台地址,如 http://your-vshell-server:8082(必填)")
		user    = flag.String("u", "admin", "登录账号")
		pass    = flag.String("p", "", "登录密码(必填,除非提供了 -token)")
		token   = flag.String("token", "", "预置 JWT,设置后跳过账号密码登录")
		port    = flag.Int("port", 19080, "MCP SSE 服务监听端口")
		mcpURL  = flag.String("mcpurl", "", "MCP 服务器对外可达地址(置于反向代理后时设置)")
		prefix  = flag.String("prefix", "/api", "v_shell API 路径前缀")
		timeout = flag.Int("timeout", 60, "单次 HTTP 请求超时(秒)")
		showVer = flag.Bool("version", false, "显示版本号并退出")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("vshell-mcp %s\n", version)
		os.Exit(0)
	}

	if *baseURL == "" {
		return nil, fmt.Errorf("缺少必填参数 -h(v_shell 地址)")
	}
	if *token == "" && *pass == "" {
		return nil, fmt.Errorf("缺少登录凭据:请提供 -p 密码(或 -token)")
	}
	if *timeout <= 0 {
		*timeout = 60
	}
	return &config{
		vshell: vshell.Config{
			BaseURL:  *baseURL,
			Prefix:   *prefix,
			Username: *user,
			Password: *pass,
			Token:    *token,
			Timeout:  time.Duration(*timeout) * time.Second,
		},
		port:   *port,
		mcpURL: *mcpURL,
	}, nil
}

func (c *config) portStr() string {
	return fmt.Sprintf("%d", c.port)
}
