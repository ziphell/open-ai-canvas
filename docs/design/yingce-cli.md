# 影策 CLI 与 Agent Skill 设计

> 状态：CLI v0.1 与后端 M1 已实现（浏览器授权、Bearer、scope、任务幂等）。CLI 作为同一账号的另一入口，复用账号配额。
> 日期：2026-11-26。
> 定位：CLI 是给 Agent 用的调用面，不是 Web 的镜像。

配套文档：

- [yingce-cli-reference.md](yingce-cli-reference.md)：逐命令参数、输出、退出码和错误目录。
- [yingce-cli-backend.md](yingce-cli-backend.md)：token 表、Bearer 中间件、浏览器授权、scope 与幂等实现。
- [yingce-cli-skill.md](yingce-cli-skill.md)：随 CLI 分发的 `SKILL.md`。

## 1. 目标与边界

### 1.1 目标

- 让任意能执行 shell 的 Agent（Claude Code、Cursor、Codex、自建 harness）调用影策。
- 一个二进制完成读项目/画布、上传素材、报价、生成、查任务、写画布、驱动云端 Agent。
- 用 token 作用域和额度做预授权，不做逐次人工审批。
- 用 `SKILL.md` 教会模型命令、异步、成本和失败处理。

### 1.2 v1 不做

- 不做 Web 审批收件箱，不暴露 `approval_decide`。
- 不做付款、充值、管理员、用户管理、技能安装。
- 不提供 MCP 子命令；v1 用 CLI + `SKILL.md` 覆盖 Agent 接入，MCP 留作后续可选形态。
- 不承诺全量 API，只承诺本文列出的命令子集。

### 1.3 与内部 Agent 工具面的关系

对外契约独立维护，业务实现复用 HTTP API。

```
yingce CLI + SKILL.md
        │  独立命令、参数、错误、版本
        ▼
   HTTP API（白名单子集）
        ▼
service / app  ←── 内部 Agent 工具也走这里
```

禁止在 CLI 内复制画布校验、归属判断、计费准入；这些必须在 service 层完成。

### 1.4 实现位置

CLI 是独立 Go module，便于单独发布和安装，不 import `backend/internal`：

```
cli/
├── go.mod
├── cmd/yingce/main.go
├── internal/cli/       命令解析、输出、退出码
├── internal/client/    HTTP 客户端与信封解包
├── internal/auth/      浏览器授权、凭据存储
├── internal/upload/    分片上传
├── internal/skill/     内嵌 SKILL.md
├── SKILL.md
└── install.sh
```

后端改动仍在 `backend/internal/handler`、`internal/app`、`internal/auth`、`internal/model`、`internal/repository` 内完成。

## 2. 契约原则

1. 命令按「资源 + 动作」命名：`canvas get`、`task wait`、`model quote`。
2. 只承诺白名单端点，未列出的接口不保证兼容。
3. 成功与失败都返回结构化 JSON；`--json` 是稳定接口，人类可读输出只是展示层。
4. 写操作必须带并发守卫或幂等键：`baseRevision`、`mutationId`、`idempotencyKey`。
5. 授权在发放 token 时完成；运行时只校验 scope、上限和资源归属。
6. 不能确认结果时失败闭合，不猜、不自动重试。

## 3. 安装与升级

### 3.1 一键安装

```bash
curl -fsSL https://<host>/cli/install.sh | bash
```

安装脚本必须：

- 识别 `darwin/linux/windows` × `amd64/arm64`。
- 先读取 `/cli/version.json` 得到当前版本，再从 `/cli/<版本号>/` 下载二进制和 `SHA256SUMS`；同一目录附带 `SKILL.md`、`version.json` 和 `install.sh`。
- 校验 SHA256，**失败即停**；不接受只告警继续。
- 默认安装到 `~/.local/bin`，Windows 为 `~/bin`。
- 不自动修改 shell 启动文件；打印 PATH 指引，用户显式传 `--update-path` 才写入。
- 不修改任何其他工具的文件（对比即梦脚本会往 `TOOLS.md` 追加规则，不采纳）。
- macOS 清理 quarantine；提供 `--prefix`、`--version`、`--dry-run`。

### 3.2 版本

| 命令               | 行为                             |
| ---------------- | ------------------------------ |
| `yingce version` | 输出 CLI 版本、契约版本、提交号；升级用安装脚本覆盖安装 |
| `yingce doctor`  | 检查 base URL、认证、服务端健康、契约兼容性     |

