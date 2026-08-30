# 全仓审计报告

**日期**：2026-08-30
**范围**：`go.work` 中全部 85 个 workspace module
**方法**：全量 `go vet` + `go test`；AST 扫描（接口方法数、struct 字段数、函数长度/参数数、导出指针字段）；按规则的 grep 取证；逐包 `go test -cover`
**基线**：`46a8e8b0e fix(interaction): redact tool authorization causes`

> 本报告取代此前的 `CODE_SMELLS.md` / `OBSERVABILITY_AND_EVALUATION.md` / `ROBUSTNESS_AND_ENGINEERING.md`（已删除）。
>
> **相邻文档**：`agent` 单模块内核复审见 [`AGENT_KERNEL_REVIEW.md`](AGENT_KERNEL_REVIEW.md)（AG1–AG5，同基线）；框架层横向对比见 [`comparative-analysis/`](comparative-analysis/README.md)。三者角度不同、并行有效。
> 与 `AGENT_KERNEL_REVIEW.md` 有两处交集，各自保留、修复时一并处理：**AG3 ≙ 本报告 B3 的 `Engine` 两锁部分**；**AG4 ≙ 本报告 A2 在 `agent` 模块内的切片**。

---

## 1. 总体健康度

**85 / 85 模块 `go vet` + `go test` 全绿。**

| 结构性指标 | 实测 | 说明 |
|---|---|---|
| ≥5 方法的接口 | **0** | 全仓、含 `agenttest` / `modeltest` / `storetest` |
| `TODO` / `FIXME` / `HACK` / `XXX` | **0** | |
| errorlint 类违规 | **0** | 无 `err == ErrX` 比较、无 `err.(T)` 裸断言、无 error type switch |
| HTTP body 泄漏 | **0** | 覆盖 40 个 `models/*` + 26 个 `vectorstores/*` |
| 最长生产函数 | 89 行 | `vectorstores/oracle/store.go:333 Search` |
| 最多函数参数 | 7 | 上轮为 10（`newEvent`），已收敛 |
| `context` 存 struct / `init()` / 可变全局 | 0 | |

「0 个 ≥5 方法接口」跨 85 模块、~460 个导出符号，是本仓最硬的一项结构成绩。

### 模块体量与测试比

| module | 生产行数 | 测试行数 | ratio | 覆盖率 |
|---|---|---|---|---|
| `agent` | 27,350 | 20,152 | 0.73 | 77.3%（root）／`goap` 88.1／`platform` 89.0／`workflow` 80.4／`interaction` 77.5／`planning` 76.3／`agenttest` 78.8 |
| `core` | 12,657 | 12,146 | 0.95 | 逐包预算，见 `scripts/check-core-coverage.sh` |
| `tools` | 5,681 | 3,825 | 0.67 | — |
| `etl` | 2,659 | 2,271 | 0.85 | — |
| `otel` | 2,367 | 2,608 | 1.10 | — |
| `rag` | 2,614 | 1,993 | 0.76 | — |
| `evaluation` | 2,101 | 1,193 | 0.56 | **89.1%**（root）／`ranking` 91.2／`text` 89.0／`judge` 77.5 |
| `mcp` | 1,028 | 1,156 | 1.12 | — |
| `skills` | 947 | 957 | 1.01 | — |
| `a2a` | 891 | 656 | 0.73 | — |

`evaluation` 的 ratio 最低（0.56）但覆盖率最高（89.1%）—— 行数比不是覆盖率的代理，此处不构成问题。

---

## 2. 本轮已关闭

### O6 —— `otel/agent` 零 `RecordError`

`otel/` 下 `RecordError` 由 **0 → 7 处**（全部在 `otel/agent`）。失败面（durability fault、incarnation conflict）现在能进 span。

### E1 / E2 / E3 —— evaluation 形状，一次做完

`evaluation` 从 ~1,100 行涨到 2,101 行；`run.go`（338 行，runner 与 slice 混合）被删除，换成：

| 原缺口 | 落地 | 位置 |
|---|---|---|
| **E1** 数据集抽象 = 0 | `Dataset[T]` —— 不可变、有序、ID 唯一，metadata 由 Dataset 克隆持有 | `evaluation/case.go` |
| **E2** 参考答案字段 = 0 | `Case[T].Subject T` —— 参考答案留在泛型参数内，框架不规定形状 | `evaluation/case.go:25` |
| **E3** Composite 串行 | `evaluateAll` + errgroup 有界并发，Composite / Suite 共用 | `evaluation/parallel.go` |
| **E7** Metric 身份非结构化 | `MetricSummary.Metric` 挂完整 Metric 身份，unit / direction / 配置不同者永不混聚 | `evaluation/summary.go:40` |
| （新增） | `ExperimentReport.Compare` —— 双向校验 case 序列与 metric 身份，只出精确 delta | `evaluation/comparison.go:40` |

