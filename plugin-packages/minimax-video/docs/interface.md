# MiniMax H3 Video 接口字段

## 入口和生命周期

- 创建：`POST /v2/video_generation`。
- 查询：`GET /v2/query/video_generation/{taskId}`。
- 鉴权：`Authorization: Bearer <apiKey>`。
- 请求：JSON；结果：异步任务，完成后下载并持久化临时 URL。

## 请求字段

| 上游字段 | 类型 | 必填 | 统一来源 | 说明 |
| --- | --- | --- | --- | --- |
| `model` | string | 是 | `request.model` | `MiniMax-H3`、`MiniMax-H3-Highspeed`、`MiniMax-H3-Lite` 或 `MiniMax-H3-Max`。 |
| `content` | block[] | 是 | prompt + inputs | 第一个 block 为 text；媒体 block 依次按 order 输出。 |
| `content[].type` | enum | 是 | media.kind | `text/image_url/video_url/audio_url`。 |
| `content[].text` | string | 文本必填 | `request.prompt` | 不重复发送顶层 prompt。 |
| `content[].image_url.url` | string | 图片 block 必填 | `media.value` | 由宿主保证公网 URL。 |
| `content[].video_url.url` | string | 视频 block 必填 | `media.value` | 参考视频。 |
| `content[].audio_url.url` | string | 音频 block 必填 | `media.value` | 参考音频/声音。 |
| `content[].role` | string | 媒体必填 | `media.role` | `first_frame/last_frame/reference_image/reference_video/reference_audio/reference_voice`。 |
| `images` | media[] | 否 | image content blocks | 图片按显式 role 与 order 映射，不使用数组下标猜测；首尾帧与参考图不能混用。 |
| `videos` | media[] | 否 | video content blocks | 参考视频按 role 与 order 映射，仅用于多模态参考生视频。 |
| `audios` | media[] | 否 | audio content blocks | 参考音频或声音按 role 与 order 映射，仅用于多模态参考生视频。 |
| `duration` | integer | 是 | `request.duration` | 4-15 秒；`MiniMax-H3-Max` 为 5-15 秒。 |
| `resolution` | string | 否 | `request.resolution` | 默认 `768P`；H3 支持 768P、2K，Highspeed/Lite/Max 支持 480P、768P。 |
| `ratio` | string | 否 | `request.aspectRatio` | 文生视频不能使用 `adaptive`，未提供时按 `16:9` 发送；首尾帧恒为 `adaptive`；多模态参考默认 `adaptive`，Lite 需显式指定具体比例。 |
| `aigc_watermark` | boolean | 否 | `request.watermark` | 是否带 AIGC 水印，默认关闭。 |
| `generate_audio` | boolean | 否 | `request.generateAudio` | 是否生成音频；渠道能力默认开启，显式关闭时发送 `false`。 |
| `seed` | integer | 否 | `providerOptions.minimax-video.seed` | 随机种子。 |
| `prompt_optimizer` | boolean | 否 | `providerOptions.minimax-video.prompt_optimizer` | 提示词优化开关。 |
| `extra.prompt_expansion_mode` | string | 否 | `providerOptions.minimax-video.prompt_expansion_mode` | 仅 `MiniMax-H3-Max` 支持，其他模型传入直接失败。 |

## 强校验

- 模型必须是 `MiniMax-H3`、`MiniMax-H3-Highspeed`、`MiniMax-H3-Lite` 或 `MiniMax-H3-Max`。
- 图片最多 9 张、视频最多 3 个、音频最多 3 个。
- 首帧和尾帧各最多 1 张；尾帧不能脱离首帧。
- 图生视频（`first_frame` / `last_frame`）与多模态参考（`reference_image` / `reference_video` / `reference_audio`）互斥，混用直接失败。
- 文生视频不能使用 `adaptive`，未提供比例时按 `16:9` 发送；首尾帧请求强制 `adaptive`。
- duration 超出 4-15 秒直接失败；`MiniMax-H3-Max` 不接受 4 秒。
- 分辨率按模型限制：H3 仅 768P、2K；Highspeed、Lite、Max 仅 480P、768P。

