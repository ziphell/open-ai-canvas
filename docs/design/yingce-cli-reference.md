# 影策 CLI 命令参考

> 状态：CLI v0.1 已实现（`cli/`），后端鉴权待接入。本文与 `cli/` 代码保持同步。
> 上游设计见 [yingce-cli.md](yingce-cli.md)。

## 0. 全局约定

### 0.1 调用形式

```bash
yingce <资源> <动作> [参数]
```

- 所有命令支持 `--json`；`--json` 输出是稳定接口。
- 未知参数、未知子命令一律报错，不忽略。
- 命令名和参数使用小写短横线；不使用缩写别名。

### 0.2 全局参数

| 参数 | 环境变量 | 说明 |
| --- | --- | --- |
| `--base-url <url>` | `YINGCE_BASE_URL` | 服务端地址，默认读配置 |
| `--token <token>` | `YINGCE_TOKEN` | 仅调试；优先度低于环境变量 |
| `--config-dir <dir>` | `YINGCE_CONFIG_DIR` | 默认 `~/.config/yingce` |
| `--json` | `YINGCE_JSON=1` | 机器可读输出 |
| `--quiet` | — | 只输出结果，不输出进度 |
| `--no-color` | `NO_COLOR` | 关闭颜色 |
| `--timeout <duration>` | — | 单次请求超时，默认 `30s` |
| `--insecure` | — | 仅 localhost 可用，跳过 TLS 校验 |
| `--debug` | `YINGCE_DEBUG=1` | 输出脱敏后的 HTTP 日志到 stderr |

### 0.3 配置文件

`~/.config/yingce/config.json`，权限 `0600`：

```json
{
  "baseUrl": "https://yingce.example.com",
  "defaultProjectId": "proj_xxx"
}
```

`~/.config/yingce/credentials.json`，权限 `0600`：

```json
{
  "token": "yc_...",
  "tokenId": "tok_...",
  "userId": "user_xxx",
  "scopes": ["read", "generate:image"],
  "expiresAt": "2027-01-26T00:00:00Z"
}
```

- 目录权限 `0700`。
- token 只写入一次，不写日志。
- `YINGCE_TOKEN` 优先于文件；`--token` 优先于环境变量，仅用于排查。

### 0.4 输出

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

- stdout 只输出结果；进度、提示写 stderr。
- 大结果用 `--out`；stdout 只保留路径和摘要。

### 0.5 退出码

| 码 | 含义 |
| --- | --- |
| 0 | 成功 |
| 2 | 参数或用法错误 |
| 3 | 未认证、token 失效或已撤销 |
| 4 | scope 或权限不足 |
| 5 | 资源不存在 |
| 6 | 冲突（revision、幂等键、状态机） |
| 7 | 限流 |
| 8 | 额度不足 |
| 9 | 服务端或上游错误 |
| 10 | 超时（不代表失败） |
| 11 | 结果不确定，需要核对 |

## 1. 认证

### `yingce login`

```bash
yingce login
yingce login --no-browser
yingce login --token-stdin
```

| 参数 | 说明 |
| --- | --- |
| `--no-browser` | 不自动打开浏览器，只打印授权 URL |
| `--token-stdin` | 从 stdin 读取 token，不落 shell history |
| `--name <name>` | token 名称，写入服务端，便于撤销 |
| `--port <n>` | 指定 loopback 端口；默认随机 |
| `--port <n>` | 指定 loopback 端口；默认随机 |

行为：

1. 生成 `state` 和 PKCE verifier，监听 `127.0.0.1:<随机端口>`。
2. 打开 `/api/auth/cli/authorize?redirect_uri=...&state=...&code_challenge=...&name=...`。
3. 用户在浏览器确认 scope 与额度，服务端 302 回 loopback 并带一次性 `code`。
4. CLI 校验 `state`，调用 `POST /auth/cli/token` 提交 `code`、`verifier` 和 `redirect_uri`。
5. 写 `credentials.json`，输出 `tokenId`、`scopes`、`expiresAt`。

失败：

