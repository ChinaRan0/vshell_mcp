# vshell-mcp

把 **v_shell 管理平台** 的命令控制、文件管理、隧道代理、监听管理、客户端生成等能力封装为 **MCP Server(SSE 传输)**,让 AI 可以直接操作平台上的任意受管主机。

## ✨ 功能特性

- **服务器信息**:查看当前连接配置(URL / 账号)、登录用户、服务端版本与授权
- **仪表盘指标**:客户端/监听/代理数量、在线数、CPU/内存/磁盘/负载、带宽、TCP/UDP 连接数
- **主机管理**:列出所有受管主机(在线/离线、系统、IP、用户、归属地)
- **命令控制**:在任意主机上执行 shell 命令(支持管道/重定向/workdir/超时)
- **文件管理**:列目录、读写文件、新建/删除/重命名、上传/下载、URL 下载(wget)
- **隧道代理**:隧道/代理的增删改查、启用/停用、改备注
- **监听管理**:查看与新增监听器(按需求**不提供删除**功能)
- **客户端生成**:生成并下载客户端(stage 反向 / listen 正向 / stageless / shellcode / dll / ebpf 等)

**共 27 个工具**,全部通过官方 MCP SDK 与真实 v_shell 实例端到端验证。

## 工作原理

```
AI 客户端(Claude Code 等)
   │  MCP over SSE  (type: sse, url: http://<MCP服务器>:19080/sse)
   ▼
vshell-mcp  (本仓库,Go 编写)
   │  HTTP API: /api/login、/api/client/list、/api/file/*、/api/tunnel/*(POST JSON)
   │  WebSocket: /api/terminal/ws(交互式终端,用于命令控制)
   ▼
v_shell 平台  (http://<v_shell地址>:8082)
```

- **命令控制**:复用 v_shell 管理台的终端 WebSocket(`/api/terminal/ws`)。每条命令开启一次交互式 shell,
  先 `stty -echo` 关闭回显、固定 prompt,再用 `MCPB_*` / `MCPD_*` 唯一标记夹取命令,返回纯 stdout。
- **文件/隧道**:直接调用 v_shell 的 `/api/file/*`、`/api/tunnel/*` HTTP 接口。
- **认证**:启动时用账号密码登录换取 JWT(`Token: <JWT>` 请求头),过期自动重新登录。

## 环境要求

- Go 1.24+(仅构建时需要;运行只需要编译好的二进制)
- 运行机器能访问 v_shell 平台的 Web 地址(HTTP / HTTPS 均可)

## 构建

```bash
go build -o vshell-mcp .
# 或
./run.sh          # 未编译时脚本会自动编译
```

## 配置与启动

全部通过**命令行参数**配置,无需任何环境变量:

```bash
./vshell-mcp -h http://your-vshell-server:8082 -u admin -p 'your_password'
```

| 参数 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `-h <url>` | ✅ | - | v_shell Web 控制台地址,如 `http://your-vshell-server:8082` |
| `-u <user>` | ⭕* | `admin` | 登录账号 |
| `-p <pass>` | ⭕* | - | 登录密码 |
| `-token` | ⭕ | - | 预置 JWT,设置后跳过账号密码登录 |
| `-port` | ⭕ | `19080` | MCP SSE 服务监听端口 |
| `-mcpurl` | ⭕ | 空 | MCP 服务器对外可达地址(置于反向代理后时设置) |
| `-prefix` | ⭕ | `/api` | v_shell API 路径前缀 |
| `-timeout` | ⭕ | `60` | 单次请求超时(秒) |
| `-help` | - | - | 显示帮助 |

> `*`:设置了 `-token` 则账号密码可省;否则 `-p` 必填(`-u` 默认 `admin`)。
> ⚠️ `-h` 已绑定为 v_shell **地址**参数(非 help);查看帮助请用 `-help`。

启动时先做一次登录/连通性校验,凭据错误会直接报错退出。正常启动:

```
v_shell MCP server listening on :19080  (SSE endpoint: http://127.0.0.1:19080/sse)
```

## 接入 AI 客户端

### Claude Code

```bash
claude mcp add vshell --type sse --url http://127.0.0.1:19080/sse
```

或写入项目 `.mcp.json` / `~/.claude.json`:

