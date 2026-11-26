#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST="${YINGCE_CLI_OUTPUT_DIR:-$ROOT/dist}"
ENSURE_SCRIPT="${YINGCE_CLI_ENSURE_SCRIPT:-$ROOT/../skills/yingce-cli/scripts/ensure-cli.sh}"
VERSION=""
TARGET="${YINGCE_CLI_PUBLISH_TARGET:-}"
RUN_TESTS=1

usage() {
  cat <<'EOF'
用法：release.sh [版本号] [--output <目录>] [--target <rsync目标>] [--skip-tests]

版本号缺省时读取 cli/VERSION。

示例：
  ./release.sh
  ./release.sh 0.1.1
  ./release.sh --output /out/cli --skip-tests
  YINGCE_CLI_PUBLISH_TARGET=user@host:/srv/yingce/cli ./release.sh

输出结构：
  <output>/version.json   最新版本指针，安装脚本默认读取
  <output>/install.sh     稳定安装入口
  <output>/<版本号>/      二进制、SHA256SUMS、version.json、SKILL.md、install.sh

环境变量：
  YINGCE_CLI_OUTPUT_DIR       输出目录，默认 cli/dist
  YINGCE_CLI_PUBLISH_TARGET   rsync 目标；留空只生成文件
  YINGCE_CLI_COMMIT           提交号；缺省读取 git，容器构建时显式传入
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --output)
      DIST="${2:?缺少 --output 的值}"
      shift 2
      ;;
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
  VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION" 2>/dev/null || true)"
fi
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

COMMIT="${YINGCE_CLI_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
CONTRACT="$(go run ./cmd/yingce version --json | sed -n 's/.*"contract":"\([^"]*\)".*/\1/p')"
if [ -z "$CONTRACT" ]; then
  echo "无法读取 CLI 契约版本" >&2
  exit 1
fi
if [ ! -f "$ENSURE_SCRIPT" ]; then
  echo "缺少 ensure-cli.sh：$ENSURE_SCRIPT" >&2
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
mkdir -p "$OUT/scripts"
cp "$ENSURE_SCRIPT" "$OUT/scripts/ensure-cli.sh"
chmod +x "$OUT/scripts/ensure-cli.sh"

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

# 根目录只放版本指针、安装入口和最新 SKILL.md；二进制只保留在各版本目录，避免重复占用体积。
cp "$OUT/version.json" "$DIST/version.json"
cp install.sh "$DIST/install.sh"
cp internal/skill/SKILL.md "$DIST/SKILL.md"
mkdir -p "$DIST/scripts"
cp "$OUT/scripts/ensure-cli.sh" "$DIST/scripts/ensure-cli.sh"

echo
echo "已生成 $OUT"
cat "$OUT/SHA256SUMS"
echo
echo "安装：curl -fsSL https://yingce.tv/cli/install.sh | bash"
echo "固定版本：curl -fsSL https://yingce.tv/cli/install.sh | bash -s -- --version $VERSION"

if [ -n "$TARGET" ]; then
  if ! command -v rsync >/dev/null 2>&1; then
    echo "发布需要 rsync" >&2
    exit 1
  fi
  echo
  echo "发布到 $TARGET"
  rsync -av --delete "$OUT/" "$TARGET/$VERSION/"
  rsync -av "$DIST/version.json" "$TARGET/version.json"
  rsync -av "$DIST/install.sh" "$TARGET/install.sh"
  rsync -av "$DIST/SKILL.md" "$TARGET/SKILL.md"
  rsync -av --delete "$DIST/scripts/" "$TARGET/scripts/"
  echo "已发布 $VERSION"
else
  echo
  echo "未设置发布目标，只生成了本地文件。上传示例："
  echo "  rsync -av --delete dist/$VERSION/ <user>@<host>:/srv/yingce/cli/$VERSION/"
  echo "  rsync -av dist/version.json <user>@<host>:/srv/yingce/cli/version.json"
  echo "  rsync -av dist/install.sh <user>@<host>:/srv/yingce/cli/install.sh"
  echo "  rsync -av dist/SKILL.md <user>@<host>:/srv/yingce/cli/SKILL.md"
  echo "  rsync -av --delete dist/scripts/ <user>@<host>:/srv/yingce/cli/scripts/"
fi
