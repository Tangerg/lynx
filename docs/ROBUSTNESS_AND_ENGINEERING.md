# 健壮性与工程能力 专项审计

> 第三轮审计。前两轮见 [`CODE_SMELLS.md`](CODE_SMELLS.md)（通用坏味道）与
> [`OBSERVABILITY_AND_EVALUATION.md`](OBSERVABILITY_AND_EVALUATION.md)（可观测/可评估能力面）。
>
> 本轮换角度：**安全与契约一致性**、**验证能力（测试质量而非覆盖率）**、**工程成熟度**。
> 每条给 `file:line` 证据 + 可复现的验证。未修改任何代码。

---

## 目录

- **S. 安全与契约一致性**
  - S1 `Executor` 路径权限契约与实现相悖，且 root 由 LLM 参数可控 ★★
  - S2 「读取不可信输入」三套策略并存
- **V. 验证能力**
  - V1 全仓 1 个 benchmark ★
  - V2 godoc Example 只有 core 有；8 个示例程序对 pkg.go.dev 不可见 ★
  - V3 文档门禁只覆盖 core
  - V4 Fuzz 全是 wire round-trip，文本处理与路径逻辑零覆盖
  - V5 `mcp` 单独用 testify，与其余 84 模块不一致
  - V6 `t.Parallel` 仅 11% 测试文件使用
- **M. 工程成熟度**（已知项，简述）

---

# S. 安全与契约一致性

## S1 · `Executor` 承诺路径权限，`LocalExecutor` 声明「不是安全边界」，且 root 由 LLM 参数可控 ★★

**性质**：LSP 契约违反 + 第二法则违反。因为消费方是 LLM，同时具备安全维度。

### 接口契约怎么写的

`tools/fs/fs.go:12-36` 的 `Executor` 接口，四个方法明确承诺路径边界：

```go
// Read applies line and byte bounds before returning detached content. It must
// reject paths outside the executor's authority and honor ctx throughout I/O.
Read(ctx context.Context, in ReadInput) (ReadOutput, error)

// Write performs exactly the overwrite-or-append policy in input. It must not
// broaden path authority, ...
Write(ctx context.Context, in WriteInput) (WriteResponse, error)

// ApplyPatch validates and applies the complete patch within the executor's
// path authority. ...
ApplyPatch(ctx context.Context, request ApplyPatchRequest) (ApplyPatchResponse, error)

// Glob evaluates the requested pattern beneath its authorized root and
// returns a bounded, stable path list. It honors ctx and never traverses
// outside the configured workspace.
Glob(ctx context.Context, in GlobInput) (GlobResponse, error)
```

### 唯一实现怎么写的

`tools/fs/local_executor.go:29-33`：

```go
type LocalExecutor struct {
	// Root, if set, anchors relative paths. "" = no confinement.
	// This is not a security jail; callers that need confinement validate
	// paths before invoking the executor.
	Root string
	...
}
```

```go
// tools/fs/local_executor.go:46-55
func (l *LocalExecutor) resolve(path string) (string, error) {
	if path == "" { return "", ErrEmptyPath }
	path = expandHome(path)
	if l.Root == "" || filepath.IsAbs(path) {
		return path, nil          // ← 绝对路径无条件放行
	}
	return filepath.Join(l.Root, path), nil   // ← Join 会 Clean，../ 直接逃逸
}
```

接口说「must reject paths outside the executor's authority」「never traverses outside the configured workspace」，实现说「This is not a security jail」。**两句话直接矛盾**，而这是接口的**唯一**实现。

### 实测逃逸（复刻 resolve 逻辑运行）

```
root=/workspace/project  input="src/main.go"                  -> /workspace/project/src/main.go
root=/workspace/project  input="../../etc/passwd"             -> /etc/passwd            ← 逃逸
root=/workspace/project  input="../../../../../../etc/shadow" -> /etc/shadow            ← 逃逸
root=/workspace/project  input="/etc/passwd"                  -> /etc/passwd            ← 绝对路径放行
root=/workspace/project  input="a/b/../../../../secret.key"   -> /secret.key            ← 逃逸
```

