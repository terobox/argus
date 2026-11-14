# 安装指南

argus 由两个独立的单文件二进制组成：

- **argus-server**：中央服务端。接收各 agent 上报的数据，用 SQLite 存储，
  提供 Web 界面、查询与告警。通常只部署一台。
- **argus-agent**：主机采集端。读取本机 `/proc` 等数据，推送到 server。
  每台被监控的机器各装一个。

本目录（`deploy/`）提供两个安装脚本，均**仅支持 Ubuntu + systemd**：

| 脚本 | 作用 |
| --- | --- |
| `install-agent.sh` | 安装 / 升级 / 卸载 agent（非交互，可管道执行） |
| `install-server.sh` | 安装 / 升级 / 卸载 server（交互式设置初始管理员，或环境变量自动化） |

## 脚本从哪里获取

两个脚本都从 GitHub 仓库直接获取（HTTPS），例如：

```bash
curl -fsSL https://raw.githubusercontent.com/terobox/argus/main/deploy/install-agent.sh | sudo bash
```

而不是由 argus server 提供一个 `/install.sh` 端点，原因：

- 安装脚本在 server 尚未安装时就需要下载，从 GitHub 获取更合理。
- **无域名场景**下 server 通常只有 `http://<IP>:8095` 这样的明文地址；把
  「未认证的 HTTP 响应」直接 `| sudo bash` 存在中间人注入风险。GitHub 的
  HTTPS 下载规避了这一点。

> 仓库名 `terobox/argus` 与分支 `main` 需要与你的实际发布一致；若改仓库名，
> 两个脚本、`docs/install.md` 以及 Web 界面生成的安装命令都要同步修改
> `REPO` / URL。

## 无域名怎么部署

不准备域名时，推荐做法：

- server 监听 `:8095`，直接用 `http://<服务器IP>:8095` 访问 Web 界面。
- agent 的 `--server` 也指向同一个 `http://<服务器IP>:8095`。
- **内网 / VPN / 单机自用**：直接明文 HTTP 即可，简单可靠。
- **跨公网**：不要把明文 HTTP 暴露公网。用反向代理（Caddy / Nginx）做 TLS
  终结并反代到 `127.0.0.1:8095`，agent 再走 `https://<域名或IP>`。
- 注意：agent 的推送客户端**会校验 TLS 证书**（没有 `insecure_skip_verify`），
  因此不要用自签证书直连；要么用受信证书，要么走内网明文 HTTP。

## 安装 server

### 交互式（推荐）

```bash
curl -fsSL https://raw.githubusercontent.com/terobox/argus/main/deploy/install-server.sh -o install-server.sh
sudo bash install-server.sh
```

脚本会提示输入初始管理员密码（不回显）。可用选项：

```bash
sudo bash install-server.sh --listen :8095 --data-dir /var/lib/argus --admin-username admin
```

### 自动化（免交互）

```bash
ARGUS_ADMIN_PASSWORD='你的密码' sudo -E bash install-server.sh
```

> `sudo -E` 用于保留环境变量；不加 `-E` 时 `sudo` 会丢弃
> `ARGUS_ADMIN_PASSWORD`，脚本会退回交互式提示。

### 做了什么

1. 从 GitHub Releases 下载 `argus-server_linux_{amd64|arm64}` 与 `checksums.txt`。
2. 校验 SHA-256，通过后安装到 `/usr/local/bin/argus-server`。
3. 创建非登录系统用户 `argus`（`--system --no-create-home --shell /usr/sbin/nologin`）。
4. 创建数据目录 `/var/lib/argus`（属主 `argus:argus`）。
5. 首次安装时生成 `/etc/argus/server.yaml`（`0640 root:argus`；升级重跑时不覆盖）。
6. 首次安装时生成 `/etc/argus/server.env` 保存初始管理员凭据（`0640 root:argus`，仅首次启动生效）。
7. 部署并启动加固的 `argus-server.service`。