`GET /api/cli/manifest` 返回最低服务端版本、契约版本和废弃命令列表，`doctor` 用它判断兼容性。

### 3.3 发布

CLI 安装包进入前端构建流程，由 web 静态目录提供；发布和升级不依赖 GitHub、GitHub Actions 或 GitHub Release。

- 版本号写在 `cli/VERSION`。版本号与提交号通过 `-ldflags` 注入 `commands.Version` / `commands.Commit`。
- Docker 的 `cli-build` 阶段交叉编译 6 个平台，产物注入 `web/public/cli/`，随 Vite 构建进入 web 镜像；`nginx.conf` 用 `/cli/` 直接提供静态文件。
- 发布目录：`/cli/version.json` 是版本指针，`/cli/install.sh` 是稳定安装入口，`/cli/SKILL.md` 是最新技能说明，`/cli/scripts/ensure-cli.sh` 供技能先装、CLI 后装时自举，`/cli/<版本号>/` 保存该版本全部文件；不复制 `latest` 目录，避免二进制重复占用体积。
- `canvas-web-entrypoint.sh` 把镜像内的 `/cli/` 同步进 `web-data` 卷；web 容器升级后旧版本目录保留，固定版本安装仍可回退。
- 需要只更新 CLI、不重建 web 镜像时，可运行 `cli/release.sh` 生成产物并通过 rsync 发布到 web 静态根；用法见脚本帮助。

## 4. 认证

### 4.1 三种方式

| 方式        | 场景         | 行为                                         |
| --------- | ---------- | ------------------------------------------ |
| 浏览器授权（默认） | 本机交互式      | `yingce login` 监听 loopback 并拉起浏览器，用户在授权页确认 |
| 粘贴 token  | CI、容器、无浏览器 | `yingce login --token-stdin`，从 stdin 读取    |
| 环境变量      | 自动化        | `YINGCE_TOKEN`，不落盘                         |

不接受网页 Cookie、账号密码、浏览器导出 JSON。

### 4.2 浏览器授权流程

采用 OAuth 2.1 授权码 + PKCE 的 loopback 模式（RFC 8252），复用现有登录 Cookie：

```
CLI                                后端                          浏览器
 │ 生成 state + PKCE verifier        │                              │
 │ 监听 127.0.0.1:<随机端口>          │                              │
 │ 打开 /api/auth/cli/authorize?...  │                              │
 │──────────────────────────────────>│ 校验 Cookie 会话              │
 │                                   │<── 用户确认 scope 与额度 ─────│
 │<── 302 loopback?code&state ───────│ 写入一次性 code（60 秒）      │
 │ POST /auth/cli/token              │                              │
 │  code + verifier + redirect_uri   │                              │
 │──────────────────────────────────>│ 校验 PKCE、state、一次性      │
 │<── accessToken + scopes ──────────│                              │
```

约束：

- 只允许 `127.0.0.1` / `::1` 回环地址，随机端口；`redirect_uri` 必须精确匹配。
- PKCE 使用 S256，`state` 必填；不接受缺少参数的授权请求。
- 授权码 60 秒有效，只能兑换一次；兑换与状态更新在同一事务完成。
- token 只在 loopback 交换时返回一次，服务端只存哈希。
- 用户拒绝时回调 `error=access_denied`，CLI 输出退出码 3。
- 无浏览器环境用 `--no-browser` 打印授权 URL，或改用 `--token-stdin`。

### 4.3 token 模型

```json
{
  "id": "tok_...",
  "name": "claude-code-laptop",
  "scopes": ["read", "canvas:write", "generate:image", "generate:video"],
  "expiresAt": "2027-01-26T00:00:00Z",
  "lastUsedAt": "2026-11-26T10:00:00Z",
  "revokedAt": null
}
```

| scope                                                  | 允许                        |
| ------------------------------------------------------ | ------------------------- |
| `read`                                                 | 项目、画布、任务、模型、素材、额度、openapi |
| `asset:upload`                                         | 上传素材                      |
| `canvas:write`                                         | 写画布                       |
| `generate:image` / `generate:video` / `generate:audio` | 对应类型生成                    |
| `task:write`                                           | 取消任务                      |
| `agent:run`                                            | 创建/取消云端 Agent 运行          |

`projectIds` 和 `models` 为空表示不额外限制。

### 4.4 存储与撤销

