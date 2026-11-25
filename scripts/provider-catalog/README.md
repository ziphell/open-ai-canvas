# 服务商目录

`catalog.json` 是仓库维护并准备发布的服务商目录；2026-11-25 已用 `https://ai.yingce.tv/v1/catalog.json` 的在线内容回填（包含影策、国模、秘塔、宇熙 Box、万有引力），随后修正了秘塔协议与 Base URL。`catalog.example.json` 仅作格式示例。

后端默认使用随应用内嵌的 `backend/internal/providercatalog/catalog.json`，编译进后端二进制，运行时不读取也不写入数据目录缓存；只有设置 `CANVAS_PROVIDER_CATALOG_URL` 时才从该地址拉取并落盘缓存（也可指向 `https://ai.yingce.tv/v1/catalog.json`）。修改本文件后执行 `cp scripts/provider-catalog/catalog.json backend/internal/providercatalog/catalog.json` 并重新构建服务端；后端测试会校验两份文件一致。

当前目录包含影策中转站、影策国模、秘塔、宇熙 Box 与万有引力。卡片按北京时间显示完整起止年份、明确到期日和剩余天数。

长期推广配置：在服务商对象中设置 `"permanent": true`，保留 `startsAt`，可以省略 `endsAt`。此时到达开始时间后持续展示，卡片显示“长期推广 · 无到期日”。定期推广保持 `permanent` 不填或为 `false`，必须设置晚于开始时间的 `endsAt`。秘塔在线目录当前为 `permanent: true`，本次备份保持该状态。平台 Logo 从 https://metaso.cn 官网左上角内联 SVG 采集，保存在 `logos/metaso.svg`；模型 Logo 使用产品现有的 `minimax` 标识。

秘塔价格按用户确认的每秒计费：480P 0.075 元、768P 0.09 元、1440P（2K）0.18 元。1 元 = 1 积分。

目录中的 MiniMax-H3 使用 `metaso-h3` 协议，保留秘塔账户的 4–30 秒时长和首尾帧混用参考图能力。由于 `metaso-h3` 的创建路径已包含 `/api/minimax`，目录 `connection.baseUrl` 改为 `https://metaso.cn`；管理端 Key 校验会请求 `https://metaso.cn/v1/models`，接入前需确认秘塔仍支持该路径。原生 H3 V2 协议为 `minimax-video`（4–15 秒、禁止首尾帧与多模态参考混用）；旧的 `/v1` 视频插件已删除。真实 API Key 连通与生成尚未联调。

发布时目录结构应为：

```
/srv/yingce-catalog/v1/catalog.json
/srv/yingce-catalog/v1/logos/metaso.svg
```

已部署至服务器，`https://ai.yingce.tv/v1/catalog.json` 与秘塔 Logo 均验证返回 HTTP 200。只更新静态目录与 Caddy 站点，未部署影策主应用。目录拉取总超时为 3 秒，失败后普通读取冷却 30 秒，手动刷新仍立即尝试；刷新接口每次只拉取一次，并发刷新共享同一次请求；已有缓存立即返回，过期时后台刷新，不会被网络请求持锁阻塞。源站（绕过 Cloudflare）HTTPS 校验也已通过；HTTP 自动 308 跳转到 HTTPS。当前能力配置支持 4–30 秒，默认 5 秒，参考图/视频/音频为 9/3/3，使用秘塔专属 `metaso-h3` 协议。目录能力配置尚未按分辨率联合限制时长，模型描述中的「2K 视频 15 秒」仍只是描述，不是请求校验。真实 API Key 连通与生成尚未联调。
