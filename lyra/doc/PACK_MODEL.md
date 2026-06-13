# PACK_MODEL.md — 跨渠道的契约式扩展(Pack 模型)

> **状态**:设计草案(blueprint),非现状。与 [`EXTENSIBILITY.md`](EXTENSIBILITY.md) 一道
> 反思并取代了已删除的 `EXTENSION_POINTS.md`——后者想"把前端的插件 kernel 镜像到后端",
> 本文论证那是错的方向,给出正解:
> **后端是库(编译期 embed),前端是 view(各按母语接线),两者只靠协议契约耦合。**
>
> 图例:**[现状]** 已落地 · **[提议]** 本文新增 · **[纪律]** 该遵守的规约 · **[不做]** 明确反向不变量。
>
> 关联:[`GREENFIELD_ARCHITECTURE.md`](GREENFIELD_ARCHITECTURE.md) · 协议契约在前端仓 `docs/API.md` / `docs/TRANSPORT.md` ·
> 项目哲学 [`../../DESIGN_PHILOSOPHY.md`](../../DESIGN_PHILOSOPHY.md)。

---

## 0 · 一句话定位

**一个"功能"= 一个 Pack;Pack 是「契约单元」,不是「编译单元」。** 它由分处不同语言/进程的
**几半**组成(后端 Go、Web 视图 TS、TUI 视图 Go、…),每半**用自己母语的机制接线**
(Go 编译期 embed、JS 运行时 sideload),彼此**永不互相 import / 互相加载**——
唯一的耦合是**协议上的语义契约**(一个 `custom` 事件 `kind` + 它的 payload schema +
一个 capability + typed tool result)。**协议就是集成边界。**

---

## 1 · 第一性原理(为什么是契约,不是 kernel)

前面绕过一圈得到的硬结论,先钉死,后面全建立在它上:

1. **插件 kernel 本质是"管理动态性"的机器**——它解决"运行时加载新代码 + 不可信代码同室 + 声明式 fold"。
   **前端三个都占**(JS 运行时加载、浏览器同地址空间、UI 是 store fold),所以 kernel 物有所值。
2. **后端是静态 Go 二进制**:无廉价运行时类型化加载;信任 = 进程边界;run loop 是主动驱动器不是被动 reducer。
   把前端 kernel 镜像到后端 = **为没有的问题付复杂度**(`DESIGN_PHILOSOPHY` §1 "为它新造一套 runtime = 设计错误信号")。
3. **所以两边不该用同一套机制,而该用同一份契约连起来。** 后端用它的母语机制(库 + 组合根 + rebuild),
   前端用它的(kernel + sideload),协议在中间。**不对称是特性,不是缺陷**——协议解耦,谁也不必迁就谁。
4. **"加能力"对能编译的人 = 写个包 + 自己的 main 里几行 + rebuild**,比任何运行时机制都简单
   (开源 + 嵌入,这就是 Go 给可信用户的"插件系统",它的名字叫"库")。**不 fork**(发散债)——**嵌入**(import lynx,写自己的组合根)。

> 本文的全部机制,都是为了让上面第 3、4 条**可发现、可协作、可多渠道分发**,而**不是**引入一台插件机器。

---

## 2 · 渠道矩阵(谁怎么接线)

你要前后端一起、多渠道分发。先把渠道和它们的**绑定模型**摊开——这是全文的骨架:

| 渠道 | 视图技术 | 视图半的接线方式 | 信任域 |
|---|---|---|---|
| **Web(浏览器)** | TS/JS | **运行时 sideload**(web kernel 拉 bundle) | 浏览器沙箱 |
| **H5 套壳桌面(Wails/webview)** | TS/JS(= 同一份 web UI 装进原生壳) | **运行时 sideload**(与 Web **同一个** view 半) | 桌面进程 |
| **TUI** | **Go(Bubble Tea)** | **编译期注册**(进 TUI 自己的二进制 / 组合根) | 本地进程 |
| **headless / 自动化** | 无 | **无视图半**,直接吃语义 payload | 调用方进程 |
| **后端 Runtime** | Go | **编译期 embed**(运维的 lyra main 组合根) | 服务进程 |

