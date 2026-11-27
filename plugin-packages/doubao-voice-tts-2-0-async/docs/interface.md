# 豆包语音合成 2.0（异步） 接口字段

## 协议身份

- 插件 ID：`doubao-voice-tts-2-0-async`。
- Provider ID：`doubao-voice-tts-2-0-async`。
- 能力：`audio`。
- 默认 Base URL：`https://openspeech.bytedance.com`。
- 鉴权驱动：`header`。
- 创建：`POST /api/v3/tts/submit`。
- 查询：`POST /api/v3/tts/query`。

## 配置字段

| 字段 | 类型 | 必填 | 含义 |
| --- | --- | --- | --- |
| `apiKey` | secret | 是 | API Key |

## 统一字段映射

| 统一字段 | 类型 | 必填 | 上游映射 | 说明 |
| --- | --- | --- | --- | --- |
| `model` | string | 是 | `X-Api-Resource-Id` | 资源 ID：seed-tts-2.0（语音合成大模型 2.0）或 seed-icl-2.0（声音复刻 2.0）。 |
| `prompt` | string | 是 | `req_params.text` | 待朗读文本，最长 10 万字符。 |
| `providerOptions` | object | 否 | `provider-specific fields` | 插件命名空间内的厂商扩展字段。 |

## 上游请求模板逐字段清单

下表由插件请求模板生成，覆盖 body、query、headers 和 multipart 文件声明中的每个字段。

| 上游位置 | 值或转换表达式 |
| --- | --- |
| `create.method` | `"POST"` |
| `create.path` | `"/api/v3/tts/submit"` |
| `create.contentType` | `"application/json"` |
| `create.headers.X-Api-Resource-Id` | `{"$coalesce":[{"$ref":"request.model"},"seed-tts-2.0"]}` |
| `create.headers.X-Api-Request-Id` | `{"$ref":"request.extra.idempotencyKey"}` |
| `create.body.user.uid` | `"yingce"` |
| `create.body.unique_id` | `{"$ref":"request.extra.idempotencyKey"}` |
| `create.body.req_params.text` | `{"$ref":"request.prompt"}` |
| `create.body.req_params.speaker` | `{"$ref":"request.extra.audioVoice"}` |
| `create.body.req_params.audio_params.format` | `{"$coalesce":[{"$ref":"request.extra.audioFormat"},"mp3"]}` |
| `create.body.req_params.audio_params.sample_rate` | `{"$if":{"condition":{"$eq":[{"$ref":"request.extra.audioFormat"},"ogg_opus"]},"then":48000,"else":24000}}` |
| `create.body.req_params.audio_params.speech_rate` | `{"$add":[{"$multiply":[{"$coalesce":[{"$if":{"condition":{"$ne":[{"$toFloat":{"$ref":"request.extra.audioSpeed"}},0]},"then":{"$toFloat":{"$ref":"request.extra.audioSpeed"}},"else":null}},1]},100]},-100]}` |
| `create.body.req_params.audio_params.loudness_rate` | `0` |
| `create.body.req_params.audio_params.enable_timestamp` | `false` |
| `poll.method` | `"POST"` |
| `poll.path` | `"/api/v3/tts/query"` |
| `poll.contentType` | `"application/json"` |
| `poll.headers.X-Api-Resource-Id` | `{"$coalesce":[{"$ref":"request.model"},"seed-tts-2.0"]}` |
| `poll.headers.X-Api-Request-Id` | `{"$ref":"taskId"}` |
| `poll.body.task_id` | `{"$ref":"taskId"}` |

## Provider 扩展键

- 无额外扩展键。

动态模型或工作流允许使用文档声明的完整 `parameters/input/extra_body` 对象；该对象是协议本身的开放 schema，不会被宿主裁剪。

## 响应映射逐字段清单

| 映射位置 | 上游路径或转换表达式 |
| --- | --- |
| `response.taskId` | `{"$coalesce":[{"$ref":"response.data.task_id"},{"$ref":"taskId"}]}` |
| `response.status` | `{"$if":{"condition":{"$ne":[{"$ref":"response.code"},20000000]},"then":"failed","else":{"$if":{"condition":{"$eq":[{"$ref":"response.data.task_status"},2]},"then":"succeeded","else":{"$if":{"condition":{"$eq":[{"$ref":"response.data.task_status"},3]},"then":"failed","else":{"$if":{"condition":{"$eq":[{"$ref":"response.data.task_status"},1]},"then":"processing","else":"pending"}}}}}}}}` |
| `response.message` | `{"$if":{"condition":{"$eq":[{"$ref":"response.data.task_status"},3]},"then":{"$if":{"condition":{"$in":[{"$lower":{"$coalesce":[{"$ref":"response.message"},""]}},["","ok"]]},"then":"语音合成任务失败","else":{"$ref":"response.message"}}},"else":{"$coalesce":[{"$ref":"response.message"},{"$ref":"message"}]}}}` |
| `response.audios` | `{"$if":{"condition":{"$ref":"response.data.audio_url"},"then":{"url":{"$ref":"response.data.audio_url"},"ephemeral":true},"else":null}}` |
| `response.usage.characters` | `{"$ref":"response.data.synthesize_text_length"}` |
| `response.resultEphemeral` | `true` |

