# 审计报告索引与处理状态

三轮审计的记录 + 处理进度。范围：`app` 之外的全部 85 个 workspace module。

**最近一次复验**（全量 `go vet` + `go test`）：**85 / 85 模块全绿**。

| 文档 | 角度 | 条目 |
|---|---|---|
| [`CODE_SMELLS.md`](CODE_SMELLS.md) | 通用坏味道：宏观架构 / 微观实现 / 反 Go 范式 | A1–A6、B1–B10、C1–C4 |
| [`OBSERVABILITY_AND_EVALUATION.md`](OBSERVABILITY_AND_EVALUATION.md) | 可观测性与可评估性**能力面** | O1–O7、E0–E8 |
| [`ROBUSTNESS_AND_ENGINEERING.md`](ROBUSTNESS_AND_ENGINEERING.md) | 安全与契约一致性 / 验证能力 / 工程成熟度 | S1–S2、V1–V6、M |

> 三份文档保留原始发现记录（含证据与失败场景），**不随修复改写**；处理状态以本表为准。

---

## 相邻文档集

| 目录 | 角度 |
|---|---|
| [`comparative-analysis/`](comparative-analysis/README.md) | **对外对比**：eino / trpc-agent-go / adk-go / agent-framework-go / GitNexus / spring-ai / embabel-agent 逐项深度分析 + 八维总对比（`SYNTHESIS.md`）。产出 17 条缺口编号 `G1–G17` 与「明确不吸收」清单 |

> 审计（本表）向内看已有代码的坏味道；对比分析向外看别人做对/做错了什么。两者的缺口编号体系独立（`A/B/C/O/E/S/V/M` vs `G`）。

---

## ✅ 已关闭（逐条复验通过）

| 项 | 修复要点 | commit |
|---|---|---|
| **B1** `quiesceTree` 所有权 | `transferred` + `defer`，6 处手工 `close` 归零；并新增 `quiescedTree` 复合能力，把「先释放屏障再放行下一个操作」的次序不变量写进注释 | `634bef522` |
| **B4** 幂等机制不一致 | `treeQuiescence` 裸 bool → `sync.Once`，与 `treeOperation` 统一 | `634bef522` |
| **S1** 路径权限契约 | `resolve` → `authorize` + `os.OpenRoot`；`filepath.IsLocal` 拒绝 `../` 与越界绝对路径。实测 5 类逃逸全部拦下，合法路径正常 | `f382905a0` |
| **A4** `Executor` 胖接口 | 拆为 `Reader`/`Writer`/`Editor`/`PatchApplier`/`Globber`/`Grepper` 六个最小端口，6 个 tool 各只依赖自己那片；联合接口取消 | `f382905a0` |
| **A6** 领域模块自埋点 | 先在根 `CLAUDE.md` 裁决条款冲突（`a2a`/`mcp` = 协议 integration，显式豁免且不扩散），再迁移：agent/rag/etl/tools/skills/evaluation OTel import 归零，新增 `otel/rag`、`otel/tool` | `4cd785351` `22bc23ad1` |
| **E0** evaluation 定位 | 拆为通用内核 + `evaluation/text` + `evaluation/retrieval` | `db508587d` |
| **A3** delta 观测链 context | `offerDelta(ctx, delta)` 全链路透传，`OnDelta` 的 ctx 不再恒为 Background | `4cd785351` 前后 |
| **O2** 失败分类不可见 | `FailureKind` / `FailureCode` 进 event 载荷，`otel/agent` 记录 `agent.failure.kind` / `agent.failure.code` | `510136845` `72b4e471e` |
| **O3** 工具身份不可见 | 新增 `otel/tool`，按 GenAI semconv 记 `gen_ai.tool.name` / `gen_ai.tool.type` / `execute_tool` —— **比原建议更好**：不动 agent wire，工具自持身份，span 靠 trace 嵌套关联 | `476648201` |
| **O4** `error.type` 恒为常量 | `%T` → `errors.Is` 对 `context.Canceled`/`DeadlineExceeded` 与 core/chat 全部 sentinel 分类 | `01817dc58` |
| **O5** 只有 traces 无 metrics | `otel/history` `chat_history.operation.duration`、`otel/vectorstore` `db.client.operation.duration`（semconv 命名）、`otel/rag`、`otel/tool` 均补齐 | `0023b6d51` |
| **O7** TTFT 只是 span event | 新增 `gen_ai.client.time_to_first_token` 指标 | `0023b6d51` |
| **A5** 覆盖率闸门失效 | 12 → 25 个包全覆盖，阈值重设到实测 −2pt；`tokenizer none` 显式标记且获得语句即失败；**新增 configured vs tracked 交叉校验，新包无法静默逃逸** —— 比原建议更强 | `573bde336` |
| **B2** 脱钩 goroutine 用 Background | 改 `context.WithoutCancel` | `2390d4c8f` |
| **B6** 静默吞 panic | `callEventListener`/`callDeltaListener` 返回 `panicked`，新增 `ObservationFailureCounts` 暴露计数 | `4744dd805` |
| **B8** `a2a` 用 `%v` 断因果链 | 改双 `%w` | `718a4594f` |
| **B9** `fmt.Errorf` 包常量 | 改 `errors.New` | `62bdcf51d` |
| **B10** nil-ctx 反应式兜底 | `contextOrBackground` 取消，改 `panic(errNilContext)`，不再静默掩盖调用方 bug | `95262f8a8` |
| **C4** 接收者命名不一致 | brave/jina `searchRequest` 统一 | `d7b25f358` |
| **S2** 无界读取 | etl 三个 reader 加 bounded source budget | `b74555e40` `8c01f6040` |
| **V4**（部分） | 新增 `tools/fs` rooted path authority fuzz | `2b44aa197` |