**两个关键观察**:

- **Go 目标(后端 + TUI)走静态编译期接线;JS 目标(Web + H5 壳)走动态运行时 sideload。**
  绑定模型按"目标是不是 Go"二分,**全部由同一份协议契约喂数据**。这就是整套设计的中心对称。
- **H5 套壳 ≈ Web**:套壳(Wails)只提供原生窗口 + IPC,跑的还是那份 web UI,**视图半复用 Web 那半**,
  不是第四种独立 adapter。所以实际只有**两类视图半:JS(sideload)/ Go-TUI(编译期)**。

---

## 3 · 三层模型(Model / 窄腰 / Views)

把 MVC 拆到**进程边界**上(pi-mono §8 的同款,也是 lyra "协议薄、业务厚、传输可换"的延伸):

```
┌─────────────────────────────────────────────────────────────┐
│  后端 Runtime = Model(纯语义)                                │
│   · run loop / tools / middleware / provider / retriever      │
│   · 只产「带类型的语义负载」,永不内嵌某个前端、不判断 kind     │
└───────────────────────────────┬─────────────────────────────┘
                                 │  窄腰 = 协议契约
                                 │  run.* / item.* / custom{kind,payload}
                                 │  capability(/v2/info) / typed tool result
                ┌────────────────┼────────────────┬─────────────┐
                ▼                ▼                ▼             ▼
        Web View(TS)      H5 壳(= Web)     TUI View(Go)    headless
        sideload          sideload          Bubble Tea       无 adapter
                                            编译期注册        直接吃 payload
```

- **Model(后端)只回答"发生了什么语义"**,不回答"它长什么样"。
- **每个 View 把同一份语义映射到自己的呈现**(DOM / Bubble Tea cell / 无)。
- **[纪律] 后端永不分支判断连上来的是谁**(`if frontend==gui` 是反模式)——
  能力协商(投递哪个 view、怎么降级)发生在 **kernel/transport ↔ 渠道**之间,**结果不回流给后端插件**。

---

## 4 · Pack 的解剖(一个功能,几半)

一个 Pack 在磁盘上是**一个仓 / 一个版本号**,内含按目标分的几半 + 一份契约声明:

```
my-feature-pack/
├── pack.json            # [提议] 契约清单:name / version /
│                        #   backend  : go import path
│                        #   web      : JS entry(可选)
│                        #   tui      : go import path(可选,Bubble Tea 组件)
│                        #   emits    : custom 事件 kind + payload JSON schema
│                        #   capability: 点亮的 feature 名
├── backend/             # Go 包:chat.CallMiddleware / chat.Tool / core.Extension
│                        #   干活后 Emit custom{kind:"my-feature", payload}
├── web/                 # TS 包:web kernel 插件,registerRenderer("my-feature", …)
└── tui/                 # Go 包:Bubble Tea 组件,注册到 TUI renderer 表 render("my-feature", …)
```

**接线(各按母语,互不加载)**:

| 半 | 接线 | 时机 |
|---|---|---|
| `backend/` | 运维的 lyra `main` import 它,经 `Config.ExtraExtensions/ExtraMiddleware` 注入 → rebuild | 编译期 |
| `tui/` | TUI 的 `main` import 它,注册进 Bubble Tea renderer 表 → rebuild | 编译期 |
| `web/` | web kernel 运行时 sideload(或 H5 壳同此) | 运行时 |
| 契约 | `pack.json` 的 `emits`/`capability` —— 后端发什么 kind、前端认什么 kind | 设计期约定 |

> **核心**:`backend` 半发 `custom{kind:"my-feature", …}`,三个视图半各自 `register("my-feature", …)`。
> 它们**只靠那个字符串 kind + JSON shape 对齐**,谁都不 import 谁。换 / 删一个视图半,后端零感知。

---

## 5 · 契约面(窄腰上必须有的几条开放车道)

