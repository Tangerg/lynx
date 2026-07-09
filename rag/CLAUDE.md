# CLAUDE.md — rag module

> 小接口 + 组合函数的 RAG 基础库:不提供固定 Pipeline,调用方用 Retriever 作窄腰,通过组合函数显式拼出需要的能力。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。具体 contract / adapter / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **RAG 是一组小接口的自由组合,不是一条框架化流水线**:contracts(转换 / 扩展 / 检索 / 精炼 / 增强)各是一个小接口,调用方用 Go 组合函数拼装。
- **adapter 在根包以具体名字暴露**:同一 RAG 域的东西先放根包,不按猜测的结构预先分包。

## 架构心智

- **Retriever 是窄腰**:围绕它用组合函数(叠加 transformer / expander / refiner)显式表达能力,而非用一个大 Config 描述整条 pipeline。
- **组合用函数,不用框架式配置**:没有 PipelineConfig / Pipeline 这类中心配置对象。
- **单包优先**:同一 RAG 域先放根包、用具体类型名表达职责,不预先拆 `rag/vectorstore`、`rag/llm` 之类子包。
- **只有 fan-out 检索并行**:多路检索 / query 扩展并发收集;transform / refine 是明确的顺序步骤。
- **Query 的 per-call metadata 走 Extra**:filter / history / tenant 等上下文跨组件传递靠它。

## 模块特有反向不变量

- ❌ **恢复 PipelineConfig / Pipeline** —— 组合用 Go 函数完成,不引框架式中心配置。
- ❌ **加 QueryRouter / DocumentJoiner 之类固定阶段** —— 路由写成自定义 Retriever,合并写成 Refiner。
- ❌ **把根包拆回 `rag/vectorstore`、`rag/llm`、`rag/ragchat`** —— 单包 + 具体命名即可。
- ❌ **为能力加大 Config / Builder** —— 小接口 + 函数组合优先,只有真实可选项才进 Config。

## 改动前必看(波及面)

- **加新能力**:先问"是否属于 RAG 域" —— 属于就放根包,除非它明显是独立的底层通用库。
- **加 concrete adapter**:用普通 struct + 具体构造名,只有真实可选项才进 Config。