**22 条已关闭**，其中 B1 / A5 / O3 三条的最终方案优于原建议。

---

## ⏳ 待处理

### 需先裁决 / 破坏性 API

| 项 | 现状实测 | 说明 |
|---|---|---|
| **A1** 协议值指针语义 | core 仍 **185 处**防御式 nil 守卫 | 影响约 30 个 model provider + 24 个 vectorstore，需分批迁移方案。`agent` 的值语义可作参照 |
| **A2** `samber/lo` | 仍 **21 个模块**直接依赖，`lo.IsNil` **113 处**（较首轮 104 略增） | 需先定 owner 落点（`core/internal` 不可跨 module 导出） |
| **E1 / E2 / E3** evaluation 形状 | 数据集抽象 **0**、参考答案字段 **0**、Composite 仍串行 | 三者互相约束，须一次设计定型；E0 已完成分层，为其铺好了路 |

### 可直接做，无 API 影响

| 项 | 现状实测 |
|---|---|
| **M** LICENSE | 仍缺；120 个 tag 已发布，法律上第三方不可用 |
| **B3** 锁归属注释 | 仍近 0（`quiescedTree` 已有注释，`Engine` 两把锁 0 行、`processLoop` 0 行） |
| **B7** god struct 字段分区注释 | `processLoop` 25 字段 0 行分区注释；与 B3 可合并（`Engine` 两组恰好按两把锁分） |
| **B5** `newEvent` 10 个位置参数 | 未变 |
| **O6** `otel/agent` 零 `RecordError` | 仍 0（`SetStatus` 有，`RecordError` 无） |
| **O1** 剩余 wrapper | `embedding` / `image` / `moderation` / `speech` / `transcription` 仍无；`embedding` 优先（RAG 成本主项） |
| **V1** benchmark | 仍 **1 个**；tokenizer / splitter / 快照编解码 / inmemory filter 均无 |
| **V2** godoc `Example` | core 27 个，**其余模块仍全部 0**（agent 254 导出符号）；8 个现成 demo 可改写 |
| **V3** 文档门禁 | 仍只覆盖 core 一个模块 |
| **V4** 剩余 fuzz | `etl` splitter、`core/jsonschema` 仍无 |
| **V5** testify | 仍只 `mcp` 一个模块用 |
| **V6** `t.Parallel` | 仍约 11% 测试文件 |
| **E4–E8** | judge 自洽性、加权组合、Metric 身份结构化、`otel/evaluation`、补指标 |

---

## 建议下一批

1. **M · LICENSE** —— 几分钟，法律阻断
2. **B3 + B7 合批** —— 纯注释：`Engine` 两把锁的归属与次序、`processLoop` 字段分区。B1 已经证明这类不变量写下来有价值
3. **O6** —— `otel/agent` 4 处补 `RecordError`
4. **V2 + V3 合批** —— 各模块补 godoc `Example`，文档门禁复用 core 现成实现扩到能力模块。投入产出比最高的一条
5. **V1** —— 关键路径 benchmark
6. 之后再进 **E1/E2/E3** 与 **A1/A2** 两组需裁决项

---

## 审计中确认「做得好」的部分

三份文档各自的附录有完整记录。摘要：

- **架构门禁**：`dev/repoarch` 1989 行可执行测试（模块分层、依赖无环、core 不 import OTel、provider 依赖岛、27 条退休布局）
- **零技术债标记**：全仓 0 处 TODO / FIXME / HACK / XXX
- **零死公共 API**：459 个导出符号全部有模块外消费者
- **Go 范式**：0 处 `context` 存 struct、0 个 `init()`、0 个可变全局、0 处 channel 冒充流式、0 处 Java 味命名、0 个 `GetX` getter
- **CI**：每模块 build / vet / test / **race** / tidy + govulncheck 豁免清单 + PR 只跑受影响模块
- **`otel/chat`**：官方 GenAI semconv + typed instrument + token 直方图 + 流式早停正确收尾
- **检索指标**：precision@k / recall@k / MRR / NDCG@k 数学实现逐条核对无误，除零路径全被 `Validate()` 挡住