- 用户拒绝：`error=access_denied`，退出码 3。
- 授权码过期、重复兑换、PKCE 或 state 不匹配：退出码 3，提示重新登录。
- 端口被占用：自动换端口重试，最多 3 次。

`--token-stdin` 直接校验 `GET /auth/session`，通过后落盘。

### `yingce logout`

撤销当前 token 并删除本地凭据。服务端失败时仍删除本地凭据，并返回警告。

### `yingce whoami`

```bash
yingce whoami --json
```

映射 `GET /auth/session`。输出用户、scope、token 过期时间和绑定的额度。

### `yingce doctor`

```bash
yingce doctor --json
```

检查：

- `GET /health/live`、`GET /health/ready`。
- 认证是否有效。
- `GET /agent/capabilities` 是否可达。
- CLI 契约版本与服务端 `GET /cli/manifest` 是否兼容。

输出每项 `ok` / `warn` / `fail`，任一 `fail` 时退出码 9。

### `yingce version`

```bash
yingce version --json
```

输出 `version`、`contract`、`commit`；发布构建由流水线注入版本与提交号，本地构建为 `0.1.0-dev`。

v0.1 不内置自更新；升级时重新运行安装脚本覆盖二进制。

## 2. 项目与画布

### `yingce project list`

| 参数 | 说明 |
| --- | --- |
| `--query <text>` | 名称过滤 |
| `--limit <n>` | 默认 20，最大 100 |
| `--cursor <c>` | 分页游标 |

### `yingce project get <id>`

```bash
yingce project get proj_xxx --with-overview --json
```

`--with-overview` 额外请求 `GET /projects/:id/overview`。

### `yingce canvas list`

| 参数 | 说明 |
| --- | --- |
| `--project <id>` | 限定项目 |
| `--limit <n>` | 默认 20 |

### `yingce canvas get <id>`

```bash
yingce canvas get canvas_xxx --out canvas.json --json
```

| 参数 | 说明 |
| --- | --- |
| `--out <file>` | 写完整画布，stdout 只回摘要 |
| `--nodes-only` | 只输出节点和连线，不含会话等字段 |
| `--include-timeline` | 包含时间轴字段 |

输出摘要至少包含 `id`、`revision`、`nodeCount`、`connectionCount`。

### `yingce canvas apply <id>`

```bash
yingce canvas apply canvas_xxx \
  --ops ops.json \
  --base-revision 12 \
  --mutation-id 0190f3f4-... \
  --json
```

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `--ops <file>` | 是 | 操作文件 |
| `--base-revision <n>` | 是 | 读取时的 `revision` |
| `--mutation-id <id>` | 是 | 重试身份；丢响应时用同一 ID 与同一 body 重放 |
| `--dry-run` | 否 | 只在本地校验并打印规范化结果 |

`ops.json`：

```json
{
  "mutationId": "0190f3f4-...",
  "baseRevision": 12,
  "operations": [
    {"type": "create_node", "node": {"id": "n1", "type": "text", "x": 0, "y": 0}},
    {"type": "patch_node", "id": "n1", "patch": {"x": 120}},
    {"type": "remove_node", "id": "n2", "expected": {"id": "n2"}},
    {"type": "create_connection", "connection": {"id": "e1", "from": "n1", "to": "n2"}},
    {"type": "update_connection", "id": "e1", "connection": {"id": "e1", "from": "n1", "to": "n3"}, "expected": {"id": "e1", "from": "n1", "to": "n2"}},
    {"type": "remove_connection", "id": "e2", "expected": {"id": "e2"}},
    {"type": "set_field", "field": "title", "value": "第一集"}
  ]
}
```

约束：

- `mutationId` 在 `--ops` 与参数中必须一致；不一致报 2。
- `baseRevision` 必须来自最近的 `canvas get`。
- 409 冲突时返回服务端当前 `revision`；CLI 不自动重放。
- 服务端仍会校验节点锁定、引用归属和字段白名单。

## 3. 任务

### `yingce task list`

