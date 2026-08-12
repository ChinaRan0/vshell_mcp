// Package mcp exposes v_shell's command control and file management as
// Model Context Protocol tools over an SSE transport.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/chinaran404/vshell-mcp/internal/vshell"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server wraps the v_shell client and registers all MCP tools.
type Server struct {
	vs   *vshell.Client
	srv  *server.MCPServer
	base string
}

// New builds the MCP server (tools defined below) for the given v_shell client.
func New(vs *vshell.Client, baseURL, version string) *Server {
	if version == "" {
		version = "dev"
	}
	s := &Server{vs: vs, base: baseURL}
	s.srv = server.NewMCPServer(
		"vshell-mcp",
		version,
		server.WithToolCapabilities(false),
		server.WithInstructions(
			"v_shell 管理平台 MCP:列出主机、执行命令(命令控制)、文件管理(列目录/读写/上传/下载/删除/重命名/新建)。"+
				"host_id 是主机 Id(见 list_hosts)。命令控制通过目标主机的交互式 shell 执行,请使用单条命令,避免交互式程序。",
		),
	)
	s.registerTools()
	return s
}

// Handler returns the underlying MCP server (for embedding).
func (s *Server) Handler() *server.MCPServer { return s.srv }

// StartSSE runs the legacy SSE transport on addr. baseURL, when non-empty,
// is advertised as the message endpoint host (use when the MCP server sits
// behind a proxy). When empty the client's own request host is used.
func (s *Server) StartSSE(addr, baseURL string) error {
	sse := server.NewSSEServer(s.srv)
	if baseURL != "" {
		sse = server.NewSSEServer(s.srv, server.WithBaseURL(baseURL))
	}
	return sse.Start(addr)
}

