# 影策 CLI 后端实现规范

> 状态：M1 已实现（`api_tokens`、`cli_auth_codes`、Bearer 中间件、scope 守卫、浏览器授权端点、任务幂等）。上传去重待接入；不引入 token 级 caps。
> 上游设计见 [yingce-cli.md](yingce-cli.md)，命令契约见 [yingce-cli-reference.md](yingce-cli-reference.md)。

## 1. 范围

只新增，不改现有行为：

- 新增 API token 与浏览器授权。
- Bearer 与现有 Cookie 并存；Cookie 路径保持原样。
- 新增 token 管理接口和页面。
- 新增任务幂等键。
- 新增 scope 强制；不引入 token 级 caps。
- 不开放管理员接口给 Bearer。

## 2. 数据模型

### 2.1 `api_tokens`

模型放在 `backend/internal/model/models_api_token.go`：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ID` | string PK | `tok_...` |
| `UserID` | string index | 归属用户 |
| `Name` | string(64) | 用户可读名称 |
| `TokenPrefix` | string(16) | 前 10 位，用于展示 |
| `TokenHash` | string(64) unique | SHA-256 hex |
| `ScopesJSON` | text | `["read", "generate:image"]` |
| `CreatedAt` | time | |
| `ExpiresAt` | time index | 最长 90 天 |
| `LastUsedAt` | *time | 60 秒节流更新 |
| `RevokedAt` | *time | 非空即失效 |

约束：

- token 明文只返回一次，服务端不保存。
- 哈希使用 SHA-256，比较使用 `hmac.Equal`。
- 列表接口只返回 `tokenPrefix`、名称、scope、额度、时间和最后使用。

### 2.2 `cli_auth_codes`

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ID` | string PK | |
| `CodeHash` | string(64) unique | 授权码哈希 |
| `UserID` | string index | 批准用户 |
| `ClientName` | string(64) | CLI 名称 |
| `RedirectURI` | string(255) | loopback 回调，兑换时精确匹配 |
| `CodeChallenge` | string(64) | PKCE S256 challenge |
| `CodeChallengeMethod` | string(8) | 固定 `S256` |
| `ScopesJSON` | text | 批准时冻结 |
| `IPHash` / `UserAgentHash` | string(64) | 审计摘要，不存原文 |
| `ExpiresAt` | time index | 60 秒 |
| `ConsumedAt` | *time | 非空即已使用 |
| `CreatedAt` | time | |

### 2.3 `task_idempotency`

不改 `tasks` 表，新增独立表，避免历史数据和大表迁移：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ID` | string PK | |
| `UserID` | string | |
| `Key` | string(128) | 客户端幂等键 |
| `RequestDigest` | string(64) | 规范化请求哈希 |
| `TaskID` | string | 首次创建的任务 |
| `CreatedAt` | time | |

唯一索引：`(UserID, Key)`。

## 3. Token 格式

- 前缀 `yc_`，后接 32 字节随机数的 base64url，长度约 46。
- 存储 `sha256(token)`，不存明文。
- 展示前缀取前 10 个字符，例如 `yc_a1b2c3d4`。
- 有效期默认 30 天，最大 90 天；过期与撤销都在每次请求校验。
- 日志脱敏规则增加 `yc_[A-Za-z0-9_-]{20,}`。

## 4. 浏览器授权流程

### 4.1 端点

| 方法 | 路径 | 鉴权 | 作用 |
| --- | --- | --- | --- |
| GET | `/api/auth/cli/authorize` | Cookie | 返回待确认的 client、scope 与额度 |
| POST | `/api/auth/cli/authorize` | Cookie | 用户确认，生成一次性 code 并返回回调地址 |
| POST | `/api/auth/cli/token` | code + PKCE | 校验并兑换 token |

前端路由 `/cli/authorize` 负责展示确认页，调用上面两个 Cookie 接口。v0.1 由后端在 `GET /api/auth/cli/authorize` 上按 `Accept: text/html` 直接渲染确认页，后续可迁移到独立前端路由。

### 4.2 流程

1. CLI 生成 `state` 和 `code_verifier`，计算 `code_challenge = base64url(sha256(verifier))`。
2. CLI 监听 `127.0.0.1:<随机端口>`，打开 `/api/auth/cli/authorize?...`。
3. 页面要求已登录；未登录先走现有登录流程。
4. `GET /api/auth/cli/authorize` 校验 `redirect_uri` 为 loopback，返回待确认信息。
5. 用户确认后 `POST /api/auth/cli/authorize`：
   - 再次校验 `redirect_uri` 与 `code_challenge`。
   - 生成 32 字节 code，存 `sha256(code)`，TTL 60 秒。
   - 返回 `redirectUrl`，由页面跳转到 loopback。
6. CLI 校验 `state`，调用 `POST /api/auth/cli/token` 提交 `code`、`code_verifier`、`redirect_uri`。
7. 服务端校验 PKCE、`redirect_uri` 精确匹配、未过期、未消费；在单条事务中标记 `consumed_at` 并签发 token。

### 4.3 约束与限流

- `redirect_uri` 只允许 `http://127.0.0.1:<port>/callback` 或 `http://[::1]:<port>/callback`，端口任意。
- 只接受 `code_challenge_method=S256`；不接受 `plain`。
- 授权码一次性；重复兑换返回 3，不返回 token。
- 用户拒绝时不生成 code，页面直接返回取消。
- `GET/POST /api/auth/cli/authorize` 每用户 20 次/分钟；`POST /api/auth/cli/token` 每 IP 60 次/分钟，失败按 401 处理。
- 写操作 scope（`canvas:write`、`generate:*`、`agent:run`）建议在确认页要求再次输入密码；页面要明确展示权限，并说明 CLI 与网页共用账号配额。