## 中转兼容边界

本插件对应 MiniMax `/v2/video_generation` 原生线协议。NewAPI Channel 1、Channel 2 和自定义 OpenAI Videos 中转必须选择各自独立插件；不能只根据模型名包含 `H3` 自动切协议。宿主只使用轮询查询任务，当前不发送 `callback_url`；创建响应按默认 `response_format=task` 解析，查询同时兼容 `task` 包装与平铺字段。

## 响应映射

- `task.id` / `task_id` / `id` 映射为统一任务 ID；查询响应缺省时保留已有任务 ID。
- `task.status` / `status` 映射为 `pending/processing/succeeded/failed/cancelled`。
- `task.content.url` / `content.url` / `video_url` / `file.url` 映射为统一视频结果。
- 创建和查询响应优先读取 `task.error.message`，再读取 `task.message` / `base_resp.status_msg` / `message` / `error.message`；具体失败原因通过任务错误返回给用户。
- `task.error.code` / `base_resp.status_code` / `error.code` 非成功值均判定为失败，即使响应 HTTP 状态为 200 或未提供任务状态。
- 完成后的临时媒体地址由宿主立即下载并持久化。

例如，参考图片尺寸不符合要求时，上游返回 `task.error.code: "2013"` 和 `task.error.message: "content[1].image_url: media dimensions must be between 256 and 5760 pixels"`。任务应展示这条具体消息；只有上游未提供可读取的错误原因时，才使用通用失败提示。

<!-- YINGCE_MANIFEST_CONTRACT_START -->
## Manifest 完整接口定义

以下 JSON 与插件包内实际 `manifest.json` 逐字段一致，覆盖插件身份、权限、配置、鉴权、参数、校验、创建、Agent、查询、取消、结果下载、响应和 Agent 响应映射。`documentation` 字段的值就是当前完整文档；为避免文档在自身内部无限递归，JSON 中仅用等义占位文本表示正文。

