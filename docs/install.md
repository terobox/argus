# 安装指南

argus 由三个可独立部署的部分组成：

- **argus-server**（Go）：中央服务端。接收各 agent 上报的数据，用 SQLite
  存储，提供 API、查询、告警与 SSE。通常只部署一台。
- **argus-agent**（Go）：主机采集端。读取本机 `/proc` 等数据，推送到 server。
  每台被监控的机器各装一个。
- **web**（React + Vite）：管理界面。构建产物由 nginx 作为静态站点提供。

部署模型：

| 组件 | 版本管理 | 说明 |
| --- | --- | --- |
| argus-server | **shipit**（`type: go`） | 一台服务器一个项目目录 |
| web | **shipit**（`type: vite`） | 与 server 同一个 repo、同一个 tag |
| argus-agent | `install-agent.sh` + 自更新 | 多机、非项目化，保持轻量 |

server 与 web 的升级完全由 shipit 完成：CI 在推送 tag 后调用 shipit webhook，
shipit 下载 release、校验 SHA-256、切换 `current` 符号链接、重启服务并做健康
检查，失败自动回滚。**argus-server 二进制本身不再提供 `update` 命令**。

本目录（`deploy/`）提供的文件：

| 文件 | 作用 |
| --- | --- |
| `install-agent.sh` | 安装 / 升级 / 卸载 agent（非交互，可管道执行） |
| `argus-server.service` | server 的 systemd 单元示例（按 shipit 契约） |
| `nginx.conf` | web 静态站点 + `/api` 反向代理示例 |

> server 不再有安装脚本。首次部署需要一次性手工完成目录、配置、systemd 单元
> 与 sudoers；之后所有版本升级都是自动的。这是 shipit 的设计：它只管版本，
> unit / sudoers / nginx 由你负责。

---

## 一、部署 server（shipit 管理）

### 0. 前置条件

- Ubuntu + systemd（其他 systemd 发行版同理，命令按包管理器调整）。
- 已安装 shipit（`shipit version` 可用）。
- 仓库已经用新的发布流程产出一个 release（包含 `argus-server-<tag>-linux-<arch>.tar.gz`）。

### 1. shipit 配置

编辑 `/etc/shipit/shipit.yaml`（`sudo -u shipit vi /etc/shipit/shipit.yaml`），加入
两个项目：

```yaml
projects:
  argus-server:
    type: go
    repo: terobox/argus
    dir: /srv/app/argus/server
    service: argus-server
    asset: argus-server
    health: http://127.0.0.1:8095/healthz

  argus-web:
    type: vite
    repo: terobox/argus
    dir: /srv/app/argus/web
    asset: argus-web
```

私有仓库需要在 shipit 配置里提供 GitHub token（`token:`，Contents: Read-only）。

### 2. 创建目录并授权

```bash
sudo mkdir -p /srv/app/argus/server/shared/data /srv/app/argus/web
sudo chown -R shipit:shipit /srv/app/argus
sudo chmod 750 /srv/app/argus /srv/app/argus/server /srv/app/argus/web
```

说明：

- 两个项目目录（`server`、`web`）是兄弟目录，满足 shipit "项目目录不能重叠" 的要求。
- SQLite 放在 `server/shared/data/`：在项目目录内，但不在 `releases/` 下，
  shipit 的清理永远不会碰它。
- 目录属主必须是 `shipit`，因为 shipit 以该用户解包、切链接，服务单元也以
  `User=shipit` 运行。

### 3. server 配置

创建 `/srv/app/argus/server/shared/server.yaml`（属主 `shipit`，权限 `0600`）。
字段含义可参考仓库中的 `server/config.example.yaml`；生产环境的所有路径都必须
改成绝对路径：

```yaml
listen_addr: ":8095"
db_path: "/srv/app/argus/server/shared/data/argus.db"

# 通过 HTTPS 提供服务时设为 true，登录 Cookie 才会带 Secure 标记。
# 先用 HTTP 跑通时保持 false，否则浏览器不会保存登录 Cookie。
cookie_secure: false

# 告警默认关闭。启用示例（去掉注释并填写 ntfy 地址）：
# alert:
#   enabled: true
#   webhook_url: "https://ntfy.sh/your-topic"
```

要点：

- **所有路径写绝对路径**。进程的工作目录是 `.../current`，相对路径会指向发行
  目录，既不可写也会在升级后丢失。
- server 只提供 API，不托管静态前端；管理界面由 nginx 从 `web/current` 提供。
- 如果 nginx 与 server 在同一台机器，且所有 agent 也都能访问 nginx，可以把
  `listen_addr` 改成 `127.0.0.1:8095`，减少暴露面。

### 4. 初始管理员

