# 豆包语音合成 3.0 接口字段

## 协议身份

- 插件 ID：`doubao-voice-tts-3-0`。
- Provider ID：`doubao-voice-tts-3-0`。
- 能力：`audio`。
- 默认 Base URL：`https://voice.cn-beijing.volces.com`。
- 鉴权驱动：`header`。
- 创建：`POST /api/v3/text-to-speech/{voice_id}/streaming`。
- 生命周期：同步响应。

## 配置字段

| 字段 | 类型 | 必填 | 含义 |
| --- | --- | --- | --- |
| `apiKey` | secret | 是 | API Key |

## 统一字段映射

| 统一字段 | 类型 | 必填 | 上游映射 | 说明 |
| --- | --- | --- | --- | --- |
| `model` | string | 是 | `model_id` | 语音合成 3.0 模型 ID，默认 doubao-seed-tts-3.0。 |
| `prompt` | string | 是 | `text` | 待朗读文本，最长 10000 字符。 |
| `providerOptions` | object | 否 | `provider-specific fields` | 插件命名空间内的厂商扩展字段。 |

## 上游请求模板逐字段清单

下表由插件请求模板生成，覆盖 body、query、headers 和 multipart 文件声明中的每个字段。

| 上游位置 | 值或转换表达式 |
| --- | --- |
| `create.method` | `"POST"` |
| `create.path` | `"/api/v3/text-to-speech/{voice_id}/streaming"` |
| `create.pathTemplate` | `{"$concat":["/api/v3/text-to-speech/",{"$ref":"request.extra.audioVoice"},"/streaming"]}` |
| `create.contentType` | `"application/json"` |
| `create.body.text` | `{"$ref":"request.prompt"}` |
| `create.body.model_id` | `{"$ref":"request.model"}` |
| `create.body.output_format.format` | `{"$coalesce":[{"$ref":"request.extra.audioFormat"},"mp3"]}` |
| `create.body.voice_settings.speed` | `{"$coalesce":[{"$if":{"condition":{"$ne":[{"$toFloat":{"$ref":"request.extra.audioSpeed"}},0]},"then":{"$toFloat":{"$ref":"request.extra.audioSpeed"}},"else":null}},1]}` |
| `create.body.voice_settings.instructions` | `{"$omitEmpty":{"$if":{"condition":{"$ref":"request.extra.audioInstructions"},"then":[{"$ref":"request.extra.audioInstructions"}],"else":null}}}` |
| `create.body.enable_subtitle` | `false` |
| `create.headers.X-Request-Id` | `{"$ref":"request.extra.idempotencyKey"}` |

## Provider 扩展键

- 无额外扩展键。

动态模型或工作流允许使用文档声明的完整 `parameters/input/extra_body` 对象；该对象是协议本身的开放 schema，不会被宿主裁剪。

## 响应映射逐字段清单

| 映射位置 | 上游路径或转换表达式 |
| --- | --- |
| `response.streamedJsonAudio` | `true` |
| `response.resultKind` | `"audio"` |

## 响应与错误

插件把上游 task/status/text/media/usage 映射为统一结果。临时媒体 URL 标记为 ephemeral，由宿主立即下载持久化。HTTP 错误、业务 code 和 error object 保持失败语义，不包装成成功。

## 兼容边界

豆包语音合成大模型 3.0 单向流式 HTTP 接口。POST /api/v3/text-to-speech/{voice_id}/streaming，音色 ID 必须由用户填写；请求体提交完整文本，响应为 JSONL 事件流：ready、audio、alignment 和 final，audio 事件携带 Base64 音频片段，按接收顺序拼接。默认模型 doubao-seed-tts-3.0，输出格式支持 mp3、pcm、ogg_opus。该协议只做文本朗读，不接收参考音频或参考图片。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "doubao-voice-tts-3-0",
  "name": "豆包语音合成 3.0",
  "version": "2.0.0",
  "author": "Volcengine / 影策",
  "description": "豆包语音合成 3.0 独立请求协议插件。",
  "documentation": "<当前插件的完整 documentation，由 README.md 与 docs/interface.md 拼接而成；为避免 JSON 递归，此处不重复展开正文。>",
  "permissions": [
    "generation.run",
    "media.read"
  ],
  "configuration": {
    "fields": [
      {
        "name": "apiKey",
        "type": "secret",
        "label": "API Key",
        "required": true
      }
    ]
  },
  "contributes": {
    "providers": [
      {
        "id": "doubao-voice-tts-3-0",
        "label": "豆包语音合成 3.0",
        "capabilities": [
          "audio"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://voice.cn-beijing.volces.com",
        "requiresPublicMediaUrls": false,
        "auth": {
          "type": "header",
          "field": "apiKey",
          "header": "X-Api-Key"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model_id",
            "description": "语音合成 3.0 模型 ID，默认 doubao-seed-tts-3.0。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "text",
            "description": "待朗读文本，最长 10000 字符。"
          },
          {
            "name": "providerOptions",
            "type": "object",
            "required": false,
            "mapping": "provider-specific fields",
            "description": "插件命名空间内的厂商扩展字段。"
          }
        ],
        "validations": [
          {
            "assert": {
              "$ref": "request.extra.audioVoice"
            },
            "message": "豆包语音合成 3.0 需要填写音色 ID"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/api/v3/text-to-speech/{voice_id}/streaming",
          "pathTemplate": {
            "$concat": [
              "/api/v3/text-to-speech/",
              {
                "$ref": "request.extra.audioVoice"
              },
              "/streaming"
            ]
          },
          "contentType": "application/json",
          "body": {
            "text": {
              "$ref": "request.prompt"
            },
            "model_id": {
              "$ref": "request.model"
            },
            "output_format": {
              "format": {
                "$coalesce": [
                  {
                    "$ref": "request.extra.audioFormat"
                  },
                  "mp3"
                ]
              }
            },
            "voice_settings": {
              "speed": {
                "$coalesce": [
                  {
                    "$if": {
                      "condition": {
                        "$ne": [
                          {
                            "$toFloat": {
                              "$ref": "request.extra.audioSpeed"
                            }
                          },
                          0
                        ]
                      },
                      "then": {
                        "$toFloat": {
                          "$ref": "request.extra.audioSpeed"
                        }
                      },
                      "else": null
                    }
                  },
                  1
                ]
              },
              "instructions": {
                "$omitEmpty": {
                  "$if": {
                    "condition": {
                      "$ref": "request.extra.audioInstructions"
                    },
                    "then": [
                      {
                        "$ref": "request.extra.audioInstructions"
                      }
                    ],
                    "else": null
                  }
                }
              }
            },
            "enable_subtitle": false
          },
          "headers": {
            "X-Request-Id": {
              "$ref": "request.extra.idempotencyKey"
            }
          }
        },
        "response": {
          "streamedJsonAudio": true,
          "resultKind": "audio"
        }
      }
    ]
  }
}
```
<!-- YINGCE_MANIFEST_CONTRACT_END -->