这是 Pack 能"加带 UI 的新能力却不撑爆核心协议"的关键。**[纪律] 只走这几条开放车道,别逼协议 envelope 膨胀**:

1. **`custom` 语义事件 / item** **[提议]**
   现状 lyra 事件是 `run.*` / `item.*`,Item 是唯一 history+streaming primitive。
   需补一条 **`custom` item kind**(`{kind:string, payload:json}`),作为 Pack 后端半产语义、前端半认渲染的通道。
   它进 items.list(可持久化)、随流走,**核心协议不为每个 Pack 加类型**。
   **[纪律] v1 只做原子投递**:`item.started` / `item.completed`,**无 `item.delta`、无 replaces**——
   否则每个 Pack 都要自定义 delta merge 语义,契约面爆炸。流式 custom 等真实需求出现再议(YAGNI)。

2. **typed tool result** **[现状/扩展]**
   工具结果已是结构化文本;Pack 的工具产 typed result,视图半按 tool name 注册渲染(pi-mono `registerToolRenderer` 同构)。

3. **capability 派生** **[提议]**
   `/v2/info` 的 `capabilities.features` **不硬编码**,而是聚合"已接线贡献"——
   一个 Pack 的后端半在场 → 点亮它的 capability → **前端据此决定加不加载对应 view 半**。
   删掉后端半,capability 自动从握手消失,`info` 代码不动。这是**前端发现"某 Pack 后端在场"的唯一机制**。
   **现实论据(漂移已发生)**:`rpc/server/server.go` 里 `"skills": false` 注着 "Off until the
   corresponding engine support lands",而 skills 已接进 engine——flag 与实际接线之间没有任何
   对齐机制,歧义已成事实。同 map 里 `memory` 已是派生(`rt.Memory() != nil`),本条只是把既有模式推广到全部。
   **[纪律] capability key 命名**:Pack 的 key = pack name(kebab-case),**不得与核心 key
   (`mcp` / `memory` / …)重名**——features 是 open map,核心与 Pack 同平面,靠这条规约避免撞名。

4. **[纪律] 强制通用兜底渲染**
   每个 `custom` kind **必须**有"无 view-adapter 时的通用呈现"(Web:通用 JSON 卡片;TUI:通用文本块;headless:就拿数据)。
   否则某渠道缺这半 Pack → 白屏 / 崩。**这是多渠道的硬约束。**

5. **契约怎么维护** **[现状纪律]**
   Go flat-struct 不映射 TS discriminated union,**靠 review 保持 FE/BE/TUI 三方对齐,不上 codegen**
   (见 lyra 既有决议)。`pack.json` 的 `emits` schema 是 review 的锚点。
   **[提议] 把锚点变成可执行的**:Pack 的后端测试读自家 `pack.json` 的 `emits` schema,
   校验实际 emit 的 payload 通过——非 codegen 的轻量对齐,防"Go 改了 shape、pack.json 没人更新"的必然漂移。

---

## 6 · 后端 embed 口子(Model 半的入口)

**[提议] 这是要补的代码缺口**,也是整套"库化"的支点:

现状:`engine.New` 内部把 guardrails(tool + memory 中间件)拼死,`PlatformConfig.Extensions` 只塞内部 resolver,
**没有给外部嵌入者追加的口子**。补:

```go
// runtime.Config / engine.Config 增加(append 到内置之后):
type Config struct {
    // ... 既有字段 ...
    ExtraExtensions []core.Extension // [提议] 嵌入者贡献的 ToolGroupResolver / ChatClientProvider / …
    ExtraGuardrails *core.Guardrails // [提议] 追加的 Call/Stream 中间件(复用既有类型,不新造)
}
```

**[纪律] 不为"贡献"新造类型**:`core.Guardrails` 已存在(Call/Stream 两个 slice),直接复用。
**追加位置写死语义**:内置链是 tool MW → memory MW(`engine.go`),Extra **append 到链尾(memory 之后)**——
想插更前位置的需求出现时再开参数,不预留(YAGNI)。