值得单独记的两处判断：

- `DistributionDelta.Present`（`comparison.go:9`）区分「一侧无值」与「差值为零」—— 缺失不会被读成 0。
- 文档明写拒绝编造统计显著性（`evaluation/doc.go`），并且代码真的没有 p 值、没有合成总分。

### 新能力 —— `core/tool` 授权窄腰

`core/tool/authorization.go`：

- 授权点选在 `Tool.Call`（唯一执行边界），因此直接调用、Registry、managed runtime 用同一套 policy。
- `Authorization` 只暴露冻结的 `Definition()` 与已过 schema 的 `Arguments()`，两者都返回detached 副本；policy 代码拿不到执行能力，绕不开 `Binding`。
- 身份、租户、同意流程、策略存储全部留在调用方，没有进 Core。

配套 `agent/interaction/invocation.go:132`：`ErrAuthorizationDenied` 映射为固定的模型可见文案（`tool %q is not authorized`），policy 诊断不外泄；`ErrHostFailure` / `context.Canceled` / `ToolInputRequiredError` 则 `present=false`，根本不进模型上下文。边界切得干净。

### 其余

| 项 | 上轮 | 本轮 |
|---|---|---|
| **B5** `newEvent` 10 个位置参数 | 10 | 最大 7（`newEffectRequest` / `newProcessController` / `newProcessState`） |
| **V4** fuzz | `tools/fs` 1 处 | **24 个** `Fuzz*` |
| **V5** testify | 1 模块 | **5 模块** |
| **V1** benchmark | 1 | 4（仍偏薄，见 §4） |
| **V3** 文档门禁 | 仅 `core` | `core` + `agent`（`agent/contract_policy_test.go`，覆盖 7 个包目录，另含退休术语门禁） |

---

## 3. 新发现

### N1 · `agent` 没有覆盖率闸门 —— 当前最大的门禁缺口

```
core   12,657 生产行  →  scripts/check-core-coverage.sh，25 包逐包预算
                          + configured vs tracked 交叉校验（新包无法静默逃逸）
agent  27,350 生产行  →  无
```

`.github/workflows/ci.yml:73-75`：

```yaml
- name: Core coverage budget
  if: matrix.module == 'core'
  run: scripts/check-core-coverage.sh
```

而 `dev/repoarch/architecture_test.go:102` 已经把「CI 必须有覆盖率闸门」写成可执行断言：

```go
for _, gate := range []string{"Core coverage budget"} {
```

**问题**：规则本身已经机制化了，清单里却只有一项，而仓库里体量最大（2.2× core）的模块不在其中。实测 agent family 覆盖率 76.3%–89.0%，数字都够 —— 但没有任何东西阻止它下滑。A5 当初的结论正是「闸门不覆盖 = 可以静默逃逸」。

**修法**：复用 `check-core-coverage.sh` 的形状新增 `scripts/check-agent-coverage.sh`（含 configured/tracked 交叉校验），CI 加一步 `if: matrix.module == 'agent'`，`architecture_test.go:102` 的 gate 清单加一项。阈值按实测 −2pt 设定。

---

### N2 · `evaluation` 是唯一没有 `CLAUDE.md` 的能力模块

`rag` / `etl` / `tools` / `skills` / `a2a` / `mcp` / `otel` / `models` / `vectorstores` / `historystores` 全有，`evaluation` 没有。

根 `CLAUDE.md` 的「加文档先问」条款前提是「每个 sub-module 已有 `CLAUDE.md`」—— 这个前提对 `evaluation` 不成立。

**为什么现在才重要**：`evaluation` 刚长出一套有非显然不变量的领域模型 —— Dataset 不可变性与 metadata 所有权、`ErrorPolicy{collect, fail_fast}` 的语义、Metric 身份聚合规则、以及「不编造统计显著性 / 不跨 unit 合成总分」这条明确的反向不变量。这些现在只活在 `doc.go` 和代码里。

**修法**：补 `evaluation/CLAUDE.md`，写清定位（subject-agnostic 内核 + `text` / `ranking` / `judge` 三个域词汇表）、所有权边界（不持有 persistence / artifact / 产品身份）与模块特有反向不变量。

---

### N3 · `evaluateAll` 的死锁前提交给了调用方

`evaluation/parallel.go:13`：

```go
group.SetLimit(maxConcurrency)   // maxConcurrency == 0 → group.Go 永久阻塞
```

