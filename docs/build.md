# 构建与发版

argus 由三个可独立部署的部分组成：

- **argus-server**（Go）：中央服务端，接收上报、提供 API 与 SSE。
- **argus-agent**（Go）：主机采集端，主动推送到 server。
- **web**（React + Vite）：管理界面。**不再内嵌进 server 二进制**，构建产物
  `web/dist` 单独提供（反向代理，或由 server 的 `web_dir` 托管）。

## 目录结构

```
server/
  cmd/argus-server/        # main.go、update.go
  api/                     # HTTP API、SSE、静态页面托管
  auth/                    # 登录会话、CSRF、节点 token
  alert/                   # 告警评估与通知
  storage/                 # SQLite、降采样、保留策略
  config/                  # server.yaml 配置
agent/
  cmd/argus-agent/         # main.go、update.go
  buffer/                  # 内存缓冲与重试
  collect/                 # /proc 采集
  push/                    # 上报客户端
  config/                  # agent.yaml 配置
pkg/                       # 仅存放 server 与 agent 共用的公共包
  config/                  # Duration、YAML Load
  metrics/                 # 指标数据结构
  protocol/                # 上报协议、端点常量
  selfupdate/              # 自更新
web/                       # 独立前端（不再有 embed.go）
```

`server/**` 与 `agent/**` 按归属扁平组织，不做跨目录依赖；两边共用的代码只放
`pkg/**`。目录边界由约定保证（不再使用 Go 的 `internal/` 限制）。

## 依赖

| 工具 | 版本 | 说明 |
| --- | --- | --- |
| Go | 1.26+（见 `go.mod`） | 构建两个二进制 |
| pnpm | 9+ | 前端（`pnpm-lock.yaml` 为 lockfile v9） |
| goreleaser | v2 | 发版 |

## 本地构建

```bash
pnpm --dir web install    # 安装前端依赖（一次即可，等价 make deps）
make web                  # 构建前端到 web/dist
make build                # 构建两个二进制到 bin/
make test                 # go test -race ./...
make lint                 # 前端 typecheck + lint
```

产物：

- `bin/argus-server`、`bin/argus-agent`
- `web/dist/`（前端静态文件）

## 前端与 server 的两种运行方式

1. **server 托管静态文件**：把 `web/dist` 放到目标机器，在 `server.yaml` 中设置
   `web_dir` 指向它（默认 `web/dist`，相对 server 进程的工作目录）。server 会以
   SPA fallback 方式提供页面。`web_dir: ""` 表示关闭，只提供 API。
2. **反向代理托管（生产推荐）**：由 Nginx/Caddy 提供 `web/dist` 静态文件，把
   `/api` 反代到 `127.0.0.1:8095`。注意 SSE 需要关闭代理缓冲
   （Nginx：`proxy_buffering off`）。

本地开发时前端由 Vite（:8096）提供，并把 `/api` 代理到 :8095，见
[local-dev.md](local-dev.md)。

## 发版

发版用 goreleaser：打 tag `vX.Y.Z` 并推送后，CI 会运行测试并产出
**Linux amd64 + arm64** 的 `argus-server` / `argus-agent` 两个二进制及
`checksums.txt`，发布到 GitHub Releases。

本地冒烟验证（不发布、不打 tag）：

```bash
make snapshot             # 等价 goreleaser release --snapshot --clean
```

goreleaser 只构建 Go 二进制；前端需要单独 `pnpm --dir web build` 并部署。
产物资产名形如 `argus-server_linux_amd64`、`argus-agent_linux_arm64`，与安装脚本、
agent 自更新（`agent/cmd/argus-agent/update.go`）保持一致。
