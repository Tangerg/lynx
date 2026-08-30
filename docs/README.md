# docs

范围：`go.work` 中全部 85 个 workspace module。

| 文档 | 角度 | 条目 |
|---|---|---|
| [`REPO_AUDIT.md`](REPO_AUDIT.md) | **全仓反馈闭环**：旧审计条目的代码修复、机制化门禁与边界裁决 | N1–N5 + 存量 A/B/O/E/S/V/M |
| [`AGENT_KERNEL_REVIEW.md`](AGENT_KERNEL_REVIEW.md) | **Agent 内核闭环**：durable tree kernel、恢复、观测和 AG1–AG5 最终状态 | AG1–AG5 |
| [`comparative-analysis/`](comparative-analysis/README.md) | **框架层对比**：Scope / Pi / Eino / tRPC-Agent-Go / ADK Go / Microsoft Agent Framework Go / Spring AI / Embabel Agent；GitNexus 仅作相邻系统证据，Flame 仅作外部应用消费者验证。综合结论见 `SYNTHESIS.md` | G1–G17 |

> 全仓审计向内看已有代码的坏味道；单模块复审在一次内核级变更之后核对代码与其契约文档是否一致；对比分析向外看不同框架在契约、状态、副作用、恢复和依赖边界上的取舍。应用层能力不参与框架评分。

`AG3` 与全仓 B3、`AG4` 与全仓 A2 的交集已经统一裁决，不再保留互相矛盾的待办描述。

---

三轮历史审计文档（`CODE_SMELLS.md` / `OBSERVABILITY_AND_EVALUATION.md` / `ROBUSTNESS_AND_ENGINEERING.md`）已删除 —— 其中 22 条已修复关闭，未关闭的已按本轮实测并入 `REPO_AUDIT.md`；留着只会误导。修复证据在 git history。