`errgroup.SetLimit(0)` 使信号量容量为 0，后续 `Go` 全部阻塞。当前安全**只因为**两个调用方各自都做了归一：

- `evaluation/suite.go:47-50` —— `0 → DefaultMaxConcurrency`，再 `min(…, len(evaluators))`
- `evaluation/composite.go:105-111` —— 同上

`evaluateAll` 自身零校验。第二法则原文点名过这类：**「把不变量交给『调用方记得别犯错』」**。失败模式是死锁而非报错，且这个包刚翻倍、后续必然有第三个调用方。

**修法**：`evaluateAll` 入口 `maxConcurrency = max(1, maxConcurrency)`，或把前置条件写成 godoc 并加断言。一行的事。

---

### N4 · `Report.Details` 无界递归

`Report.Details []Report` 带完整 json tag（`evaluation/report.go:22`），三条路径无界递归：

- `Report.Clone()` — `report.go:39`
- `Report.Validate()` — `report.go:69`
- `summarize` 内的 `summarizeMetric` — `summary.go:151`

全仓 `grep -rn 'depth\|Depth' evaluation/` 无结果 —— 没有任何深度上界。

**触发条件**：Host 把 `ExperimentReport` 持久化后再解码回来，深度就来自外部输入 → 栈溢出。这与已修复的 **S2（etl 无界读取）** 属同一类：公开的 wire-shaped 类型没有 bounded budget。

**优先级低**（需要 Host 解码不可信 Report 才触发，框架自身不提供该解码路径），但类别相同，记在这里以免下次从同一根因冒出来。

---

### N5 · 两处错误处理残留 + 一个零成本棘轮

| 位置 | 现状 | 应为 |
|---|---|---|
| `core/chat/tool.go:94` | `fmt.Errorf("tool output details must be one valid RFC 7493 JSON document")` —— 无格式化动词 | `errors.New(...)`；全仓最后 1 处 |
| `tools/web/search.go:124` | `fmt.Errorf("%w: %q: %v", ErrInvalidDomain, raw, err)` —— `err` 用 `%v`，断因果链 | `%w: %q: %w`（Go 1.20+ 支持多 `%w`） |

其余 33 处 `%v` 全部作用于 `recover()` 返回的 `any`，是正确用法，不改。

**`.golangci.yml` 未启用 `errorlint`**。当前启用：`errcheck` / `govet`(enable-all, 仅关 fieldalignment) / `ineffassign` / `misspell` / `staticcheck` / `unused`。

`errorlint` 正好抓上表第二行，并把根 `CLAUDE.md` 的「包装错误一律 `%w`（才能 `errors.Is/As`）」从「靠 review」变成「靠编译」。**存量违规只有 1 处** —— 这是零成本棘轮，不是清理工程。

---

## 4. 未关闭的老账（本轮实测）

### 需先裁决 / 破坏性 API

| 项 | 实测 | 说明 |
|---|---|---|
| **A1** 协议值指针语义 | 全仓 **198** 个导出指针字段；`core/chat` **18**、`rag` 8、`core/image` 5、`core/embedding` 3、`core/transcription` 3 | provider wire DTO 用指针表达 tri-state 是正当的（`models/*` 占大头）；根因在 `core/chat` 等协议包。影响 ~30 个 model provider + 26 个 vectorstore，需分批迁移方案。`agent` 的值语义可作参照 |
| **A2** `samber/lo` | **21 个模块**直接依赖；全仓 **132 次**调用，其中 **`lo.IsNil` 119 次（90%）**，其余仅 `MapErr`×6、`CoalesceMapOrEmpty`×4、`Substring`/`SliceToMap`/`RuneLength` 各 1 | 21 个模块扛一个第三方依赖，实质为了一个函数。该函数需求正当（检测 interface 中的 typed nil），且 `core/tool/tool.go:70 isNilTool` 已手写同样的 reflect 逻辑 —— 落点问题已被识别，只差定 owner（`core/internal` 不可跨 module 导出）。`agent` 模块内的切片另见 **AG4** |

### 可直接做，无 API 影响

