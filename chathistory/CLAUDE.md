# CLAUDE.md — chathistory package family

> 基于 `core/chat.Message` 的 history contract、middleware 与 provider packages —— 给上层提供不绑定数据库的可替换存储边界。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。后端名录 / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **一个 `chathistory.Store` 接口,多个可组合实现**(Write / Read / Clear):上层只依赖能力契约。
- 每个 provider 位于 `chathistory/<provider>` 独立 package；需要数据库 SDK 的 provider 同时是叶子 module，根包不得 import 数据库 SDK、OTel 或具体 store。

## 架构心智

- **canonical JSON envelope**:所有数据只读写当前 `core/chat.Message` tagged wire；历史数据迁移由应用显式执行，不在库内保留兼容分支。
- **按 conversation_id 分区**:每个会话独立查询路径,避免跨会话扫表。
- **顺序靠单调序号 / 列表追加,不靠时间戳**:高并发下 timestamp 排序会乱。
- **schema 初始化是显式开关**:production 通常预先 migrate,关掉自动建表。
- **自定义表名必过 SQL identifier 校验**:防注入 —— 信任边界在此。

## 模块特有反向不变量

- ❌ **跨后端数据迁移工具** —— 是 ops 的事,不是 SDK 的职责。
- ❌ **在本模块写 schema migration** —— schema 由调用方 migrate,本模块只约定形状。
- ❌ **把数据库或可观测性依赖带回根模块** —— provider 自己拥有持久化机制；OTel 只由 `otel` module 在组合根显式装饰。

## 改动前必看(波及面)

- **动 `chat.Message` 序列化**:所有 provider 的本地持久化边界必须同步并保留当前 wire 测试。
- **加新后端**:在 `chathistory/<backend>` 建独立 package；只有引入第三方依赖时才建 module。实现 `chathistory.Store`，按 conversation_id 分区，禁止导入兄弟 provider。
