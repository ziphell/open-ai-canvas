# MiniMax 语音合成 接口字段

## 协议身份

- 插件 ID：`minimax-tts`。
- Provider ID：`minimax-tts`。
- 能力：`audio`。
- 默认 Base URL：`https://api.minimax.cn`。
- 鉴权驱动：`bearer`。
- 创建：`POST /v1/t2a_v2`。
- 生命周期：同步响应。

## 配置字段

| 字段 | 类型 | 必填 | 含义 |
| --- | --- | --- | --- |
| `apiKey` | secret | 是 | API Key |

## 统一字段映射

| 统一字段 | 类型 | 必填 | 上游映射 | 说明 |
| --- | --- | --- | --- | --- |
| `model` | string | 是 | `model` | 语音合成模型 ID，例如 speech-2.8-hd、speech-2.8-turbo。 |
| `prompt` | string | 是 | `text` | 待朗读文本。 |
| `providerOptions` | object | 否 | `provider-specific fields` | 插件命名空间内的厂商扩展字段。 |

## 上游请求模板逐字段清单

下表由插件请求模板生成，覆盖 body、query、headers 和 multipart 文件声明中的每个字段。

| 上游位置 | 值或转换表达式 |
| --- | --- |
| `create.method` | `"POST"` |
| `create.path` | `"/v1/t2a_v2"` |
| `create.contentType` | `"application/json"` |
| `create.body.model` | `{"$coalesce":[{"$ref":"request.model"},"speech-2.8-hd"]}` |
| `create.body.text` | `{"$ref":"request.prompt"}` |
| `create.body.stream` | `false` |
| `create.body.voice_setting.voice_id` | `{"$ref":"request.extra.audioVoice"}` |
| `create.body.voice_setting.speed` | `{"$coalesce":[{"$if":{"condition":{"$ne":[{"$toFloat":{"$ref":"request.extra.audioSpeed"}},0]},"then":{"$toFloat":{"$ref":"request.extra.audioSpeed"}},"else":null}},1]}` |
| `create.body.voice_setting.vol` | `1` |
| `create.body.voice_setting.pitch` | `0` |
| `create.body.audio_setting.sample_rate` | `32000` |
| `create.body.audio_setting.bitrate` | `128000` |
| `create.body.audio_setting.format` | `{"$coalesce":[{"$ref":"request.extra.audioFormat"},"mp3"]}` |
| `create.body.audio_setting.channel` | `1` |

## Provider 扩展键

- 无额外扩展键。

动态模型或工作流允许使用文档声明的完整 `parameters/input/extra_body` 对象；该对象是协议本身的开放 schema，不会被宿主裁剪。

## 响应映射逐字段清单

| 映射位置 | 上游路径或转换表达式 |
| --- | --- |
| `response.status` | `{"$if":{"condition":{"$eq":[{"$ref":"response.base_resp.status_code"},0]},"then":"succeeded","else":"failed"}}` |
| `response.message` | `{"$coalesce":[{"$ref":"response.base_resp.status_msg"},{"$ref":"message"}]}` |
| `response.audios` | `{"$if":{"condition":{"$ref":"response.data.audio"},"then":{"dataUrl":{"$concat":["data:",{"$if":{"condition":{"$eq":[{"$ref":"request.extra.audioFormat"},"wav"]},"then":"audio/wav","else":{"$if":{"condition":{"$eq":[{"$ref":"request.extra.audioFormat"},"flac"]},"then":"audio/flac","else":{"$if":{"condition":{"$eq":[{"$ref":"request.extra.audioFormat"},"pcm"]},"then":"audio/pcm","else":{"$if":{"condition":{"$eq":[{"$ref":"request.extra.audioFormat"},"opus"]},"then":"audio/ogg","else":"audio/mpeg"}}}}}}}},";base64,",{"$hexToBase64":{"$ref":"response.data.audio"}}]},"mimeType":{"$if":{"condition":{"$eq":[{"$ref":"request.extra.audioFormat"},"wav"]},"then":"audio/wav","else":{"$if":{"condition":{"$eq":[{"$ref":"request.extra.audioFormat"},"flac"]},"then":"audio/flac","else":{"$if":{"condition":{"$eq":[{"$ref":"request.extra.audioFormat"},"pcm"]},"then":"audio/pcm","else":{"$if":{"condition":{"$eq":[{"$ref":"request.extra.audioFormat"},"opus"]},"then":"audio/ogg","else":"audio/mpeg"}}}}}}}}},"else":null}}` |
| `response.usage.audio_length_ms` | `{"$ref":"response.extra_info.audio_length"}` |
| `response.usage.characters` | `{"$ref":"response.extra_info.usage_characters"}` |
| `response.errorPaths[0]` | `"base_resp.status_code"` |
| `response.messagePaths[0]` | `"base_resp.status_msg"` |
| `response.messagePaths[1]` | `"message"` |

## 响应与错误

插件把上游 task/status/text/media/usage 映射为统一结果。临时媒体 URL 标记为 ephemeral，由宿主立即下载持久化。HTTP 错误、业务 code 和 error object 保持失败语义，不包装成成功。