### 更进一步：Glob/Grep 的 root 由 LLM 直接控制

这一条比路径逃逸更直接。完整链路：

**1. `Path` 是暴露给模型的 schema 字段**（`tools/fs/glob.go:13`）：

```go
type GlobRequest struct {
	Pattern string `json:"pattern" jsonschema:"minLength=1" ...`
	Path    string `json:"path,omitempty" jsonschema_description:"Directory to search under. Defaults to the workspace root."`
	...
}
```

**2. Tool 层原样透传**（`tools/fs/glob.go:59-64`）：

```go
res, err := g.executor.Glob(ctx, GlobInput{
	Pattern: req.Pattern,
	Root:    req.Path,        // ← LLM 参数进入 Root
	...
})
```

**3. 调用方 root 优先级高于执行器 root**（`tools/fs/local_executor.go:60-62`）：

```go
func (l *LocalExecutor) rootDir(callerRoot string) string {
	return expandHome(cmp.Or(callerRoot, l.Root, "."))
	//     ↑ expandHome 让 "~/.ssh" 也能用   ↑ callerRoot 排在 l.Root 前面
}
```

所以模型只要发出 `{"pattern": "**/*", "path": "/"}`，就能得到全盘 glob —— **无视执行器配置的 `Root`**。`{"path": "~/.ssh"}` 同样有效。

接口注释写的是「never traverses outside the configured workspace」。

### 为什么「调用方自己校验」不成立

第二法则原文：

> ❌ **把不变量交给「调用方记得别犯错」**

而且这里的「输入」不是普通调用方参数，是 **LLM 生成的工具参数** —— 按定义就是不可信输入（prompt injection 让模型发出 `{"path": "~/.ssh"}` 是最基础的攻击面）。把边界推给每个消费方，意味着每个装配 `tools/fs` 的应用都要自己重新实现一遍路径约束，而**当前仓库里没有任何一处这样做**（`tools/fs/*_test.go` 全部传 `nil` 用默认 `LocalExecutor("")`，即完全无约束）。

### 仓库自己已经有正确原语

`skills/directory.go:26` 用的是 Go 1.24+ 的 `os.OpenRoot`：

```go
root, err := os.OpenRoot(string(r))
```

`os.Root` 在**系统调用层**保证不逃逸（`openat` + `RESOLVE_BENEATH` 语义），`../`、绝对路径、符号链接全部挡住。

**全仓 `os.OpenRoot` 只出现这一处** —— 风险更低的 skills 用了，风险最高的 `tools/fs`（LLM 驱动的读/写/编辑/打补丁/遍历）没用。

### 处理方向（三选一，需裁决）

- **A（推荐）**：`LocalExecutor` 改用 `os.Root`，`Root != ""` 时真正强制约束；`Root == ""` 保留当前无约束语义并在接口注释里显式说明这是 opt-out。同时让 `GlobInput.Root` 只能在执行器 root **之下**取子目录，不能覆盖。
- **B**：承认无约束是有意的，**改接口注释** —— 删掉「must reject paths outside authority」「never traverses outside the configured workspace」，明确写成「路径约束是调用方责任」。至少消除契约矛盾。
- **C**：拆成两个实现（`LocalExecutor` 无约束 / `RootedExecutor` 强约束），由装配处选择。

参考 `tools/CLAUDE.md` 对 shell 的既有立场（「禁止给 shell 加 root 限制 —— 信任调用方，要 jail 在外层」）——**但 `fs` 与 `shell` 立场不同**：`shell` 接口没有承诺路径权限，`fs` 接口承诺了。所以无论选哪条，`fs` 当前状态都是自相矛盾的。

---

