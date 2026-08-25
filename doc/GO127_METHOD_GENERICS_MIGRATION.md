# Go 1.27 方法泛型迁移

Lynx 全仓模块统一使用 Go 1.27。方法泛型只用于已有领域 owner 能直接承载类型参数的边缘，不把 Core SPI、Agent Kernel 或运行时状态泛型化。

## Breaking changes

Core metadata 解码从自由函数迁到 `metadata.Map`：

```go
// before
value, found, err := metadata.Decode[Options](values, key)

// after
value, found, err := values.Decode[Options](key)
```

Agent typed edge 删除了无生产消费者的 `Typed[I,O]`、`NewTyped`、`ErrInvalidTypedAdapter`、`DecodeInput`、`DecodeOutput` 和 `interaction.DecodeArtifact`。对应 owner 现在直接提供：

```go
input, err := deployment.Descriptor().EncodeInput(request)
value, err := input.Decode[Request]()
result, err := deployment.Descriptor().DecodeOutput[Response](output)
artifactValue, err := artifact.Decode[Evidence]()
```

这些迁移不提供 alias 或兼容 wrapper。Provider adapter 也直接拥有 SDK 结果与流的泛型桥接：OpenAI、Anthropic、Google 通过各自的 `api.wrapResult[T]` / `api.wrapSequence[T]` 转换 provider error，不再把行为留在 owner 旁边的包级泛型函数中。

构造器、DSL 运算符、schema factory 和跨类型 capability projection 没有合法 receiver，继续保留自由函数。