```json
{
  "mcpServers": {
    "vshell": {
      "type": "sse",
      "url": "http://127.0.0.1:19080/sse"
    }
  }
}
```

> MCP 服务运行在其它机器时,`url` 填该机器的地址与端口;若开了 `-mcpurl`,填它生成的地址。

### 其它支持 SSE MCP 的客户端

通用配置:`type: sse`,`url: http://<mcp服务器>:19080/sse`。

## 工具清单(27 个)

> 通用说明:`host_id` 为「客户端管理」里的主机 Id(见 `list_hosts`);`local_path` 等本地路径均指 **MCP 服务器所在机器** 的路径。

### 服务器信息(2)

| 工具 | 参数 | 说明 |
|---|---|---|
| `get_server_info` | - | 当前连接配置(URL/账号/API 前缀)、登录用户、服务端版本/端口/授权 |
| `get_dashboard` | - | 服务器仪表盘指标(客户端/监听/代理数量、在线数、CPU/内存/磁盘/负载、带宽、TCP/UDP 连接、版本/授权到期) |

### 主机与命令控制(2)

| 工具 | 参数 | 说明 |
|---|---|---|
| `list_hosts` | `host_id`? `online_only`? | 列出所有主机;`host_id` 查单台,`online_only=true` 只查在线 |
| `execute_command` | `host_id`* `command`* `workdir`? `timeout_seconds`? | 在主机上执行 shell 命令,返回 stdout;默认超时 60s |

### 文件管理(12)

| 工具 | 参数 | 说明 |
|---|---|---|
| `list_directory` | `host_id`* `path`? | 列出主机目录(默认 `/`) |
| `get_disks` | `host_id`* | 主机磁盘/挂载信息 |
| `read_file` | `host_id`* `path`* `filename`* | 读取文件;二进制自动 base64(带 `base64_encoded` 标记) |
| `write_file` | `host_id`* `path`* `filename`* `content`* | 写入/覆盖文件 |
| `create_directory` | `host_id`* `path`* `dirname`* | 新建目录 |
| `create_file` | `host_id`* `path`* `filename`* | 新建空文件 |
| `delete_file` | `host_id`* `path`* `filename`* | 删除单个文件/目录 |
| `delete_files` | `host_id`* `path`* `names`* | 批量删除 |
| `rename_file` | `host_id`* `path`* `target`* `new_name`* | 重命名/移动(可带路径) |
| `upload_file` | `host_id`* `path`* `local_path`* `remote_name`? | 上传本地文件到主机;默认以本地 basename 命名 |
| `download_file` | `host_id`* `path`* `filename`* `local_path`? | 下载主机文件到本地(经服务端中转,支持大文件) |
| `wget_file` | `host_id`* `path`* `url`* | 让主机通过 URL 下载文件 |

### 隧道 / 代理(8,对应 `/tunnel/list`)

| 工具 | 参数 | 说明 |
|---|---|---|
| `list_tunnels` | - | 列出所有隧道:Id、绑定客户端、模式、端口、目标、账号、启用/停用、客户端在线状态 |
| `add_tunnel` | `client_id`* `port`* `mode`? `target`? `username`? `password`? `remark`? | 新建隧道;`mode` 为 `socks5`/`http`/`tcp`/`udp`(默认 socks5),`tcp`/`udp` 必须带 `target`(格式 `10.1.1.99:80`) |
| `edit_tunnel` | `id`* + 上述字段 | 编辑隧道配置 |
| `set_tunnel_remark` | `id`* `remark`* | 修改隧道备注 |
| `start_tunnel` | `id`* | 启用隧道 |
| `stop_tunnel` | `id`* | 停用隧道 |
| `delete_tunnel` | `id`* | 删除单条隧道 |
| `delete_tunnels` | `ids`* | 批量删除隧道(Id 数组) |

### 监听器(2,对应 `/listener/list`)

> 按需求:**仅支持查看与新增,禁止删除监听器**(未提供删除工具)。