安装完成后：

- 打开 `http://<服务器IP>:8095`，用 `admin`（或你指定的用户名）与刚才的密码登录。
- 在 Web 界面「Settings」页创建节点，得到 node-id 与 token（**token 只显示一次**）。
- 首次登录成功后，可删除 `/etc/argus/server.env`（初始管理员已写入数据库）。

## 安装 agent

在 server 的 Web 界面创建节点后，界面会生成一条安装命令，复制到目标机器执行即可：

```bash
curl -fsSL https://raw.githubusercontent.com/terobox/argus/main/deploy/install-agent.sh \
  | sudo bash -s -- --server http://<服务器IP>:8095 --node-id <node-id> --token <token>
```

也可手动执行：

```bash
sudo bash install-agent.sh --server http://<服务器IP>:8095 --node-id <node-id> --token <token>
```

选项：`--public-ip` 手动指定公网 IP（默认不探测，保护隐私）；`--version vX.Y.Z`
安装指定版本；`--uninstall` 卸载。token 也可用环境变量 `ARGUS_NODE_TOKEN` 传入，
比命令行更不易泄露。

### 做了什么

1. 下载并 SHA-256 校验 `argus-agent_linux_{arch}`，安装到 `/usr/local/bin/argus-agent`。
2. 创建非登录系统用户 `argus-agent`。
3. 写配置 `/etc/argus/agent.yaml`（`0640 root:argus-agent`）。
4. 部署并启动加固的 `argus-agent.service`，立即开始采集并推送。

## 工作原理

- **推送模型**：agent 定时把数据 POST 到 server 的上报端点，server 不主动连接
  agent。这天然适配 NAT / 防火墙后的主机。
- **采集周期**：默认每秒采一次快指标（CPU/内存/网络/磁盘 IO），每 30s 采慢指标
  （磁盘容量、登录用户数），每 15s 推送一批；内存缓冲 600 条（约 10 分钟），
  网络抖动时本地暂存、自动重试。
- **存储分层**：server 端按 1s / 15s / 5m 三层时序降采样——原始 1s 数据保留 1 小时，
  15s 保留 24 小时，5m 保留 30 天，控制 SQLite 体积。
- 指标语义（计数器是「开机以来累计值」，速率在读取时由差值推导）见
  [metrics.md](metrics.md)。

## 配置文件

### `/etc/argus/server.yaml`

| 键 | 默认 | 说明 |
| --- | --- | --- |
| `listen_addr` | `:8095` | 监听地址 |
| `db_path` | `argus.db` | SQLite 文件路径 |
| `job_interval` | `15s` | 后台任务（告警评估、过期清理）周期 |
| `session_ttl` | `168h` | 登录会话有效期 |
| `cookie_secure` | `false` | HTTPS 下应设为 `true` |
| `alert` | 关闭 | 告警配置，见下 |

时长均用 Go 时长字符串：`30s`、`5m`、`1h`、`168h`。未列出的键使用默认值。

告警示例（ntfy webhook）：

```yaml
alert:
  enabled: true
  webhook_url: "https://ntfy.sh/your-topic"
  offline_after: 1m      # 超过多久未上报判定离线
  sustain: 5m            # 阈值评估窗口
  cpu_threshold: 90      # 5m 平均 CPU > 90% 告警
  mem_threshold: 90
  disk_threshold: 90
  cooldown: 1h           # 同类告警冷却
```

### `/etc/argus/agent.yaml`

| 键 | 默认 | 说明 |
| --- | --- | --- |
| `server_url` | —（必填） | server 地址，`http(s)://host[:port]` |
| `node_id` | —（必填） | 节点 ID |
| `token` | —（必填） | 节点 token |
| `collect_interval` | `1s` | 快指标采集周期 |
| `slow_interval` | `30s` | 慢指标采集周期 |
| `push_interval` | `15s` | 推送周期 |
| `buffer_capacity` | `600` | 内存缓冲条数 |
| `public_ip` | 空 | 手动指定公网 IP（省略则不探测） |
| `gzip` | `true` | 上报是否 gzip 压缩 |