| 项 | 实测 |
|---|---|
| **M** LICENSE + 根 README | 均缺；已发布 **120 个 tag**。法律上第三方目前不可用 |
| **B3 + B7** 锁归属 / 字段分区注释 | `agent/engine.go:85,88` 两把锁（`treeOperationsMu` + `mu`）**零注释**，字段仅靠空行分组；实测两锁**从不嵌套持有**（`treeOperationsMu` 仅在 `tree_operation.go` 内使用，无交叉），正是「现在成立、下次改动会破」的不变量 —— 即 **AG3**。`agent/process_state.go processState` 28 字段、`agent/tree_runtime.go treeRuntime` 21 字段、`agent/process.go processController` 19 字段，均无分区注释 |
| **O1** 剩余 otel wrapper | `embedding` / `image` / `moderation` / `speech` / `transcription` 仍无（现有：`chat` / `agent` / `rag` / `tool` / `history` / `vectorstore` / `slog`）。`embedding` 优先（RAG 成本主项） |
| **V2** godoc `Example` | **27 个，全部在 `core`**；其余 84 个模块仍为 0（`agent` 254 个导出符号 0 个 Example）。`agent/examples/` 下 8 个可运行 demo 可改写 |
| **V1** benchmark | **4 个**。durable contract §15 要求的七项测量（TreeSnapshot 编码耗时/字节、boundary callback p50/p95/p99、owner-line 占用率、每 root 连续串行 commit 数等）**一项未跑** —— 这是唯一还可能推翻 durable kernel 架构判断的缺口 |
| **V3** 文档门禁 | 已覆盖 `core` + `agent`；`rag` / `etl` / `tools` / `skills` / `evaluation` / `otel` / `a2a` / `mcp` 仍无 |
| **V6** `t.Parallel` | **51 / 522** 测试文件 = 9.8% |
| **E4–E6 / E8** | judge 自洽性、加权组合校验、`otel/evaluation` 仍无 |

---

## 5. 建议批次（按性价比）

1. **`errorlint`** —— 改 1 行配置 + 修 1 处（`tools/web/search.go:124`），把已有规则永久锁死
2. **N1 · `agent` 覆盖率闸门** —— 新脚本 + CI 一步 + `repoarch:102` 清单加一项
3. **N2 · `evaluation/CLAUDE.md`** —— 补齐模块上下文
4. **B3 + B7 合批** —— 纯注释：`Engine` 两锁归属与「从不嵌套」、`processState`/`treeRuntime`/`processController` 字段分区
5. **N3 + N5 微修** —— `evaluateAll` 前置条件、`core/chat/tool.go:94`
6. **M · LICENSE** —— 几分钟，法律阻断
7. **V1 · benchmark** —— 尤其 durable contract §15 的七项；架构判断的最后一块证据
8. 之后再进 **A1 / A2** 两组需裁决项

---

## 6. 复验命令

```bash
# 全量绿
scripts/check.sh build vet test race tidy
scripts/check-core-coverage.sh

# 本报告的关键取证
grep -rn 'lo\.[A-Za-z]*' --include='*.go' . | grep -v _test | grep -o 'lo\.[A-Za-z]*' | sort | uniq -c
grep -rn '^func Benchmark' --include='*_test.go' . | wc -l
grep -rn '^func Example'   --include='*_test.go' . | wc -l
grep -rl 't.Parallel()'    --include='*_test.go' . | wc -l
grep -rn 'RecordError' otel/ | wc -l
(cd agent && go test -cover ./...)
(cd evaluation && go test -cover ./...)
```

---

## 附录 · 审计中确认「做得好」的部分

- **架构门禁**：`dev/repoarch` 2,118 行可执行测试 —— 模块分层、依赖无环、core 不 import OTel、provider 依赖岛、退休布局、CI 步骤清单元断言
- **`agent` 契约门禁**：`contract_policy_test.go` 强制公开接口有文档且参数具名，并封禁已退休的执行术语
- **零技术债标记**：全仓 0 处 `TODO` / `FIXME` / `HACK` / `XXX`
- **ISP**：全仓 0 个 ≥5 方法的接口
- **错误契约**：0 处 errorlint 类违规；34 处 `%v` 中 33 处作用于 `recover()` 的 `any`（正确）
- **panic 隔离**：`agent` 每个 Strategy 入口（`startExecution` / `restoreExecution` / `stepExecution` / `Descriptor` / `Snapshot`）与每个 Host 回调（`tree_durability.go:325/341/357`）都有 `recover` → typed error，panic 只失败该 Process
- **Step 确定性**：`ARCHITECTURE.md:234` + `ENGINEERING_STANDARDS.md:213` 明确规定 Step context 从 `context.Background()` 构造、不含 value/deadline/cause，`tree_jobs.go:26` 与之一致 —— 这是刻意的确定性约束，不是 B2 回归
- **契约测试复用**：`core/modeltest` 被 52 个文件消费、`core/vectorstore/storetest` 被 64 个文件消费，覆盖全部 40 个 model provider 与 26 个 vectorstore
- **HTTP 卫生**：40 个 provider + 26 个 store 无一处 response body 泄漏
- **CI**：每模块 build / vet / test / **race** / tidy + golangci-lint + govulncheck 豁免清单 + PR 只跑受影响模块