## 响应与错误

插件把上游 task/status/text/media/usage 映射为统一结果。临时媒体 URL 标记为 ephemeral，由宿主立即下载持久化。HTTP 错误、业务 code 和 error object 保持失败语义，不包装成成功。

## 兼容边界

豆包异步长文本语音合成。POST /api/v3/tts/submit 提交任务，task_id 由 unique_id（宿主幂等键）回传；POST /api/v3/tts/query 查询，task_status：1 处理中、2 成功、3 失败，成功后返回 audio_url（有效期 1 小时）。单次最多 10 万字符，音频在服务端保存 7 天。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "doubao-voice-tts-2-0-async",
  "name": "豆包语音合成 2.0（异步）",
  "version": "2.0.0",
  "author": "Volcengine / 影策",
  "description": "豆包语音合成 2.0（异步） 独立请求协议插件。",
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
        "id": "doubao-voice-tts-2-0-async",
        "label": "豆包语音合成 2.0（异步）",
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
        "baseUrl": "https://openspeech.bytedance.com",
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
            "mapping": "X-Api-Resource-Id",
            "description": "资源 ID：seed-tts-2.0（语音合成大模型 2.0）或 seed-icl-2.0（声音复刻 2.0）。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "req_params.text",
            "description": "待朗读文本，最长 10 万字符。"
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
            "message": "豆包语音合成 2.0（异步）需要填写音色 ID"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/api/v3/tts/submit",
          "contentType": "application/json",
          "headers": {
            "X-Api-Resource-Id": {
              "$coalesce": [
                {
                  "$ref": "request.model"
                },
                "seed-tts-2.0"
              ]
            },
            "X-Api-Request-Id": {
              "$ref": "request.extra.idempotencyKey"
            }
          },
          "body": {
            "user": {
              "uid": "yingce"
            },
            "unique_id": {
              "$ref": "request.extra.idempotencyKey"
            },
            "req_params": {
              "text": {
                "$ref": "request.prompt"
              },
              "speaker": {
                "$ref": "request.extra.audioVoice"
              },
              "audio_params": {
                "format": {
                  "$coalesce": [
                    {
                      "$ref": "request.extra.audioFormat"
                    },
                    "mp3"
                  ]
                },
                "sample_rate": {
                  "$if": {
                    "condition": {
                      "$eq": [
                        {
                          "$ref": "request.extra.audioFormat"
                        },
                        "ogg_opus"
                      ]
                    },
                    "then": 48000,
                    "else": 24000
                  }
                },
                "speech_rate": {
                  "$add": [
                    {
                      "$multiply": [
                        {
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
                        100
                      ]
                    },
                    -100
                  ]
                },
                "loudness_rate": 0,
                "enable_timestamp": false
              }
            }
          }
        },
        "poll": {
          "method": "POST",
          "path": "/api/v3/tts/query",
          "contentType": "application/json",
          "headers": {
            "X-Api-Resource-Id": {
              "$coalesce": [
                {
                  "$ref": "request.model"
                },
                "seed-tts-2.0"
              ]
            },
            "X-Api-Request-Id": {
              "$ref": "taskId"
            }
          },
          "body": {
            "task_id": {
              "$ref": "taskId"
            }
          }
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.data.task_id"
              },
              {
                "$ref": "taskId"
              }
            ]
          },
          "status": {
            "$if": {
              "condition": {
                "$ne": [
                  {
                    "$ref": "response.code"
                  },
                  20000000
                ]
              },
              "then": "failed",
              "else": {
                "$if": {
                  "condition": {
                    "$eq": [
                      {
                        "$ref": "response.data.task_status"
                      },
                      2
                    ]
                  },
                  "then": "succeeded",
                  "else": {
                    "$if": {
                      "condition": {
                        "$eq": [
                          {
                            "$ref": "response.data.task_status"
                          },
                          3
                        ]
                      },
                      "then": "failed",
                      "else": {
                        "$if": {
                          "condition": {
                            "$eq": [
                              {
                                "$ref": "response.data.task_status"
                              },
                              1
                            ]
                          },
                          "then": "processing",
                          "else": "pending"
                        }
                      }
                    }
                  }
                }
              }
            }
          },
          "message": {
            "$if": {
              "condition": {
                "$eq": [
                  {
                    "$ref": "response.data.task_status"
                  },
                  3
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
                              "$ref": "response.message"
                            },
                            ""
                          ]
                        }
                      },
                      [
                        "",
                        "ok"
                      ]
                    ]
                  },
                  "then": "语音合成任务失败",
                  "else": {
                    "$ref": "response.message"
                  }
                }
              },
              "else": {
                "$coalesce": [
                  {
                    "$ref": "response.message"
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
                "$ref": "response.data.audio_url"
              },
              "then": {
                "url": {
                  "$ref": "response.data.audio_url"
                },
                "ephemeral": true
              },
              "else": null
            }
          },
          "usage": {
            "characters": {
              "$ref": "response.data.synthesize_text_length"
            }
          },
          "resultEphemeral": true
        }
      }
    ]
  }
}
```
<!-- YINGCE_MANIFEST_CONTRACT_END -->
