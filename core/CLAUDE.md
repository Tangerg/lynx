# CLAUDE.md — core module

> lynx 生态的**协议定义层**:document / model(chat·embedding·image·audio·moderation)/ tokenizer / vectorstore / evaluation 的接口与消息类型都在此定义,具体实现落在外圈模块。这是整个生态的**窄腰**。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。具体类型 / 字段 / 文件布局以代码与 godoc 为准 —— 本则只讲宏观。

---

## 定位

- **接口在 core,实现在外圈**:models / chathistory / vectorstores / documentreaders 各自实现 core 定义的契约。core 只依赖 `pkg`,**不依赖任何业务模块** —— 它是依赖 DAG 的最底层,动它波及全生态。
- **协议层,不是逻辑层**:core 承载可复用的原语与契约(消息形态、handler 骨架、pipeline stage、filter 语言),业务判断留给消费方。

## 架构心智

- **一个泛型骨架,多模态具化**:call / stream 两条 handler 共用同一个泛型骨架,各模态具化自己的 Request / Response —— 加模态靠加类型(OCP),不改骨架。
- **消息是 sealed interface**:Message 家族靠未导出方法封口,保证 type switch 穷尽、外部不能私自加子类型 —— 这是"所有适配器行为一致"的地基。
- **拉模型流式**:streaming 一律走 `iter.Seq2`,不自定义 iterator、不用 channel(ctx 可在每步前检查、无 goroutine 泄漏)。
- **middleware 按 call / stream 显式分流**:各走 typed chain,传错类型在编译期暴露,不用 `any` 路由。
- **可选值用指针表达"未设"**:Options 的可选字段为指针,nil = "用 provider 默认",与零值区分。
- **vectorstore 自带一门 filter mini-language**:parser → AST → analyzer;各后端实现 visitor,把 AST 转成自己的方言。

## 模块特有反向不变量

- ❌ **给 sealed 的 Message 家族加子类型** —— 封口是设计意图,加了会破坏所有穷尽 type switch。
- ❌ **用 channel 取代 `iter.Seq2` 做流式** —— 拉模型是既定标准。
- ❌ **把业务逻辑塞进 core** —— core 是协议层,逻辑属于消费方 / 外圈实现。

## 改动前必看(波及面)

- **动 Message 或 handler 签名**:整个生态的适配器都依赖,爆炸半径 = 全部 models 与消费方。
- **动 vectorstore filter AST**:所有 vectorstores 后端的 visitor 都要同步跟进方言转换。
- **加新模态**:复用现有泛型骨架另起一个 `model/<modality>`,写自己的 Request / Response / Options,不改骨架。
