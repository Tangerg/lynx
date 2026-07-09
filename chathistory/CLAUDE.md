# CLAUDE.md — chathistory module

> core 的 `chat/history.Store` 的多数据库后端实现 —— 给真上 production 的 chat history 提供后端选择(轻量的文件 / SQLite 走 lyra 内置,不在此)。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。后端名录 / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **一个 history.Store 接口,多个数据库后端**(Write / Read / Clear):上层不关心底层是哪家 DB。
- 与 lyra 内置的文件 / SQLite 存储互补:这里覆盖需要独立数据库的 production 场景。

## 架构心智

- **canonical JSON envelope**:所有后端都走 `chat.Message` 的统一序列化 —— 数据形态与后端解耦,换后端可迁移。
- **按 conversation_id 分区**:每个会话独立查询路径,避免跨会话扫表。
- **顺序靠单调序号 / 列表追加,不靠时间戳**:高并发下 timestamp 排序会乱。
- **schema 初始化是显式开关**:production 通常预先 migrate,关掉自动建表。
- **自定义表名必过 SQL identifier 校验**:防注入 —— 信任边界在此。

## 模块特有反向不变量

- ❌ **跨后端数据迁移工具** —— 是 ops 的事,不是 SDK 的职责。
- ❌ **在本模块写 schema migration** —— schema 由调用方 migrate,本模块只约定形状。

## 改动前必看(波及面)

- **动 `chat.Message` 序列化**:所有后端都靠 canonical JSON envelope,必须同步。
- **加新后端**:实现 history.Store,按 conversation_id 分区,序列化走 `chat.Message` JSON。
