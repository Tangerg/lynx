# Spring AI：生态内的模型组合框架

证据基线：`spring-ai` 提交 `e5e277fd08a017c5a3efc57aef377d2a067e91dd`。

## 框架层判断

Spring AI 的首要目标是让模型、ChatClient、Advisor、ToolCallback、Memory、VectorStore 和 RAG 以 Spring 习惯组合。它优化的是企业应用中的集成一致性和声明式扩展，不是独立的 durable Agent process kernel。

把 Spring 的 starter 数量、RAG 组件或生态规模直接与 Scope 内核比较没有意义；本轮只分析其框架组合方式。

## 可复核证据

- `Model`、`StreamingModel` 与 `ChatModel` 形成模型调用抽象。
- `ChatClient` 以 fluent API 组织 prompt、advisor、tool 和返回值。
- `Advisor` 进入有序调用链，并可在调用前后修改请求和响应。
- `ToolCallingAdvisor` 把工具循环实现为可组合 advisor。
- Chat Memory、VectorStore 与 Modular RAG 通过 Spring Bean 和模块协作。
- 可观测性与 Spring Observability 生态结合。

## 八维对照

| 维度 | Spring AI 的实际取舍 | 与 Scope 的关键差异 |
| --- | --- | --- |
| 协议边界 | 统一模型接口，深度遵循 Spring 类型和容器 | Scope 不依赖应用容器，跨宿主更中立 |
| 最小契约 | ChatClient/Model 普通调用非常直接 | Scope 受管 Execution 更重，但另有 client 层 |
| 状态所有权 | Memory、Bean 和应用服务共同持有 | Scope 明确 Host 与执行快照边界 |
| 副作用 | Client、Advisor、ToolCallback 直接执行 | Scope 先生成 Effect 再执行 |
| 编排 | Advisor chain 与应用工作流 | 不提供 Scope 式受管子 Process 树 |
| 恢复 | Memory 与外部存储集成 | 没有统一 execution snapshot/effect replay |
| 扩展 | Ordered Advisor、Bean、Observation | Scope middleware 更独立，生态自动装配较少 |
| 依赖 | starter/module 与 Spring 生态一致 | Scope 叶子模块避免容器绑定，组装更显式 |

## Scope 应该借鉴什么

1. **Advisor 的可发现组合体验。** 模型调用增强、工具循环和观测都能沿一条调用链理解。
2. **面向接口的能力装配。** Memory、VectorStore 和 RAG 是可替换协作者，而不是必须进入 Agent 内核的全局服务。
3. **普通调用优先。** 简单模型调用不需要先建立执行图或持久化 Process。
4. **一致的 observability 约定。** 生态统一的命名和指标比暴露大量低层 hook 更有用。

## Scope 不应照搬什么

- 不应引入容器生命周期或全局 Bean 式依赖解析。
- 不应把 RAG、Memory、VectorStore 等应用能力重新聚合进 Agent 内核。
- 不应使用 Advisor 链替代需要精确身份的 Effect 执行。
- 不应因为自动装配方便而隐藏实际依赖方向。

## 最终定位

Spring AI 在 Spring 应用中提供了更低摩擦的模型与工具组合。Scope 更适合需要宿主无关、可恢复执行语义的系统。两者的核心差别是生态组合与执行内核，而不是语言或功能数量。