```json
{
  "apiVersion": "yingce.plugin/v2",
  "id": "minimax-video",
  "name": "MiniMax H3 Video",
  "version": "2.2.0",
  "author": "MiniMax / 影策",
  "description": "MiniMax H3 视频生成协议，保留首帧、尾帧、参考图、视频和音频角色语义。",
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
        "id": "minimax-video",
        "label": "MiniMax H3 Video",
        "capabilities": [
          "video"
        ],
        "scopes": [
          "admin.system-channel",
          "user.custom-channel",
          "canvas",
          "creation",
          "agent"
        ],
        "baseUrl": "https://api.minimax.cn",
        "requiresPublicMediaUrls": true,
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
            "description": "MiniMax-H3、MiniMax-H3-Highspeed、MiniMax-H3-Lite 或 MiniMax-H3-Max。"
          },
          {
            "name": "prompt",
            "type": "string",
            "required": true,
            "mapping": "content[type=text].text",
            "description": "仅写入 text content block，避免与顶层 prompt 重复。"
          },
          {
            "name": "images",
            "type": "media[]",
            "mapping": "content[type=image_url]",
            "description": "保留 first_frame、last_frame、reference_image 等 role；首尾帧与参考图不能混用。"
          },
          {
            "name": "videos",
            "type": "media[]",
            "mapping": "content[type=video_url]",
            "description": "映射为 reference_video，仅用于多模态参考生视频。"
          },
          {
            "name": "audios",
            "type": "media[]",
            "mapping": "content[type=audio_url]",
            "description": "映射为 reference_audio/reference_voice，仅用于多模态参考生视频。"
          },
          {
            "name": "duration",
            "type": "integer",
            "required": true,
            "values": [
              "4",
              "5",
              "6",
              "7",
              "8",
              "9",
              "10",
              "11",
              "12",
              "13",
              "14",
              "15"
            ],
            "mapping": "duration",
            "description": "4-15 秒整数（MiniMax-H3-Max 为 5-15 秒）。"
          },
          {
            "name": "resolution",
            "type": "string",
            "mapping": "resolution",
            "description": "MiniMax-H3 支持 768P、2K；Highspeed、Lite、Max 支持 480P、768P。"
          },
          {
            "name": "aspectRatio",
            "type": "string",
            "mapping": "ratio",
            "description": "文生视频必须给出具体比例，未提供时按 16:9 发送；首尾帧恒为 adaptive；多模态参考默认 adaptive，Lite 需指定具体比例。"
          },
          {
            "name": "watermark",
            "type": "boolean",
            "mapping": "aigc_watermark",
            "description": "AIGC 水印，默认关闭。"
          },
          {
            "name": "generateAudio",
            "type": "boolean",
            "mapping": "generate_audio",
            "description": "映射 request.generateAudio；渠道能力默认开启。"
          },
          {
            "name": "seed",
            "type": "integer",
            "mapping": "seed",
            "description": "providerOptions.minimax-video.seed。"
          },
          {
            "name": "prompt_optimizer",
            "type": "boolean",
            "mapping": "prompt_optimizer",
            "description": "providerOptions.minimax-video.prompt_optimizer。"
          },
          {
            "name": "promptExpansionMode",
            "type": "string",
            "mapping": "extra.prompt_expansion_mode",
            "description": "providerOptions.minimax-video.prompt_expansion_mode，仅 MiniMax-H3-Max 支持。"
          }
        ],
        "validations": [
          {
            "assert": {
              "$in": [
                {
                  "$lower": {
                    "$trim": {
                      "$ref": "request.model"
                    }
                  }
                },
                [
                  "minimax-h3",
                  "minimax-h3-highspeed",
                  "minimax-h3-lite",
                  "minimax-h3-max"
                ]
              ]
            },
            "message": "MiniMax 视频仅支持 MiniMax-H3、MiniMax-H3-Highspeed、MiniMax-H3-Lite 或 MiniMax-H3-Max"
          },
          {
            "assert": {
              "$gt": [
                {
                  "$len": {
                    "$trim": {
                      "$ref": "request.prompt"
                    }
                  }
                },
                0
              ]
            },
            "message": "MiniMax 视频提示词不能为空"
          },
          {
            "assert": {
              "$lte": [
                {
                  "$len": {
                    "$ref": "request.images"
                  }
                },
                9
              ]
            },
            "message": "MiniMax H3 最多支持 9 张图片"
          },
          {
            "assert": {
              "$lte": [
                {
                  "$len": {
                    "$ref": "request.videos"
                  }
                },
                3
              ]
            },
            "message": "MiniMax H3 最多支持 3 个参考视频"
          },
          {
            "assert": {
              "$lte": [
                {
                  "$len": {
                    "$ref": "request.audios"
                  }
                },
                3
              ]
            },
            "message": "MiniMax H3 最多支持 3 个参考音频"
          },
          {
            "assert": {
              "$and": [
                {
                  "$gte": [
                    {
                      "$ref": "request.duration"
                    },
                    4
                  ]
                },
                {
                  "$lte": [
                    {
                      "$ref": "request.duration"
                    },
                    15
                  ]
                }
              ]
            },
            "message": "MiniMax H3 duration 必须在 4-15 秒之间"
          },
          {
            "assert": {
              "$or": [
                {
                  "$ne": [
                    {
                      "$lower": {
                        "$trim": {
                          "$ref": "request.model"
                        }
                      }
                    },
                    "minimax-h3-max"
                  ]
                },
                {
                  "$gte": [
                    {
                      "$ref": "request.duration"
                    },
                    5
                  ]
                }
              ]
            },
            "message": "MiniMax-H3-Max duration 必须在 5-15 秒之间"
          },
          {
            "assert": {
              "$or": [
                {
                  "$and": [
                    {
                      "$eq": [
                        {
                          "$lower": {
                            "$trim": {
                              "$ref": "request.model"
                            }
                          }
                        },
                        "minimax-h3"
                      ]
                    },
                    {
                      "$in": [
                        {
                          "$upper": {
                            "$trim": {
                              "$coalesce": [
                                {
                                  "$ref": "request.resolution"
                                },
                                "768P"
                              ]
                            }
                          }
                        },
                        [
                          "768P",
                          "2K"
                        ]
                      ]
                    }
                  ]
                },
                {
                  "$and": [
                    {
                      "$not": {
                        "$eq": [
                          {
                            "$lower": {
                              "$trim": {
                                "$ref": "request.model"
                              }
                            }
                          },
                          "minimax-h3"
                        ]
                      }
                    },
                    {
                      "$in": [
                        {
                          "$upper": {
                            "$trim": {
                              "$coalesce": [
                                {
                                  "$ref": "request.resolution"
                                },
                                "768P"
                              ]
                            }
                          }
                        },
                        [
                          "480P",
                          "768P"
                        ]
                      ]
                    }
                  ]
                }
              ]
            },
            "message": "MiniMax-H3 分辨率仅支持 768P、2K；Highspeed、Lite、Max 仅支持 480P、768P"
          },
          {
            "assert": {
              "$lte": [
                {
                  "$len": {
                    "$filter": {
                      "from": {
                        "$ref": "request.images"
                      },
                      "as": "image",
                      "where": {
                        "$eq": [
                          {
                            "$ref": "image.role"
                          },
                          "first_frame"
                        ]
                      }
                    }
                  }
                },
                1
              ]
            },
            "message": "MiniMax H3 最多只能有一个首帧"
          },
          {
            "assert": {
              "$lte": [
                {
                  "$len": {
                    "$filter": {
                      "from": {
                        "$ref": "request.images"
                      },
                      "as": "image",
                      "where": {
                        "$eq": [
                          {
                            "$ref": "image.role"
                          },
                          "last_frame"
                        ]
                      }
                    }
                  }
                },
                1
              ]
            },
            "message": "MiniMax H3 最多只能有一个尾帧"
          },
          {
            "assert": {
              "$or": [
                {
                  "$eq": [
                    {
                      "$len": {
                        "$filter": {
                          "from": {
                            "$ref": "request.images"
                          },
                          "as": "image",
                          "where": {
                            "$eq": [
                              {
                                "$ref": "image.role"
                              },
                              "last_frame"
                            ]
                          }
                        }
                      }
                    },
                    0
                  ]
                },
                {
                  "$eq": [
                    {
                      "$len": {
                        "$filter": {
                          "from": {
                            "$ref": "request.images"
                          },
                          "as": "image",
                          "where": {
                            "$eq": [
                              {
                                "$ref": "image.role"
                              },
                              "first_frame"
                            ]
                          }
                        }
                      }
                    },
                    1
                  ]
                }
              ]
            },
            "message": "MiniMax H3 使用尾帧时必须同时提供首帧"
          },
          {
            "assert": {
              "$or": [
                {
                  "$eq": [
                    {
                      "$len": {
                        "$filter": {
                          "from": {
                            "$ref": "request.images"
                          },
                          "as": "image",
                          "where": {
                            "$in": [
                              {
                                "$ref": "image.role"
                              },
                              [
                                "first_frame",
                                "last_frame"
                              ]
                            ]
                          }
                        }
                      }
                    },
                    0
                  ]
                },
                {
                  "$eq": [
                    {
                      "$add": [
                        {
                          "$len": {
                            "$ref": "request.videos"
                          }
                        },
                        {
                          "$len": {
                            "$ref": "request.audios"
                          }
                        },
                        {
                          "$len": {
                            "$filter": {
                              "from": {
                                "$ref": "request.images"
                              },
                              "as": "image",
                              "where": {
                                "$in": [
                                  {
                                    "$ref": "image.role"
                                  },
                                  [
                                    "reference_image",
                                    "subject_reference",
                                    "style_reference"
                                  ]
                                ]
                              }
                            }
                          }
                        }
                      ]
                    },
                    0
                  ]
                }
              ]
            },
            "message": "MiniMax 图生视频（首帧/尾帧）与多模态参考（参考图/参考视频/参考音频）不能混用"
          },
          {
            "assert": {
              "$or": [
                {
                  "$eq": [
                    {
                      "$trim": {
                        "$ref": "request.aspectRatio"
                      }
                    },
                    ""
                  ]
                },
                {
                  "$in": [
                    {
                      "$lower": {
                        "$trim": {
                          "$ref": "request.aspectRatio"
                        }
                      }
                    },
                    [
                      "21:9",
                      "16:9",
                      "4:3",
                      "1:1",
                      "3:4",
                      "9:16"
                    ]
                  ]
                }
              ]
            },
            "message": "MiniMax 文生视频不能使用 adaptive，必须指定具体画幅比例；未提供时按 16:9 发送"
          },
          {
            "assert": {
              "$or": [
                {
                  "$not": {
                    "$eq": [
                      {
                        "$lower": {
                          "$trim": {
                            "$ref": "request.model"
                          }
                        }
                      },
                      "minimax-h3-lite"
                    ]
                  }
                },
                {
                  "$gt": [
                    {
                      "$len": {
                        "$filter": {
                          "from": {
                            "$ref": "request.images"
                          },
                          "as": "image",
                          "where": {
                            "$in": [
                              {
                                "$ref": "image.role"
                              },
                              [
                                "first_frame",
                                "last_frame"
                              ]
                            ]
                          }
                        }
                      }
                    },
                    0
                  ]
                },
                {
                  "$in": [
                    {
                      "$lower": {
                        "$trim": {
                          "$ref": "request.aspectRatio"
                        }
                      }
                    },
                    [
                      "21:9",
                      "16:9",
                      "4:3",
                      "1:1",
                      "3:4",
                      "9:16"
                    ]
                  ]
                }
              ]
            },
            "message": "MiniMax-H3-Lite 多模态参考生视频暂不支持 adaptive，请指定具体画幅比例"
          },
          {
            "assert": {
              "$or": [
                {
                  "$eq": [
                    {
                      "$ref": "request.providerOptions.minimax-video.prompt_expansion_mode"
                    },
                    null
                  ]
                },
                {
                  "$eq": [
                    {
                      "$lower": {
                        "$trim": {
                          "$ref": "request.model"
                        }
                      }
                    },
                    "minimax-h3-max"
                  ]
                }
              ]
            },
            "message": "prompt_expansion_mode 仅 MiniMax-H3-Max 支持"
          }
        ],
        "create": {
          "method": "POST",
          "path": "/v2/video_generation",
          "contentType": "application/json",
          "body": {
            "model": {
              "$ref": "request.model"
            },
            "content": {
              "$concatArrays": [
                [
                  {
                    "type": "text",
                    "text": {
                      "$ref": "request.prompt"
                    }
                  }
                ],
                {
                  "$map": {
                    "from": {
                      "$sortByOrder": {
                        "$ref": "request.images"
                      }
                    },
                    "as": "media",
                    "in": {
                      "type": "image_url",
                      "image_url": {
                        "url": {
                          "$ref": "media.value"
                        }
                      },
                      "role": {
                        "$coalesce": [
                          {
                            "$ref": "media.role"
                          },
                          "reference_image"
                        ]
                      }
                    }
                  }
                },
                {
                  "$map": {
                    "from": {
                      "$sortByOrder": {
                        "$ref": "request.videos"
                      }
                    },
                    "as": "media",
                    "in": {
                      "type": "video_url",
                      "video_url": {
                        "url": {
                          "$ref": "media.value"
                        }
                      },
                      "role": {
                        "$coalesce": [
                          {
                            "$ref": "media.role"
                          },
                          "reference_video"
                        ]
                      }
                    }
                  }
                },
                {
                  "$map": {
                    "from": {
                      "$sortByOrder": {
                        "$ref": "request.audios"
                      }
                    },
                    "as": "media",
                    "in": {
                      "type": "audio_url",
                      "audio_url": {
                        "url": {
                          "$ref": "media.value"
                        }
                      },
                      "role": {
                        "$coalesce": [
                          {
                            "$ref": "media.role"
                          },
                          "reference_audio"
                        ]
                      }
                    }
                  }
                }
              ]
            },
            "duration": {
              "$ref": "request.duration"
            },
            "resolution": {
              "$upper": {
                "$trim": {
                  "$coalesce": [
                    {
                      "$ref": "request.resolution"
                    },
                    "768P"
                  ]
                }
              }
            },
            "ratio": {
              "$if": {
                "condition": {
                  "$gt": [
                    {
                      "$len": {
                        "$filter": {
                          "from": {
                            "$ref": "request.images"
                          },
                          "as": "image",
                          "where": {
                            "$in": [
                              {
                                "$ref": "image.role"
                              },
                              [
                                "first_frame",
                                "last_frame"
                              ]
                            ]
                          }
                        }
                      }
                    },
                    0
                  ]
                },
                "then": "adaptive",
                "else": {
                  "$if": {
                    "condition": {
                      "$eq": [
                        {
                          "$add": [
                            {
                              "$len": {
                                "$ref": "request.images"
                              }
                            },
                            {
                              "$len": {
                                "$ref": "request.videos"
                              }
                            },
                            {
                              "$len": {
                                "$ref": "request.audios"
                              }
                            }
                          ]
                        },
                        0
                      ]
                    },
                    "then": {
                      "$coalesce": [
                        {
                          "$ref": "request.aspectRatio"
                        },
                        "16:9"
                      ]
                    },
                    "else": {
                      "$coalesce": [
                        {
                          "$ref": "request.aspectRatio"
                        },
                        {
                          "$if": {
                            "condition": {
                              "$eq": [
                                {
                                  "$lower": {
                                    "$trim": {
                                      "$ref": "request.model"
                                    }
                                  }
                                },
                                "minimax-h3-lite"
                              ]
                            },
                            "then": "16:9",
                            "else": "adaptive"
                          }
                        }
                      ]
                    }
                  }
                }
              }
            },
            "aigc_watermark": {
              "$coalesce": [
                {
                  "$ref": "request.watermark"
                },
                false
              ]
            },
            "generate_audio": {
              "$ref": "request.generateAudio"
            },
            "seed": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.minimax-video.seed"
              }
            },
            "prompt_optimizer": {
              "$omitEmpty": {
                "$ref": "request.providerOptions.minimax-video.prompt_optimizer"
              }
            },
            "extra": {
              "$if": {
                "condition": {
                  "$and": [
                    {
                      "$eq": [
                        {
                          "$lower": {
                            "$trim": {
                              "$ref": "request.model"
                            }
                          }
                        },
                        "minimax-h3-max"
                      ]
                    },
                    {
                      "$ne": [
                        {
                          "$ref": "request.providerOptions.minimax-video.prompt_expansion_mode"
                        },
                        null
                      ]
                    }
                  ]
                },
                "then": {
                  "prompt_expansion_mode": {
                    "$ref": "request.providerOptions.minimax-video.prompt_expansion_mode"
                  }
                },
                "else": null
              }
            }
          }
        },
        "poll": {
          "method": "GET",
          "path": "/v2/query/video_generation/{{taskId}}"
        },
        "response": {
          "taskId": {
            "$coalesce": [
              {
                "$ref": "response.task.id"
              },
              {
                "$ref": "response.task_id"
              },
              {
                "$ref": "response.id"
              },
              {
                "$ref": "taskId"
              }
            ]
          },
          "status": {
            "$coalesce": [
              {
                "$ref": "response.task.status"
              },
              {
                "$ref": "response.status"
              },
              "pending"
            ]
          },
          "message": {
            "$coalesce": [
              {
                "$ref": "response.task.error.message"
              },
              {
                "$ref": "response.task.message"
              },
              {
                "$ref": "response.base_resp.status_msg"
              },
              {
                "$ref": "response.message"
              },
              {
                "$ref": "response.error.message"
              }
            ]
          },
          "videos": {
            "$coalesce": [
              {
                "$ref": "response.task.content.url"
              },
              {
                "$ref": "response.content.url"
              },
              {
                "$ref": "response.video_url"
              },
              {
                "$ref": "response.file.url"
              }
            ]
          },
          "errorPaths": [
            "task.error.code",
            "base_resp.status_code",
            "error.code"
          ],
          "resultEphemeral": true
        }
      }
    ]
  }
}
```
<!-- YINGCE_MANIFEST_CONTRACT_END -->
