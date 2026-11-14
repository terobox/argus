# 构建与发版

argus 的 server 二进制**内嵌了前端**（`web/dist`），因此发版前必须先构建前端，
否则 server 能编译但没有任何页面。goreleaser 的 `before` 钩子会自动完成这一步。

## 依赖

| 工具 | 版本 | 说明 |
| --- | --- | --- |
| Go | 1.26+（见 `go.mod`） | 构建两个二进制 |
| pnpm | 9+ | 前端（`pnpm-lock.yaml` 为 lockfile v9） |
| goreleaser | v2 | 发版 |

## 本地开发构建

```bash
pnpm --dir web install    # 安装前端依赖（一次即可，等价 make deps）
make web                  # 构建前端到 web/dist
make build                # 构建两个二进制到 bin/（server 内嵌刚构建的前端）
make test                 # go test -race ./...
make lint                 # 前端 typecheck + lint
```

## 发版

发版用 goreleaser：打 tag `vX.Y.Z` 并推送后，CI（`.github/workflows/release.yml`）
会安装依赖、构建前端、跑测试，再用 goreleaser 产出 **Linux amd64 + arm64** 的
`argus-server` / `argus-agent` 两个二进制及 `checksums.txt`，并发布到 GitHub Releases。

本地冒烟验证（不发布、不打 tag）：

```bash
make snapshot             # 等价 goreleaser release --snapshot --clean
```

goreleaser 的 `before` 钩子会自动执行 `pnpm --dir web build`，因此 `make release` /
`make snapshot` 无需再手动构建前端。产物资产名形如
`argus-server_linux_amd64`、`argus-agent_linux_arm64`，与安装脚本、agent 自更新
（`cmd/agent/update.go`）保持一致。