// tools registers every MCP tool and its handler.
func (s *Server) registerTools() {
	// --- server info ---
	s.addTool(mcp.NewTool("get_server_info",
		mcp.WithDescription("查看当前连接的 v_shell 服务器信息:连接地址(URL)、登录账号、API 前缀、当前登录用户,以及服务端版本/端口/授权等。"+
			"适合在开始操作前确认连接配置。"),
	), s.handlerServerInfo)

	s.addTool(mcp.NewTool("get_dashboard",
		mcp.WithDescription("获取 v_shell 服务器仪表盘(dashboard)信息,即管理台 /dashboard 页面展示的指标:"+
			"客户端/监听/代理数量、在线数、CPU/内存/磁盘/负载、收发带宽、TCP/UDP 连接数、版本、授权到期时间等。"),
	), s.handlerDashboard)

	// --- tunnel / proxy ---
	s.addTool(mcp.NewTool("list_tunnels",
		mcp.WithDescription("列出所有隧道/代理(对应管理台 /tunnel/list)。返回每条隧道的 Id、绑定客户端 Id、模式(socks5/http/tcp/udp)、服务端端口、目标、账号、启用/停用状态、客户端在线状态。"),
	), s.handlerListTunnels)

	s.addTool(mcp.NewTool("add_tunnel",
		mcp.WithDescription("新建隧道/代理。mode 可选 socks5 / http / tcp / udp。tcp/udp 需要 target(格式 10.1.1.99:80);socks5/http 可设 username/password 作认证。"),
		mcp.WithString("client_id", mcp.Required(), mcp.Description("绑定客户端(主机) Id,见 list_hosts")),
		mcp.WithString("mode", mcp.Description("隧道模式,默认 socks5;socks5/http/tcp/udp")),
		mcp.WithNumber("port", mcp.Required(), mcp.Description("服务端监听端口")),
		mcp.WithString("target", mcp.Description("目标 IP:端口,tcp/udp 必填")),
		mcp.WithString("username", mcp.Description("socks5/http 认证用户名,可选")),
		mcp.WithString("password", mcp.Description("socks5/http 认证密码,可选")),
		mcp.WithString("remark", mcp.Description("备注,可选")),
	), s.handlerAddTunnel)

	s.addTool(mcp.NewTool("delete_tunnel",
		mcp.WithDescription("删除单条隧道/代理。"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("隧道 Id,见 list_tunnels")),
	), s.handlerDeleteTunnel)

	s.addTool(mcp.NewTool("delete_tunnels",
		mcp.WithDescription("批量删除隧道/代理。"),
		mcp.WithArray("ids", mcp.Description("隧道 Id 数组")),
	), s.handlerDeleteTunnels)

	s.addTool(mcp.NewTool("start_tunnel",
		mcp.WithDescription("启用指定隧道/代理。"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("隧道 Id")),
	), s.handlerStartTunnel)

	s.addTool(mcp.NewTool("stop_tunnel",
		mcp.WithDescription("停用指定隧道/代理。"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("隧道 Id")),
	), s.handlerStopTunnel)

	s.addTool(mcp.NewTool("edit_tunnel",
		mcp.WithDescription("编辑隧道/代理的配置(客户端、模式、端口、目标、账号、备注)。"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("隧道 Id")),
		mcp.WithString("client_id", mcp.Description("绑定客户端 Id")),
		mcp.WithString("mode", mcp.Description("模式:socks5/http/tcp/udp")),
		mcp.WithNumber("port", mcp.Description("服务端端口")),
		mcp.WithString("target", mcp.Description("目标 IP:端口")),
		mcp.WithString("username", mcp.Description("用户名")),
		mcp.WithString("password", mcp.Description("密码")),
		mcp.WithString("remark", mcp.Description("备注")),
	), s.handlerEditTunnel)

	s.addTool(mcp.NewTool("set_tunnel_remark",
		mcp.WithDescription("修改隧道/代理的备注。"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("隧道 Id")),
		mcp.WithString("remark", mcp.Required(), mcp.Description("新备注")),
	), s.handlerSetTunnelRemark)

	// --- listener management (add + view only; deletion intentionally NOT exposed) ---
	s.addTool(mcp.NewTool("list_listeners",
		mcp.WithDescription("查看所有监听器(对应管理台 /listener/list)。返回每个监听器的 Id、模式、监听地址、外网连接地址、心跳间隔、超时心跳次数、Vkey、加密盐、备注、运行状态。"+
			"支持按运行状态与模式过滤。"),
		mcp.WithString("status", mcp.Description("可选:'true'/'false',按运行状态过滤")),
		mcp.WithString("mode", mcp.Description("可选:按模式过滤,如 tcp/kcp/ws/dns/doh/dot/oss")),
	), s.handlerListListeners)

	s.addTool(mcp.NewTool("add_listener",
		mcp.WithDescription("新建监听器。mode 可选 tcp/kcp/ws/dns/doh/dot/oss(默认 tcp)。"+
			"各模式所需地址:tcp/kcp 用 connect_addr,ws 用 ws_connect_addr,dns/doh/dot 用 dns_domain 与 public_dns,oss 用 oss_url。"+
			"listen_addr 为服务端监听地址(如 0.0.0.0:21480),vkey 与 encrypt_salt 必填。"),
		mcp.WithString("client_id", mcp.Description("可选:绑定客户端 Id(0 表示暂不绑定)")),
		mcp.WithString("mode", mcp.Description("监听模式,默认 tcp")),
		mcp.WithString("listen_addr", mcp.Required(), mcp.Description("服务端监听地址,如 0.0.0.0:21480")),
		mcp.WithString("connect_addr", mcp.Description("外网连接地址,tcp/kcp 使用,如 1.2.3.4:21480")),
		mcp.WithString("ws_connect_addr", mcp.Description("ws 连接地址,ws 模式使用")),
		mcp.WithString("dns_domain", mcp.Description("NS 域名,dns/doh/dot 模式使用")),
		mcp.WithString("public_dns", mcp.Description("公共 DNS,dns/doh/dot 模式使用")),
		mcp.WithString("oss_url", mcp.Description("Bucket 域名,oss 模式使用")),
		mcp.WithNumber("disconnect_timeout", mcp.Description("超时心跳次数,默认 30")),
		mcp.WithNumber("ping_interval", mcp.Description("心跳间隔(秒),默认 10")),
		mcp.WithString("vkey", mcp.Description("Vkey 认证密钥,必填")),
		mcp.WithString("encrypt_salt", mcp.Description("流量加密盐,必填")),
		mcp.WithString("remark", mcp.Description("备注,可选")),
	), s.handlerAddListener)

	// --- client generation (download) ---
	s.addTool(mcp.NewTool("generate_client",
		mcp.WithDescription("生成并下载 v_shell 客户端(对应管理台 /download 客户端生成页),保存到 MCP 服务器本地路径。\n"+
			"kind 取值与所需参数:\n"+
			"- stage   反向客户端(需要监听器):listener_id + arch\n"+
			"- listen  正向客户端:arch + tp(tcp/kcp/ws)+ host + port + vkey + salt(+upx)\n"+
			"- listendll DLL 正向客户端:同上(arch 用 windows_amd64.dll / windows_i386.dll)\n"+
			"- stageless 无阶段客户端(需要监听器):listener_id + arch(+upx +proxy)\n"+
			"- shellcode shellcode(需要监听器):listener_id + arch(windows_amd64/windows_i386)+ format(.bin/.c/.raw.txt)\n"+
			"- dll     DLL(需要监听器):listener_id + arch(windows_amd64.dll/windows_i386.dll 或 loader)(+upx +proxy)\n"+
			"- ebpf    eBPF 客户端:arch + tp + vkey + salt(服务端固定 0.0.0.0:49319,listen+ebpf)\n"+
			"arch 取值: darwin_amd64 / darwin_arm64 / linux_amd64 / linux_i386 / linux_arm64 / linux_arm / windows_amd64(.exe/.dll)/ windows_i386(.exe/.dll)。\n"+
			"监听器 Id 用 list_listeners 查看。save_path 为 MCP 服务器上的保存路径(必填)。"),
		mcp.WithString("kind", mcp.Required(), mcp.Description("生成类型:stage / listen / listendll / stageless / shellcode / dll / ebpf")),
		mcp.WithString("save_path", mcp.Required(), mcp.Description("保存到 MCP 服务器本地的文件路径,如 /tmp/client/stage_linux")),
		mcp.WithString("arch", mcp.Description("客户端类型/架构,如 linux_amd64、windows_amd64.exe、windows_amd64.dll、loader")),
		mcp.WithString("listener_id", mcp.Description("监听器 Id(stage/stageless/shellcode/dll 必填)")),
		mcp.WithString("tp", mcp.Description("通讯类型 tcp/kcp/ws(listen/listendll/ebpf 使用)")),
		mcp.WithString("host", mcp.Description("服务端地址或 IP(listen/listendll 使用)")),
		mcp.WithNumber("port", mcp.Description("服务端端口(listen/listendll 使用)")),
		mcp.WithString("vkey", mcp.Description("通信凭证 Vkey(listen/listendll/ebpf 必填)")),
		mcp.WithString("salt", mcp.Description("流量加密盐(listen/listendll/ebpf 必填)")),
		mcp.WithString("upx", mcp.Description("是否 UPX 压缩:'true'/'false',默认 false")),
		mcp.WithString("proxy", mcp.Description("连接代理,格式 socks5://127.0.0.1:1080(dll/stageless 可选)")),
		mcp.WithString("format", mcp.Description("shellcode 格式:.bin / .c / .raw.txt(shellcode 必填)")),
	), s.handlerGenerateClient)

	// --- host discovery ---
	s.addTool(mcp.NewTool("list_hosts",
		mcp.WithDescription("列出 v_shell 管理平台中的所有主机/客户端(在线与离线)。返回每台主机的 Id、IP、主机名、系统、用户、位置、状态等。"+
			"通过 host_id 可查询单台主机详情,online_only=true 只返回在线主机。"),
		mcp.WithString("host_id", mcp.Description("可选:只返回指定主机 Id")),
		mcp.WithString("online_only", mcp.Description("可选:'true'/'false',是否只返回在线主机")),
	), s.handlerListHosts)

	// --- command control ---
	s.addTool(mcp.NewTool("execute_command",
		mcp.WithDescription("在指定主机上执行 shell 命令(命令控制)。通过目标主机的交互式终端执行,返回命令的标准输出。"+
			"支持管道/重定向等任意 shell 语法;请使用非交互式命令。workdir 可指定执行目录。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id(见 list_hosts)")),
		mcp.WithString("command", mcp.Required(), mcp.Description("要执行的 shell 命令")),
		mcp.WithString("workdir", mcp.Description("可选:命令执行前先 cd 到的目录")),
		mcp.WithNumber("timeout_seconds", mcp.Description("可选:超时秒数,默认 60")),
	), s.handlerExecCommand)

	// --- file management ---
	s.addTool(mcp.NewTool("list_directory",
		mcp.WithDescription("列出主机上指定目录的内容(文件管理)。返回每个条目的名称、类型(目录/文件)、大小、权限、修改时间。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
		mcp.WithString("path", mcp.Description("可选:目录路径,默认 '/'")),
	), s.handlerListDirectory)

	s.addTool(mcp.NewTool("get_disks",
		mcp.WithDescription("获取主机的磁盘/挂载信息。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
	), s.handlerGetDisks)

	s.addTool(mcp.NewTool("read_file",
		mcp.WithDescription("读取主机上文件的内容。二进制内容会以 base64 返回并在返回的 base64_encoded 字段中标注。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
		mcp.WithString("path", mcp.Required(), mcp.Description("文件所在目录路径")),
		mcp.WithString("filename", mcp.Required(), mcp.Description("文件名")),
	), s.handlerReadFile)

	s.addTool(mcp.NewTool("write_file",
		mcp.WithDescription("向主机写入文件(创建或覆盖)。content 为完整文件内容。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
		mcp.WithString("path", mcp.Required(), mcp.Description("文件所在目录路径")),
		mcp.WithString("filename", mcp.Required(), mcp.Description("文件名")),
		mcp.WithString("content", mcp.Required(), mcp.Description("文件内容")),
	), s.handlerWriteFile)

	s.addTool(mcp.NewTool("create_directory",
		mcp.WithDescription("在主机上新建目录。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
		mcp.WithString("path", mcp.Required(), mcp.Description("父目录路径")),
		mcp.WithString("dirname", mcp.Required(), mcp.Description("要创建的目录名")),
	), s.handlerCreateDirectory)

	s.addTool(mcp.NewTool("create_file",
		mcp.WithDescription("在主机上新建空文件。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
		mcp.WithString("path", mcp.Required(), mcp.Description("目录路径")),
		mcp.WithString("filename", mcp.Required(), mcp.Description("文件名")),
	), s.handlerCreateFile)

	s.addTool(mcp.NewTool("delete_file",
		mcp.WithDescription("删除主机上的文件或目录。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
		mcp.WithString("path", mcp.Required(), mcp.Description("目录路径")),
		mcp.WithString("filename", mcp.Required(), mcp.Description("要删除的文件/目录名")),
	), s.handlerDeleteFile)

	s.addTool(mcp.NewTool("delete_files",
		mcp.WithDescription("批量删除主机上的文件或目录。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
		mcp.WithString("path", mcp.Required(), mcp.Description("目录路径")),
		mcp.WithArray("names", mcp.Description("要删除的文件/目录名列表")),
	), s.handlerDeleteFiles)

	s.addTool(mcp.NewTool("rename_file",
		mcp.WithDescription("重命名或移动主机上的文件/目录。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
		mcp.WithString("path", mcp.Required(), mcp.Description("文件所在目录路径")),
		mcp.WithString("target", mcp.Required(), mcp.Description("当前文件名")),
		mcp.WithString("new_name", mcp.Required(), mcp.Description("新文件名(可含路径以移动)")),
	), s.handlerRenameFile)

	s.addTool(mcp.NewTool("wget_file",
		mcp.WithDescription("让主机通过 URL 下载文件到指定目录(使用主机自带 wget/curl 能力)。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
		mcp.WithString("path", mcp.Required(), mcp.Description("保存目录路径")),
		mcp.WithString("url", mcp.Required(), mcp.Description("要下载的 URL")),
	), s.handlerWgetFile)

	s.addTool(mcp.NewTool("upload_file",
		mcp.WithDescription("将 MCP 服务器本地的文件上传到主机目录。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
		mcp.WithString("path", mcp.Required(), mcp.Description("主机上的目标目录")),
		mcp.WithString("local_path", mcp.Required(), mcp.Description("MCP 服务器本地文件路径")),
		mcp.WithString("remote_name", mcp.Description("可选:主机上的文件名,默认取本地文件名")),
	), s.handlerUploadFile)

	s.addTool(mcp.NewTool("download_file",
		mcp.WithDescription("将主机上的文件下载到 MCP 服务器本地(经 v_shell 服务端中转)。"),
		mcp.WithString("host_id", mcp.Required(), mcp.Description("目标主机 Id")),
		mcp.WithString("path", mcp.Required(), mcp.Description("文件所在目录路径")),
		mcp.WithString("filename", mcp.Required(), mcp.Description("文件名")),
		mcp.WithString("local_path", mcp.Description("可选:保存到本地的路径,默认当前目录下的文件名")),
	), s.handlerDownloadFile)
}

func (s *Server) addTool(t mcp.Tool, fn func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	s.srv.AddTool(t, fn)
}

// --- helpers ---

func resultJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("marshal result: " + err.Error()), err
	}
	return mcp.NewToolResultText(string(b)), nil
}

func hostID(r mcp.CallToolRequest) (int, error) {
	idStr := r.GetString("host_id", "")
	if idStr == "" {
		return 0, fmt.Errorf("缺少 host_id 参数")
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, fmt.Errorf("host_id 必须为整数: %s", idStr)
	}
	return id, nil
}

// --- handlers ---

func (s *Server) handlerServerInfo(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg := s.vs.Info()
	user, err := s.vs.GetUserInfo(ctx)
	if err != nil {
		return mcp.NewToolResultError("获取用户信息失败: " + err.Error()), nil
	}
	db, err := s.vs.GetDashboard(ctx)
	if err != nil {
		return mcp.NewToolResultError("获取服务器信息失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{
		"connection": cfg,
		"user":       user,
		"server": map[string]any{
			"version":        db.Version,
			"web_port":       db.WebPort,
			"basic_auth":     db.WebBasicAuth,
			"license_time":   db.LicTime,
			"vip":            db.VIP,
			"log_level":      db.LogLevel,
			"log_path":       db.LogPath,
			"authorized_client_count": db.ClientNum,
		},
	})
}

func (s *Server) handlerDashboard(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	db, err := s.vs.GetDashboard(ctx)
	if err != nil {
		return mcp.NewToolResultError("获取仪表盘信息失败: " + err.Error()), nil
	}
	return resultJSON(db)
}

// --- client generation handler ---

func (s *Server) handlerGenerateClient(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kind := r.GetString("kind", "")
	savePath := r.GetString("save_path", "")
	req := vshell.GenerateClientRequest{
		Kind:       kind,
		SavePath:   savePath,
		Arch:       r.GetString("arch", ""),
		ListenerID: int(r.GetFloat("listener_id", 0)),
		TP:         r.GetString("tp", ""),
		Host:       r.GetString("host", ""),
		Port:       int(r.GetFloat("port", 0)),
		Vkey:       r.GetString("vkey", ""),
		Salt:       r.GetString("salt", ""),
		UPX:        r.GetString("upx", ""),
		Proxy:      r.GetString("proxy", ""),
		Format:     r.GetString("format", ""),
	}
	switch kind {
	case vshell.GenStage, vshell.GenStageless, vshell.GenShellcode, vshell.GenDll:
		if req.ListenerID <= 0 {
			return mcp.NewToolResultError("kind=" + kind + " 需要 listener_id(监听器 Id)"), nil
		}
	case vshell.GenListen, vshell.GenListendll:
		if req.Arch == "" || req.TP == "" || req.Host == "" || req.Port <= 0 || req.Vkey == "" || req.Salt == "" {
			return mcp.NewToolResultError("kind=" + kind + " 需要 arch/tp/host/port/vkey/salt 参数"), nil
		}
	case vshell.GenEbpf:
		if req.Arch == "" || req.TP == "" || req.Vkey == "" || req.Salt == "" {
			return mcp.NewToolResultError("kind=ebpf 需要 arch/tp/vkey/salt 参数"), nil
		}
	default:
		return mcp.NewToolResultError("kind 必须是 stage/listen/listendll/stageless/shellcode/dll/ebpf"), nil
	}
	if kind == vshell.GenShellcode && (req.Arch == "" || req.Format == "") {
		return mcp.NewToolResultError("kind=shellcode 需要 arch 与 format(.bin/.c/.raw.txt)参数"), nil
	}
	if req.Arch == "" {
		return mcp.NewToolResultError("缺少 arch 参数"), nil
	}
	if savePath == "" {
		return mcp.NewToolResultError("缺少 save_path 参数"), nil
	}

	serverName, err := s.vs.GenerateClient(ctx, req)
	if err != nil {
		return mcp.NewToolResultError("生成客户端失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{
		"kind": kind, "arch": req.Arch, "save_path": savePath,
		"server_filename": serverName, "generated": true,
	})
}

// --- listener handlers ---

func (s *Server) handlerListListeners(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var status *bool
	if v := r.GetString("status", ""); v != "" {
		b, _ := strconv.ParseBool(v)
		status = &b
	}
	mode := r.GetString("mode", "")
	listeners, err := s.vs.ListListeners(ctx, status, mode)
	if err != nil {
		return mcp.NewToolResultError("查看监听器失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"listeners": listeners, "total": len(listeners)})
}

func (s *Server) handlerAddListener(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	listenAddr := r.GetString("listen_addr", "")
	if listenAddr == "" {
		return mcp.NewToolResultError("缺少 listen_addr 参数(服务端监听地址)"), nil
	}
	vkey := r.GetString("vkey", "")
	salt := r.GetString("encrypt_salt", "")
	if vkey == "" || salt == "" {
		return mcp.NewToolResultError("vkey 与 encrypt_salt 为必填"), nil
	}
	req := vshell.AddListenerRequest{
		ClientId:          int(r.GetFloat("client_id", 0)),
		Mode:              r.GetString("mode", "tcp"),
		Remark:            r.GetString("remark", ""),
		ListenAddr:        listenAddr,
		ConnectAddr:       r.GetString("connect_addr", ""),
		WsConnectAddr:     r.GetString("ws_connect_addr", ""),
		DNSDomain:         r.GetString("dns_domain", ""),
		PublicDNS:         r.GetString("public_dns", ""),
		DisconnectTimeout: int(r.GetFloat("disconnect_timeout", 30)),
		PingInterval:      int(r.GetFloat("ping_interval", 10)),
		Vkey:              vkey,
		EncryptSalt:       salt,
		OssUrl:            r.GetString("oss_url", ""),
	}
	if err := s.vs.AddListener(ctx, req); err != nil {
		return mcp.NewToolResultError("新建监听器失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{
		"mode": req.Mode, "listen_addr": req.ListenAddr,
		"client_id": req.ClientId, "remark": req.Remark, "added": true,
	})
}

// --- tunnel handlers ---

func tunnelID(r mcp.CallToolRequest) (int, error) {
	id, err := r.RequireInt("id")
	if err != nil {
		return 0, fmt.Errorf("id 参数必须为整数")
	}
	return id, nil
}

func (s *Server) handlerListTunnels(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tunnels, err := s.vs.ListTunnels(ctx)
	if err != nil {
		return mcp.NewToolResultError("列出隧道失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"tunnels": tunnels, "total": len(tunnels)})
}

func (s *Server) handlerAddTunnel(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	clientID := int(r.GetFloat("client_id", 0))
	port := int(r.GetFloat("port", 0))
	if clientID <= 0 || port <= 0 {
		return mcp.NewToolResultError("client_id 与 port 必须为正整数"), nil
	}
	mode := r.GetString("mode", "socks5")
	req := vshell.AddTunnelRequest{
		ClientId: clientID,
		Mode:     mode,
		Port:     port,
		Target:   r.GetString("target", ""),
		Username: r.GetString("username", ""),
		Password: r.GetString("password", ""),
		Remark:   r.GetString("remark", ""),
	}
	if (mode == "tcp" || mode == "udp") && req.Target == "" {
		return mcp.NewToolResultError("mode=" + mode + " 需要 target 参数(格式 10.1.1.99:80)"), nil
	}
	if err := s.vs.AddTunnel(ctx, req); err != nil {
		return mcp.NewToolResultError("新建隧道失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{
		"client_id": clientID, "mode": mode, "port": port,
		"target": req.Target, "remark": req.Remark, "added": true,
	})
}

func (s *Server) handlerDeleteTunnel(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := tunnelID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.vs.DeleteTunnel(ctx, id); err != nil {
		return mcp.NewToolResultError("删除隧道失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"id": id, "deleted": true})
}

func (s *Server) handlerDeleteTunnels(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ids := r.GetIntSlice("ids", nil)
	if len(ids) == 0 {
		return mcp.NewToolResultError("缺少 ids 参数(隧道 Id 数组)"), nil
	}
	if err := s.vs.DeleteTunnels(ctx, ids); err != nil {
		return mcp.NewToolResultError("批量删除隧道失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"ids": ids, "deleted": true})
}

func (s *Server) handlerStartTunnel(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := tunnelID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.vs.StartTunnel(ctx, id); err != nil {
		return mcp.NewToolResultError("启用隧道失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"id": id, "started": true})
}

func (s *Server) handlerStopTunnel(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := tunnelID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.vs.StopTunnel(ctx, id); err != nil {
		return mcp.NewToolResultError("停用隧道失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"id": id, "stopped": true})
}

func (s *Server) handlerEditTunnel(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := tunnelID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	req := vshell.AddTunnelRequest{
		ClientId: int(r.GetFloat("client_id", 0)),
		Mode:     r.GetString("mode", ""),
		Port:     int(r.GetFloat("port", 0)),
		Target:   r.GetString("target", ""),
		Username: r.GetString("username", ""),
		Password: r.GetString("password", ""),
		Remark:   r.GetString("remark", ""),
	}
	if req.ClientId == 0 || req.Mode == "" || req.Port == 0 {
		return mcp.NewToolResultError("编辑至少需要 client_id、mode、port 参数"), nil
	}
	if err := s.vs.EditTunnel(ctx, id, req); err != nil {
		return mcp.NewToolResultError("编辑隧道失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"id": id, "edited": true})
}

func (s *Server) handlerSetTunnelRemark(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := tunnelID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	remark := r.GetString("remark", "")
	if remark == "" {
		return mcp.NewToolResultError("缺少 remark 参数"), nil
	}
	if err := s.vs.SetTunnelRemark(ctx, id, remark); err != nil {
		return mcp.NewToolResultError("修改备注失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"id": id, "remark": remark, "saved": true})
}

func (s *Server) handlerListHosts(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	online := false
	if v := r.GetString("online_only", ""); v != "" {
		online, _ = strconv.ParseBool(v)
	}
	hosts, err := s.vs.ListHosts(ctx, &online)
	if err != nil {
		return mcp.NewToolResultError("列出主机失败: " + err.Error()), nil
	}
	filterID, _ := hostID(r)
	if filterID > 0 {
		kept := hosts[:0]
		for _, h := range hosts {
			if h.Id == filterID {
				kept = append(kept, h)
			}
		}
		hosts = kept
	}
	if len(hosts) == 0 {
		return mcp.NewToolResultText("未找到匹配的主机(请确认 host_id 与在线状态)。"), nil
	}
	return resultJSON(hosts)
}

func (s *Server) handlerExecCommand(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	command := r.GetString("command", "")
	if command == "" {
		return mcp.NewToolResultError("缺少 command 参数"), nil
	}
	workdir := r.GetString("workdir", "")
	timeout := time.Duration(r.GetFloat("timeout_seconds", 60)) * time.Second
	out, err := s.vs.Exec(ctx, id, command, workdir, timeout)
	if err != nil {
		return mcp.NewToolResultError("命令执行失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{
		"host_id":  id,
		"command":  command,
		"exit_code": 0,
		"output":   out,
	})
}

func (s *Server) handlerListDirectory(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path := r.GetString("path", "/")
	items, err := s.vs.ListDirectory(ctx, id, path)
	if err != nil {
		return mcp.NewToolResultError("列目录失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"host_id": id, "path": path, "items": items, "total": len(items)})
}

func (s *Server) handlerGetDisks(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	disks, err := s.vs.GetDisks(ctx, id)
	if err != nil {
		return mcp.NewToolResultError("获取磁盘信息失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"host_id": id, "disks": disks})
}

func (s *Server) handlerReadFile(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path := r.GetString("path", "")
	filename := r.GetString("filename", "")
	if path == "" || filename == "" {
		return mcp.NewToolResultError("缺少 path 或 filename 参数"), nil
	}
	content, base64Enc, err := s.vs.ReadFile(ctx, id, path, filename)
	if err != nil {
		return mcp.NewToolResultError("读取文件失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{
		"host_id": id, "path": path, "filename": filename,
		"content": content, "base64_encoded": base64Enc,
	})
}

func (s *Server) handlerWriteFile(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path := r.GetString("path", "")
	filename := r.GetString("filename", "")
	content := r.GetString("content", "")
	if path == "" || filename == "" {
		return mcp.NewToolResultError("缺少 path 或 filename 参数"), nil
	}
	if err := s.vs.WriteFile(ctx, id, path, filename, content); err != nil {
		return mcp.NewToolResultError("写入文件失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"host_id": id, "path": path, "filename": filename, "written": true})
}

func (s *Server) handlerCreateDirectory(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path := r.GetString("path", "")
	dirname := r.GetString("dirname", "")
	if path == "" || dirname == "" {
		return mcp.NewToolResultError("缺少 path 或 dirname 参数"), nil
	}
	if err := s.vs.CreateDirectory(ctx, id, path, dirname); err != nil {
		return mcp.NewToolResultError("新建目录失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"host_id": id, "path": path, "dirname": dirname, "created": true})
}

func (s *Server) handlerCreateFile(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path := r.GetString("path", "")
	filename := r.GetString("filename", "")
	if path == "" || filename == "" {
		return mcp.NewToolResultError("缺少 path 或 filename 参数"), nil
	}
	if err := s.vs.CreateFile(ctx, id, path, filename); err != nil {
		return mcp.NewToolResultError("新建文件失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"host_id": id, "path": path, "filename": filename, "created": true})
}

func (s *Server) handlerDeleteFile(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path := r.GetString("path", "")
	filename := r.GetString("filename", "")
	if path == "" || filename == "" {
		return mcp.NewToolResultError("缺少 path 或 filename 参数"), nil
	}
	if err := s.vs.DeleteFile(ctx, id, path, filename); err != nil {
		return mcp.NewToolResultError("删除失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"host_id": id, "path": path, "filename": filename, "deleted": true})
}

func (s *Server) handlerDeleteFiles(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path := r.GetString("path", "")
	names := r.GetStringSlice("names", nil)
	if path == "" || len(names) == 0 {
		return mcp.NewToolResultError("缺少 path 或 names 参数"), nil
	}
	if err := s.vs.DeleteFiles(ctx, id, path, names); err != nil {
		return mcp.NewToolResultError("批量删除失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"host_id": id, "path": path, "names": names, "deleted": true})
}

func (s *Server) handlerRenameFile(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path := r.GetString("path", "")
	target := r.GetString("target", "")
	newName := r.GetString("new_name", "")
	if path == "" || target == "" || newName == "" {
		return mcp.NewToolResultError("缺少 path/target/new_name 参数"), nil
	}
	if err := s.vs.RenameFile(ctx, id, path, target, newName); err != nil {
		return mcp.NewToolResultError("重命名失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"host_id": id, "path": path, "target": target, "new_name": newName, "renamed": true})
}

func (s *Server) handlerWgetFile(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path := r.GetString("path", "")
	urlStr := r.GetString("url", "")
	if path == "" || urlStr == "" {
		return mcp.NewToolResultError("缺少 path 或 url 参数"), nil
	}
	if err := s.vs.WgetFile(ctx, id, path, urlStr); err != nil {
		return mcp.NewToolResultError("wget 失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"host_id": id, "path": path, "url": urlStr, "started": true})
}

func (s *Server) handlerUploadFile(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path := r.GetString("path", "")
	local := r.GetString("local_path", "")
	if path == "" || local == "" {
		return mcp.NewToolResultError("缺少 path 或 local_path 参数"), nil
	}
	remoteName := r.GetString("remote_name", "")
	remote, err := s.vs.UploadFile(ctx, id, path, local, remoteName)
	if err != nil {
		return mcp.NewToolResultError("上传失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{"host_id": id, "path": path, "local_path": local, "remote": remote, "uploaded": true})
}

func (s *Server) handlerDownloadFile(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := hostID(r)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	path := r.GetString("path", "")
	filename := r.GetString("filename", "")
	local := r.GetString("local_path", "")
	if path == "" || filename == "" {
		return mcp.NewToolResultError("缺少 path 或 filename 参数"), nil
	}
	n, err := s.vs.DownloadFile(ctx, id, path, filename, local)
	if err != nil {
		return mcp.NewToolResultError("下载失败: " + err.Error()), nil
	}
	return resultJSON(map[string]any{
		"host_id": id, "path": path, "filename": filename,
		"local_path": local, "bytes": n, "downloaded": true,
	})
}