## 5. Bearer 中间件

### 5.1 挂载位置

- 在 `api := r.Group("/api")` 之后、业务路由注册之前挂载 `AuthenticateBearer(svc)`。
- `currentUser`（[auth.go](file:///home/ryan/code/open-ai-canvas/backend/internal/handler/auth.go#L654-L700)）先读中间件写入的身份，再回退现有 Cookie 逻辑，保证所有 handler 不用改签名。

### 5.2 行为

1. 无 `Authorization: Bearer`：直接放行，走现有 Cookie。
2. 有 Bearer：
   - 解析 token，查 `api_tokens`；校验未撤销、未过期、用户启用。
   - `/admin/*` 一律 403，即使 scope 包含 admin。
   - 校验 `X-Canvas-Expected-User`（若提供）必须等于 token 用户。
   - 写入 `c.Set("auth.user", user)`、`c.Set("auth.token", token)`。
   - `last_used_at` 超过 60 秒才更新；更新失败不影响请求。
   - 失败统一返回 401，不区分 token 不存在、过期或撤销。
3. Bearer 与 Cookie 同时存在：Bearer 优先；两者身份不一致直接 401，避免混淆。

## 6. Scope 强制

建议沿用 `StorageReadOnlyGuard` 的路径匹配方式，新增 `TokenScopeGuard`，在中间件层做路由级判断，service 层做对象级判断。

| 路径（方法） | 必需 scope |
| --- | --- |
| `GET /projects`、`/projects/:id`、`/projects/:id/overview` | `read` |
| `GET /canvas-projects`、`/canvas-projects/:id` | `read` |
| `PATCH /canvas-projects/:id/operations`、`POST .../edit-lease` | `canvas:write` |
| `GET /tasks`、`GET /tasks/:id` | `read` |
| `POST /tasks/:id/cancel` | `task:write` |
| `GET /model-catalog`、`POST /models/:id/quote`、`POST /model-catalog/quote` | `read` |
| `GET /wallet`、`GET /openapi.yaml` | `read` |
| `GET /agent/*` | `read` |
| `POST /agent/runs`、`POST /agent/runs/:id/cancel` | `agent:run` |
| `POST /resources`、`/resources/uploads/**` | `asset:upload` |
| `POST /tasks` | 按 `type` / `input.mode` 判定 `generate:image` / `generate:video` / `generate:audio` |
| 其他路径 | Bearer 一律 403 |

`POST /tasks` 的 scope 不能只看路径，必须在 service 归一化请求后判定；不确定性时按拒绝处理。

## 7. 额度模型：复用账号配额

CLI 是同一账号的另一个入口，不引入 token 级 caps：

- 积分余额、同时任务数、上传日额度、存储、画布/素材数量全部沿用 `user_id` 级配额。
- 请求限流继续按 `user_id` 计数，CLI 与 Web 共享同一个桶。
- token 只负责身份认证和 scope 准入，不绑定预算、项目或模型白名单。
- 需要限制单个自动化客户端时再单独设计，不进入 v1。

失败沿用现有错误：`quota_exceeded`、`insufficient_credits`、`active_task_limit`。CLI 的 `--max-credits` 仍保留，作为调用前的本地保护。

## 8. 任务幂等（已实现）

- `POST /tasks` 读取 `X-Idempotency-Key`；也可在 JSON 中传 `idempotencyKey`，两者冲突时返回 400。
- 归一化请求后计算 `RequestDigest`：`userID + type + operation + logicalModelId + prompt + canonical(input)`。
- 在创建任务与预留积分的同一事务中写 `task_idempotency`。
- 唯一键冲突时读取已有记录：
  - digest 相同：返回已有任务，`data.replayed=true`，不重复扣费。
  - digest 不同：409 `idempotency_conflict`。
- 键格式 8–128 字符，仅允许字母、数字、`-`、`_`。
- 记录保留 7 天，由 worker 每小时清理。
- 未传键时保持现状，不生成隐式键；CLI 会自己生成并打印。

## 9. Token 管理接口

| 方法 | 路径 | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/auth/tokens` | Cookie | 列表，不含明文；M1 已实现 |
| POST | `/api/auth/tokens` | Cookie | 创建并返回明文一次；M2，M1 由浏览器授权签发 |
| DELETE | `/api/auth/tokens/:id` | Cookie | 按 id 撤销；M2 |
| DELETE | `/api/auth/tokens/current` | Bearer | 撤销当前 token；M1 已实现 |

创建请求：

```json
{
  "name": "claude-code-laptop",
  "scopes": ["read", "generate:image"],
  "expiresInDays": 30
}
```

约束：

- scope 必须在白名单内；未知 scope 返回 400。
- `expiresInDays` 1–90，默认 30。
- 创建与撤销写 `api_token_events`。
- Bearer 不能调用这些管理接口，避免 token 自我提权。

## 10. 审计与脱敏

- API 日志增加 `token_id`，不记录 token。
- `api_token_events` 记录 `create`、`revoke`、`cli_authorize`、`cli_deny`；M2 接入，M1 先记录授权码上的 IP/UA 摘要。
- 日志脱敏新增 token 正则；错误响应、诊断包、URL 中不得出现 token。
- 授权批准与拒绝都记录 IP 与 user agent 摘要（截断、哈希）。

## 11. 开关与限流

- 环境变量 `CANVAS_API_TOKENS_ENABLED`，默认 `false`；管理员开启后 token 接口才生效。
- 关闭时 Bearer 一律 401，`doctor` 提示“服务端未开启 CLI 访问”。
- 每个 token 复用用户级限流，并增加独立的每分钟请求上限，取两者较小值。
- 生成类接口沿用现有任务限流和积分准入。

## 12. 迁移与测试

### 迁移

- 新增 `api_tokens`、`cli_auth_codes`、`task_idempotency`、`api_token_events` 四张表。
- 不修改现有表；不回填历史数据。
- 迁移放在 `backend/internal/database/migration_api_token.go`，并登记到现有迁移列表。

### 必测

- Bearer 解析：正常、过期、撤销、用户停用、admin 路径拒绝。
- 浏览器授权：批准、拒绝、过期、重复兑换、PKCE 不匹配、state 缺失、redirect_uri 非 loopback 或跨端口不匹配。
- scope：每个路径的允许与拒绝；`POST /tasks` 按模式判定。
- 额度：CLI 与 Cookie 请求共用同一账号的积分、并发和存储配额。
- 幂等：同键同 body 返回原任务；同键不同 body 返回 409；并发提交只创建一个任务。
- 审计：token 明文不出现在日志、错误和诊断包。
- 回归：Cookie 登录、管理后台、画布实时、Agent SSE 不受影响。

## 13. 上线顺序

1. 迁移与模型。
2. Bearer 中间件与 `currentUser` 兼容层（开关关闭）。
3. Token 管理接口与页面。
4. 浏览器授权流程。
5. scope 强制。
6. 任务幂等。
7. 打开 `CANVAS_API_TOKENS_ENABLED`，发布 CLI。