首次部署后打开 nginx 提供的地址，会进入初始化页面：设置管理员用户名和密码
（至少 8 位），创建后自动登录。初始化页只在数据库还没有任何管理员时可用，
创建完成后该接口返回 409，永久关闭。

> 建议首次部署后尽快完成初始化。如果不希望在初始化窗口内对公网开放，可先限制
> 访问来源，或让 server 只监听 `127.0.0.1:8095`，通过 SSH 隧道访问前端完成初始化。

无人值守 / 自动化部署可以改用环境变量播种。创建
`/srv/app/argus/server/shared/.env`（属主 `shipit`，权限 `0600`）：

```bash
ARGUS_ADMIN_USERNAME=admin
ARGUS_ADMIN_PASSWORD='换成一个强密码'
```

启动时若数据库里还没有管理员，会自动创建；此时 Web 初始化页不会出现。密码建议
用单引号包住，并避免密码本身包含单引号；systemd 的 `EnvironmentFile` 会按 shell
风格解析引号。systemd 以 root 读取该文件后注入进程环境，`0600` 即可。

> 忘记密码：当前版本没有内置重置入口。可停服后删除管理员记录再重新初始化，
> 例如 `sudo -u shipit sqlite3 /srv/app/argus/server/shared/data/argus.db
> 'DELETE FROM admins;'`，然后重启服务，重新访问页面完成初始化（统计数据保留）。

### 5. systemd 单元与 sudoers

把仓库中的 `deploy/argus-server.service` 拷贝到服务器，然后：

```bash
sudo install -m 0644 argus-server.service /etc/systemd/system/argus-server.service
sudo systemctl daemon-reload
sudo systemctl enable argus-server     # 先不 start，首次部署会启动
```

单元中与 shipit 相关的两条硬性要求（`shipit check` 会校验，部署重启前也会再
校验一次）：

- `User=shipit`：shipit 拒绝重启任何其他用户运行的单元，这是它的安全模型。
- `WorkingDirectory=/srv/app/argus/server/current`：必须正好是项目目录下的
  `current`。

其余加固项（`ProtectSystem=strict`、空 `CapabilityBoundingSet` 等）按需保留。
注意 `ReadWritePaths` 必须指向 **`shared/` 目录**，因为 SQLite 会在同目录创建
`-wal` / `-shm` 文件。

sudoers（`sudo visudo -f /etc/sudoers.d/shipit`）：

```
shipit ALL=(root) NOPASSWD: /usr/bin/systemctl restart argus-server
```

### 6. 校验并首次部署

```bash
sudo shipit check
```

检查通过后：

```bash
sudo shipit deploy argus-server    # 没有 tag 参数时部署最新 release
sudo shipit status
```

health 检查会访问 `http://127.0.0.1:8095/healthz`；该端点不需要登录，只确认进程
存活且数据库可达。失败的 release 会被 shipit 自动回滚到 `previous`。

### 7. web 与 nginx

web 是独立的 `vite` 项目，部署后文件位于 `/srv/app/argus/web/current`。
nginx 直接指向这个符号链接；切换是原子的，**不需要 reload nginx**。

把仓库中的 `deploy/nginx.conf` 拷到服务器，然后：

```bash
sudo install -m 0644 nginx.conf /etc/nginx/sites-available/argus
sudo ln -sf /etc/nginx/sites-available/argus /etc/nginx/sites-enabled/argus
sudo usermod -aG shipit www-data       # Debian/Ubuntu；其他发行版换成 nginx 用户
sudo nginx -t && sudo systemctl reload nginx
```

说明：

- 项目目录是 `0750 shipit:shipit`，nginx 用户必须在 `shipit` 组里才能读。
- `/api/` 反代到 `127.0.0.1:8095`，并关闭 `proxy_buffering` 以支持 SSE。
- 先部署 server，再部署 web：

  ```bash
  sudo shipit deploy argus-server
  sudo shipit deploy argus-web
  ```

### 8. 首次登录

打开 `http://<服务器IP>/`（或你的域名）：没有管理员时先进入初始化页创建账号；
之后用该账号登录。若使用了 env 播种，登录成功后请清空 `.env` 中的密码：

```bash
sudo -u shipit sh -c 'printf "" > /srv/app/argus/server/shared/.env'
```

然后在 Web 界面「Settings」页创建节点，得到 node-id 与 token（token 只显示
一次），用于下面的 agent 安装。

### 9. 无域名 / TLS

- **内网或单机自用**：nginx 监听 80，直接访问 `http://<IP>/`；agent 推送可用
  `http://<IP>/api/...` 或直连 `http://<IP>:8095`。此时 `cookie_secure` 设为
  `false`。
- **跨公网**：不要把明文 HTTP 暴露公网。让 nginx 做 TLS 终结（或用 Caddy），
  server 只监听本机，`cookie_secure: true`。
