# CLAUDE.md — historystores provider family

> `core/history` 拥有基于 `core/chat.Message` 的 contract、middleware 与内存参考实现；本目录只容纳外部持久化 provider。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。后端名录 / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **一个 `core/history.Store` 能力模型，多个可组合实现**：上层只依赖所需的 Reader/Writer/Clearer 等小接口。
- 每个外部 provider 位于 `historystores/<provider>` 独立叶子 module；本目录不存在聚合 module，数据库 SDK 和 OTel 不得跨 provider 扩散。

## 架构心智

- **canonical JSON envelope**:所有数据只读写当前 `core/chat.Message` tagged wire；历史数据迁移由应用显式执行，不在库内保留兼容分支。
- **按 conversation_id 分区**:每个会话独立查询路径,避免跨会话扫表。
- **顺序靠单调序号 / 列表追加,不靠时间戳**:高并发下 timestamp 排序会乱。
- **schema 初始化是显式开关**:production 通常预先 migrate,关掉自动建表。
- **自定义表名必过 SQL identifier 校验**:防注入 —— 信任边界在此。

## 模块特有反向不变量

- ❌ **跨后端数据迁移工具** —— 是 ops 的事,不是 SDK 的职责。
- ❌ **在本模块写 schema migration** —— schema 由调用方 migrate,本模块只约定形状。
- ❌ **把数据库或可观测性依赖带回 Core** —— provider 自己拥有持久化机制；OTel 只由 `otel` module 在组合根显式装饰。

## 改动前必看(波及面)

- **动 `chat.Message` 序列化**:所有 provider 的本地持久化边界必须同步并保留当前 wire 测试。
- **加新后端**：在 `historystores/<backend>` 建独立 module，实现 `core/history` 的真实能力，按 conversation_id 分区，禁止导入兄弟 provider。