| 工具 | 参数 | 说明 |
|---|---|---|
| `list_listeners` | `status`? `mode`? | 查看所有监听器:Id、模式、监听地址、外网连接地址、心跳间隔、超时心跳次数、Vkey、加密盐、备注、运行状态 |
| `add_listener` | `listen_addr`* `mode`? `vkey`* `encrypt_salt`* `client_id`? `connect_addr`? `ws_connect_addr`? `dns_domain`? `public_dns`? `oss_url`? `disconnect_timeout`? `ping_interval`? `remark`? | 新建监听器;`mode` 为 `tcp`/`kcp`/`ws`/`dns`/`doh`/`dot`/`oss`(默认 tcp),各模式地址要求见工具描述 |

### 客户端生成(1,对应 `/download` 客户端生成页)

| 工具 | 参数 | 说明 |
|---|---|---|
| `generate_client` | `kind`* `save_path`* `arch`? `listener_id`? `tp`? `host`? `port`? `vkey`? `salt`? `upx`? `proxy`? `format`? | 生成并下载客户端到 `save_path`。`kind`:`stage`(需 `listener_id`+`arch`)/ `listen`(需 `arch`+`tp`+`host`+`port`+`vkey`+`salt`)/ `listendll`(同 listen,arch 为 .dll)/ `stageless`(需 `listener_id`+`arch`)/ `shellcode`(需 `listener_id`+`arch`+`format`)/ `dll`(需 `listener_id`+`arch`)/ `ebpf`(需 `arch`+`tp`+`vkey`+`salt`) |

(* = 必填;? = 可选)

## 测试

```bash
# 对真实 v_shell 实例跑完整冒烟(需配好 -h -u -p)
VSHELL_URL=http://your-vshell-server:8082 VSHELL_USERNAME=admin VSHELL_PASSWORD=xxx \
  go run ./cmd/smoke

# 用官方 MCP Python SDK 连接运行中的 MCP 服务并调用工具
python3 tests/client_smoke.py
```

## 项目结构

```
.
├── main.go                 入口:命令行参数解析、登录校验、启动 SSE 服务
├── internal/
│   ├── vshell/             v_shell 客户端
│   │   ├── client.go       HTTP 封装(登录、Token 刷新、统一响应解析)
│   │   ├── terminal.go     终端 WebSocket 命令执行
│   │   ├── files.go        文件管理操作
│   │   ├── tunnel.go       隧道/代理管理操作
│   │   ├── listener.go     监听器管理操作(仅查看/新增,无删除)
│   │   ├── client_gen.go   客户端生成/下载
│   │   ├── dashboard.go    服务器信息与仪表盘
│   │   └── types.go        数据结构
│   └── mcp/server.go       MCP 工具定义与处理器
├── cmd/smoke/main.go       冒烟测试程序
├── tests/client_smoke.py   官方 MCP SDK 验证脚本
├── docs/API_NOTES.md       v_shell Web API 逆向笔记
└── run.sh                  启动脚本(自动编译 + 参数透传)
```

## 安全说明

- **本工具能执行任意命令、增删文件与隧道**,属于强能力工具,请只对你自己拥有管理权限的 v_shell 平台使用。
- `get_server_info` 会把登录密码原样返回给 AI 客户端。若不需要回显密码,可自行改动
  `internal/mcp/server.go` 的 `handlerServerInfo`,将 `connection.password` 置为 `"******"`。
- 命令控制走的是目标主机的真实 shell,交互式程序(`vim`、`passwd`、`top` 等)会卡住,请用非交互命令。
- 启动参数中的密码会出现在进程命令行(`ps` 可见),生产环境建议结合密钥管理或 `-token` 使用。

## 常见问题

- **启动报"无法连接 v_shell"** — 检查 `-h` 地址是否可达、`-u`/`-p` 是否正确、账号是否有权。
- **`execute_command` 返回空 / 超时** — 目标主机可能离线(`IsConnect=false`),或命令是交互式程序。
  耗时命令在调用时调大 `timeout_seconds` 参数(如 `timeout_seconds=300`)。
- **`download_file` 失败** — 文件在主机侧须存在且当前用户可读;下载是异步任务,大文件需等待轮询完成。
- **`add_tunnel` 报参数错误** — 检查:`mode=tcp/udp` 必须给 `target`;`port` 不能与已有隧道/服务端口冲突。
- **MCP 客户端连接不上** — 确认 MCP 服务器已启动且端口可访问;检查防火墙;用了 `-mcpurl` 时确认对外地址可达。