- 凭据文件 `~/.config/yingce/credentials.json`，权限 `0600`；目录 `0700`。
- 环境变量优先于文件；`--token` 仅用于调试，文档不推荐。
- `yingce logout` 调用撤销接口并删除本地文件。
- 用户在 Web 撤销后，下一次请求返回 401；CLI 不得自动重新登录。

## 5. 命令面

### 5.1 辅助命令

| 命令                            | 端点                                                               | 说明                       |
| ----------------------------- | ---------------------------------------------------------------- | ------------------------ |
| `yingce version`              | 本地                                                               | 版本信息                     |
| `yingce doctor [--json]`      | `GET /health/live`、`GET /health/ready`、`GET /agent/capabilities` | 诊断                       |
| `yingce login [--no-browser]` | 浏览器授权三个端点（新增）                                                    | 登录                       |
| `yingce login --token-stdin`  | 无                                                                | 粘贴 token                 |
| `yingce logout`               | `DELETE /auth/tokens/current`（新增）                                | 撤销当前 token               |
| `yingce whoami`               | `GET /auth/session`（需兼容 Bearer）                                  | 当前用户与 scope              |
| `yingce skill`                | 本地                                                               | 输出 SKILL.md 原文，由调用方保存或注入 |

### 5.2 核心命令

| #  | 命令                         | 端点                                                               | 必需 scope     | 状态       |
| -- | -------------------------- | ---------------------------------------------------------------- | ------------ | -------- |
| 1  | `yingce project list`      | `GET /projects`                                                  | read         | 已有       |
| 2  | `yingce project get <id>`  | `GET /projects/:id`、`/overview`                                  | read         | 已有       |
| 3  | `yingce canvas list`       | `GET /canvas-projects`                                           | read         | 已有       |
| 4  | `yingce canvas get <id>`   | `GET /canvas-projects/:id`                                       | read         | 已有       |
| 5  | `yingce canvas apply <id>` | `PATCH /canvas-projects/:id/operations`                          | canvas:write | 已有       |
| 6  | `yingce task list`         | `GET /tasks`                                                     | read         | 已有       |
| 7  | `yingce task get <id>`     | `GET /tasks/:id`                                                 | read         | 已有       |
| 8  | `yingce task wait <id>`    | `GET /tasks/:id` 轮询                                              | read         | 已有       |
| 9  | `yingce task cancel <id>`  | `POST /tasks/:id/cancel`                                         | task:write   | 已有       |
| 10 | `yingce model list`        | `GET /model-catalog`                                             | read         | 已有       |
| 11 | `yingce model quote`       | `POST /models/:id/quote`（逻辑模型）、`POST /model-catalog/quote`（渠道模型） | read         | 已有       |
| 12 | `yingce asset list`        | `GET /projects/:id/assets`                                       | read         | 已有       |
| 13 | `yingce asset upload`      | `POST /resources`（小文件）、`POST /resources/uploads`（分片）             | asset:upload | 已有       |
| 14 | `yingce generate`          | `POST /tasks`                                                    | generate:\*  | 已有，需补幂等键 |
| 15 | `yingce credit`            | `GET /wallet`                                                    | read         | 已有       |
| 16 | `yingce run create`        | `POST /agent/runs`                                               | agent:run    | 已有       |
| 17 | `yingce run get <id>`      | `GET /agent/runs/:id`                                            | read         | 已有       |
| 18 | `yingce run events <id>`   | `GET /agent/runs/:id/events`                                     | read         | 已有       |
| 19 | `yingce run cancel <id>`   | `POST /agent/runs/:id/cancel`                                    | agent:run    | 已有       |
| 20 | `yingce openapi`           | `GET /openapi.yaml`                                              | read         | 已有       |

`run create` 的 `permissionMode` 只允许 `read_only` 和 `auto`。`request_approval` 不对外开放，因为外部没有审批面。

`task list --recent` 由 CLI 在本地按创建时间排序，复用现有 `pageSize` 和 `projectId` 参数，不新增服务端参数。

## 6. 输出与退出码

### 6.1 `--json`

成功：

```json
{"ok": true, "data": {}, "meta": {"requestId": "req_...", "durationMs": 123}}
```

失败：

```json
{
  "ok": false,
  "error": {"code": 40301, "reason": "quota_exceeded", "message": "积分不足"},
  "meta": {"requestId": "req_..."}
}
```

