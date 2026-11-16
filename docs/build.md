# 构建与发版

argus 由三个可独立部署的部分组成：

- **argus-server**（Go）：中央服务端，接收上报、提供 API 与 SSE。由 shipit
  作为 `go` 项目管理版本。
- **argus-agent**（Go）：主机采集端，主动推送到 server。由安装脚本与自更新
  管理版本（多机、非项目化）。
- **web**（React + Vite）：管理界面。构建产物 `web/dist` 由 nginx 直接提供，
  由 shipit 作为 `vite` 项目管理版本，与 server 同 tag 发布。

## 目录结构

```
server/
  cmd/argus-server/        # main.go（仅 version 子命令，无 update）
  api/                     # HTTP API、SSE、/healthz（不托管前端）
  auth/                    # 登录会话、CSRF、节点 token
  alert/                   # 告警评估与通知
  storage/                 # SQLite、降采样、保留策略
  config/                  # server 配置；config.example.yaml 为提交的示例
agent/
  cmd/argus-agent/         # main.go、update.go（自更新保留）
  buffer/                  # 内存缓冲与重试
  collect/                 # /proc 采集
  push/                    # 上报客户端
  config/                  # agent 配置；config.example.yaml 为提交的示例
pkg/                       # 仅存放 server 与 agent 共用的公共包
  config/                  # Duration、YAML Load
  metrics/                 # 指标数据结构
  protocol/                # 上报协议、端点常量
  selfupdate/              # 自更新（仅 agent 使用）
web/                       # 独立前端（不内嵌进 server）
scripts/package.sh         # 生成 release 资产（shipit + agent 命名）
deploy/                    # systemd 单元、nginx 示例、agent 安装脚本
```

`server/**` 与 `agent/**` 按归属扁平组织，不做跨目录依赖；两边共用的代码只放
`pkg/**`。目录边界由约定保证（不使用 Go 的 `internal/` 限制）。

## 依赖

| 工具 | 版本 | 说明 |
| --- | --- | --- |
| Go | 1.26+（见 `go.mod`） | 构建两个二进制 |
| pnpm | 9+ | 前端（`pnpm-lock.yaml` 为 lockfile v9） |

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

## 本地开发

前端由 Vite（:8096）提供，并把 `/api` 代理到 :8095，前后端完全分离；
server 只提供 API，不托管任何静态文件。详见 [local-dev.md](local-dev.md)。

生产环境同样分离：nginx 提供 `web/current`，`/api` 反代到 server。见
[install.md](install.md)。

## 发版

打 tag `vX.Y.Z` 并推送后，`.github/workflows/release.yml` 会：

1. 安装前端依赖并构建 `web/dist`；
2. 跑 `go test -race ./...`；
3. 调用 `scripts/package.sh` 生成全部 release 资产；
4. 用 `gh release` 发布到 GitHub Releases；
5. 依次调用 shipit 部署 `argus-server`、`argus-web`（server 先、web 后）。

### Release 资产

| 资产 | 内容 | 消费方 |
| --- | --- | --- |
| `argus-server-<tag>-linux-amd64.tar.gz` | 根目录 `argus-server` | shipit（`type: go`） |
| `argus-server-<tag>-linux-arm64.tar.gz` | 同上 | shipit |
| `argus-web-<tag>-any.tar.gz` | 根目录 `index.html` + `assets/` | shipit（`type: vite`） |
| `argus-agent_linux_amd64` | 裸二进制 | `install-agent.sh`、`argus-agent update` |
| `argus-agent_linux_arm64` | 裸二进制 | 同上 |
| `checksums.txt` | 上述全部资产的 SHA-256 | shipit 校验、agent 安装/自更新校验 |

> agent 的资产名必须保持 `argus-agent_linux_<arch>`：现有安装脚本与自更新
> 直接按这个名字下载。shipit 只读取它需要的那一条 checksum，多余条目无害。

### 本地打包（不发布）

```bash
make package                 # 当前 HEAD 正好是 tag 时使用该 tag
make package TAG=v1.2.3      # 或显式指定
```

产物在 `dist/`，可以直接检查内容：

```bash
tar -tzf dist/argus-server-v1.2.3-linux-amd64.tar.gz
tar -tzf dist/argus-web-v1.2.3-any.tar.gz
```

> shipit 的 `go` 类型要求包内根目录是可执行文件 `argus-server`；`vite` 类型要求
> 根目录有 `index.html`。`scripts/package.sh` 已按这两个约定打包。

### 首次接入 shipit 的注意点

旧版本发布的是裸二进制 `argus-server_linux_amd64`，shipit 无法消费。**第一次由
shipit 部署前，必须先用新的发布流程产出一个包含 tar.gz 资产的 release**
（即推送本改动之后的第一个 tag）。
