# MiniMax H3 Video 协议插件

该包是 MiniMax H3 原生 V2 视频协议，provider ID 为 `minimax-video`。画布、创作页和短剧生产只提交统一 `GenerationRequest`；插件将图片、视频和音频按显式 role 转成 MiniMax `content[]`。

重点更新：

- 模型支持 `MiniMax-H3`、`MiniMax-H3-Highspeed`、`MiniMax-H3-Lite`、`MiniMax-H3-Max`。
- `duration` 支持 4-15 秒整数，`MiniMax-H3-Max` 为 5-15 秒；超出范围直接失败，不静默裁剪。
- `resolution` 支持 480P、768P、2K；`MiniMax-H3` 仅 768P、2K，Highspeed、Lite、Max 仅 480P、768P。
- 比例写入上游实际使用的 `ratio`：文生视频不能使用 `adaptive`，未提供时按 `16:9` 发送；首尾帧恒为 `adaptive`；多模态参考默认 `adaptive`，`MiniMax-H3-Lite` 需显式指定具体比例。
- 图生视频（首帧/尾帧）与多模态参考（参考图/参考视频/参考音频）互斥，混用直接失败。
- `watermark` 映射为 `aigc_watermark`。
- `generateAudio` 映射为 `generate_audio`；渠道能力默认开启，显式关闭时发送 `false`。
- prompt 只放入 text block，不再同时发送顶层 prompt。
- 首帧、尾帧和 reference image 依据 role 转换，不按 `0/1` 猜测。
- 官方 `MiniMax-H3-Max` 可通过 `providerOptions.minimax-video.prompt_expansion_mode` 传入 `extra.prompt_expansion_mode`：`disabled`、`balanced`、`quality`，默认 `balanced`；未设置时不发送 `extra`。其他中转渠道不保证支持提示词扩写。
- 当前仍不发送 `callback_url`；创建响应按 `response_format=task` 解析，查询兼容 `task` 与平铺结构。

完整字段见 [docs/interface.md](docs/interface.md)。
