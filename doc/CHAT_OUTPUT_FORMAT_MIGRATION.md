# Chat OutputFormat 迁移

本次变更是 breaking change，不提供旧 API 别名。目标是让格式意图在 Core 只建模一次，由
provider adapter 映射原生协议，并让同步与流式结果复用同一个 decoder。

## Core 请求

provider-specific 的 `response_format`、`format`、`output_config.format` 等扩展字段改为：

```go
format, err := chat.NewJSONSchemaOutputFormat("answer", schema)
request.Options.OutputFormat = &format
```

无 Schema JSON 使用 `chat.NewOutputFormat(chat.OutputFormatJSON)`；文本使用
`chat.NewOutputFormat(chat.OutputFormatText)`。`OutputFormat` 是深拷贝、可校验的充血值，
JSON Schema 的 name/schema 不变量由它自己维护。

provider adapter 的映射如下：

| Provider API | 原生字段 |
|---|---|
| OpenAI Chat Completions | `response_format` |
| OpenAI Responses | `text.format` |
| Anthropic Messages | `output_config.format` |
| AWS Bedrock Converse | `outputConfig.textFormat` |
| Google GenerateContent | `response_mime_type` + `response_json_schema` |
| Ollama | `format` |

adapter 优先使用原生字段；只有目标协议不能表达请求格式时才注入统一 prompt fallback。
同一字段若同时从 extension 提供会直接报错，避免双重事实来源。

## Typed decode

删除 `CallStructured` 等额外调用面，改为拥有 contract 与 decoder 的
`chatclient.OutputFormat[T]`：

```go
format := chatclient.JSON[Answer]()
request.Options.OutputFormat = format.Contract()

// 非流式
answer, err := format.Decode(chatclient.Once(client.Call(ctx, request)))

// 流式
answer, err = format.Decode(client.Stream(ctx, request))
```

`Once` 只把一个完整响应提升成单元素序列；两种调用最终经过同一套累积、JSON 提取、
校验和 decode 逻辑，不再维护同步/流式两份实现。