于是运维**不 fork、不碰 lyra core**,在**自己的 repo**里:

```go
// my-lyra/main.go —— 我的组合根,import lynx 当库
rt, _ := lyraruntime.New(ctx, lyraruntime.Config{
    ChatClient:      client,
    ExtraGuardrails: myPack.Guardrails(),   // ← 我的包,我维护
    ExtraExtensions: myPack.Extensions(),
    // ... 其余照常
})
```

**复用既有那一个扩展机制**:`ExtraExtensions` 走 agent 的 `collectExtensions[T]`(per-run model 的 `ChatClientProvider`、
per-session cwd 的 `ToolGroupResolver` 已经验证这条路);中间件追加到 chat `MiddlewareManager` 链尾。
**不新增第二套机制**(`DESIGN_PHILOSOPHY` §2.3 一个扩展机制)。

> "加中间件"由此退化成:**一个自己的包 + main 里几行 + rebuild**。零运行时机器、编译期类型安全、可组合、不发散。

---

## 7 · 能力协商与降级(多渠道异构怎么收口)

- **客户端握手上报**:`kind`(web / tui / headless)+ 它支持的 surface 集。
- **runtime/kernel 据此**:只向该渠道投递**匹配的 view 半**,并决定降级到通用兜底(§5.4)。
- **[纪律] 这个协商结果不给后端插件**——后端永远只发一份语义,**降级是 View 侧的事**。
- **TUI 特例**:Bubble Tea 是 Go 静态二进制,它"支持哪些 kind"= **它编译期注册了哪些 renderer**;
  没注册的 kind 自动走通用文本兜底。Web 侧则是"sideload 了哪些 renderer"。**同一份语义,两种绑定,各自降级。**

---

## 8 · 信任与隔离(第一方 vs 第三方)

| 来源 | 机制 | 隔离 |
|---|---|---|
| **第一方 Pack(你的常态)** | 后端半 embed + rebuild;视图半进各自 build | **进程信任**——你控全链,无需沙箱;契约靠 review |
| **第三方·工具能力** | **MCP**(进程外,已是一等公民) | **OS 进程隔离**(白送),运行时接入、用户自控、不重编 lyra |
| **第三方·纯前端 view** | web kernel sideload(前端的动态本职) | 浏览器沙箱 |

- **[不做] 后端进程内插件运行时 / yaegi / Go `.so`**:
  第一方不需要(rebuild 更简单);第三方要隔离 → 进程外(MCP/子进程)更优且已存在。
  yaegi 解决的是我们没有的问题(运行时加载),解决不了我们真有的(隔离),且丢类型安全 + Go 版本保真风险。
- **[现状]** 信任边界 = 进程(lyra 无 user 概念;auth/billing/多租户**永远在未来 facade**,不进 Pack、不进协议)。

---

## 9 · 明确不做(反向不变量,血泪结论)

- ❌ **在后端造 kernel / Host / Point / Plugin / Disposable / registry 运行时框架**——`DESIGN_PHILOSOPHY` §1 设计错误信号。
- ❌ **把 run loop 做成"贡献进 kernel 的 RunStrategy"**——它是主动驱动器,不是被动 reducer,镜像前端只加间接不加价值。
- ❌ **为每类扩展开一个具名 `Point` 插槽**——用 Go 泛型的**一个** `collectExtensions[T]`(§2.3)。
- ❌ **后端进程内动态加载新代码(yaegi/.so)**——见 §8。
- ❌ **后端插件分支判断前端 kind**——Model 不耦合 View(§3 纪律)。
- ❌ **Pack 两半互相 import / 互相加载**——只靠协议 `custom` kind + 契约对齐(§4)。
- ❌ **fork lynx 改 core 来扩展**——嵌入(import 当库 + 自己的组合根),避开发散债。
- ❌ **Pack 逼核心协议 envelope 膨胀**——只走 `custom` / typed tool / 自定义 content-block 这几条开放车道(§5)。

---

## 10 · 落地路径(渐进,每步独立跑绿)

