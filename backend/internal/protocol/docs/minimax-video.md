# MiniMax H3 视频

MiniMax 视频插件使用 `/v2/video_generation` 创建异步任务。它同时发送顶层 `prompt` 和结构化 `content[]`：文本为 text item，参考图片为带 `reference_image` role 的 image URL item。

## 接口与鉴权

{{OPERATIONS}}

```http
POST {channel_base_url}/v2/video_generation
GET  {channel_base_url}/v2/query/video_generation/{task_id}
Authorization: Bearer <MINIMAX_API_KEY>
Content-Type: application/json
```

## 模型与约束

当前 MiniMax V2 官方文档列出 `MiniMax-H3`、`MiniMax-H3-Highspeed`、`MiniMax-H3-Lite` 与 `MiniMax-H3-Max`：

- 时长：均为整数秒；`MiniMax-H3`、Highspeed、Lite 支持 4–15 秒，`MiniMax-H3-Max` 支持 5–15 秒。
- 分辨率：`MiniMax-H3` 支持 `768P`、`2K`；Highspeed、Lite、Max 支持 `480P`、`768P`。
- 比例：文生视频不能使用 `adaptive`，未提供时按 `16:9` 发送；首尾帧恒为 `adaptive`；多模态参考默认 `adaptive`，`MiniMax-H3-Lite` 需显式指定具体比例。
- 图生视频（首帧/尾帧）与多模态参考（参考图/参考视频/参考音频）互斥，混用直接失败。

当前 adapter 默认 6 秒；模型名和其他限制仍以账户实际开放能力为准。

## 参数与字段映射

{{PARAMETERS}}

每张图片构造为 `content` 中的 `image_url` item。`extra` 可覆盖 `model`、`prompt`、`duration`、`content`、`resolution`、`aspect_ratio`。`generate_audio` 映射统一音频开关，渠道能力默认开启。官方 `MiniMax-H3-Max` 可通过 `providerOptions.minimax-video.prompt_expansion_mode` 发送 `extra.prompt_expansion_mode`（`disabled`/`balanced`/`quality`，默认 `balanced`，未设置时不发送 `extra`）；其他中转渠道不保证支持提示词扩写。参考视频、参考音频和水印由对应字段按需发送，其中 `callback_url` 当前不发送。

## 创建任务示例

```bash
curl "{channel_base_url}/v2/video_generation" \
  -H "Authorization: Bearer <MINIMAX_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "model":"YOUR_MINIMAX_VIDEO_MODEL",
    "prompt":"保持角色一致，人物转身看向镜头",
    "duration":6,
    "content":[
      {"type":"text","text":"保持角色一致，人物转身看向镜头"},
      {"type":"image_url","image_url":{"url":"https://example.com/character.png"},"role":"reference_image"}
    ],
    "resolution":"768P",
    "aspect_ratio":"16:9"
  }'
```

## 查询、文件与结果

```bash
curl "{channel_base_url}/v2/query/video_generation/task_xxx" \
  -H "Authorization: Bearer <MINIMAX_API_KEY>"
```

当前 V2 官方成功响应在 `task.content.url` 返回视频地址，通用 parser 会先合并 `task`，再从 `content.url` 提取结果。官方仅允许查询最近 7 天任务，视频 URL 有时效，应立即转存。当前 adapter 支持参考视频、参考音频和 `aigc_watermark`；`callback_url` 仍未发送，任务结果统一通过轮询查询获取。

## 官方资料

- [MiniMax 创建视频生成任务](https://platform.minimaxi.com/docs/api-reference/video-generation-v2-create)
- [MiniMax 查询任务](https://platform.minimaxi.com/docs/api-reference/video-generation-v2-query)

{{CONTRACT}}
