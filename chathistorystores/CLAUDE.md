# CLAUDE.md — chathistorystores module family

> `chathistory` 契约的数据库叶子适配器。项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)，契约规则见 [`../chathistory/CLAUDE.md`](../chathistory/CLAUDE.md)。

## 定位

- 每个 `chathistorystores/<backend>` 都是独立 Go module，只携带自己的数据库 SDK。
- 所有叶子单向依赖 `chathistory` 契约与仓库私有的 storage/OTel kit；叶子之间不得互相 import。

## 不变量

- 持久化 wire 统一经过 `internal/chathistorykit/codec`，不保留历史格式兼容分支。
- conversation 内顺序使用 shared monotonic sequence 或数据库原生有序追加语义，不用墙钟时间排序。
- schema 初始化必须显式开启；自定义标识符必须经过 shared identifier 校验。
- 禁止 `replace`、跨后端迁移工具、平级 SDK 依赖以及把具体 client 类型泄露到 `chathistory` 根契约。