每步 `go build && go vet && go test ./...` 全绿再 commit,commit 写清 why。

1. **[后端口子]** `runtime.Config` / `engine.Config` 加 `ExtraExtensions` / `ExtraGuardrails`,append 到内置之后;
   `engine.New` 的 guardrails 改成"内置 + 追加"组合。**逻辑不动,只开口子。** 加一个嵌入冒烟测试(外部式 main 注入一个 no-op 中间件,验证生效)。
2. **[契约·capability 派生]** `/v2/info` 的 `capabilities.features` 改由"已接线贡献"聚合,而非硬编码常量。
   先把现有 mcp/memory/… 这些已知 feature 接上,行为不变,只是改成派生。
3. **[契约·custom 车道]** 协议加 `custom` item kind(`{kind, payload}`):进 items.list、随流投递、有通用兜底渲染约定。
   **前置:先在前端仓 `docs/API.md` 定契约形态**(lyra 纪律:动 `rpc/protocol` 先对前端契约)。
   后端牵连 `translator.go` 翻译路径 + items.list 持久化;通用兜底 renderer 是前端工作。v1 原子投递(§5.1)。
   跑通一条端到端 `custom` 事件收尾。
4. **[TUI 视图底座] [暂缓]** TUI(Bubble Tea)侧建一张 **renderer 表 + 编译期注册**(= Web kernel 的静态对偶),
   接 §3 的 `custom`/tool result,默认通用文本兜底。**从主路径摘除,作 TUI 立项时的第一步**——
   TUI 目前只有 inprocess transport 底座,Bubble Tea 端不存在,现在建表是 speculative(YAGNI)。
   摘除后 1→2→3→5 仍闭环(Web 通用兜底已覆盖"无 adapter 渠道"的验证)。
5. **[Pack 规范]** 定 `pack.json` schema + 一个**样板 Pack**(backend + web + tui 三半 + 契约),
   作为"加一个功能"的活模板;文档化"嵌入式 main"的写法。
   样板 Pack 须含 §5.5 的可执行契约测试(后端 emit 的 payload 过自家 `emits` schema 校验)。
6. **[第三方车道]** 维持 MCP 为第三方工具的进程外车道;按需补 Web 第三方 view 的 sideload 信任分级(默认 deny)。

> 1–2 是纯增量、零风险、马上有用;3 是契约底座;5–6 把"多渠道 Pack"变成可发现的常规动作;
> 4 暂缓至 TUI 立项。

---

## 11 · FE ↔ BE ↔ TUI 对称表

| 维度 | 后端(Go) | Web/H5(TS) | TUI(Go/Bubble Tea) |
|---|---|---|---|
| 角色 | Model(纯语义) | View | View |
| 绑定 | 编译期 embed(组合根) | 运行时 sideload(kernel) | 编译期注册(组合根) |
| 扩展机制 | `collectExtensions[T]` + Config 口子 | kernel registry | renderer 表 |
| Pack 半入口 | `ExtraExtensions/ExtraMiddleware` | `registerRenderer(kind)` | `render(kind)` 注册 |
| 信任 | 进程(第一方全信任) | 浏览器沙箱 | 本地进程 |
| 第三方 | 进程外(MCP/子进程) | sideload(默认 deny) | 编译期(= 第一方档) |
| 与对方耦合 | **仅协议 `custom`/capability/tool result** | 同左 | 同左 |

---

## 一句话收尾

**不要建插件系统——建两个口子 + 一条纪律。**
口子一:后端 `Config` 的 `ExtraExtensions/ExtraMiddleware`(库化的支点);
口子二:协议的 `custom` 车道 + capability 派生(Pack 跨进程对齐的契约面)。
纪律:**Model/View 拆到进程边界,后端只产语义、永不判断前端;每个语义有通用兜底。**
于是一个 Pack = 同仓同版的几半,**Go 目标编译期 embed、JS 目标运行时 sideload,全靠协议契约连起来**——
多渠道(Web / H5 壳 / Bubble Tea TUI / headless)各按母语接线,后端一套语义喂所有。
