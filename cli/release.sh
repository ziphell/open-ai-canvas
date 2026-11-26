#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST="$ROOT/dist"
VERSION=""
TARGET="${YINGCE_CLI_PUBLISH_TARGET:-}"
RUN_TESTS=1

usage() {
  cat <<'EOF'
用法：release.sh <版本号> [--target <rsync目标>] [--skip-tests]

示例：
  ./release.sh 0.1.0
  YINGCE_CLI_PUBLISH_TARGET=user@host:/srv/yingce/cli ./release.sh 0.1.0

发布目录结构：
  <target>/<版本号>/   本次版本的全部文件
  <target>/latest/     指向最新版本的文件副本
  <target>/install.sh  稳定安装入口

环境变量：
  YINGCE_CLI_PUBLISH_TARGET   rsync 目标；留空只生成文件，不发布
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --target)
      TARGET="${2:?缺少 --target 的值}"
      shift 2
      ;;
    --skip-tests)
      RUN_TESTS=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      echo "未知参数：$1" >&2
      usage
      exit 2
      ;;
    *)
      [ -z "$VERSION" ] || {
        echo "只接受一个版本号" >&2
        exit 2
      }
      VERSION="$1"
      shift
      ;;
  esac
done

if [ -z "$VERSION" ]; then
  usage
  exit 2
fi
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "版本号应形如 0.1.0 或 0.2.0-rc.1：$VERSION" >&2
  exit 2
fi

cd "$ROOT"

if [ "$RUN_TESTS" -eq 1 ]; then
  unformatted="$(gofmt -l .)"
  if [ -n "$unformatted" ]; then
    printf '%s\n' "$unformatted" >&2
    echo "需要先运行 gofmt -w" >&2
    exit 1
  fi
  echo "检查：go vet"
  go vet ./...
  echo "检查：go test"
  go test -count=1 ./...
fi

COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
CONTRACT="$(go run ./cmd/yingce version --json | sed -n 's/.*"contract":"\([^"]*\)".*/\1/p')"
if [ -z "$CONTRACT" ]; then
  echo "无法读取 CLI 契约版本" >&2
  exit 1
fi

OUT="$DIST/$VERSION"
rm -rf "$OUT"
mkdir -p "$OUT"

for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  goos="${target%/*}"
  goarch="${target#*/}"
  asset="yingce_${goos}_${goarch}"
  if [ "$goos" = "windows" ]; then
    asset="${asset}.exe"
  fi
  echo "构建 $asset"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath \
    -ldflags="-s -w -X yingce/cli/internal/commands.Version=$VERSION -X yingce/cli/internal/commands.Commit=$COMMIT" \
    -o "$OUT/$asset" ./cmd/yingce
done

cp internal/skill/SKILL.md "$OUT/SKILL.md"
cp install.sh "$OUT/install.sh"

cat > "$OUT/version.json" <<EOF
{
  "version": "$VERSION",
  "contractVersion": "$CONTRACT",
  "commit": "$COMMIT",
  "buildTime": "$BUILD_TIME"
}
EOF

(
  cd "$OUT"
  if command -v sha256sum >/dev/null 2>&1; then
    LC_ALL=C sha256sum yingce_* > SHA256SUMS
  else
    LC_ALL=C shasum -a 256 yingce_* > SHA256SUMS
  fi
)

# latest 是最新版本的文件副本，安装脚本默认读取这里；升级不依赖任何代码托管平台。
rm -rf "$DIST/latest"
mkdir -p "$DIST/latest"
cp "$OUT"/* "$DIST/latest/"
cp install.sh "$DIST/install.sh"

echo
echo "已生成 $OUT"
cat "$OUT/SHA256SUMS"
echo
echo "默认安装：curl -fsSL https://yingce.tv/cli/install.sh | bash"
echo "固定版本：curl -fsSL https://yingce.tv/cli/install.sh | bash -s -- --version $VERSION"

if [ -n "$TARGET" ]; then
  if ! command -v rsync >/dev/null 2>&1; then
    echo "发布需要 rsync" >&2
    exit 1
  fi
  echo
  echo "发布到 $TARGET"
  rsync -av --delete "$OUT/" "$TARGET/$VERSION/"
  rsync -av --delete "$DIST/latest/" "$TARGET/latest/"
  rsync -av "$DIST/install.sh" "$TARGET/install.sh"
  echo "已发布 $VERSION"
else
  echo
  echo "未设置发布目标，只生成了本地文件。上传示例："
  echo "  rsync -av --delete dist/$VERSION/ <user>@<host>:/srv/yingce/cli/$VERSION/"
  echo "  rsync -av --delete dist/latest/ <user>@<host>:/srv/yingce/cli/latest/"
  echo "  rsync -av dist/install.sh <user>@<host>:/srv/yingce/cli/install.sh"
fi
