# CLAUDE.md — models provider family

> LLM / embedding / image / audio 各家 provider 的统一适配层:每个 provider 一个独立子包,全部实现 core 定义的 Model 接口。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。provider 名录 / 成熟度 / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **把各家异构 SDK 收敛成 core 的统一 Model 接口**:上层只见 `chat.Model` 等契约,看不到某一家 SDK 的形状。
- **`models` 只是命名空间**：不存在聚合 `models` module；每个 `models/<provider>` 都是可独立选择、发布和升级的叶子 module。
- **加 provider = 复制现成结构 + 换 SDK 调用**,绝不为某一家改 scope 协议。

## 架构心智

- **每 provider 一个自洽子包,固定三件套**:Config(校验 + 工厂)、实现 core 接口的 Model、端点/方言组合。原生 adapter 使用 provider 自有 Model；明确兼容同一 OpenAI/Anthropic wire 的端点用 type alias 提升共享 Model，禁止再包一层只转发 `Call`/`Stream` 的空结构体。
- **wire protocol 是更低一层的实现**：跨 provider 复用的 OpenAI 与 Anthropic wire 分别位于独立 `models/protocol/openai`、`models/protocol/anthropic` module。provider 可以向下依赖 protocol，protocol 不能反向依赖 provider；provider 之间禁止互相 import。公开 Config、DTO 和方法签名不得泄露 wire/SDK 类型，但允许像 MCP Go SDK 提升底层消息类型一样，用 alias 提升完全相同的共享可执行 Model。provider-private 的 `internal/protocol` 必须继续由外层 Model 封装。
- **不抽公共基类**:各家 SDK 的 shape 差异大于相似度,强抽 helper 是虚假 DRY —— 宁可每家重复。
- **适配策略分几档**(靠这个判断新 provider 落哪档):原生跟自家 SDK / 委托 OpenAI 客户端改 BaseURL / 一个 provider 同时暴露 OpenAI 与 Anthropic 两种 API / 托管平台走 IAM(无 API key)/ 本地容器。
- **两级 options 解析**:模型默认配置通过 `Options.Resolve` 应用一次请求级覆盖;provider 专属参数走类型化提取器,不手动 type-assert。
- **流式逐事件累积**:原生 provider 或共享 protocol owner 的 accumulator 把 SSE delta 拼成 chunk,上层再 stitch 成完整消息 —— 用 `iter.Seq2`,不用 channel；兼容 provider 不复制同一 wire 的 accumulator。
- **能力差异按 provider 填空**:reasoning signature(续流必需)有的家有、有的没有,适配层用中性字节承载,不强求统一。
- **公共契约测试归契约 owner**：行为套件归 `core/modeltest`，跨 provider 构造/API 一致性归 `dev/providerconformance`；provider module 不复制 conformance 或跨厂商 helper。扩展参数直接用 `core/metadata.Map.Decode`，provider-local transport helper 留在自己的 module。

## 模块特有反向不变量

- ❌ **跨不同 wire protocol 共享 request/response mapper** —— shape 差异 > 相似度,共享 = 虚假 DRY。只有明确宣称兼容同一官方 wire protocol 的端点才复用对应 protocol 实现。
- ❌ **加 retry layer** —— SDK 自带重试(见 root 共用反向不变量)。
- ❌ **给 provider 加 OAuth / token refresh** —— 用户填 key,401 让 UI 提示重填。
- ❌ **把 defaults/metadata 伪装成 Model 能力** —— `core/chat.Model` 只有 `Call`；默认值由 provider 构造配置持有，per-request override 使用普通 `chat.Options` 值。
- ❌ **复制测试基础设施或包一层 metadata 解码** —— 契约 suite/fixture 只有 `core/modeltest` 一份；provider 边界显式解码自己的扩展类型。
- ❌ **为共享 wire Model 造单字段转发壳** —— 兼容 provider 只拥有 Config、校验、端点和 dialect；Model 用 alias 直接提升。只有 `internal/protocol` 或确有 provider 状态/行为时才保留 façade。

## 改动前必看(波及面)

- **动 core 的 chat / Model 接口**:全部 provider 都要跟,先估适配成本再动。
- **加新 provider**:拿最完整的那家当 reference,复制三件套,别改形状。
- **改流式累积逻辑**:每家一份,跑对应 provider 的 stream 测试。