- stdout 只输出结果；进度、日志、提示写 stderr。
- 默认人类可读；`--json` 下不得混入颜色或进度。
- 大字段用 `--out <file>` 落盘，stdout 只回路径和摘要。

### 6.2 退出码

| 退出码 | 含义                   |
| --- | -------------------- |
| 0   | 成功                   |
| 2   | 参数错误                 |
| 3   | 未认证 / token 失效       |
| 4   | 权限或 scope 不足         |
| 5   | 资源不存在                |
| 6   | 冲突（revision、幂等键、状态机） |
| 7   | 限流                   |
| 8   | 额度不足                 |
| 9   | 服务端或上游错误             |
| 10  | 超时（不代表失败）            |
| 11  | 结果不确定，需要核对           |

`reason` 映射为退出码，不解析 `msg`。例如 `agent_conversation_stale_parent` → 6，`quota_exceeded` → 8。

## 7. 异步、幂等与不确定状态

### 7.1 提交语义

```bash
yingce generate --mode video --model <id> --prompt "..." --duration 5 --poll 0
```

- 默认 `--poll 0`：只提交，返回 `taskId`、`status`、`quote`。
- `--poll N`：最多等待 N 秒；超时返回当前状态，退出码 0，附 `pollTimeout: true`。
- 需要阻塞到底时使用 `yingce task wait <id> --max-wait 10m`。
- `--download-dir` 在终态时下载产物，返回本地路径。

### 7.2 幂等

- `POST /tasks` 接受 `X-Idempotency-Key`；缺省时 CLI 生成并先打印，便于恢复。
- `canvas apply` 用 `mutationId` + `baseRevision`。
- `run create` 必须带 `idempotencyKey`。
- 同一幂等键不同请求体返回 409。

### 7.3 不确定状态

提交超时或连接中断时：

1. 不要自动重试。
2. 用 `yingce task list --recent` 或 `yingce task get <taskId>` 核对。
3. 仍无法确认时返回退出码 11，提示用户到任务中心核对。
4. 上游已接受的生成任务不能声称已取消。

## 8. 本地文件与素材

```bash
yingce generate --mode image --prompt "..." \
  --ref ./character.png --ref ./scene.jpg
```

- CLI 接收本地路径，自动完成上传并替换为 `resourceId`。
- 小文件走 `POST /resources`；大于 8 MiB 走分片上传。
- 上传携带 SHA-256；服务端按「用户 + 哈希」去重，返回已有资源（需后端补充）。
- 日志和 `--json` 只输出 `resourceId`，不输出 base64 或带签名的 URL。
- 也可以用 `--ref-resource <id>` 复用已上传素材。

## 9. 画布写入

```bash
yingce canvas get <id> --out canvas.json
yingce canvas apply <id> --ops ops.json --base-revision <rev> --mutation-id <uuid>
```

- `canvas get` 必须返回 `revision` 和节点摘要；大画布用 `--out`。
- `canvas apply` 必须带 `baseRevision` 和 `mutationId`；冲突返回 409。
- 冲突后只能重新读取、重新生成操作，不得盲目重试同一批。
- `--dry-run` 在本地校验操作并打印规范化结果，不调用写接口。
- 删除、锁定、跨用户引用由服务端拒绝；CLI 不自行放宽。

## 10. Agent 集成

### 10.1 `yingce skill`

```bash
yingce skill
yingce skill --json
```

- 把内置的 `SKILL.md` 原样输出到 stdout；`--json` 时返回 `{"ok":true,"data":{"content":"..."}}`。
- CLI 不写 Claude Code、Cursor、Codex 或其他工具的目录，也不修改用户配置；保存位置由宿主 Agent 或用户决定。
- 服务端同时提供 `/cli/SKILL.md` 与 `/cli/<版本号>/SKILL.md`，供 Agent 直接读取或人工下载。

### 10.2 Skill 优先分发

除了先装 CLI，也支持像可灵一样先装 skill，再由 skill 引导安装和授权：

```bash
npx skills add https://github.com/ddcat-ai/open-ai-canvas/tree/main/skills/yingce-cli
```

