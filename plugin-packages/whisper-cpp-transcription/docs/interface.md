# whisper.cpp 转写 接口字段

## 协议身份

- 插件 ID：`whisper-cpp-transcription`。
- Provider ID：`whisper-cpp-transcription`。
- 能力：`transcription`。
- 默认 Base URL：`http://127.0.0.1:8080`。
- 鉴权驱动：`none`。
- 创建：`POST /inference`。
- 生命周期：同步响应。

## 配置字段

| 字段 | 类型 | 必填 | 含义 |
| --- | --- | --- | --- |


## 统一字段映射

| 统一字段 | 类型 | 必填 | 上游映射 | 说明 |
| --- | --- | --- | --- | --- |
| `model` | string | 否 | `model` | 转写模型 ID；本地 whisper.cpp 通常忽略。 |
| `audios` | media[] | 是 | `file` | 待转写的音视频文件，作为 multipart file 上传。 |
| `prompt` | string | 否 | `language` | 可选语言代码，例如 zh、en。 |
| `providerOptions` | object | 否 | `provider-specific fields` | 插件命名空间内的厂商扩展字段。 |

## 上游请求模板逐字段清单

下表由插件请求模板生成，覆盖 body、query、headers 和 multipart 文件声明中的每个字段。

| 上游位置 | 值或转换表达式 |
| --- | --- |
| `create.method` | `"POST"` |
| `create.path` | `"/inference"` |
| `create.contentType` | `"multipart/form-data"` |
| `create.body.response_format` | `"verbose_json"` |
| `create.body.language` | `{"$omitEmpty":{"$coalesce":[{"$ref":"request.extra.language"},{"$ref":"request.prompt"}]}}` |
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
| `response.transcription.segments` | `{"$ref":"response.segments"}` |
| `response.transcription.language` | `{"$ref":"response.language"}` |
| `response.errorPaths[0]` | `"error.code"` |
| `response.errorPaths[1]` | `"error"` |
| `response.messagePaths[0]` | `"error.message"` |
| `response.messagePaths[1]` | `"message"` |

## 响应与错误

插件把上游 task/status/text/media/usage 映射为统一结果。临时媒体 URL 标记为 ephemeral，由宿主立即下载持久化。HTTP 错误、业务 code 和 error object 保持失败语义，不包装成成功。

## 兼容边界

本地 whisper.cpp HTTP 服务协议。POST /inference，multipart 提交 file，response_format=verbose_json；响应 segments[].start/end 为秒、text 为识别文本。服务地址由渠道 Base URL 配置，默认不鉴权；音视频不出本机。若服务部署在本机或私网，需要管理员在 CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS 中显式放行对应主机。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "whisper-cpp-transcription",
  "name": "whisper.cpp 转写",
  "version": "2.0.0",
  "author": "whisper.cpp / 影策",
  "description": "whisper.cpp 转写 独立请求协议插件。",
  "documentation": "<当前插件的完整 documentation，由 README.md 与 docs/interface.md 拼接而成；为避免 JSON 递归，此处不重复展开正文。>",
  "permissions": [
    "generation.run",
    "media.read"
  ],
  "configuration": {
    "fields": []
  },
  "contributes": {
    "providers": [
      {
        "id": "whisper-cpp-transcription",
        "label": "whisper.cpp 转写",
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
        "baseUrl": "http://127.0.0.1:8080",
        "requiresPublicMediaUrls": false,
        "auth": {
          "type": "none",
          "field": "apiKey"
        },
        "parameters": [
          {
            "name": "model",
            "type": "string",
            "required": false,
            "mapping": "model",
            "description": "转写模型 ID；本地 whisper.cpp 通常忽略。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "required": true,
            "mapping": "file",
            "description": "待转写的音视频文件，作为 multipart file 上传。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": false,
            "mapping": "language",
            "description": "可选语言代码，例如 zh、en。"
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
            "message": "whisper.cpp 转写需要连接音视频文件"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/inference",
          "contentType": "multipart/form-data",
          "body": {
            "response_format": "verbose_json",
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
              "$ref": "response.segments"
            },
            "language": {
              "$ref": "response.language"
            }
          },
          "errorPaths": [
            "error.code",
            "error"
          ],
          "messagePaths": [
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
