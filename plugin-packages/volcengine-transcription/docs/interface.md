# 火山引擎录音文件识别 接口字段

## 协议身份

- 插件 ID：`volcengine-transcription`。
- Provider ID：`volcengine-transcription`。
- 能力：`transcription`。
- 默认 Base URL：`https://openspeech.bytedance.com`。
- 鉴权驱动：`header`。
- 创建：`POST /api/v3/auc/bigmodel/submit`。
- 查询：`POST /api/v3/auc/bigmodel/query`。

## 配置字段

| 字段 | 类型 | 必填 | 含义 |
| --- | --- | --- | --- |
| `apiKey` | secret | 是 | API Key |

## 统一字段映射

| 统一字段 | 类型 | 必填 | 上游映射 | 说明 |
| --- | --- | --- | --- | --- |
| `model` | string | 否 | `X-Api-Resource-Id` | 转写模型 ID，默认 volc.seedasr.auc（豆包录音文件识别模型 2.0）。 |
| `audios` | media[] | 是 | `audio.url` | 待转写音频的公网地址，支持 wav、mp3、ogg、aac、m4a 等格式。 |
| `prompt` | string | 否 | `language` | 可选语言代码，例如 zh-CN、en-US。 |
| `providerOptions` | object | 否 | `provider-specific fields` | 插件命名空间内的厂商扩展字段。 |

## 上游请求模板逐字段清单

下表由插件请求模板生成，覆盖 body、query、headers 和 multipart 文件声明中的每个字段。

| 上游位置 | 值或转换表达式 |
| --- | --- |
| `create.method` | `"POST"` |
| `create.path` | `"/api/v3/auc/bigmodel/submit"` |
| `create.contentType` | `"application/json"` |
| `create.headers.X-Api-Resource-Id` | `{"$coalesce":[{"$ref":"request.model"},"volc.seedasr.auc"]}` |
| `create.headers.X-Api-Request-Id` | `{"$ref":"request.extra.idempotencyKey"}` |
| `create.headers.X-Api-Sequence` | `"-1"` |
| `create.body.audio.url` | `{"$ref":"request.audios.0.url"}` |
| `create.body.audio.format` | `{"$coalesce":[{"$ref":"request.extra.audioFormat"},"wav"]}` |
| `create.body.request.model_name` | `"bigmodel"` |
| `create.body.request.show_utterances` | `true` |
| `create.body.request.language` | `{"$omitEmpty":{"$coalesce":[{"$ref":"request.extra.language"},{"$ref":"request.prompt"}]}}` |
| `poll.method` | `"POST"` |
| `poll.path` | `"/api/v3/auc/bigmodel/query"` |
| `poll.contentType` | `"application/json"` |
| `poll.headers.X-Api-Resource-Id` | `{"$coalesce":[{"$ref":"request.model"},"volc.seedasr.auc"]}` |
| `poll.headers.X-Api-Request-Id` | `{"$ref":"taskId"}` |
| `poll.body` | `{}` |

## Provider 扩展键

- 无额外扩展键。

动态模型或工作流允许使用文档声明的完整 `parameters/input/extra_body` 对象；该对象是协议本身的开放 schema，不会被宿主裁剪。

## 响应映射逐字段清单

| 映射位置 | 上游路径或转换表达式 |
| --- | --- |
| `response.headers.statusCode` | `"X-Api-Status-Code"` |
| `response.headers.message` | `"X-Api-Message"` |
| `response.taskId` | `{"$ref":"request.extra.idempotencyKey"}` |
| `response.status` | `{"$if":{"condition":{"$eq":[{"$ref":"headers.statusCode"},"20000000"]},"then":"succeeded","else":{"$if":{"condition":{"$in":[{"$ref":"headers.statusCode"},["20000001","20000002"]]},"then":"processing","else":{"$if":{"condition":{"$eq":[{"$ref":"headers.statusCode"},""]},"then":"pending","else":"failed"}}}}}}` |
| `response.message` | `{"$if":{"condition":{"$eq":[{"$ref":"headers.statusCode"},"20000003"]},"then":"未检测到人声","else":{"$ref":"headers.message"}}}` |
| `response.transcription.segments` | `{"$map":{"from":{"$ref":"response.result.utterances"},"as":"u","in":{"startMs":{"$ref":"u.start_time"},"endMs":{"$ref":"u.end_time"},"text":{"$ref":"u.text"}}}}` |
| `response.transcription.language` | `{"$ref":"request.extra.language"}` |
| `response.errorPaths[0]` | `"error.code"` |
| `response.messagePaths[0]` | `"message"` |

## 响应与错误

插件把上游 task/status/text/media/usage 映射为统一结果。临时媒体 URL 标记为 ephemeral，由宿主立即下载持久化。HTTP 错误、业务 code 和 error object 保持失败语义，不包装成成功。

