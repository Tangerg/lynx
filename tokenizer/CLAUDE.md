# CLAUDE.md — tokenizer module

> Provider-neutral tokenizer 能力契约。具体词表与第三方 tokenizer 实现属于独立上层模块。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。

## 定位

- **纯 SPI**：只定义 `TextEstimator`、`Encoder`、`Decoder` 与它们的最小组合 `Tokenizer`。
- **只依赖 stdlib**：契约不 import tiktoken、provider SDK、Core 或其他 sibling module。
- **接口由消费语义决定**：只计数的 provider 不需要伪装成可逆编码器。

## 模块特有反向不变量

- ❌ 放入词表、模型名映射、缓存、默认 encoding 或第三方实现。
- ❌ 为便利给小接口增加方法，迫使只需要计数/编码/解码的实现承担无关能力。
- ❌ 通过 `go.work` 或 `replace` 掩盖独立发布依赖。