## 升级与卸载

**升级（推荐：自更新命令，不动配置）**：agent 与 server 都内置了 `update`
子命令，从 GitHub Releases 下载最新版、校验 SHA-256、原子替换二进制，
**不会触碰配置文件**：

```bash
# agent
sudo /usr/local/bin/argus-agent update
sudo systemctl restart argus-agent

# server
sudo /usr/local/bin/argus-server update
sudo systemctl restart argus-server
```

`update` 检测到已是最新版本时会提示 `already up to date` 并跳过下载。

**升级（备用：重跑安装脚本）**：也可以重新运行安装脚本（幂等，会用 `--version`
指定版本覆盖二进制并重启服务）。server 脚本升级时**不会覆盖已有的
`/etc/argus/server.yaml` 与 `server.env`**（仅首次安装时生成），因此手动改过的
告警、`cookie_secure`、`listen_addr` 等配置都会保留；agent 脚本则会按你传入的
`--server / --node-id / --token` 重新生成 `agent.yaml`。

> `update` 命令走 GitHub API `releases/latest`（未认证，每 IP 每小时 60 次），
> 偶发限流时退回重跑安装脚本即可（脚本走静态下载地址，不受 API 限流影响）。

> 出于安全考虑，**不提供自动/远程升级**：agent / server 均以加固的非 root 用户运行，
> 不应替换 root 拥有的二进制；升级一律由运维手动执行（如上 `update` 命令）。

**卸载 agent**：

```bash
sudo bash install-agent.sh --uninstall
```

**卸载 server**：

```bash
sudo bash install-server.sh --uninstall
```

卸载会停止并删除服务、二进制与配置，但**保留数据目录与系统用户**，避免误删历史数据。

## 安全设计

- **来源可信**：二进制只从 GitHub 官方 releases 下载，安装前校验 `checksums.txt`
  中的 SHA-256；脚本本身走 HTTPS。
- **最小权限**：agent / server 均以非登录系统用户运行，无 shell、无家目录。
- **systemd 加固**：两个单元都启用
  `NoNewPrivileges`、`ProtectSystem=strict`（除 server 数据目录外全盘只读）、
  `ProtectHome`、`PrivateTmp`、`PrivateDevices`、`ProtectKernelTunables`、
  `ProtectKernelModules`、`RestrictSUIDSGID`、`LockPersonality`、
  `RestrictNamespaces`、`MemoryDenyWriteExecute`、空 `CapabilityBoundingSet`
  （无任何 capability）、`RestrictAddressFamilies`（仅 TCP/UDP/Unix）、
  `SystemCallArchitectures=native`。server 额外放开 `ReadWritePaths=<数据目录>`
  以写入 SQLite。
- **敏感信息**：节点 token 在 server 端只存 SHA-256 哈希；agent 配置 `0640`，
  仅 root 与该服务用户可读。初始管理员凭据写在 `server.env`（`0640`），
  首次启动后可删除。

## 常见问题

- **想用 80/443 端口**：普通用户不能绑定 <1024 端口。建议用反向代理；若坚持直连，
  需在 systemd 单元加 `AmbientCapabilities=CAP_NET_BIND_SERVICE`（不推荐）。
- **agent 连不上 server**：确认 server 监听 `:8095`（即所有接口），防火墙放行该端口，
  且 `--server` 用的是 agent 所在网络能访问到的 IP。
- **`MemoryDenyWriteExecute` 启动失败**：极老内核可能出现。去掉 systemd 单元中该行后
  `systemctl daemon-reload && systemctl restart <服务>` 即可（纯 Go 二进制一般无此问题）。
- **改了 `server.env` 密码不生效**：初始管理员只在「首次启动且当前没有任何管理员」时
  创建，之后 env 文件中的凭据不再被读取，可安全删除。
