# CLAUDE.md — vectorstores module

> 各家向量数据库后端的统一适配层:每个后端按真实能力实现 core 的 `Indexer` / `Searcher` / `IDDeleter` / `FilterDeleter`,并把稳定的 filter `Predicate` 编译成本地查询方言。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。后端名录 / 成熟度 / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **一组小能力接口,多个数据库后端**:上层只要求实际调用的能力,不通过胖 `Store` 强迫只读或不可删后端伪实现方法。
- **加后端 = 实现真实能力 + 写 filter compiler**,不动统一的 filter 语义。

## 架构心智

- **filter 公共面只表达语义**:`Predicate`/`Selector` 与 `Parse` 是稳定门面；同包私有 scanner/token/递归下降 parser 直接构造唯一 AST。后端 compiler 实现公开 `filter.Visitor`，把同一语义树译成自家方言(JSONB 路径 / 扁平字典 / 嵌套查询 …)；外部扩展通过 `filter.Visit` 校验一次并按顺序组合多个 compiler/interpreter。
- **每后端固定两件**:backend(实现其能力集合)+ compiler(Predicate → 方言);共享工具(文档 IO / OTel / SQL identifier 校验 / conformance 套件)沉到内部包。
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

- **动 core 的 filter AST/Visitor**:所有后端 compiler 都要跟,先跑共享 conformance 套件。
- **动能力接口**:是 core 的契约,爆炸半径 = 实现该能力的后端 + 对应消费方(如 rag)。
- **加新后端**:拿内存实现当模板,只实现真实能力并注册进 conformance 套件。
