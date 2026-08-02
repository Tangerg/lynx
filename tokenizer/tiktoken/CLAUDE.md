# CLAUDE.md — tokenizer/tiktoken module

> `tokenizer` SPI 的 tiktoken 具体实现模块。

## 定位

- 只适配 `github.com/pkoukk/tiktoken-go` 到 `tokenizer` 的小接口。
- vocabulary 由调用方显式选择；不存在跨模型正确的默认值。
- 本模块可以依赖底层 `tokenizer`，底层契约不得反向依赖本模块。

## 模块特有反向不变量

- ❌ 放入 provider 请求、模型路由、文本切分或应用缓存策略。
- ❌ 使用 `replace` 让发布构建依赖仓库布局。
