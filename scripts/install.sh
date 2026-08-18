#!/usr/bin/env bash
# cm (cmd-mgr) 快速安装脚本
# 用法: curl -fsSL https://raw.githubusercontent.com/HenC49/cmd-mgr/main/scripts/install.sh | bash
# 可选环境变量:
#   CM_VERSION=v0.1.0        指定版本（带 v 前缀），默认 latest
#   CM_INSTALL_DIR=/path     指定安装目录，默认 /usr/local/bin（不可写且无 sudo 时退回 ~/.local/bin）
#   CM_SKIP_INTEGRATE=1      跳过 shell 集成写入
set -euo pipefail

REPO="HenC49/cmd-mgr"
BIN="cm"

msg()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m==> 警告:\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m==> 错误:\033[0m %s\n' "$*" >&2; exit 1; }

# ---------- 平台检测 ----------
case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux)  os=linux ;;
  *) die "不支持的系统 $(uname -s)。Windows 请在 PowerShell 执行: irm https://raw.githubusercontent.com/${REPO}/main/scripts/install.ps1 | iex" ;;
esac
case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "不支持的架构 $(uname -m)" ;;
esac

asset="cm-${os}-${arch}.tar.gz"
base="${CM_BASE_URL:-https://github.com/${REPO}/releases}"
if [ -n "${CM_VERSION:-}" ]; then
  url="${base}/download/${CM_VERSION}/${asset}"
else
  url="${base}/latest/download/${asset}"
fi

# ---------- 下载 ----------
command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || die "需要 curl 或 wget"
fetch() {
  if command -v curl >/dev/null 2>&1; then curl -fsSL "$1"; else wget -qO- "$1"; fi
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

msg "下载 ${url}"
fetch "$url" > "$tmp/${asset}" || die "下载失败（请确认 Release 已发布且包含 ${asset}）"

# sha256 校验（checksums.txt 不存在或不含该条目时跳过）
if fetch "${url%/*}/checksums.txt" > "$tmp/checksums.txt" 2>/dev/null \
   && grep -q " ${asset}\$" "$tmp/checksums.txt"; then
  want="$(grep " ${asset}\$" "$tmp/checksums.txt" | awk '{print $1}')"
  if command -v sha256sum >/dev/null 2>&1; then
    got="$(sha256sum "$tmp/${asset}" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    got="$(shasum -a 256 "$tmp/${asset}" | awk '{print $1}')"
  else
    got=""
  fi
  if [ -n "$got" ]; then
    [ "$got" = "$want" ] || die "sha256 校验失败（got ${got}, want ${want}）"
    msg "sha256 校验通过"
  fi
fi

tar -xzf "$tmp/${asset}" -C "$tmp"
[ -f "$tmp/${BIN}" ] || die "压缩包中没有找到 ${BIN}"

# ---------- 安装 ----------
install_to() { install -m 0755 "$tmp/${BIN}" "$1/${BIN}"; }

if [ -n "${CM_INSTALL_DIR:-}" ]; then
  mkdir -p "${CM_INSTALL_DIR}"
  install_to "${CM_INSTALL_DIR}"
  dest="${CM_INSTALL_DIR}"
elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
  install_to /usr/local/bin
  dest=/usr/local/bin
elif command -v sudo >/dev/null 2>&1; then
  msg "安装到 /usr/local/bin 需要提权，可能要求输入密码"
  sudo mkdir -p /usr/local/bin
  sudo install -m 0755 "$tmp/${BIN}" "/usr/local/bin/${BIN}"
  dest=/usr/local/bin
else
  dest="$HOME/.local/bin"
  mkdir -p "$dest"
  install_to "$dest"
fi
installed="${dest}/${BIN}"
msg "已安装到 ${installed}（$("${installed}" --version 2>/dev/null || echo dev)）"

# ---------- PATH ----------
case ":${PATH}:" in
  *":${dest}:"*) ;;
  *)
    warn "${dest} 不在当前 PATH 中"
    case "$(basename "${SHELL:-/bin/sh}")" in
      zsh) rc="$HOME/.zshrc" ;;
      *)   rc="$HOME/.bashrc" ;;
    esac
    if ! grep -qs "${dest}" "$rc"; then
      printf '\n# cm (cmd-mgr) 安装脚本添加\nexport PATH="%s:$PATH"\n' "${dest}" >> "$rc"
      msg "已把 ${dest} 写入 ${rc}（重启终端生效）"
    fi
    ;;
esac

# ---------- shell 集成 ----------
if [ -z "${CM_SKIP_INTEGRATE:-}" ]; then
  "${installed}" init --install \
    || warn "shell 集成写入失败，可稍后手动执行: ${installed} init --install"
fi

msg "安装完成。新开终端后直接运行 cm 即可（或先 source 对应 rc 文件）"