## S2 · 「读取不可信输入」三套策略并存

同一仓库对「把外部字节读进内存」有三种互不一致的做法：

| 策略 | 位置 |
|---|---|
| `io.LimitReader` 显式封顶 | `tools/httpreq/response.go:22`、`models/ollama/api.go:77,125`、`models/mistral/api.go:144` |
| 自定义 `MaxBytes` 字段 | `tools/fs/fs.go:44`、`tools/fs/local_file.go:41-42` |
| **完全无上限** | `etl/markdown/reader.go:73`、`etl/json/reader.go:43`、`etl/text/reader.go:36`、`models/luma/image.go:235`、`models/lmnt/api.go:67`、`models/deepgram/api.go:155`、`models/protocol/openai/audio_tts.go:123` |

`etl` 三个 reader 尤其值得看：ETL 的输入是**用户提供的文档**，在 RAG 场景里通常来自上传。三个 reader 都是裸 `io.ReadAll(r.source)`，没有任何上限 —— 单个超大文档即可耗尽内存。

不必强求统一到一个数值，但应当有一条明确约定：哪一层负责封顶、默认值多少、超限是截断（`tools` 的既有立场是「超限截断而非报错」）还是拒绝。当前是三种做法各自为政。

---

# V. 验证能力

> 覆盖率不是问题（core 多数包 85–95%，agent 77%，capability 模块 72–97%）。
> 本节看的是**覆盖率之外的验证手段**。

## V1 · 全仓 1 个 benchmark ★

```
core/vectorstore/filter/benchmark_test.go:9:func BenchmarkParse(b *testing.B)
```

**480 个测试文件、约 10 万行生产代码，只有 1 个 benchmark。**

这意味着**零性能回归防护**。而这个仓库里性能敏感的路径不少：

- `core/vectorstore/filter` 的 parser / optimizer / visitor（唯一有 benchmark 的地方）
- `core/tokenizer` + `tokenizers/tiktoken` —— token 计数在每次请求前都跑
- `etl` 的 splitter（`etl/splitter_token.go`、`etl/markdown/splitter.go:413`）—— 索引期对全量文档跑
- `core/vectorstore/inmemory/filter.go`（527 行）—— 内存过滤逐条求值
- `agent` 的快照序列化（`process_snapshot.go` 475 行、`tree_snapshot.go` 683 行）—— 每次快照全树编解码
- `core/metadata` 的 `Map` 操作 —— 协议热路径

顶尖 Go 库（stdlib、grpc-go、otel-go）的惯例是关键路径都有 benchmark 并在 CI 比对。当前状态下，一次「看起来无害」的重构把 tokenizer 或 splitter 拖慢 10 倍，没有任何机制会发现。

## V2 · godoc Example 只有 core 有；8 个示例程序对 pkg.go.dev 不可见 ★

| 模块 | 导出符号 | godoc `Example` |
|---|---|---|
| `core` | 283 | **27** |
| **`agent`** | **254** | **0** |
| `tools` | 93 | 0 |
| `rag` | 70 | 0 |
| `etl` | 44 | 0 |
| `evaluation` | 21 | 0 |
| `otel` | 18 | 0 |
| `mcp` | 14 | 0 |
| `skills` | 12 | 0 |
| `a2a` | 8 | 0 |

`agent` 有 254 个导出符号 —— 几乎与 core 持平 —— **一个 godoc Example 都没有**。用户在 pkg.go.dev 上看到 `agent.NewEngine`，只有签名和散文描述，没有一行可运行代码。

### 关键点：示例存在，但放错了地方

`agent/examples/` 下有 **8 个示例程序、1924 行**（autonomous、composition、workflow_patterns、evaluator_optimizer、orchestrator_workers…），质量不低。

但它们是 `package main`：

```go
// agent/examples/autonomous/main.go:1
// Command autonomous demonstrates an Interaction in which the model chooses a
```

