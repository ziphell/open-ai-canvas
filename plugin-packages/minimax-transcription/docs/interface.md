# MiniMax 语音识别 接口字段

## 协议身份

- 插件 ID：`minimax-transcription`。
- Provider ID：`minimax-transcription`。
- 能力：`transcription`。
- 默认 Base URL：`https://api.minimax.cn`。
- 鉴权驱动：`bearer`。
- 创建：`POST /v1/speech_to_text`。
- 生命周期：同步响应。

## 配置字段

| 字段 | 类型 | 必填 | 含义 |
| --- | --- | --- | --- |
| `apiKey` | secret | 是 | API Key |

## 统一字段映射

| 统一字段 | 类型 | 必填 | 上游映射 | 说明 |
| --- | --- | --- | --- | --- |
| `model` | string | 否 | `model` | 转写模型 ID，默认 asr-1.0。 |
| `audios` | media[] | 是 | `file` | 待转写音频文件，作为 multipart file 上传。 |
| `prompt` | string | 否 | `language 请求头` | 可选语言代码，例如 zh、yue、en；留空启用混合语言识别。 |
| `providerOptions` | object | 否 | `provider-specific fields` | 插件命名空间内的厂商扩展字段。 |

## 上游请求模板逐字段清单

下表由插件请求模板生成，覆盖 body、query、headers 和 multipart 文件声明中的每个字段。

| 上游位置 | 值或转换表达式 |
| --- | --- |
| `create.method` | `"POST"` |
| `create.path` | `"/v1/speech_to_text"` |
| `create.contentType` | `"multipart/form-data"` |
| `create.headers.language` | `{"$omitEmpty":{"$coalesce":[{"$ref":"request.extra.language"},{"$ref":"request.prompt"}]}}` |
| `create.body.model` | `{"$coalesce":[{"$ref":"request.model"},"asr-1.0"]}` |
| `create.body.response_format` | `"verbose_json"` |
| `create.body.timestamp_level` | `"sentence"` |
| `create.files[0].name` | `"file"` |
| `create.files[0].source` | `{"$first":{"$ref":"request.audios"}}` |
| `create.files[0].filename` | `"audio.wav"` |
| `create.files[0].mimeType` | `"audio/wav"` |

## Provider 扩展键

- 无额外扩展键。

动态模型或工作流允许使用文档声明的完整 `parameters/input/extra_body` 对象；该对象是协议本身的开放 schema，不会被宿主裁剪。

## 响应映射逐字段清单

| 映射位置 | 上游路径或转换表达式 |
| --- | --- |
| `response.transcription.segments` | `{"$map":{"from":{"$ref":"response.segments"},"as":"s","in":{"start":{"$ref":"s.start"},"end":{"$ref":"s.end"},"text":{"$ref":"s.text"}}}}` |
| `response.transcription.language` | `{"$ref":"request.extra.language"}` |
| `response.errorPaths[0]` | `"base_resp.status_code"` |
| `response.errorPaths[1]` | `"error.code"` |
| `response.errorPaths[2]` | `"error"` |
| `response.messagePaths[0]` | `"base_resp.status_msg"` |
| `response.messagePaths[1]` | `"error.message"` |
| `response.messagePaths[2]` | `"message"` |

## 响应与错误

插件把上游 task/status/text/media/usage 映射为统一结果。临时媒体 URL 标记为 ephemeral，由宿主立即下载持久化。HTTP 错误、业务 code 和 error object 保持失败语义，不包装成成功。

## 兼容边界

MiniMax POST /v1/speech_to_text 同步语音识别协议。multipart 提交 model、file 与 response_format=verbose_json，timestamp_level=sentence；language 通过请求头发送。音频支持 wav、aiff、flac、m4a、mp3、aac、opus、ogg，单文件不超过 500 秒和 50 MB；超过限制时上游返回 400/413。verbose_json 响应返回 text、duration（秒）、n_speakers 和 segments[]（start/end 秒、speaker、text）。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "minimax-transcription",
  "name": "MiniMax 语音识别",
  "version": "2.0.0",
  "author": "MiniMax / 影策",
  "description": "MiniMax 语音识别 独立请求协议插件。",
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
        "id": "minimax-transcription",
        "label": "MiniMax 语音识别",
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
            "required": false,
            "mapping": "model",
            "description": "转写模型 ID，默认 asr-1.0。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": true,
            "mapping": "file",
            "description": "待转写音频文件，作为 multipart file 上传。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": false,
            "mapping": "language 请求头",
            "description": "可选语言代码，例如 zh、yue、en；留空启用混合语言识别。"
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
            "message": "MiniMax 语音识别需要连接音视频文件"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v1/speech_to_text",
          "contentType": "multipart/form-data",
          "headers": {
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
          },
          "body": {
            "model": {
              "$coalesce": [
                {
                  "$ref": "request.model"
                },
                "asr-1.0"
              ]
            },
            "response_format": "verbose_json",
            "timestamp_level": "sentence"
          },
          "files": [
            {
              "name": "file",
              "source": {
                "$first": {
                  "$ref": "request.audios"
                }
              },
              "filename": "audio.wav",
              "mimeType": "audio/wav"
            }
          ]
        },
        "response": {
          "transcription": {
            "segments": {
              "$map": {
                "from": {
                  "$ref": "response.segments"
                },
                "as": "s",
                "in": {
                  "start": {
                    "$ref": "s.start"
                  },
                  "end": {
                    "$ref": "s.end"
                  },
                  "text": {
                    "$ref": "s.text"
                  }
                }
              }
            },
            "language": {
              "$ref": "request.extra.language"
            }
          },
          "errorPaths": [
            "base_resp.status_code",
            "error.code",
            "error"
          ],
          "messagePaths": [
            "base_resp.status_msg",
            "error.message",
            "message"
          ]
        }
      }
    ]
  }
}
```
<!-- YINGCE_MANIFEST_CONTRACT_END -->
