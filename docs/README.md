# docs

范围：`go.work` 中全部 85 个 workspace module。

| 文档 | 角度 | 条目 |
|---|---|---|
| [`REPO_AUDIT.md`](REPO_AUDIT.md) | **全仓审计**（2026-08-30，基线 `46a8e8b0e`）：健康度实测、本轮已关闭项、新发现、未关闭老账、建议批次与复验命令 | N1–N5 + 存量 A/B/O/E/S/V/M |
| [`AGENT_KERNEL_REVIEW.md`](AGENT_KERNEL_REVIEW.md) | **单模块复审**：`agent` durable tree kernel 落地后的全量复核（同基线） | AG1–AG5 |
| [`comparative-analysis/`](comparative-analysis/README.md) | **框架层对比**：Scope / Pi / Eino / tRPC-Agent-Go / ADK Go / Microsoft Agent Framework Go / Spring AI / Embabel Agent；GitNexus 仅作相邻系统证据，Flame 仅作外部应用消费者验证。综合结论见 `SYNTHESIS.md` | G1–G17 |

> 全仓审计向内看已有代码的坏味道；单模块复审在一次内核级变更之后核对代码与其契约文档是否一致；对比分析向外看不同框架在契约、状态、副作用、恢复和依赖边界上的取舍。应用层能力不参与框架评分。

**两处交集**（各自保留，修复时一并处理）：`AG3` ≙ `REPO_AUDIT` 的 `B3`（`Engine` 两锁注释）；`AG4` ≙ `REPO_AUDIT` 的 `A2` 在 `agent` 模块内的切片。

---

三轮历史审计文档（`CODE_SMELLS.md` / `OBSERVABILITY_AND_EVALUATION.md` / `ROBUSTNESS_AND_ENGINEERING.md`）已删除 —— 其中 22 条已修复关闭，未关闭的已按本轮实测并入 `REPO_AUDIT.md`；留着只会误导。修复证据在 git history。