- agent 的推送客户端**会校验 TLS 证书**，不要用自签证书直连；要么走受信证书，
  要么在内网使用明文 HTTP。

---

## 二、安装 agent

agent 保持原有方式：由 server 的 Web 界面生成安装命令，在目标机器执行。Settings →
Nodes 列表每行的 **Install** 按钮可随时为该节点重新生成安装命令：token 只保存
SHA-256 哈希、无法再次显示，所以按钮实际执行的是**轮换** token——旧 token 立即
失效，目标机上的 agent 会一直报 401，直到用新命令重装配置为止。

```bash
curl -fsSL https://raw.githubusercontent.com/terobox/argus/main/deploy/install-agent.sh \
  | sudo bash -s -- --server http://<服务器IP>:8095 --node-id <node-id> --token <token>
```

也可先下载再执行：

```bash
curl -fsSL https://raw.githubusercontent.com/terobox/argus/main/deploy/install-agent.sh -o install-agent.sh
sudo bash install-agent.sh --server http://<服务器IP>:8095 --node-id <node-id> --token <token>
```

选项：

- `--public-ip` 手动指定公网 IP（默认不探测，保护隐私）；
- `--version vX.Y.Z` 安装指定版本；
- `--uninstall` 卸载；
- token 也可用环境变量 `ARGUS_NODE_TOKEN` 传入，比命令行更不易泄露。

`--server` 可以指向 nginx 的地址（`https://argus.example.com`）或直连
`http://<IP>:8095`，取决于你的网络拓扑。

### 做了什么

1. 下载并 SHA-256 校验 `argus-agent_linux_{arch}`，安装到 `/usr/local/bin/argus-agent`。
2. 创建非登录系统用户 `argus-agent`。
3. 写配置 `/etc/argus/agent.yaml`（`0640 root:argus-agent`）。
4. 部署并启动加固的 `argus-agent.service`，立即开始采集并推送。

---

## 三、工作原理

- **推送模型**：agent 定时把数据 POST 到 server 的上报端点，server 不主动连接
  agent。这天然适配 NAT / 防火墙后的主机。
- **采集周期**：默认每秒采一次快指标（CPU/内存/网络/磁盘 IO），每 30s 采慢指标
  （磁盘容量、登录用户数），每 15s 推送一批；内存缓冲 600 条（约 10 分钟），
  网络抖动时本地暂存、自动重试。
- **存储分层**：server 端按 1s / 15s / 5m 三层时序降采样——原始 1s 数据保留
  1 小时，15s 保留 24 小时，5m 保留 30 天，控制 SQLite 体积。
- 指标语义（计数器是「开机以来累计值」，速率在读取时由差值推导）见
  [metrics.md](metrics.md)。

---

## 四、配置文件

### `/srv/app/argus/server/shared/server.yaml`

| 键 | 默认 | 说明 |
| --- | --- | --- |
| `listen_addr` | `:8095` | 监听地址 |
| `db_path` | `argus.db` | SQLite 文件路径（生产请写绝对路径） |
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

---

## 五、升级与回滚

### server 与 web

一次 `git tag vX.Y.Z && git push --tags` 会同时发布并部署两端：

```
CI 构建 → 发布 release → shipit 部署 argus-server → shipit 部署 argus-web
```

**顺序是 server 先、web 后**：新前端可能调用新 server 才有的 API；server 的 API
必须保持向后兼容（只增不删）。

手动操作（在服务器上，root 或 `sudo shipit ...`）：

```bash
sudo shipit deploy argus-server         # 部署最新 release
sudo shipit deploy argus-web v1.2.3     # 部署指定 tag
sudo shipit status                      # 当前 / 上一版本与服务状态
sudo shipit list argus-server           # 磁盘上的 release
sudo shipit rollback argus-server       # 回到 previous
sudo shipit rollback argus-web v1.2.2   # 回到指定版本
```

**回滚顺序与部署相反：先 web、后 server**。这样可以避免新前端先遇到被回滚掉的
新 API。

注意：回滚只回滚代码，不回滚数据。SQLite schema 迁移必须保持向后兼容（只新增
列/表，不删除、不改语义），否则 server 回滚后可能无法启动。

server 二进制不再提供 `update`；版本只由 shipit 管理。

### agent

agent 仍使用自更新（从 GitHub Releases 下载 `argus-agent_linux_{arch}`、校验
SHA-256、原子替换二进制），不会触碰配置：

```bash
sudo /usr/local/bin/argus-agent update
sudo systemctl restart argus-agent
```

也可以重跑 `install-agent.sh`（幂等：覆盖二进制、配置与 unit）。注意脚本结尾的
`systemctl enable --now` 对已在运行的服务不会重启，重装后要
`sudo systemctl restart argus-agent` 才会用上新二进制/新配置；仅升级二进制时用
上面的 `argus-agent update` 即可，不需要 token。

