#!/usr/bin/env bash
#
# argus-server 安装脚本（仅限 Ubuntu / systemd）。
#
# 从 GitHub 官方 releases 下载 argus-server 二进制，SHA-256 校验后安装，
# 以非登录系统用户运行，并部署一个最大限度加固的 systemd 服务。
# 支持重复执行升级（幂等）与 --uninstall 卸载。
#
# 用法（交互式设置初始管理员密码，请先下载再运行）：
#   curl -fsSL https://raw.githubusercontent.com/terobox/argus/main/deploy/install-server.sh \
#     -o install-server.sh
#   sudo bash install-server.sh
#
# 自动化场景可用环境变量 ARGUS_ADMIN_USERNAME / ARGUS_ADMIN_PASSWORD 免交互。
# 详细原理与配置说明见 docs/install.md。
set -euo pipefail

REPO="terobox/argus"
GITHUB="https://github.com"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/argus"
SERVICE_USER="argus"
SERVICE="argus-server.service"

LISTEN=":8095"
DATA_DIR="/var/lib/argus"
ADMIN_USER="${ARGUS_ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${ARGUS_ADMIN_PASSWORD:-}"
VERSION=""
UNINSTALL=0

usage() {
  cat <<'EOF'
用法:
  1) 交互式（推荐）：
       curl -fsSL https://raw.githubusercontent.com/terobox/argus/main/deploy/install-server.sh -o install-server.sh
       sudo bash install-server.sh
  2) 自动化（免交互）：
       ARGUS_ADMIN_PASSWORD='...' sudo -E bash install-server.sh

选项:
  --listen <addr>       监听地址（默认 :8095）
  --data-dir <dir>      数据目录，存放 SQLite 数据库（默认 /var/lib/argus）
  --admin-username <u>  初始管理员用户名（默认 admin）
  --version <vX.Y.Z>    可选，安装指定版本（默认安装最新版）
  --uninstall           卸载 argus-server（停止服务并删除二进制/配置，保留数据目录）
  -h, --help            显示帮助

初始管理员仅在首次启动（且当前无任何管理员）时创建，之后可删除 server.env。
EOF
}

die() { echo "argus-server 安装: $*" >&2; exit 1; }

# --- 参数解析 ---
while [ $# -gt 0 ]; do
  case "$1" in
    --listen)         LISTEN="${2:-}";         shift 2 ;;
    --data-dir)       DATA_DIR="${2:-}";       shift 2 ;;
    --admin-username) ADMIN_USER="${2:-}";     shift 2 ;;
    --version)        VERSION="${2:-}";        shift 2 ;;
    --uninstall)      UNINSTALL=1;             shift ;;
    -h|--help)        usage; exit 0 ;;
    *) die "未知参数: $1（--help 查看用法）" ;;
  esac
done

# --- 卸载分支 ---
if [ "$UNINSTALL" = 1 ]; then
  systemctl disable --now "$SERVICE" >/dev/null 2>&1 || true
  rm -f "/etc/systemd/system/$SERVICE"
  systemctl daemon-reload
  rm -f "$INSTALL_DIR/argus-server"
  rm -f "$CONFIG_DIR/server.yaml"
  rm -f "$CONFIG_DIR/server.env"
  echo "已卸载 argus-server（保留数据目录 $DATA_DIR 与系统用户 $SERVICE_USER）"
  exit 0
fi

# --- 前置检查 ---
[ "$(id -u)" = 0 ] || die "请以 root 运行（sudo）"
[ "$(uname -s)" = "Linux" ] || die "仅支持 Linux"
command -v curl >/dev/null 2>&1 || die "需要 curl"
command -v systemctl >/dev/null 2>&1 || die "需要 systemd（Ubuntu 20.04+）"

# --- 初始管理员密码 ---
if [ -z "$ADMIN_PASSWORD" ]; then
  if [ -t 0 ]; then
    read -rsp "设置初始管理员密码（$ADMIN_USER）: " ADMIN_PASSWORD
    echo
  else
    die "缺少初始管理员密码：请用 ARGUS_ADMIN_PASSWORD 环境变量，或交互式运行（先下载再 sudo bash install-server.sh）"
  fi
