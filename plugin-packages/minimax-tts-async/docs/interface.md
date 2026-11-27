# MiniMax 语音合成（异步） 接口字段

## 协议身份

- 插件 ID：`minimax-tts-async`。
- Provider ID：`minimax-tts-async`。
- 能力：`audio`。
- 默认 Base URL：`https://api.minimax.cn`。
- 鉴权驱动：`bearer`。
- 创建：`POST /v1/t2a_async_v2`。
- 查询：`GET /v1/query/t2a_async_query_v2`。

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
| `create.path` | `"/v1/t2a_async_v2"` |
| `create.contentType` | `"application/json"` |
| `create.body.model` | `{"$coalesce":[{"$ref":"request.model"},"speech-2.8-hd"]}` |
| `create.body.text` | `{"$ref":"request.prompt"}` |
| `create.body.voice_setting.voice_id` | `{"$ref":"request.extra.audioVoice"}` |
| `create.body.voice_setting.speed` | `{"$coalesce":[{"$if":{"condition":{"$ne":[{"$toFloat":{"$ref":"request.extra.audioSpeed"}},0]},"then":{"$toFloat":{"$ref":"request.extra.audioSpeed"}},"else":null}},1]}` |
| `create.body.voice_setting.vol` | `1` |
| `create.body.voice_setting.pitch` | `0` |
| `create.body.audio_setting.audio_sample_rate` | `32000` |
| `create.body.audio_setting.bitrate` | `128000` |
| `create.body.audio_setting.format` | `{"$coalesce":[{"$ref":"request.extra.audioFormat"},"mp3"]}` |
| `create.body.audio_setting.channel` | `1` |
| `poll.method` | `"GET"` |
| `poll.path` | `"/v1/query/t2a_async_query_v2"` |
| `poll.query.task_id` | `{"$ref":"taskId"}` |
| `result.method` | `"GET"` |
| `result.path` | `"/v1/files/retrieve"` |
| `result.query.file_id` | `{"$ref":"resultId"}` |

## Provider 扩展键

- 无额外扩展键。

动态模型或工作流允许使用文档声明的完整 `parameters/input/extra_body` 对象；该对象是协议本身的开放 schema，不会被宿主裁剪。

## 响应映射逐字段清单

| 映射位置 | 上游路径或转换表达式 |
| --- | --- |
| `response.taskId` | `{"$coalesce":[{"$ref":"response.task_id"},{"$ref":"taskId"}]}` |
| `response.resultId` | `{"$ref":"response.file_id"}` |
| `response.status` | `{"$coalesce":[{"$ref":"response.status"},{"$if":{"condition":{"$eq":[{"$ref":"response.base_resp.status_code"},0]},"then":"pending","else":"failed"}}]}` |
| `response.message` | `{"$if":{"condition":{"$in":[{"$lower":{"$ref":"response.status"}},["failed","expired"]]},"then":{"$if":{"condition":{"$in":[{"$lower":{"$coalesce":[{"$ref":"response.base_resp.status_msg"},""]}},["","success","ok"]]},"then":"语音合成任务失败","else":{"$ref":"response.base_resp.status_msg"}}},"else":{"$coalesce":[{"$ref":"response.base_resp.status_msg"},{"$ref":"message"}]}}}` |
| `response.audios` | `{"$if":{"condition":{"$ref":"response.file.download_url"},"then":{"url":{"$ref":"response.file.download_url"},"ephemeral":true},"else":null}}` |
| `response.usage.characters` | `{"$ref":"response.usage_characters"}` |
| `response.resultEphemeral` | `true` |
| `response.errorPaths[0]` | `"base_resp.status_code"` |
| `response.messagePaths[0]` | `"base_resp.status_msg"` |
| `response.messagePaths[1]` | `"message"` |

## 响应与错误

插件把上游 task/status/text/media/usage 映射为统一结果。临时媒体 URL 标记为 ephemeral，由宿主立即下载持久化。HTTP 错误、业务 code 和 error object 保持失败语义，不包装成成功。

## 兼容边界

MiniMax 异步长文本语音合成。POST /v1/t2a_async_v2 创建任务，GET /v1/query/t2a_async_query_v2 查询状态（Processing/Success/Failed/Expired），成功后用 file_id 调 GET /v1/files/retrieve 获取 download_url。默认域名 https://api.minimax.cn。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "minimax-tts-async",
  "name": "MiniMax 语音合成（异步）",
  "version": "2.0.0",
  "author": "MiniMax / 影策",
  "description": "MiniMax 语音合成（异步） 独立请求协议插件。",
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
        "id": "minimax-tts-async",
        "label": "MiniMax 语音合成（异步）",
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
            "message": "MiniMax 语音合成（异步）需要填写音色 ID"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/t2a_async_v2",
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
              "audio_sample_rate": 32000,
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
        "poll": {
          "method": "GET",
          "path": "/v1/query/t2a_async_query_v2",
          "query": {
            "task_id": {
              "$ref": "taskId"
            }
          }
        },
        "result": {
          "method": "GET",
          "path": "/v1/files/retrieve",
          "query": {
            "file_id": {
              "$ref": "resultId"
            }
          }
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "taskId"
              }
            ]
          },
          "resultId": {
            "$ref": "response.file_id"
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.status"
              },
              {
                "$if": {
                  "condition": {
                    "$eq": [
                      {
                        "$ref": "response.base_resp.status_code"
                      },
                      0
                    ]
                  },
                  "then": "pending",
                  "else": "failed"
                }
              }
            ]
          },
          "message": {
            "$if": {
              "condition": {
                "$in": [
                  {
                    "$lower": {
                      "$ref": "response.status"
                    }
                  },
                  [
                    "failed",
                    "expired"
                  ]
                ]
              },
              "then": {
                "$if": {
                  "condition": {
                    "$in": [
                      {
                        "$lower": {
                          "$coalesce": [
                            {
                              "$ref": "response.base_resp.status_msg"
                            },
                            ""
                          ]
                        }
                      },
                      [
                        "",
                        "success",
                        "ok"
                      ]
                    ]
                  },
                  "then": "语音合成任务失败",
                  "else": {
                    "$ref": "response.base_resp.status_msg"
                  }
                }
              },
              "else": {
                "$coalesce": [
                  {
                    "$ref": "response.base_resp.status_msg"
                  },
                  {
                    "$ref": "message"
                  }
                ]
              }
            }
          },
          "audios": {
            "$if": {
              "condition": {
                "$ref": "response.file.download_url"
              },
              "then": {
                "url": {
                  "$ref": "response.file.download_url"
                },
                "ephemeral": true
              },
              "else": null
            }
          },
          "usage": {
            "characters": {
              "$ref": "response.usage_characters"
            }
          },
          "resultEphemeral": true,
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
