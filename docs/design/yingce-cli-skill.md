# 影策 CLI Skill

> 这是随 CLI 分发的 `SKILL.md` 设计稿；实际分发文件为 `cli/internal/skill/SKILL.md`，同一内容也放在 `skills/yingce-cli/SKILL.md`，可通过 `npx skills add` 安装，由 `yingce skill` 原样输出。
> 完整参数以 `yingce <命令> --help` 为准；对外契约见 `docs/design/yingce-cli-reference.md`。

---

```yaml
---
name: yingce-cli
description: 用影策 CLI 在 Agent 中读取项目与画布、上传素材、报价并生成图片/视频、查询任务、驱动云端 Agent。用户提到影策、画布、分镜、短剧、生成图片或视频时使用。
---
```

# 影策 CLI

影策是 AI 影视与短剧创作工作台。通过 `yingce`，你可以在不打开网页的情况下读取画布、上传参考素材、报价、生成图片/视频并跟踪任务。

## 何时使用

- 用户要求查看或修改影策里的项目、画布、分镜。
- 用户要求生成图片、视频、音频，或基于已有素材继续创作。
- 用户要求把一项创作任务交给影策的云端 Agent。
- 用户提供了本地素材路径，需要上传后生成。

## 何时不要用

- 需要付款、充值、管理后台、用户权限：让用户到 Web 操作。
- 需要逐次人工审批的动作：CLI 不提供审批，只有 token 授权和账号额度。
- 用户没有明确要生成，或没有确认成本：先报价并询问，不要直接提交。

## 安装与认证

```bash
yingce version --json
yingce doctor --json
```

- 命令不存在：先询问用户是否安装，得到同意后再执行安装脚本；不要下载未知来源的二进制。
- 未认证：运行 `yingce login`，它会拉起浏览器。如果没有自动打开，把打印的授权链接交给用户，让用户在浏览器里确认 scope 和额度。
- 无浏览器环境（CI、容器）：让用户在 Web 设置里创建 token，再用 `yingce login --token-stdin` 或 `YINGCE_TOKEN` 注入。
- **不要索要密码、Cookie 或浏览器导出的登录 JSON。**
- token 失效返回退出码 3；让用户重新登录，不要尝试绕过。

## 先判断意图

```
只读 / 查状态        → project / canvas / task
改画布              → canvas get → 生成 ops → canvas apply
出图片 / 视频        → model list → model quote → generate → task wait
多步创作 / 批量      → run create → run events
付款 / 管理 / 审批   → 转 Web
```

不确定参数时先运行 `yingce <命令> --help`，不要猜。

## 硬性规则

1. **生成前必须报价**：先 `yingce model quote`，把积分、模型和规格告诉用户，再 `yingce generate`。
2. **重试必须复用同一个 `--idempotency-key`**：提交超时或连接中断时，用上次输出里的 `idempotencyKey` 重试；不要生成新键，也不要先建新任务。服务端返回 `replayed: true` 是正常重放。
3. **写画布前先读版本**：`canvas get` 拿 `revision`，`canvas apply` 必须带 `--base-revision` 和新的 `--mutation-id`。
4. **本地文件用绝对路径**：`--ref /abs/path/a.png`。不要贴 base64，不要传远程 URL。
5. **不要猜 ID**：项目、画布、模型、任务、资源 ID 一律从 list/get 结果里取。
6. **不要打印凭据**：不要把 token、Cookie、签名 URL 写进回复或日志。
7. **额度和权限不足立即停止**：报告缺哪个 scope 或额度，不要反复重试。
8. **不要自动重试生成**：只有携带同一幂等键的提交才可安全重放。
9. **大结果落盘**：用 `--out` / `--download-dir`，stdout 只保留摘要。
10. **保持任务账本**：每次提交记录 `taskId`、用途、状态，结束时对账。

## 命令速查

| 目的 | 命令 |
| --- | --- |
| 诊断 | `yingce doctor --json` |
| 当前身份 | `yingce whoami --json` |
| 登录 | `yingce login`（无浏览器时 `yingce login --no-browser`） |
| 项目列表 | `yingce project list --json` |
| 项目详情 | `yingce project get <id> --with-overview --json` |
| 画布列表 | `yingce canvas list --project <id> --json` |
| 读画布 | `yingce canvas get <id> --out canvas.json --json` |
| 写画布 | `yingce canvas apply <id> --ops ops.json --base-revision <rev> --mutation-id <uuid> --json` |
| 模型目录 | `yingce model list --mode <image|video|audio> --json` |
| 报价 | `yingce model quote --model <id> --mode <image|video|audio> [--operation <op>] --prompt "..." [--duration N] [--ref /abs/a.png] --json` |
| 生成 | `yingce generate --model <id> --mode <image|video|audio> [--operation <op>] --prompt "..." [--duration N] [--ref /abs/a.png] [--poll N] --json` |
| 任务列表 | `yingce task list [--project <id>] [--recent] --json` |
| 任务详情 | `yingce task get <id> --json` |
| 等待任务 | `yingce task wait <id> --max-wait 10m --download-dir /abs/out --json` |
| 取消任务 | `yingce task cancel <id> --json` |
| 素材列表 | `yingce asset list --project <id> --json` |
| 上传素材 | `yingce asset upload /abs/a.png --kind image --json` |
| 额度 | `yingce credit --json` |
| 云端 Agent | `yingce run create --prompt "..." --canvas <id> --permission-mode auto --budget-credits N --idempotency-key <key> --json` |
| 运行详情 | `yingce run get <id> --json` |
| 运行事件 | `yingce run events <id> --after <seq> --json` |
| 取消运行 | `yingce run cancel <id> --json` |
| 接口契约 | `yingce openapi --out openapi.yaml` |