fi
[ -n "$ADMIN_PASSWORD" ] || die "管理员密码不能为空"

# 单引号会破坏 systemd EnvironmentFile 解析，直接拒绝，避免埋坑。
case "$ADMIN_USER$ADMIN_PASSWORD" in
  *"'"*) die "用户名/密码不能包含单引号（'）" ;;
esac

# --- 架构与下载地址 ---
case "$(uname -m)" in
  x86_64|amd64)   ARCH=amd64 ;;
  aarch64|arm64)  ARCH=arm64 ;;
  *) die "不支持的架构: $(uname -m)（仅支持 amd64/arm64）" ;;
esac
ASSET="argus-server_linux_${ARCH}"
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

echo "==> 下载 argus-server ($ARCH) ..."
download "$URL_BASE/$ASSET" "$TMP/$ASSET"
download "$URL_BASE/checksums.txt" "$TMP/checksums.txt"

echo "==> 校验 SHA-256 ..."
expected="$(tr -d '\r' < "$TMP/checksums.txt" | awk -v f="$ASSET" '$2==f || $2=="./"f {print $1; exit}')"
[ -n "$expected" ] || die "checksums.txt 中找不到 $ASSET 的校验值"
actual="$(sha256sum "$TMP/$ASSET" | awk '{print $1}')"
[ "$expected" = "$actual" ] || die "SHA-256 校验失败，二进制可能被篡改，已中止"

echo "==> 安装二进制到 $INSTALL_DIR ..."
install -m 0755 "$TMP/$ASSET" "$INSTALL_DIR/argus-server"

# --- 系统用户与数据目录 ---
if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER" \
    || die "创建系统用户 $SERVICE_USER 失败"
fi
mkdir -p "$DATA_DIR"
chown "$SERVICE_USER":"$SERVICE_USER" "$DATA_DIR"

# --- 配置文件 ---
mkdir -p "$CONFIG_DIR"
umask 077
cat > "$CONFIG_DIR/server.yaml" <<EOF
# argus server 配置，字段说明见 docs/install.md。
# 未列出的键使用内置默认值。

listen_addr: "$LISTEN"
db_path: "$DATA_DIR/argus.db"

# 通过 HTTPS 提供服务时改为 true，登录 Cookie 才会加 Secure 标记。
cookie_secure: false

# 告警默认关闭。启用示例（去掉下面注释并填写 ntfy 地址）：
# alert:
#   enabled: true
#   webhook_url: "https://ntfy.sh/your-topic"
EOF
chown root:"$SERVICE_USER" "$CONFIG_DIR/server.yaml"
chmod 0640 "$CONFIG_DIR/server.yaml"

# --- 初始管理员（仅首次启动生效） ---
{
  printf "ARGUS_ADMIN_USERNAME='%s'\n" "$ADMIN_USER"
  printf "ARGUS_ADMIN_PASSWORD='%s'\n" "$ADMIN_PASSWORD"
} > "$CONFIG_DIR/server.env"
chown root:"$SERVICE_USER" "$CONFIG_DIR/server.env"
chmod 0640 "$CONFIG_DIR/server.env"

# --- systemd 单元（加固） ---
cat > "/etc/systemd/system/$SERVICE" <<EOF
[Unit]
Description=argus server (central monitoring)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
EnvironmentFile=-$CONFIG_DIR/server.env
ExecStart=$INSTALL_DIR/argus-server -config $CONFIG_DIR/server.yaml
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
ReadWritePaths=$DATA_DIR
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
echo "✅ argus-server 安装完成"
echo "   版本: $("$INSTALL_DIR/argus-server" version)"
echo "   监听: $LISTEN"
echo "   数据目录: $DATA_DIR"
echo "   配置文件: $CONFIG_DIR/server.yaml"
echo "   已用 $ADMIN_USER 创建初始管理员；之后可删除 $CONFIG_DIR/server.env"
echo "   查看日志: journalctl -u $SERVICE -f"