## 兼容边界

MiniMax 同步语音合成 HTTP 接口。POST /v1/t2a_v2，非流式一次性返回 hex 编码音频，插件转换为 data URL 后由宿主持久化。voice_setting.voice_id 必填，audio_setting 支持 mp3、wav、flac 等格式。默认域名 https://api.minimax.cn。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "minimax-tts",
  "name": "MiniMax 语音合成",
  "version": "2.0.0",
  "author": "MiniMax / 影策",
  "description": "MiniMax 语音合成 独立请求协议插件。",
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
        "id": "minimax-tts",
        "label": "MiniMax 语音合成",
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
        "baseUrl": "https://api.minimax.cn",
        "requiresPublicMediaUrls": false,
        "auth": {
          "type": "bearer",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": true,
            "mapping": "model",
            "description": "语音合成模型 ID，例如 speech-2.8-hd、speech-2.8-turbo。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "text",
            "description": "待朗读文本。"
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
            "message": "MiniMax 语音合成需要填写音色 ID"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/t2a_v2",
          "contentType": "application/json",
          "body": {
            "model": {
              "$coalesce": [
                {
                  "$ref": "request.model"
                },
                "speech-2.8-hd"
              ]
            },
            "text": {
              "$ref": "request.prompt"
            },
            "stream": false,
            "voice_setting": {
              "voice_id": {
                "$ref": "request.extra.audioVoice"
              },
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
              "vol": 1,
              "pitch": 0
            },
            "audio_setting": {
              "sample_rate": 32000,
              "bitrate": 128000,
              "format": {
                "$coalesce": [
                  {
                    "$ref": "request.extra.audioFormat"
                  },
                  "mp3"
                ]
              },
              "channel": 1
            }
          }
        },
        "response": {
          "status": {
            "$if": {
              "condition": {
                "$eq": [
                  {
                    "$ref": "response.base_resp.status_code"
                  },
                  0
                ]
              },
              "then": "succeeded",
              "else": "failed"
            }
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.base_resp.status_msg"
              },
              {
                "$ref": "message"
              }
            ]
          },
          "audios": {
            "$if": {
              "condition": {
                "$ref": "response.data.audio"
              },
              "then": {
                "dataUrl": {
                  "$concat": [
                    "data:",
                    {
                      "$if": {
                        "condition": {
                          "$eq": [
                            {
                              "$ref": "request.extra.audioFormat"
                            },
                            "wav"
                          ]
                        },
                        "then": "audio/wav",
                        "else": {
                          "$if": {
                            "condition": {
                              "$eq": [
                                {
                                  "$ref": "request.extra.audioFormat"
                                },
                                "flac"
                              ]
                            },
                            "then": "audio/flac",
                            "else": {
                              "$if": {
                                "condition": {
                                  "$eq": [
                                    {
                                      "$ref": "request.extra.audioFormat"
                                    },
                                    "pcm"
                                  ]
                                },
                                "then": "audio/pcm",
                                "else": {
                                  "$if": {
                                    "condition": {
                                      "$eq": [
                                        {
                                          "$ref": "request.extra.audioFormat"
                                        },
                                        "opus"
                                      ]
                                    },
                                    "then": "audio/ogg",
                                    "else": "audio/mpeg"
                                  }
                                }
                              }
                            }
                          }
                        }
                      }
                    },
                    ";base64,",
                    {
                      "$hexToBase64": {
                        "$ref": "response.data.audio"
                      }
                    }
                  ]
                },
                "mimeType": {
                  "$if": {
                    "condition": {
                      "$eq": [
                        {
                          "$ref": "request.extra.audioFormat"
                        },
                        "wav"
                      ]
                    },
                    "then": "audio/wav",
                    "else": {
                      "$if": {
                        "condition": {
                          "$eq": [
                            {
                              "$ref": "request.extra.audioFormat"
                            },
                            "flac"
                          ]
                        },
                        "then": "audio/flac",
                        "else": {
                          "$if": {
                            "condition": {
                              "$eq": [
                                {
                                  "$ref": "request.extra.audioFormat"
                                },
                                "pcm"
                              ]
                            },
                            "then": "audio/pcm",
                            "else": {
                              "$if": {
                                "condition": {
                                  "$eq": [
                                    {
                                      "$ref": "request.extra.audioFormat"
                                    },
                                    "opus"
                                  ]
                                },
                                "then": "audio/ogg",
                                "else": "audio/mpeg"
                              }
                            }
                          }
                        }
                      }
                    }
                  }
                }
              },
              "else": null
            }
          },
          "usage": {
            "audio_length_ms": {
              "$ref": "response.extra_info.audio_length"
            },
            "characters": {
              "$ref": "response.extra_info.usage_characters"
            }
          },
          "errorPaths": [
            "base_resp.status_code"
          ],
          "messagePaths": [
            "base_resp.status_msg",
            "message"
          ]
        }
      }
    ]
  }
}
```
<!-- YINGCE_MANIFEST_CONTRACT_END -->
