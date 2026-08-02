# Tokenizer 实现模块迁移

`github.com/Tangerg/lynx/tokenizer` 现在是只依赖标准库的能力契约模块。
原先内嵌的 tiktoken adapter 已成为独立模块：
`github.com/Tangerg/lynx/tokenizer/tiktoken`。

Go import path 和公开 API 没有变化，但模块依赖必须按实际使用声明：

- 只使用 `TextEstimator` / `Encoder` / `Decoder` / `Tokenizer` 接口的模块，只 require
  `github.com/Tangerg/lynx/tokenizer`；
- 直接构造 `tiktoken.Tokenizer` 的模块，另行 require
  `github.com/Tangerg/lynx/tokenizer/tiktoken`。

新的单向依赖为：具体 tiktoken 实现 → tokenizer SPI → stdlib。SPI 不再把词表库、正则引擎
或 UUID 实现传递给只需要接口的 provider 与 document pipeline。
