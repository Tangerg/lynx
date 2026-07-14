# CLAUDE.md — models module

> LLM / embedding / image / audio 各家 provider 的统一适配层:每个 provider 一个独立子包,全部实现 core 定义的 Model 接口。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。provider 名录 / 成熟度 / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **把各家异构 SDK 收敛成 core 的统一 Model 接口**:上层只见 `chat.Model` 等契约,看不到某一家 SDK 的形状。
- **加 provider = 复制现成结构 + 换 SDK 调用**,绝不为某一家改 lynx 协议。

## 架构心智

- **每 provider 一个子包,固定三件套**:Config(校验 + 工厂)、Model(实现 core 接口)、request/response helper(消息格式转换 + 结果累积)。原生 adapter 使用 `NewChat`；兼容 facade 使用 `NewOpenAIChat`/`NewAnthropicChat`。形状一致,**但 provider 映射不共享**。
- **不抽公共基类**:各家 SDK 的 shape 差异大于相似度,强抽 helper 是虚假 DRY —— 宁可每家重复。
- **适配策略分几档**(靠这个判断新 provider 落哪档):原生跟自家 SDK / 委托 OpenAI 客户端改 BaseURL / 一个 provider 同时暴露 OpenAI 与 Anthropic 两种 API / 托管平台走 IAM(无 API key)/ 本地容器。
- **两级 options 合并**:模型默认 + 请求级叠加;provider 专属参数走类型化提取器,不手动 type-assert。
- **流式逐事件累积**:每 provider 自己的 accumulator 把 SSE delta 拼成 chunk,上层再 stitch 成完整消息 —— 用 `iter.Seq2`,不用 channel。
- **能力差异按 provider 填空**:reasoning signature(续流必需)有的家有、有的没有,适配层用中性字节承载,不强求统一。

## 模块特有反向不变量

- ❌ **在 provider 之间共享 request/response helper** —— shape 差异 > 相似度,共享 = 虚假 DRY。
- ❌ **加 retry layer** —— SDK 自带重试(见 root 共用反向不变量)。
- ❌ **给 provider 加 OAuth / token refresh** —— 用户填 key,401 让 UI 提示重填。
- ❌ **把 defaults/metadata 伪装成 Model 能力** —— `core/chat.Model` 只有 `Call`；默认值由 provider 构造配置持有，per-request override 使用普通 `chat.Options` 值。

## 改动前必看(波及面)

- **动 core 的 chat / Model 接口**:全部 provider 都要跟,先估适配成本再动。
- **加新 provider**:拿最完整的那家当 reference,复制三件套,别改形状。
- **改流式累积逻辑**:每家一份,跑对应 provider 的 stream 测试。
