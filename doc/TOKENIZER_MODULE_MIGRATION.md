# Tokenizer 模块迁移

Tokenizer 的 provider-neutral contract 现在由 stdlib-only 的 Core module 拥有：

```text
github.com/Tangerg/lynx/core/tokenizer
```

tiktoken 词表实现是独立的可选 module：

```text
github.com/Tangerg/lynx/tokenizers/tiktoken
```

这是 breaking import-path change，不保留旧路径 alias：

- 只消费 `TextEstimator`、`Encoder`、`Decoder` 或 `Tokenizer` 的代码 import `core/tokenizer`，并 require `github.com/Tangerg/lynx/core`；
- 直接构造 tiktoken 实现的代码 import `tokenizers/tiktoken`，并额外 require 该实现 module。

依赖方向固定为：具体 tokenizer 实现 → `core/tokenizer` → 标准库。词表、模型名映射和第三方 tokenizer 库不会进入 Core，也不会传递给只需要能力接口的模块。
