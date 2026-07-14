# CLAUDE.md — a2a module

> lynx 对 Agent-to-Agent(A2A)协议的**薄适配**:协议 wire、AgentCard、transport、task 生命周期都由官方 A2A SDK 承担,本模块不做第二套。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。具体符号 / SDK 版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **根包放 A2A 周边 helper 与 tool adapter**:resolve AgentCard、打开 client、内容投影、server executor、把远端 agent 折成 `tools.Tool`。
- 双向:既把远端 A2A agent 变成本地可调工具,也把 lynx 的文本流能力暴露成 A2A endpoint。

## 架构心智

- **薄适配,不重写协议状态**:JSON-RPC envelope、SSE、AgentCard schema 一律用官方 SDK,不自造。
- **远端 agent 只以 `[]tools.Tool` 暴露**:批量 resolve 是一个普通函数,生命周期由它返回的 close 函数表达 —— 不公开 SDK client、不做 Provider / cache / Registry。
- **A2A tool 的输入统一为一个 message 字段**:A2A 是消息协议,不是 typed function call。
- **内容投影 text-first**:把 A2A 内容投影成 lynx 的文本优先语义。
- **executor 事件序列必须合法**:提交 → 工作中 → 产出增量 → 完成 / 失败 / 取消;空增量跳过(SDK 会拒空产出)。

## 模块特有反向不变量

- ❌ **自己写协议状态**(JSON-RPC / SSE / AgentCard schema)—— 用官方 SDK。
- ❌ **加 Provider / cache / Registry** —— 批量连接就是普通函数,生命周期靠返回的 close 函数。
- ❌ **按子域拆包** —— 同一 A2A 适配域先放根包。

## 改动前必看(波及面)

- **先看官方 A2A SDK 的接口形状**:本模块只做薄适配,不维护自己的协议状态。
- **多轮 input-required / auth-required**:会改变 server 侧 agent 的形状,必须单独设计,不要顺手塞。