## 兼容边界

火山引擎录音文件识别标准版 HTTP 异步协议。提交 POST /api/v3/auc/bigmodel/submit，音频必须是公网 URL；查询 POST /api/v3/auc/bigmodel/query，提交和查询共用 X-Api-Request-Id。业务状态码在响应头 X-Api-Status-Code：20000000 成功、20000001 处理中、20000002 排队中，其余失败；成功响应的 result.utterances[].start_time/end_time 为毫秒。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "volcengine-transcription",
  "name": "火山引擎录音文件识别",
  "version": "2.0.0",
  "author": "Volcengine / 影策",
  "description": "火山引擎录音文件识别 独立请求协议插件。",
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
        "id": "volcengine-transcription",
        "label": "火山引擎录音文件识别",
        "capabilities": [
          "transcription"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://openspeech.bytedance.com",
        "requiresPublicMediaUrls": true,
        "auth": {
          "type": "header",
          "field": "apiKey",
          "header": "X-Api-Key"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": false,
            "mapping": "X-Api-Resource-Id",
            "description": "转写模型 ID，默认 volc.seedasr.auc（豆包录音文件识别模型 2.0）。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": true,
            "mapping": "audio.url",
            "description": "待转写音频的公网地址，支持 wav、mp3、ogg、aac、m4a 等格式。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": false,
            "mapping": "language",
            "description": "可选语言代码，例如 zh-CN、en-US。"
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
              "$gt": [
                {
                  "$len": {
                    "$ref": "request.audios"
                  }
                },
                0
              ]
            },
            "message": "火山引擎转写需要连接音视频文件"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/api/v3/auc/bigmodel/submit",
          "contentType": "application/json",
          "headers": {
            "X-Api-Resource-Id": {
              "$coalesce": [
                {
                  "$ref": "request.model"
                },
                "volc.seedasr.auc"
              ]
            },
            "X-Api-Request-Id": {
              "$ref": "request.extra.idempotencyKey"
            },
            "X-Api-Sequence": "-1"
          },
          "body": {
            "audio": {
              "url": {
                "$ref": "request.audios.0.url"
              },
              "format": {
                "$coalesce": [
                  {
                    "$ref": "request.extra.audioFormat"
                  },
                  "wav"
                ]
              }
            },
            "request": {
              "model_name": "bigmodel",
              "show_utterances": true,
              "language": {
                "$omitEmpty": {
                  "$coalesce": [
                    {
                      "$ref": "request.extra.language"
                    },
                    {
                      "$ref": "request.prompt"
                    }
                  ]
                }
              }
            }
          }
        },
        "poll": {
          "method": "POST",
          "path": "/api/v3/auc/bigmodel/query",
          "contentType": "application/json",
          "headers": {
            "X-Api-Resource-Id": {
              "$coalesce": [
                {
                  "$ref": "request.model"
                },
                "volc.seedasr.auc"
              ]
            },
            "X-Api-Request-Id": {
              "$ref": "taskId"
            }
          },
          "body": {}
        },
        "response": {
          "headers": {
            "statusCode": "X-Api-Status-Code",
            "message": "X-Api-Message"
          },
          "taskId": {
            "$ref": "request.extra.idempotencyKey"
          },
          "status": {
            "$if": {
              "condition": {
                "$eq": [
                  {
                    "$ref": "headers.statusCode"
                  },
                  "20000000"
                ]
              },
              "then": "succeeded",
              "else": {
                "$if": {
                  "condition": {
                    "$in": [
                      {
                        "$ref": "headers.statusCode"
                      },
                      [
                        "20000001",
                        "20000002"
                      ]
                    ]
                  },
                  "then": "processing",
                  "else": {
                    "$if": {
                      "condition": {
                        "$eq": [
                          {
                            "$ref": "headers.statusCode"
                          },
                          ""
                        ]
                      },
                      "then": "pending",
                      "else": "failed"
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
                    "$ref": "headers.statusCode"
                  },
                  "20000003"
                ]
              },
              "then": "未检测到人声",
              "else": {
                "$ref": "headers.message"
              }
            }
          },
          "transcription": {
            "segments": {
              "$map": {
                "from": {
                  "$ref": "response.result.utterances"
                },
                "as": "u",
                "in": {
                  "startMs": {
                    "$ref": "u.start_time"
                  },
                  "endMs": {
                    "$ref": "u.end_time"
                  },
                  "text": {
                    "$ref": "u.text"
                  }
                }
              }
            },
            "language": {
              "$ref": "request.extra.language"
            }
          },
          "errorPaths": [
            "error.code"
          ],
          "messagePaths": [
            "message"
          ]
        }
      }
    ]
  }
}
```
<!-- YINGCE_MANIFEST_CONTRACT_END -->