- 仓库里的 `skills/yingce-cli/SKILL.md` 是分发入口；skills 工具把它安装到宿主自己的技能目录，不属于 CLI 写别人的目录。
- skill 内容自带「检测 CLI → 安装 → 浏览器授权 → 使用命令」的完整步骤，因此可以反过来先有 skill、后有 CLI。
- `skills/yingce-cli/scripts/ensure-cli.sh` 负责自举：先找 PATH，再找自己的缓存 `~/.cache/yingce/cli/current`，校验最低版本；默认只报告 `ready`，显式加 `--install` 才安装到自己的缓存目录，不写 Agent 目录。
- skill 与 CLI 分别升级：重新运行 `npx skills add ...` 更新 skill，重新运行安装脚本更新 CLI；`yingce skill` 输出当前 CLI 内置的同版本内容，可用于比对。
- `cli/internal/skill/SKILL.md` 与 `skills/yingce-cli/SKILL.md` 由测试保证一致。

### 10.3 为什么 v1 不做 MCP

- 覆盖面：能跑 shell 的 Agent 都能用 CLI；MCP 只是其中一部分客户端的额外通道。
- 维护面：MCP 要单独维护工具 schema、错误映射、长任务和版本协商，收益不足以进入 v1。
- 授权面：MCP 不解决审批；没有审批工具时，它和 CLI 一样只是调用方。
- 已有 SKILL.md 能教会模型命令、报价、异步和失败处理，足够覆盖主要流程。

MCP 留作后续可选形态，不进入 v1 契约。

如果以后新增 MCP 子命令，认证必须**自动跟随 CLI 凭据**：读取同一个 `~/.config/yingce/credentials.json` 或 `YINGCE_TOKEN`，不引入第二套登录。非交互进程不能自己完成浏览器授权；token 失效时返回结构化错误，提示用户或 Agent 执行 `yingce login`（或 `yingce login --no-browser` 打印 URL），而不是在 JSON-RPC 里弹登录流程。

## 11. 安全与限额

- 除 localhost 外必须 HTTPS；`--insecure` 仅用于本地调试。
- token 不进 URL、不进日志、不进错误信息；错误输出统一脱敏。
- 凭据文件 `0600`；CI 只用环境变量。
- 服务端在创建任务、写画布、上传、创建 Agent 运行时强制 scope；积分、并发、存储沿用账号级配额。
- 所有写操作记录 `token_id`、动作、对象、费用、结果。
- 撤销 token 后立即失效；CLI 不缓存授权结果。
- SSRF、私网上游、CORS 等既有边界不变，CLI 不提供绕过参数。

## 12. 后端缺口

| 缺口           | 内容                                                                               | 优先级 |
| ------------ | -------------------------------------------------------------------------------- | --- |
| Bearer 鉴权    | `api_tokens` 表、哈希校验、`lastUsedAt`、撤销                                              | P0  |
| 浏览器授权        | `GET/POST /api/auth/cli/authorize`、`POST /api/auth/cli/token`、`cli_auth_codes` 表 | P0  |
| token 管理     | Web 页面 + `GET/POST/DELETE /auth/tokens`                                          | P0  |
| scope 强制     | 按路由与任务模式校验                                                                       | P0  |
| 任务幂等         | `POST /tasks` 接受 `X-Idempotency-Key`；已实现                                         | P1  |
| 上传去重         | 按用户 + SHA-256 复用资源                                                               | P1  |
| CLI manifest | `GET /cli/manifest` 返回契约、最低版本与废弃命令；`doctor` 已接入                                  | 已实现 |
| OpenAPI 子集   | 只文档化白名单端点                                                                        | P2  |

## 13. 落地顺序与验收

### 阶段

1. **M1 认证基座**：token 表、Bearer 中间件、浏览器授权、`login/logout/whoami/doctor/version`。
2. **M2 只读与生成**：project/canvas/task/model/asset/credit、quote、generate、`task wait`。
3. **M3 写入与集成**：`canvas apply`、`skill`、SKILL.md、安装脚本。
4. **M4 加固**：幂等、上传去重、审计、文档、限流。

### 验收

- 干净 macOS/Linux 上 `curl | bash` 安装成功，SHA256 不符时停止。
- 授权码过期、用户拒绝、重复兑换都有明确错误；state 或 PKCE 不匹配直接拒绝。
- 撤销 token 后下一次请求返回 401，且 CLI 不自动重登。
- 缺 scope 返回退出码 4；额度不足返回 8；冲突返回 6。
- 提交超时不会产生重复任务；无法确认时返回 11。
- `canvas apply` 冲突不产生部分写入。
- `--json` 输出稳定，stdout 无多余内容。
- SKILL.md 能引导模型完成「读画布 → 报价 → 生成 → 等待 → 下载」全流程。
