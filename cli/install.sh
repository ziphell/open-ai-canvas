#!/usr/bin/env bash
set -euo pipefail

PROGRAM="yingce"
BASE_URL="${YINGCE_CLI_BASE_URL:-https://yingce.tv/cli}"
VERSION="${YINGCE_CLI_VERSION:-latest}"
INSTALL_DIR="${YINGCE_CLI_INSTALL_DIR:-}"
UPDATE_PATH=0
DRY_RUN=0

usage() {
  cat <<'EOF'
用法：install.sh [--prefix <dir>] [--version <版本号>] [--update-path] [--dry-run]

参数：
  --prefix <dir>          安装目录，默认 ~/.local/bin（Windows 为 ~/bin）
  --version <版本号>      版本目录，默认 latest
  --update-path           把安装目录写入 shell 启动文件
  --dry-run               只打印将下载的版本、来源和目标，不下载

环境变量：
  YINGCE_CLI_BASE_URL     发布目录，默认 https://yingce.tv/cli
  YINGCE_CLI_VERSION      版本目录，默认 latest
  YINGCE_CLI_INSTALL_DIR  安装目录
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --prefix)
      INSTALL_DIR="${2:?缺少 --prefix 的值}"
      shift 2
      ;;
    --version)
      VERSION="${2:?缺少 --version 的值}"
      shift 2
      ;;
    --update-path)
      UPDATE_PATH=1
      shift
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "未知参数：$1" >&2
      usage
      exit 2
      ;;
  esac
done

os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Darwin) os_name="darwin" ;;
  Linux) os_name="linux" ;;
  MINGW*|MSYS*|CYGWIN*) os_name="windows" ;;
  *)
    echo "不支持的操作系统：$os" >&2
    exit 1
    ;;
esac

case "$arch" in
  x86_64|amd64) arch_name="amd64" ;;
  arm64|aarch64) arch_name="arm64" ;;
  *)
    echo "不支持的架构：$arch" >&2
    exit 1
    ;;
esac

if [ -z "$INSTALL_DIR" ]; then
  if [ "$os_name" = "windows" ]; then
    INSTALL_DIR="$HOME/bin"
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi

binary="${PROGRAM}_${os_name}_${arch_name}"
if [ "$os_name" = "windows" ]; then
  binary="${binary}.exe"
  target="${INSTALL_DIR}/${PROGRAM}.exe"
else
  target="${INSTALL_DIR}/${PROGRAM}"
fi

download() {
  url="$1"
  output="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$output"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$output" "$url"
    return
  fi
  echo "需要 curl 或 wget" >&2
  exit 1
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
    return
  fi
  if command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$1" | awk '{print $NF}'
    return
  fi
  echo "没有可用的 SHA-256 工具，拒绝安装" >&2
  exit 1
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

base="${BASE_URL%/}/${VERSION}"

if [ "$DRY_RUN" -eq 1 ]; then
  echo "dry-run: ${PROGRAM} ${VERSION} (${os_name}/${arch_name})"
  echo "  来源：${base}/${binary}"
  echo "  校验：${base}/SHA256SUMS 中的 ${binary}"
  echo "  目标：${target}"
  exit 0
fi

echo "下载 ${base}/${binary}"
download "${base}/${binary}" "${tmp_dir}/${binary}"
download "${base}/SHA256SUMS" "${tmp_dir}/SHA256SUMS"

expected="$(awk -v name="$binary" '$2 == name || $2 == "./" name {print $1}' "${tmp_dir}/SHA256SUMS" | head -n 1)"
if [ -z "$expected" ]; then
  echo "SHA256SUMS 中没有 ${binary}，拒绝安装" >&2
  exit 1
fi
actual="$(sha256 "${tmp_dir}/${binary}")"
if [ "$expected" != "$actual" ]; then
  echo "SHA256 校验失败：期望 ${expected}，实际 ${actual}" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
cp "${tmp_dir}/${binary}" "$target"
chmod +x "$target"
if [ "$os_name" = "darwin" ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$target" >/dev/null 2>&1 || true
fi

echo "已安装：$target"
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    if [ "$UPDATE_PATH" -eq 1 ]; then
      case "$(basename "${SHELL:-}")" in
        zsh) rc="${ZDOTDIR:-$HOME}/.zshrc" ;;
        bash) rc="$HOME/.bashrc" ;;
        *) rc="$HOME/.profile" ;;
      esac
      mkdir -p "$(dirname "$rc")"
      touch "$rc"
      if ! grep -Fqx "export PATH=\"${INSTALL_DIR}:\$PATH\"" "$rc"; then
        printf '\nexport PATH="%s:$PATH"\n' "$INSTALL_DIR" >>"$rc"
        echo "已把 ${INSTALL_DIR} 写入 ${rc}"
      fi
    else
      echo "提示：把 ${INSTALL_DIR} 加入 PATH，或重新运行并加 --update-path"
    fi
    ;;
esac

echo "下一步："
echo "  ${PROGRAM} --version"
echo "  ${PROGRAM} login"
