# CLAUDE.md — pkg module

> monorepo 的**工具层基础设施**:generics 集合、并发原语、流式处理、JSON Schema 生成等。core / agent / models / vectorstores 都依赖 pkg,**pkg 不依赖任何业务模块** —— 这是 zero-cycle 的关键护栏。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。子包名录 / 调用方数 / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **纯工具,零业务**:pkg 是 DAG 的底,只放跨业务模块可复用的通用原语,不 import 任何 `core` / `agent` / `models` / `vectorstores`。
- **是被整个 monorepo 依赖的基础层**:改它波及面最大,exported API 改动尤其要慎(见下)。

## 架构心智

- **按职能分组、每子包一个 niche**:集合 / 数据类型 / 并发 / 流式 / 编码 / 基础原语。彼此独立,消费方只拉自己用到的。
- **generics 强制**:集合与通用 API 用类型参数,公开 API 禁 `any` / `interface{}`。
- **iterator-first**:优先 range-over-func 而非 `ForEach(func(T))` —— 调用方拿到 break / 提前退出。
- **流式不缓冲**:增量处理优先;处理不可信输入(LLM 输出、XML / JSON)强制 buffer 上限,防 OOM。
- **小到不必抽的直接放**:极小 helper 不二次封装 stdlib。
- **零消费者的子包是有意的工具箱,不是死代码**:pkg 是通用 toolkit,某些导出暂无消费方是备用,不在 dead-code 清扫里删。

## 模块特有反向不变量

- ❌ **import 任何业务模块** —— 会形成循环,CI 应锁这条。
- ❌ **加业务概念** —— 只放纯工具;带业务语义的分类(如 retry 的 Transient / NonTransient)已被否(见 root 共用反向不变量)。

## 改动前必看(波及面)

- **加新子包**:先问"stdlib 为什么不够" —— 只在 stdlib 真不够或跨业务模块要复用时才加。
- **改 exported API**:pkg 被全 monorepo 依赖,宁可加新函数也别改老签名;**任何破坏性改动先咨询 scope + 影响面**(见 root 强约定)。
- **改 XML / JSON parser 的 buffer 上限**:跑 fuzz,覆盖恶意 LLM 输出。