> agent 不提供自动/远程升级：它以加固的非 root 用户运行，不应替换 root 拥有的
> 二进制；升级一律由运维手动执行。

---

## 六、卸载

### server 与 web

shipit 只删除它自己创建的东西；项目目录与 systemd 单元需要手动处理：

```bash
sudo systemctl disable --now argus-server
sudo rm -f /etc/systemd/system/argus-server.service
sudo systemctl daemon-reload
sudo rm -f /etc/sudoers.d/shipit          # 若其中只有 argus-server 这一行

# SQLite 在 shared/data/ 下；如需保留历史数据，先备份再删项目目录
sudo cp /srv/app/argus/server/shared/data/argus.db /root/argus.db.bak   # 可选
sudo rm -rf /srv/app/argus
```

然后从 `/etc/shipit/shipit.yaml` 中移除两个 project。nginx 站点按需删除。

> shipit 只清理 `releases/` 下的旧版本，永远不会动 `shared/`；上面的 `rm -rf`
> 会连数据一起删除，执行前确认已备份或确实不再需要。

### agent

```bash
sudo bash install-agent.sh --uninstall
```

卸载会停止并删除服务、二进制与配置，但保留系统用户，避免误删历史数据。

---

## 七、安全设计

- **来源可信**：二进制只从 GitHub Releases 下载。shipit 校验 release 的
  `checksums.txt`；agent 安装脚本与自更新同样校验 SHA-256。脚本本身走 HTTPS。
- **版本与权限分离**：shipit 以 `shipit` 用户解包、切链接；项目服务必须以
  `User=shipit` 运行，shipit 在重启前会再次校验，防止被篡改为 root 服务。
  唯一的 root 能力是 sudoers 中精确到服务的 `systemctl restart`。
- **目录权限**：项目目录 `0750 shipit:shipit`；nginx 通过加入 `shipit` 组获得
  读权限。`shared/.env` 为 `0600`，nginx 即使在同一组也读不到。
- **systemd 加固**：server 单元启用 `NoNewPrivileges`、`ProtectSystem=strict`、
  `ProtectHome`、`PrivateTmp`、`PrivateDevices`、`ProtectKernelTunables`、
  `ProtectKernelModules`、`RestrictSUIDSGID`、`LockPersonality`、
  `RestrictNamespaces`、`MemoryDenyWriteExecute`、空 `CapabilityBoundingSet`、
  `RestrictAddressFamilies`（仅 TCP/UDP/Unix）、`SystemCallArchitectures=native`。
  只有 `ReadWritePaths` 指向的 `shared/` 可写。
- **健康检查**：`/healthz` 未认证、无敏感信息，只确认进程与数据库可用；它用于
  部署健康检查，不参与业务鉴权。
- **敏感信息**：节点 token 在 server 端只存 SHA-256 哈希；agent 配置 `0640`，
  仅 root 与该服务用户可读。若使用 env 播种，初始管理员密码放在 `shared/.env`
  （`0600`），首次登录后应清空；Web 初始化创建的管理员密码只以 bcrypt 哈希落库。

---

## 八、常见问题

- **浏览器直接访问 `:8095` 看到 welcome JSON**：这是预期行为。server 只提供
  API，不托管前端；页面走 nginx。
- **`shipit check` 报 unit 找不到 / User 不对 / WorkingDirectory 不对**：按
  `deploy/argus-server.service` 检查 `User=shipit` 与
  `WorkingDirectory=/srv/app/argus/server/current`，然后
  `systemctl daemon-reload`。
- **`sudo: a password is required`（shipit 部署时）**：sudoers 行缺失或与单元名
  不完全一致。
- **agent 连不上 server**：确认 `--server` 地址在 agent 所在网络可达；如果走
  nginx，确认 `/api/` 反代与 `proxy_buffering off` 已配置；如果直连，确认
  server 监听 `:8095` 且防火墙放行。
- **nginx 403 / 权限不足**：`sudo usermod -aG shipit www-data` 后重新登录或
  `systemctl reload nginx`；确认目录是 `0750 shipit:shipit`。
- **`MemoryDenyWriteExecute` 启动失败**：极老内核可能出现。去掉单元中该行后
  `systemctl daemon-reload && systemctl restart argus-server` 即可（纯 Go 二进制
  一般无此问题）。
- **改了 `.env` 密码不生效**：env 播种只在“首次启动且当前没有任何管理员”时
  创建，之后不再读取；管理员创建后可安全清空该文件。忘记密码见「4. 初始管理员」。
- **回滚后 server 起不来**：通常是数据库 schema 不兼容。检查迁移是否只做了新增；
  必要时从 `shared/data` 的备份恢复。

本地开发（MacBook + 远程主机上报）见 [local-dev.md](local-dev.md)。
