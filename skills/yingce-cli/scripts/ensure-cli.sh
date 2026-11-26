#!/usr/bin/env bash
set -euo pipefail

MIN_VERSION="${YINGCE_CLI_MIN_VERSION:-0.1.0}"
BASE_URL="${YINGCE_CLI_BASE_URL:-https://yingce.tv/cli}"
CACHE_DIR="${YINGCE_CLI_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/yingce/cli/current}"
INSTALL=0

usage() {
  cat <<'EOF'
用法：ensure-cli.sh [--install] [--base-url <url>] [--prefix <dir>]

检测 yingce 是否存在且版本满足要求，输出 {ready, cli_path, version, min_version}。

参数：
  --install            缺失或版本过低时安装到自己的缓存目录
  --base-url <url>     发布目录，默认 https://yingce.tv/cli
  --prefix <dir>       缓存目录，默认 ~/.cache/yingce/cli/current

环境变量：
  YINGCE_CLI_MIN_VERSION  最低版本，默认 0.1.0
  YINGCE_CLI_BASE_URL     发布目录
  YINGCE_CLI_CACHE_DIR    缓存目录

不会写入 Agent、Claude Code、Cursor、Codex 或其它工具的目录。
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --install)
      INSTALL=1
      shift
      ;;
    --base-url)
      BASE_URL="${2:?缺少 --base-url 的值}"
      shift 2
      ;;
    --prefix)
      CACHE_DIR="${2:?缺少 --prefix 的值}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "未知参数：$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) BINARY_NAME="yingce.exe" ;;
  *) BINARY_NAME="yingce" ;;
esac

resolve_path() {
  local target="$1" dir
  dir="$(cd "$(dirname "$target")" && pwd -P)"
  printf '%s/%s' "$dir" "$(basename "$target")"
}

cli_version() {
  "$1" version --json 2>/dev/null | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1
}

version_ge() {
  awk -v a="$1" -v b="$2" 'BEGIN {
    sub(/^v/, "", a); sub(/^v/, "", b);
    sub(/[-+].*/, "", a); sub(/[-+].*/, "", b);
    n = split(a, A, "."); m = split(b, B, ".");
    for (i = 1; i <= n || i <= m; i++) {
      x = (i <= n ? A[i] + 0 : 0);
      y = (i <= m ? B[i] + 0 : 0);
      if (x > y) exit 0;
      if (x < y) exit 1;
    }
    exit 0;
  }'
}

candidate=""
candidate_version=""
for path in "$(command -v yingce 2>/dev/null || true)" "$CACHE_DIR/$BINARY_NAME"; do
  [ -n "$path" ] || continue
  [ -x "$path" ] || continue
  version="$(cli_version "$path")"
  [ -n "$version" ] || continue
  if version_ge "$version" "$MIN_VERSION"; then
    candidate="$path"
    candidate_version="$version"
    break
  fi
done

if [ -z "$candidate" ]; then
  if [ "$INSTALL" -ne 1 ]; then
    printf '{"ready":false,"cli_path":null,"version":null,"min_version":"%s"}\n' "$MIN_VERSION"
    exit 1
  fi
  mkdir -p "$CACHE_DIR"
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT
  installer="$tmp_dir/install.sh"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${BASE_URL%/}/install.sh" -o "$installer"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$installer" "${BASE_URL%/}/install.sh"
  else
    echo "需要 curl 或 wget 下载安装脚本" >&2
    exit 1
  fi
  # 安装脚本的提示写入 stderr，保证本脚本 stdout 只有 JSON。
  YINGCE_CLI_BASE_URL="$BASE_URL" bash "$installer" --prefix "$CACHE_DIR" >&2
  candidate="$CACHE_DIR/$BINARY_NAME"
  candidate_version="$(cli_version "$candidate")"
  if [ -z "$candidate_version" ] || ! version_ge "$candidate_version" "$MIN_VERSION"; then
    printf '{"ready":false,"cli_path":null,"version":null,"min_version":"%s"}\n' "$MIN_VERSION"
    exit 1
  fi
fi

printf '{"ready":true,"cli_path":"%s","version":"%s","min_version":"%s"}\n' "$(resolve_path "$candidate")" "$candidate_version" "$MIN_VERSION"
