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
- **只有 fan-out 检索并行**:多路检索 / query 扩展并发收集;transform / refine / evaluation 是明确的顺序步骤。并发结果按声明顺序归并，任一分支失败则本次检索失败，不把不完整结果伪装成完整命中。
- **同一文档身份只占一个检索名额**:相同非空 Document ID 保留最高分候选,同分按首次身份出现稳定决胜;`TopK` 在截断前完成该唯一化,不能让 refiner 顺序决定结果正确性。
- **Query 的 per-call metadata 走类型化 ValueKey**：filter / history / tenant 等上下文通过
  不可变 Query envelope 传递；公开 API 不暴露 string-key `any` map，同名异型会显式报错。
  引用型 value 仍归调用方所有，并行检索时必须只读。
- **evaluation 是独立策略域**:`rag/evaluation` 只依赖最小 Chat Model 和普通 Query/Answer/Context 值，不反向耦合 Document/VectorStore 或固定 RAG pipeline。

## 模块特有反向不变量

- ❌ **恢复 PipelineConfig / Pipeline** —— 组合用 Go 函数完成,不引框架式中心配置。
- ❌ **加 QueryRouter / DocumentJoiner 之类固定阶段** —— 路由写成自定义 Retriever,合并写成 Refiner。
- ❌ **把根包拆回 `rag/vectorstore`、`rag/llm`、`rag/ragchat`** —— 单包 + 具体命名即可；独立的 evaluation 策略域除外。
- ❌ **为能力加大 Config / Builder** —— 小接口 + 函数组合优先,只有真实可选项才进 Config。
- ❌ **让重复候选占用 TopK 或保留低分首次命中** —— 多 Retriever 合并必须保留同 ID 最高分,`Dedup` 与 `TopK` 组合顺序不得改变唯一 Top K。
- ❌ **把 nil/空输出静默降级成 identity** —— 可选能力用显式 Identity/Nop；组合器构造期拒绝 nil，空模型输出、空 expansion 和并发分支错误必须返回。

## 改动前必看(波及面)

- **加新能力**:先问"是否属于 RAG 域" —— 属于就放根包,除非它明显是独立的底层通用库。
- **加 concrete adapter**:用普通 struct + 具体构造名,只有真实可选项才进 Config。