`package main` 的程序**不会出现在 pkg.go.dev 的任何符号下**。也就是说这 1924 行示例，对通过 pkg.go.dev 发现这个库的人来说等于不存在。

Go 生态里 `Example` 函数是 API 的主要发现途径 —— 它直接渲染在对应符号下方，且被 `go test` 编译执行（保证不腐烂）。当前只有 core 享受到这个收益。

**这条投入产出比很高**：把每个模块最核心的 3–5 个入口补上 `Example`，pkg.go.dev 的可用性会有质变，而且现成的 8 个 demo 可以直接改写。

## V3 · 文档门禁只覆盖 core

`TestPublicPackagesHaveDocsAndRunnableExamples` 全仓只有一处：`core/internal/arch/documentation_test.go:15`，且只遍历 `targetPublicPackages`（core 自己的包）。

于是 core 的 283 个导出符号有强制的「必须有 package doc + 可运行示例」门禁，其余 534 个导出符号（agent 254 + tools 93 + rag 70 + …）完全没有。

这与 V2 是同一个根因：**门禁没跟着能力模块走**，跟 `CODE_SMELLS.md` A6（OTel 禁令只对 core 强制）是同一类问题 —— 门禁范围停在 core，能力模块靠自觉。

## V4 · Fuzz 全是 wire round-trip，文本处理与路径逻辑零覆盖

20 个 fuzz 目标，分布如下：

```
core/chat            FuzzPartJSON FuzzMessageJSON FuzzRequestJSON FuzzResponseJSON
core/metadata        FuzzMapJSON
core/media           FuzzMediaJSON
core/vectorstore/filter  FuzzParse                       ← 唯一的解析器 fuzz
agent                FuzzInputJSONRoundTrip FuzzTreeSnapshotJSONRoundTrip
                     FuzzTransitionJSONRoundTrip FuzzExecutionStateJSONRoundTrip
                     FuzzDeploymentRefJSONRoundTrip FuzzSnapshotJSONRoundTrip
agent/planning       FuzzWorldStateJSON FuzzPlanJSON FuzzOutputJSON
                     FuzzExecutionStateRestore FuzzPlanningProtocol
agent/workflow       FuzzWorkflowExecutionStateRestore
agent/interaction    FuzzExecutionStateRestore
```

**19/20 是 JSON round-trip 或状态恢复**，只有 `FuzzParse` 是真正的解析器 fuzz。

而最典型的 fuzz 目标一个都没有：

| 应该 fuzz | 当前 | 为什么值得 |
|---|---|---|
| `etl` 文本切分器 | 0 | 对任意用户文档做索引切分 —— 越界、死循环、UTF-8 边界的经典高发区 |
| `etl/markdown` splitter（552 行） | 0 | 同上，且 markdown 结构解析更复杂 |
| `tools/fs` 路径处理 | 0 | 与 S1 直接相关：`expandHome` / `resolve` / patch 路径解析 |
| `core/jsonschema` | 0 | schema 派生，输入是任意 Go 类型 |

JSON round-trip fuzz 覆盖的是「协议值能否安全往返」，覆盖不到「把任意字节切成块」和「把任意字符串解析成路径」这两类真正容易崩的逻辑。

## V5 · `mcp` 单独使用 testify，与其余 84 模块不一致

`stretchr/testify` 只出现在 `mcp` 一个模块的 7 个测试文件里；其余 84 个模块全部使用标准库 `testing`。

法则并未禁止 testify，但「一个能力、一个出口」的精神同样适用于测试风格：同一仓库两套断言习惯，会让贡献者在不同模块间切换时不知道该跟哪套，review 标准也不统一。要么统一到 stdlib（与多数一致），要么明确写进规约说明为何 mcp 例外。

## V6 · `t.Parallel` 仅 11% 测试文件使用

54 / 480 个测试文件调用 `t.Parallel()`。