所有命令都支持 `--json`。失败时按退出码判断类型，不要解析文案。

## 标准流程

### 1. 找到目标画布

```bash
yingce project list --json
yingce canvas list --project <projectId> --json
yingce canvas get <canvasId> --out /tmp/canvas.json --json
```

从 `canvas.json` 读取 `revision`、节点列表和已有素材引用。不要把整份 JSON 贴回给用户。

### 2. 生成图片

```bash
yingce model list --mode image --json
yingce model quote --model <logicalModelId> --mode image \
  --prompt "赛博朋克城市夜景，霓虹倒影" \
  --ref /abs/character.png --json

yingce generate --model <logicalModelId> --mode image \
  --prompt "赛博朋克城市夜景，霓虹倒影" \
  --ref /abs/character.png --poll 0 --json
```

把报价中的积分、尺寸和模型告诉用户，确认后再执行生成。

### 3. 生成视频

```bash
yingce model quote --model <id> --mode video --operation image2video \
  --prompt "镜头缓慢推进" --duration 5 --ref /abs/first.png --json

yingce generate --model <id> --mode video --operation image2video \
  --prompt "镜头缓慢推进" --duration 5 --ref /abs/first.png --poll 0 --json

yingce task wait <taskId> --max-wait 10m --download-dir /abs/out --json
```

异步任务在终态前不要重复提交。

### 4. 写画布

```bash
yingce canvas get <canvasId> --out /tmp/canvas.json --json
# 根据 revision 和现有节点生成 /tmp/ops.json
yingce canvas apply <canvasId> --ops /tmp/ops.json \
  --base-revision <revision> --mutation-id <uuid> --dry-run --json
yingce canvas apply <canvasId> --ops /tmp/ops.json \
  --base-revision <revision> --mutation-id <uuid> --json
```

最小 `ops.json`：

```json
{
  "mutationId": "<uuid>",
  "baseRevision": 12,
  "operations": [
    {"type": "create_node", "node": {"id": "n1", "type": "text", "x": 0, "y": 0}},
    {"type": "patch_node", "id": "n1", "patch": {"x": 120}}
  ]
}
```

- 409 冲突：重新 `canvas get`，基于新 revision 重建操作；不要重放旧批次。
- 一次尽量只做一个可解释的改动，便于用户撤销。
- 删除、锁定、跨用户引用由服务端拒绝，不要尝试绕过。

### 5. 交给云端 Agent

```bash
yingce run create --prompt "把第 3 段分镜改成雨夜追车" \
  --canvas <canvasId> --permission-mode auto \
  --budget-credits 100 --budget-tasks 4 \
  --idempotency-key <uuid> --json
```

- 只使用 `read_only` 或 `auto`。
- 必须带预算和幂等键。
- 用 `run events <id> --after <seq> --follow` 增量跟随；不要每轮从 0 重读。
- 运行失败时先读 `run get`，不要立刻重建一轮。

### 6. 批量生成

1. 先为一条样例报价并生成，确认模型、规格和成本。
2. 逐条提交，每条记录 `taskId` 和用途。
3. 每批结束用 `task list --recent` 对账。
4. 额度接近上限时停止，向用户报告剩余预算。

## 成本与额度

- 生成前先 `yingce credit --json` 看可用额度。
- 再 `model quote` 看单次成本。
- 单次或今日额度不足时直接停止，不要拆分规避上限。
- 批量任务按「单条报价 × 条数」估算，向用户说明总成本。

## 失败处理

| 退出码 | 含义 | 行动 |
| --- | --- | --- |
| 2 | 参数错误 | 修正参数，不要重试原命令 |
| 3 | 未认证 | 让用户完成 `login` |
| 4 | 权限不足 | 报告缺哪个 scope，让用户调整 token |
| 5 | 不存在 | 重新 list/get 取 ID |
| 6 | 冲突 | 重新读取版本或历史，再决定是否重提 |
| 7 | 限流 | 等待后重试，保持同一幂等键 |
| 8 | 额度不足 | 停止生成，报告剩余额度 |
| 9 | 服务端或上游错误 | 读 `reason`，不要盲目重试扣费操作 |
| 10 | 超时 | 查任务状态；不代表失败。重试时复用同一 `idempotencyKey` |
| 11 | 结果不确定 | 到任务中心核对，不要自动重提 |

## 输出给用户

成功时只报告：

- 做了什么（项目/画布/任务）
- `taskId` 或 `runId`
- 当前状态
- 消耗或预估积分
- 产物路径

`--json` 成功：

```json
{"ok": true, "data": {}, "meta": {"requestId": "req_..."}}
```

失败：

```json
{"ok": false, "error": {"code": 40301, "reason": "quota_exceeded", "message": "积分不足"}}
```

不要把完整 JSON、内部 requestId 或签名 URL 展示给用户，除非用户要求排查。

## 禁止事项

- 不要使用网页 Cookie、账号密码或导出的登录 JSON。
- 不要假设 `confirm: true` 或类似参数代表审批。
- 不要在没有报价的情况下生成。
- 不要用新的幂等键重试失败的生成任务；同一个 `idempotencyKey` 的重试是安全的。
- 不要修改不属于当前用户的资源。
- 不要调用管理员、付款、充值、审批相关接口。
- 不要把 token 写进命令历史、日志或回复。