| 参数 | 说明 |
| --- | --- |
| `--project <id>` | 限定项目 |
| `--status <s>` | `queued` / `running` / `succeeded` / `failed` / `cancelled` |
| `--type <t>` | 任务类型 |
| `--limit <n>` | 默认 20 |
| `--recent` | 按创建时间倒序，CLI 本地排序 |

### `yingce task get <id>`

```bash
yingce task get task_xxx --out task.json --json
```

输出状态、进度、模型、积分、失败原因和产物列表。

### `yingce task wait <id>`

```bash
yingce task wait task_xxx --max-wait 10m --interval 3s --download-dir ./out --json
```

| 参数 | 说明 |
| --- | --- |
| `--max-wait <duration>` | 最长等待，默认 10m |
| `--interval <duration>` | 轮询间隔，默认 3s |
| `--download-dir <dir>` | 成功后下载产物 |

- 轮询到终态或超时；超时退出码 10，附当前状态。
- `--download-dir` 在成功后下载产物，返回本地绝对路径。
- 429 时按 `Retry-After` 或指数退避。

### `yingce task cancel <id>`

映射 `POST /tasks/:id/cancel`。已进入上游的任务可能继续完成，输出中必须说明。

## 4. 模型与报价

### `yingce model list`

| 参数 | 说明 |
| --- | --- |
| `--mode <image|video|audio>` | 过滤能力 |
| `--json` | 输出逻辑模型、能力、可选参数和价格区间 |

### `yingce model quote`

```bash
# 逻辑模型
yingce model quote --model logical_xxx \
  --mode video --operation image2video \
  --prompt "..." --duration 5 \
  --ref ./first.png --json

# 渠道模型
yingce model quote --channel ch_xxx --channel-model seedance-2.0 \
  --mode video --operation image2video --duration 5 --json
```

| 参数 | 说明 |
| --- | --- |
| `--model <id>` | 与 `--channel` 二选一 |
| `--channel <id>` + `--channel-model <key>` | 渠道直连 |
| `--mode` | `image` / `video` / `audio` |
| `--operation` | 省略时按模式与参考推导 |
| `--prompt` / `--prompt-file` | 提示词 |
| `--ref <path>` | 参考文件，可重复；只用于统计数量与类型 |
| `--ref-resource <id>` | 已有素材 |
| `--duration` / `--size` / `--quality` | 规格 |
| `--input <json>` | 高级参数，合并进 intent |

输出包含 `amountMicrocredits`、`billingMode`、`estimated`、`quoteHash` 和规格。

## 5. 素材

### `yingce asset list`

| 参数 | 说明 |
| --- | --- |
| `--project <id>` | 必填 |
| `--kind <image|video|audio|text>` | 过滤 |
| `--limit <n>` | 默认 20 |

### `yingce asset upload <path>`

```bash
yingce asset upload ./ref.png --kind image --json
```

- 小文件走 `POST /resources`；大于 8 MiB 走分片上传。
- 计算 SHA-256；服务端支持时按「用户 + 哈希」去重。
- 输出 `resourceId`、`kind`、`size`、`mimeType`、`sha256`。
- 只接受本地绝对路径；不接受 URL。

## 6. 生成

```bash
yingce generate \
  --mode video \
  --model logical_xxx \
  --operation image2video \
  --prompt "镜头缓慢推进" \
  --ref ./first.png \
  --duration 5 \
  --poll 0 \
  --json
```

| 参数 | 说明 |
| --- | --- |
| `--mode` | 必填：`image` / `video` / `audio` |
| `--model` 或 `--channel` + `--channel-model` | 必填 |
| `--operation` | 省略时按模式和参考推导 |
| `--prompt` / `--prompt-file` | 二选一 |
| `--ref <path>` | 本地参考，可重复；自动上传 |
| `--ref-resource <id>` | 已有素材，可重复 |
| `--duration` / `--size` / `--quality` / `--image-resolution` | 规格 |
| `--project <id>` | 归属项目 |
| `--poll <seconds>` | 0 表示只提交；最大 600 |
| `--download-dir <dir>` | 终态后下载 |
| `--idempotency-key <key>` | 不传时 CLI 生成并打印 |
| `--max-credits <n>` | 报价超过该积分时拒绝提交 |

