#!/usr/bin/env bash
#
# argus-agent 安装脚本（仅限 Ubuntu / systemd）。
#
# 从 GitHub 官方 releases 下载 argus-agent 二进制，SHA-256 校验后安装，
# 以非登录系统用户运行，并部署一个最大限度加固的 systemd 服务。
# 支持重复执行升级（幂等）与 --uninstall 卸载。
#
# 用法（非交互，可直接管道执行）：
#   curl -fsSL https://raw.githubusercontent.com/terobox/argus/main/deploy/install-agent.sh \
#     | sudo bash -s -- --server <url> --node-id <id> --token <token>
#
# 详细原理与配置说明见 docs/install.md。
set -euo pipefail

REPO="terobox/argus"
GITHUB="https://github.com"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/argus"
SERVICE_USER="argus-agent"
SERVICE="argus-agent.service"

SERVER=""
NODE_ID=""
TOKEN=""
PUBLIC_IP=""
VERSION=""
UNINSTALL=0

usage() {
  cat <<'EOF'
用法: curl -fsSL https://raw.githubusercontent.com/terobox/argus/main/deploy/install-agent.sh \
       | sudo bash -s -- [选项]

选项:
  --server <url>      argus 服务端地址（必填，如 http://192.168.1.10:8095）
  --node-id <id>      节点 ID（必填，与服务端创建节点时一致）
  --token <token>     节点 token（必填，仅服务端创建节点时显示一次）
  --public-ip <ip>    可选，手动指定对外公网 IP
  --version <vX.Y.Z>  可选，安装指定版本（默认安装最新版）
  --uninstall         卸载 argus-agent（停止服务并删除二进制/配置）
  -h, --help          显示帮助

token 也可用环境变量 ARGUS_NODE_TOKEN 传入（比命令行更不易泄露）。
EOF
}

die() { echo "argus-agent 安装: $*" >&2; exit 1; }

# --- 参数解析 ---
while [ $# -gt 0 ]; do
  case "$1" in
    --server)    SERVER="${2:-}";    shift 2 ;;
    --node-id)   NODE_ID="${2:-}";   shift 2 ;;
    --token)     TOKEN="${2:-}";     shift 2 ;;
    --public-ip) PUBLIC_IP="${2:-}"; shift 2 ;;
    --version)   VERSION="${2:-}";   shift 2 ;;
    --uninstall) UNINSTALL=1;        shift ;;
    -h|--help)   usage; exit 0 ;;
    *) die "未知参数: $1（--help 查看用法）" ;;
  esac
done

# token 也可通过环境变量传入（未指定 --token 时生效）。
if [ -z "$TOKEN" ] && [ -n "${ARGUS_NODE_TOKEN:-}" ]; then
  TOKEN="$ARGUS_NODE_TOKEN"
fi

# --- 卸载分支 ---
if [ "$UNINSTALL" = 1 ]; then
  systemctl disable --now "$SERVICE" >/dev/null 2>&1 || true
  rm -f "/etc/systemd/system/$SERVICE"
  systemctl daemon-reload
  rm -f "$INSTALL_DIR/argus-agent"
  rm -f "$CONFIG_DIR/agent.yaml"
  echo "已卸载 argus-agent（保留系统用户 $SERVICE_USER，可手动: userdel $SERVICE_USER）"
  exit 0
fi

# --- 前置检查 ---
[ "$(id -u)" = 0 ] || die "请以 root 运行（sudo）"
[ "$(uname -s)" = "Linux" ] || die "仅支持 Linux"
command -v curl >/dev/null 2>&1 || die "需要 curl"
command -v systemctl >/dev/null 2>&1 || die "需要 systemd（Ubuntu 20.04+）"
[ -n "$SERVER" ]  || die "缺少 --server <url>"
[ -n "$NODE_ID" ] || die "缺少 --node-id <id>"
[ -n "$TOKEN" ]   || die "缺少 --token <token>（或环境变量 ARGUS_NODE_TOKEN）"

# --- 架构与下载地址 ---
case "$(uname -m)" in
  x86_64|amd64)   ARCH=amd64 ;;
  aarch64|arm64)  ARCH=arm64 ;;
  *) die "不支持的架构: $(uname -m)（仅支持 amd64/arm64）" ;;
esac
ASSET="argus-agent_linux_${ARCH}"
if [ -n "$VERSION" ]; then
  URL_BASE="$GITHUB/$REPO/releases/download/$VERSION"
else
  URL_BASE="$GITHUB/$REPO/releases/latest/download"
fi

download() {
  curl -fsSL --retry 3 --retry-delay 2 -o "$2" "$1" || die "下载失败: $1"
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "==> 下载 argus-agent ($ARCH) ..."
download "$URL_BASE/$ASSET" "$TMP/$ASSET"
download "$URL_BASE/checksums.txt" "$TMP/checksums.txt"

echo "==> 校验 SHA-256 ..."
expected="$(tr -d '\r' < "$TMP/checksums.txt" | awk -v f="$ASSET" '$2==f || $2=="./"f {print $1; exit}')"
[ -n "$expected" ] || die "checksums.txt 中找不到 $ASSET 的校验值"
actual="$(sha256sum "$TMP/$ASSET" | awk '{print $1}')"
[ "$expected" = "$actual" ] || die "SHA-256 校验失败，二进制可能被篡改，已中止"

echo "==> 安装二进制到 $INSTALL_DIR ..."
install -m 0755 "$TMP/$ASSET" "$INSTALL_DIR/argus-agent"

# --- 系统用户 ---
if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER" \
    || die "创建系统用户 $SERVICE_USER 失败"
fi

# --- 配置文件 ---
mkdir -p "$CONFIG_DIR"
umask 077
{
  printf 'server_url: "%s"\n' "$SERVER"
  printf 'node_id: "%s"\n' "$NODE_ID"
  printf 'token: "%s"\n' "$TOKEN"
  [ -n "$PUBLIC_IP" ] && printf 'public_ip: "%s"\n' "$PUBLIC_IP"
} > "$CONFIG_DIR/agent.yaml"
chown root:"$SERVICE_USER" "$CONFIG_DIR/agent.yaml"
chmod 0640 "$CONFIG_DIR/agent.yaml"

# --- systemd 单元（加固） ---
cat > "/etc/systemd/system/$SERVICE" <<EOF
[Unit]
Description=argus agent (host metrics collector)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
ExecStart=$INSTALL_DIR/argus-agent -config $CONFIG_DIR/agent.yaml
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
RestrictNamespaces=true
MemoryDenyWriteExecute=true
CapabilityBoundingSet=
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
SystemCallArchitectures=native

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now "$SERVICE"
sleep 1
systemctl --no-pager --lines=0 status "$SERVICE" || true

echo ""
echo "✅ argus-agent 安装完成"
echo "   版本: $("$INSTALL_DIR/argus-agent" version)"
echo "   配置文件: $CONFIG_DIR/agent.yaml"
echo "   查看日志: journalctl -u $SERVICE -f"