两个影响：一是 CI 时间（85 个模块串行跑），二是**并发问题暴露不足** —— `t.Parallel` 配合 `-race`（CI 已开启）是发现共享状态竞态的主要手段。考虑到 `CODE_SMELLS.md` B3 指出的「23 个带锁结构体、0 处锁归属注释」，并行测试的价值更高。

值得注意的是 `dev/repoarch` 和 `core/internal/arch` 的架构测试**都用了** `t.Parallel`，说明团队知道这个实践，只是没铺开。

---

# M. 工程成熟度

> 你已经指出这块「肯定还没有」，这里只做记录，不展开。

| 项 | 状态 |
|---|---|
| `LICENSE` | ❌ 无 |
| 根 `README.md` | ❌ 无（仅 `otel/README.md`、`agent/examples/README.md`、`models/catalog/configs/README.md`） |
| `CONTRIBUTING.md` / `SECURITY.md` / `CODE_OF_CONDUCT.md` / `CHANGELOG.md` | ❌ 无 |
| `.github/` | 仅 `workflows/ci.yml`；无 issue / PR 模板 |
| 版本标签 | 120 个：88×v0.0.1、24×v0.0.2、6×v0.0.3、`agent/v0.1.0`、`agent/v0.2.0` |

唯一想强调的一点：**已经打了 120 个 tag 并且 module path 可 `go get`，但仓库没有 LICENSE**。按著作权默认规则这是「保留所有权利」，第三方在法律上不能使用 —— 这一条比其余成熟度项都更靠前，且补起来成本最低。

---

# 本轮核实为「做得好」的部分

| 项 | 结论 |
|---|---|
| CI 跑 race | `scripts/check.sh build vet test **race** tidy`，每个模块都跑 |
| CI 跑 govulncheck | `scripts/check-vulnerabilities.sh` + jq 强制的已评审豁免清单 |
| CI 模块选择 | PR 只跑受影响模块（`scripts/affected-modules.sh`），push 跑全量 |
| CI 拓扑门禁 | `dev/repoarch` 以 `GOWORK=off` 独立跑，先于所有模块检查 |
| `go mod tidy -diff` | 每模块强制，无漂移 |
| Fuzz 语料 | 4 个 testdata 目录，fuzz 有种子语料 |
| package 文档覆盖 | 64/74 个包有 `// Package ...`，缺的只有 `examples` 与 `internal/arch` |
| 密钥处理 | 未发现 API key 进入错误串或日志的路径 |
| `skills` 路径约束 | 使用 `os.OpenRoot`，是全仓最强的路径防护 |

---

# 建议处理顺序

| 顺序 | 项 | 成本 | 破坏性 |
|---|---|---|---|
| 1 | **M · LICENSE** | 极低 | 否 |
| 2 | **S1** 裁决路径契约（A/B/C 三选一） | 选 B 极低；选 A 中等 | 选 A/C 是 |
| 3 | **V2** 各模块补 godoc `Example`（现成 demo 可改写） | 低，收益极高 | 否 |
| 4 | **V3** 文档门禁扩到能力模块 | 低（复用 core 现成实现） | 否 |
| 5 | **V4** 补 `etl` splitter 与 `tools/fs` 路径 fuzz | 低 | 否 |
| 6 | **V1** 关键路径 benchmark（tokenizer / splitter / 快照编解码 / 内存过滤） | 中 | 否 |
| 7 | **S2** 统一不可信输入读取约定 | 中 | 可能加字段 |
| 8 | **V5 / V6** 测试风格统一、铺开 `t.Parallel` | 低但面广 | 否 |

**如果只做一件事**：先补 LICENSE（法律阻断，成本几分钟）。
**如果只改一处代码**：裁决 S1 —— 它是本轮唯一同时触及**接口契约正确性**和**安全边界**的问题，且当前接口注释与唯一实现直接互相否定，无论最终选哪条路，现状都是错的。