重试时必须复用上次输出的 `idempotencyKey`；服务端会返回原任务并带 `replayed=true`，不会重复扣费。

提交成功：

```json
{
  "ok": true,
  "data": {
    "taskId": "task_xxx",
    "status": "queued",
    "idempotencyKey": "0190f3f4-...",
    "quote": {"amountMicrocredits": 120000000, "billingMode": "fixed"}
  }
}
```

参数到 `CreateTaskRequest` 的翻译集中在一个模块，并由契约测试覆盖；不得在命令层散落字段拼装。

## 7. 额度

### `yingce credit`

映射 `GET /wallet`。输出可用积分、冻结积分、今日消耗和 token 的剩余额度。

生成前建议先 `credit` 再 `quote`，额度不足直接停止。

## 8. 云端 Agent

### `yingce run create`

```bash
yingce run create \
  --prompt "把第 3 段分镜改成雨夜追车" \
  --canvas canvas_xxx \
  --permission-mode auto \
  --budget-credits 100 \
  --budget-tasks 4 \
  --idempotency-key 0190f3f4-... \
  --json
```

| 参数 | 说明 |
| --- | --- |
| `--prompt` / `--prompt-file` | 必填 |
| `--canvas <id>` | 可选；不传为无画布对话 |
| `--model <logicalModelId>` | 可选 |
| `--permission-mode` | 只允许 `read_only` / `auto` |
| `--budget-credits` | 必须 ≤ token 上限 |
| `--budget-tasks` / `--budget-video-seconds` | 可选 |
| `--reasoning-mode` | `off` / `auto` / `deep` |
| `--idempotency-key` | 必填，8–128 字符 |

### `yingce run get <id>`

输出状态、事件数、审批对象（只读）、产物和失败原因。

### `yingce run events <id>`

```bash
yingce run events run_xxx --after 42 --follow --timeout 300 --json
```

- 游标是事件 `seq`，不是任务 ID。
- `--follow` 持续输出新事件；超时退出码 10，不影响运行。

### `yingce run cancel <id>`

取消运行。已进入上游的媒体任务按单条任务规则继续完成。

## 9. 其他

### `yingce openapi`

```bash
yingce openapi --out openapi.yaml
```

保存服务端规范，用于排查；不用于生成客户端。

### `yingce setup`

```bash
yingce setup --agent claude-code --write
```

| 参数 | 说明 |
| --- | --- |
| `--agent <claude-code|cursor|codex|generic>` | 目标 Agent |
| `--dir <path>` | `generic` 时必填 |
| `--write` | 实际写入；缺省只打印路径 |

只写本工具自己的 `SKILL.md`，不修改其他文件。

## 10. 错误目录

| `reason` | 退出码 | 行动 |
| --- | --- | --- |
| `unauthorized` | 3 | 重新登录 |
| `token_revoked` / `token_expired` | 3 | 重新登录 |
| `forbidden` / `scope_missing` | 4 | 在 token 设置里补 scope |
| `not_found` | 5 | 重新 list/get 取 ID |
| `canvas_revision_conflict` | 6 | 重新读取画布 |
| `idempotency_conflict` | 6 | 换幂等键或核对原请求 |
| `agent_conversation_stale_parent` | 6 | 同步历史后再续轮 |
| `agent_run_state_busy` | 6 | 稍后重试 |
| `rate_limited` | 7 | 按 `Retry-After` 退避 |
| `quota_exceeded` | 8 | 停止并报告剩余额度 |
| `upstream_error` | 9 | 读原因，不自动重试扣费操作 |
| `timeout` | 10 | 查任务状态 |
| `unknown_state` | 11 | 到任务中心核对 |

## 11. 兼容性

- 本文属于 `cli/v1`。新增可选参数不升版本；删除参数、改变语义或输出字段才升版本。
- `GET /cli/manifest` 返回 `contractVersion`、`minCliVersion`、`deprecated`；`doctor` 负责提示。
- 未在本文列出的服务端接口不属于对外契约。
