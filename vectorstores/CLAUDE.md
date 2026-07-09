# CLAUDE.md — vectorstores module

> 各家向量数据库后端的统一适配层:每个后端实现 core 的 `vectorstore.Store`,并用 Visitor 把 lynx 统一的 filter AST 编译成本地查询方言。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。后端名录 / 成熟度 / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **一套 Store 接口,多个数据库后端**:上层不关心是哪家向量库,只见 core 的 Store 契约。
- **加后端 = 实现 Store + 写一个 filter Visitor**,不动统一的 filter 语义。

## 架构心智

- **filter mini-language 定义在 core**(lexer → parser → AST),后端只提供 **Visitor**:统一的 filter 语义,各后端把同一棵 AST 译成自家方言(JSONB 路径 / 扁平字典 / 嵌套查询 …)。
- **每后端固定两件**:store(实现接口)+ visitor(AST → 方言);共享工具(文档 IO / filter 解析 / OTel / SQL identifier 校验 / conformance 套件)沉到内部包。
- **向量编码与距离度量因 DB 而异**:在适配层归一化成统一区间,上层拿到一致的 score。
- **schema 初始化是显式开关**:开则建表建索引,关则假设已 provisioned —— 绝不静默 ALTER。
- **批量 upsert 两级切分**:调用方注入 batcher 控 embedding 批量,后端再按自家 API 上限二次切分。
- **conformance 套件免费拿覆盖**:新后端在测试里注册一次,共享的 filter 形状符合性套件即覆盖它(验证遍历不报错,不验证输出逐字相等)。

## 模块特有反向不变量

- ❌ **跨后端数据迁移工具** —— 是 ops 的事,不是 SDK 的职责。
- ❌ **给 filter AST 加业务概念节点**(如会话 id)—— AST 是通用 filter,业务字段走 metadata。
- ❌ **在 store 端 reshape 向量维度** —— 维度靠 Config / embedding 协商,不在写入端改形。
- ❌ **静默改后端 schema** —— 初始化开关关掉时假设 schema 已存在。

## 改动前必看(波及面)

- **动 core 的 filter AST**:所有后端 visitor 都要跟,先跑共享 conformance 套件。
- **动 Store 接口**:是 core 的契约,爆炸半径 = 所有后端 + 所有消费方(如 rag)。
- **加新后端**:拿内存实现当模板,实现 Store + Visitor,注册进 conformance 套件。
