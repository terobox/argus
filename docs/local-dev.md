# 本地开发（MacBook + 远程主机主动上报）

这套流程让你在本地 MacBook 上同时运行 argus-server 与前端（Vite 热更新），
远程主机上的 argus-agent 主动把指标推送到你的 Mac。agent 采用「主动推送」模型，
因此只需要**远程主机能访问到 Mac 的 8095 端口**，无需在远程主机放行入站流量。

## 组成

| 进程 | 位置 | 端口 | 作用 |
| --- | --- | --- | --- |
| argus-server | 本机 Mac | 8095 | API + SSE 实时流 |
| Vite dev server | 本机 Mac | 8096 | 前端热更新，`/api` 代理到 8095 |
| argus-agent | 远程主机 | — | 采集指标并 `POST` 到 Mac 的 8095 |

浏览器只访问 `http://localhost:8096`。Vite 把 `/api`（含 SSE）代理到
`http://127.0.0.1:8095`，Cookie 因此保持同源，登录与 CSRF 行为和生产一致，
不需要配置 CORS。

## 1. 准备本机

需要 Go 1.26+ 与 pnpm 9+。前端依赖由你执行安装（构建/测试我可以跑）：

```bash
pnpm --dir web install
```

## 2. 本地 server 配置

在仓库根目录创建 `server.local.yaml`（已被 `.gitignore` 忽略）：

```yaml
listen_addr: ":8095"        # 监听所有网卡，远程主机才连得进来
db_path: "argus.local.db"   # 本地开发数据库，已被 .gitignore 忽略
web_dir: ""                 # 前端交给 Vite，server 不托管静态文件
cookie_secure: false        # 本地是 http://localhost
```

## 3. 启动

终端 A（server）：

```bash
ARGUS_ADMIN_USERNAME=admin ARGUS_ADMIN_PASSWORD='dev-password' \
  go run ./server/cmd/argus-server -config server.local.yaml
# 或：ARGUS_ADMIN_PASSWORD='dev-password' make run-server CONFIG=server.local.yaml
```

> 初始管理员只在「数据库里还没有任何管理员」时创建；之后修改环境变量不再生效。
> 想重置：停掉 server，删除 `argus.local.db*` 后再启动。

终端 B（前端）：

```bash
pnpm --dir web dev        # 等价于 make run-web
```

如果 server 不在 `127.0.0.1:8095`，用环境变量覆盖代理目标：

```bash
ARGUS_SERVER_URL=http://127.0.0.1:18095 pnpm --dir web dev
```

打开 `http://localhost:8096` 登录，在 Settings 页创建节点，记下 node-id 与 token
（token 只显示一次）。

## 4. 让远程主机连到你的 Mac

agent 需要访问 Mac 的 8095。以下三种方式任选其一。

### 方案 A：Tailscale（推荐，长期开发）

两台机器都安装 Tailscale 并登录同一 tailnet，在 Mac 上查看地址：

```bash
tailscale ip -4          # 例如 100.101.102.103
```

远程 agent 的 `server_url` 使用 `http://100.101.102.103:8095`。tailnet 内流量本身
加密，走明文 HTTP 即可。macOS 首次可能弹窗询问是否允许 `argus-server`（或其父进程）
接受入站连接，选择允许。

### 方案 B：SSH 反向隧道（临时，无需额外软件）

在** Mac **上主动连接远程主机，把远程的 8095 映射回 Mac 的 8095：

```bash
ssh -N -R 127.0.0.1:8095:127.0.0.1:8095 -o ServerAliveInterval=30 user@remote-host
```

远程 agent 的 `server_url` 使用 `http://127.0.0.1:8095`。隧道断开后重新执行即可。

### 方案 C：Cloudflare Tunnel（需要临时公网地址时）

```bash
cloudflared tunnel --url http://127.0.0.1:8095
```

命令会输出一个 `https://xxx.trycloudflare.com` 地址，远程 agent 直接使用它。
该地址是临时公网入口，会暴露管理界面，仅用于短期联调；管理员密码要足够强。

> 不建议在路由器上把 8095 直接映射到公网。

## 5. 在远程主机上运行 agent

### 方式 1：交叉编译后拷贝（跟随当前代码，推荐）

在 Mac 的仓库根目录：

```bash
GOOS=linux GOARCH=amd64 go build -o bin/argus-agent-linux ./agent/cmd/argus-agent
scp bin/argus-agent-linux user@remote-host:/tmp/argus-agent
```

在远程主机上写 `/etc/argus/agent.yaml`：

```yaml
server_url: "http://100.101.102.103:8095"   # 换成第 4 步得到的地址
node_id: "<创建节点时得到的 node-id>"
token: "<创建节点时得到的 token>"
```

运行：

```bash
sudo install -m 0755 /tmp/argus-agent /usr/local/bin/argus-agent
sudo /usr/local/bin/argus-agent -config /etc/argus/agent.yaml
# 需要常驻时，按 docs/install.md 中的 systemd 单元配置
```

### 方式 2：发布版安装脚本

如果 release 中已有可用版本，按 [install.md](install.md) 安装，并把 `--server`
指向第 4 步得到的地址。

## 6. 验证

- 打开 `http://localhost:8096`，Overview 应在约 15s 内出现远程节点，曲线实时滚动。
- DevTools → Network 中的 `stream` 请求应保持 pending（SSE 经过 Vite 代理正常）。
- 前端只连 8096；如果直接打开 `http://localhost:8095`，本地 `web_dir` 为空，
  只有 API，页面会是 404，这是预期行为。

## 常见问题

- **页面能开但接口 401/404**：确认 server 在 8095、Vite 代理配置已生效（改动
  `vite.config.ts` 后需重启 Vite）；不要直接访问 8095 当页面用。
- **远程节点一直不出现**：在远程主机执行
  `curl -v http://<地址>:8095/api/v1/ingest`（`POST` 端点，`curl` 会得到
  405/400 而非连接失败即可），确认网络可达；再检查 Tailscale/隧道状态与
  macOS 防火墙。
- **token 泄漏**：在 Settings 删除该节点后重建，并更新远程 agent 配置。
- **时间戳异常**：agent 使用远程主机的本地时间，server 不做时区换算；
  请确保远程主机开启 NTP。
