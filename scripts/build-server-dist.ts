#!/usr/bin/env bun
/**
 * 构建可解压直接运行的 Linux x64 单机包。
 *
 *   bun run dist:linux
 *
 * 产物：dist/open-ai-canvas-server-<version>-linux-x64.tar.gz
 * 包内含 Go 后端、构建后的前端、官方插件包、内嵌 Agent runtime 及其 Node 运行时。
 * Docker 部署仍由 Nginx 托管前端；这里通过 CANVAS_WEB_DIST_DIR 让后端直接托管前端。
 */

import {
  chmodSync,
  closeSync,
  cpSync,
  existsSync,
  mkdirSync,
  openSync,
  readFileSync,
  readdirSync,
  readSync,
  realpathSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs'
import { join, resolve } from 'node:path'

const ROOT = resolve(import.meta.dir, '..')
const DIST_DIR = join(ROOT, 'dist')
const OUT = join(DIST_DIR, 'server')
const WEB_DIR = join(ROOT, 'web')
const BACKEND_DIR = join(ROOT, 'backend')
const PI_DIR = join(BACKEND_DIR, 'agent-runtime', 'pi')
const VERSION = readFileSync(join(ROOT, 'VERSION'), 'utf8').trim()
const ARCHIVE = join(DIST_DIR, `open-ai-canvas-server-${VERSION}-linux-x64.tar.gz`)
const TOP_LEVEL = `open-ai-canvas-server-${VERSION}`

function step(title: string): void {
  console.log(`\n=== ${title} ===`)
}

function fail(message: string): never {
  console.error(`\n✗ ${message}`)
  process.exit(1)
}

function run(name: string, cmd: string[], options: { cwd?: string; env?: Record<string, string> } = {}): void {
  console.log(`  → ${name}`)
  const result = Bun.spawnSync({
    cmd,
    cwd: options.cwd ?? ROOT,
    stdout: 'inherit',
    stderr: 'inherit',
    env: { ...(process.env as Record<string, string>), ...(options.env ?? {}) },
  })
  if (result.exitCode !== 0) fail(`${name} 失败（exit ${result.exitCode}）`)
}

function requireExecutable(name: string): string {
  const found = Bun.which(name)
  if (!found) fail(`PATH 中找不到构建所需的 ${name}`)
  return found
}

function describeBinary(file: string): { format: 'elf' | 'other'; arch: string } {
  const fd = openSync(file, 'r')
  const head = Buffer.alloc(20)
  try {
    readSync(fd, head, 0, 20, 0)
  } finally {
    closeSync(fd)
  }
  if (head[0] === 0x7f && head[1] === 0x45 && head[2] === 0x4c && head[3] === 0x46) {
    const machine = head.readUInt16LE(18)
    return { format: 'elf', arch: machine === 0x3e ? 'x64' : machine === 0xb7 ? 'arm64' : `machine-0x${machine.toString(16)}` }
  }
  return { format: 'other', arch: 'unknown' }
}

function git(args: string[]): string {
  const result = Bun.spawnSync({ cmd: ['git', ...args], cwd: ROOT })
  return result.exitCode === 0 ? result.stdout.toString().trim() : ''
}

function dirSize(path: string): string {
  const result = Bun.spawnSync({ cmd: ['du', '-sh', path] })
  return result.stdout.toString().trim().split('\t')[0] ?? '?'
}

function requirePaths(entries: ReadonlyArray<readonly [path: string, label: string]>): void {
  const missing = entries.filter(([relative]) => !existsSync(join(OUT, relative))).map(([relative, label]) => `${relative} (${label})`)
  if (missing.length > 0) fail(`交付包缺少必要文件：\n  - ${missing.join('\n  - ')}`)
}

// ---------------------------------------------------------------------------
// 预检
// ---------------------------------------------------------------------------

step('预检')
if (process.platform !== 'linux' || process.arch !== 'x64') {
  fail(`该交付包只支持在 linux-x64 上构建，当前主机是 ${process.platform}-${process.arch}`)
}
if (!existsSync(join(ROOT, 'VERSION'))) fail('请在仓库根目录执行')
const goPath = requireExecutable('go')
const nodePath = realpathSync(requireExecutable('node'))
requireExecutable('tar')
const nodeBinary = describeBinary(nodePath)
if (nodeBinary.format !== 'elf' || nodeBinary.arch !== 'x64') {
  fail(`随包 Node 必须是 linux-x64 ELF，当前 ${nodePath} 是 ${nodeBinary.format}/${nodeBinary.arch}`)
}
console.log(`  ✓ go ${goPath}`)
console.log(`  ✓ node ${nodePath}`)
console.log(`  ✓ bun ${process.execPath}`)
console.log(`  ✓ 版本 ${VERSION}`)

const buildCommit = git(['rev-parse', '--short', 'HEAD']) || 'unknown'
const buildTime = new Date().toISOString()
const buildEnv = { CANVAS_BUILD_VERSION: VERSION, CANVAS_BUILD_COMMIT: buildCommit, CANVAS_BUILD_TIME: buildTime }

rmSync(OUT, { recursive: true, force: true })
rmSync(ARCHIVE, { force: true })
mkdirSync(join(OUT, 'bin'), { recursive: true })
mkdirSync(join(OUT, 'vendor', 'node', 'bin'), { recursive: true })

// ---------------------------------------------------------------------------
// 前端
// ---------------------------------------------------------------------------

step('构建前端')
if (!existsSync(join(WEB_DIR, 'node_modules'))) {
  run('安装前端依赖', ['bun', 'install', '--frozen-lockfile'], { cwd: WEB_DIR })
} else {
  console.log('  ✓ web/node_modules 已存在')
}
run('vite build', ['bun', '--bun', './node_modules/vite/bin/vite.js', 'build'], { cwd: WEB_DIR, env: buildEnv })
if (!existsSync(join(WEB_DIR, 'dist', 'index.html'))) fail('前端构建未生成 web/dist/index.html')
cpSync(join(WEB_DIR, 'dist'), join(OUT, 'web'), { recursive: true })
console.log('  ✓ web/')

// ---------------------------------------------------------------------------
// 后端
// ---------------------------------------------------------------------------

step('构建后端')
const ldflags = [
  '-s',
  '-w',
  `-X yingce/backend/internal/buildinfo.Version=${VERSION}`,
  `-X yingce/backend/internal/buildinfo.Commit=${buildCommit}`,
  `-X yingce/backend/internal/buildinfo.BuildTime=${buildTime}`,
].join(' ')
run('go build', ['go', 'build', '-trimpath', '-ldflags', ldflags, '-o', join(OUT, 'bin', 'yingce-backend'), './cmd/server'], {
  cwd: BACKEND_DIR,
  env: { CGO_ENABLED: '1', GOOS: 'linux', GOARCH: 'amd64' },
})
chmodSync(join(OUT, 'bin', 'yingce-backend'), 0o755)
console.log('  ✓ bin/yingce-backend')

// ---------------------------------------------------------------------------
// 内嵌 Agent runtime 与 Node
// ---------------------------------------------------------------------------

step('打包内嵌 Agent runtime')
if (!existsSync(join(PI_DIR, 'node_modules'))) {
  run('安装 Agent runtime 依赖', ['npm', 'ci', '--omit=dev', '--ignore-scripts'], { cwd: PI_DIR })
} else {
  console.log('  ✓ agent-runtime/pi/node_modules 已存在')
}
const piDest = join(OUT, 'backend', 'agent-runtime', 'pi')
mkdirSync(piDest, { recursive: true })
for (const file of ['package.json', 'package-lock.json', 'agent-runtime.mjs']) {
  cpSync(join(PI_DIR, file), join(piDest, file))
}
cpSync(join(PI_DIR, 'node_modules'), join(piDest, 'node_modules'), { recursive: true })
console.log(`  ✓ backend/agent-runtime/pi（${dirSize(piDest)}）`)

cpSync(nodePath, join(OUT, 'vendor', 'node', 'bin', 'node'))
chmodSync(join(OUT, 'vendor', 'node', 'bin', 'node'), 0o755)
console.log(`  ✓ vendor/node/bin/node（${describeBinary(join(OUT, 'vendor', 'node', 'bin', 'node')).arch}）`)

// ---------------------------------------------------------------------------
// 官方插件包与配置文件
// ---------------------------------------------------------------------------

step('打包官方插件包')
cpSync(join(ROOT, 'plugin-packages'), join(OUT, 'plugin-packages'), { recursive: true })
// 支付插件包含各平台二进制；交付包只面向 linux-x64，移除其余平台避免压缩包翻倍。
// 后端按 provider-<goos>-<goarch> 优先解析，保留 provider-linux-amd64 即可。
let trimmedProviders = 0
for (const pkg of readdirSync(join(OUT, 'plugin-packages'))) {
  const backendDir = join(OUT, 'plugin-packages', pkg, 'backend')
  if (!existsSync(backendDir)) continue
  for (const file of readdirSync(backendDir)) {
    if (!file.startsWith('provider') || file === 'provider-linux-amd64') continue
    rmSync(join(backendDir, file), { force: true })
    trimmedProviders += 1
  }
}
console.log(`  ✓ 移除非 linux-x64 支付插件二进制 ${trimmedProviders} 个`)
const pluginCount = readdirSync(join(OUT, 'plugin-packages')).filter((name) => name.endsWith('.yingce-plugin')).length
if (pluginCount === 0) fail('plugin-packages 下没有 .yingce-plugin 包')
console.log(`  ✓ plugin-packages（${pluginCount} 个插件包，${dirSize(join(OUT, 'plugin-packages'))}）`)

for (const file of ['VERSION', 'LICENSE', 'NOTICE']) {
  if (existsSync(join(ROOT, file))) cpSync(join(ROOT, file), join(OUT, file))
}

// ---------------------------------------------------------------------------
// 启动脚本
// ---------------------------------------------------------------------------

step('生成启动脚本')

const launcher = `#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# .env 只补充调用方未设置的变量；systemd、容器或命令行传入的环境变量优先。
if [ -f "$ROOT/.env" ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in ''|'#'*) continue ;; esac
    key="\${line%%=*}"
    value="\${line#*=}"
    case "$key" in *[!A-Za-z0-9_]*) continue ;; esac
    if ! printenv "$key" >/dev/null 2>&1; then
      export "$key=$value"
    fi
  done < "$ROOT/.env"
fi

export GIN_MODE="\${GIN_MODE:-release}"
export CANVAS_BACKEND_ADDR="\${CANVAS_BACKEND_ADDR:-:8080}"
export CANVAS_BACKEND_DATA_DIR="\${CANVAS_BACKEND_DATA_DIR:-$ROOT/data}"
export CANVAS_WEB_DIST_DIR="\${CANVAS_WEB_DIST_DIR:-$ROOT/web}"
export CANVAS_OFFICIAL_PLUGIN_DIR="\${CANVAS_OFFICIAL_PLUGIN_DIR:-$ROOT/plugin-packages}"
export CANVAS_PI_RUNTIME_DIR="\${CANVAS_PI_RUNTIME_DIR:-$ROOT/backend/agent-runtime/pi}"

# 随包 Node 供内嵌 Agent runtime 使用。
export PATH="$ROOT/vendor/node/bin:$PATH"

exec "$ROOT/bin/yingce-backend" "$@"
`
writeFileSync(join(OUT, 'bin', 'canvas-server'), launcher)

const startSh = `#!/bin/sh
set -eu
DIR="$(cd "$(dirname "$0")" && pwd)"
exec "$DIR/bin/canvas-server" "$@"
`
writeFileSync(join(OUT, 'start.sh'), startSh)

const installSh = `#!/bin/sh
set -eu

DIR="$(cd "$(dirname "$0")" && pwd)"

chmod +x "$DIR/bin/canvas-server" "$DIR/bin/yingce-backend" "$DIR/start.sh"
if [ -f "$DIR/vendor/node/bin/node" ]; then
  chmod +x "$DIR/vendor/node/bin/node"
fi
for provider in "$DIR"/plugin-packages/*/backend/provider*; do
  [ -f "$provider" ] && chmod +x "$provider"
done

if [ ! -f "$DIR/.env" ]; then
  cat > "$DIR/.env" <<'ENVFILE'
# 影策单机部署配置；修改后重新执行 start.sh 生效。
CANVAS_BACKEND_ADDR=:8080
# 对外可访问地址，用于 Agent 读取本地素材的短时签名链接；留空则本地存储无法签出链接。
CANVAS_PUBLIC_BASE_URL=
# 首次启动自动迁移 SQLite 结构；改用 PostgreSQL 时设为 false 并执行 migrate-schema。
CANVAS_AUTO_MIGRATE=true
CANVAS_REGISTRATION_ENABLED=false
CANVAS_ALLOW_PRIVATE_UPSTREAMS=false
CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS=
ENVFILE
  chmod 600 "$DIR/.env"
fi

mkdir -p "$DIR/data"
chmod 700 "$DIR/data"

if ! command -v ffmpeg >/dev/null 2>&1; then
  echo "提示：未找到 ffmpeg，视频抽帧与时间线渲染相关功能不可用。"
fi

echo "初始化完成。启动：$DIR/start.sh"
`
writeFileSync(join(OUT, 'install.sh'), installSh)

for (const script of [join(OUT, 'bin', 'canvas-server'), join(OUT, 'start.sh'), join(OUT, 'install.sh')]) {
  chmodSync(script, 0o755)
}
console.log('  ✓ bin/canvas-server、start.sh、install.sh')

// ---------------------------------------------------------------------------
// 校验与归档
// ---------------------------------------------------------------------------

step('校验交付包')
requirePaths([
  ['bin/canvas-server', '启动脚本'],
  ['bin/yingce-backend', '后端'],
  ['start.sh', '入口脚本'],
  ['install.sh', '初始化脚本'],
  ['web/index.html', '前端'],
  ['plugin-packages', '官方插件包'],
  ['backend/agent-runtime/pi/agent-runtime.mjs', 'Agent runtime'],
  ['backend/agent-runtime/pi/node_modules/@earendil-works/pi-coding-agent', 'Agent runtime 依赖'],
  ['vendor/node/bin/node', 'Node 运行时'],
  ['VERSION', '版本号'],
])
console.log('  ✓ 必要文件齐全')

step('生成压缩包')
const entries = readdirSync(OUT)
const tar = Bun.spawnSync({
  cmd: [
    'tar',
    '-czf',
    ARCHIVE,
    // 解压后只有一个带版本号的根目录，避免把 bin/ web/ 等散落到共享目录。
    '--transform',
    `flags=rh;s,^,${TOP_LEVEL}/,`,
    '--sort=name',
    '--owner=0',
    '--group=0',
    '--numeric-owner',
    '-C',
    OUT,
    ...entries,
  ],
  stdout: 'inherit',
  stderr: 'inherit',
  env: { ...(process.env as Record<string, string>), LD_LIBRARY_PATH: '' },
})
if (tar.exitCode !== 0) fail('tar 打包失败')

const digest = new Bun.CryptoHasher('sha256').update(readFileSync(ARCHIVE)).digest('hex')
writeFileSync(`${ARCHIVE}.sha256`, `${digest}  ${ARCHIVE.split('/').pop()}\n`)

console.log(`\n✓ ${ARCHIVE}`)
console.log(`  ${(statSync(ARCHIVE).size / 1024 / 1024).toFixed(0)} MB 压缩包 · ${dirSize(OUT)} 解压`)
console.log(`  sha256 ${digest}`)
console.log(`\n运行：`)
console.log(`  tar xzf ${ARCHIVE.split('/').pop()} && cd ${TOP_LEVEL}`)
console.log(`  ./install.sh && ./start.sh`)
