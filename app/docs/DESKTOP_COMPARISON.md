# Desktop 形态对比 —— app/desktop vs 桌面 GUI AI 应用

> **对比对象**:`app/desktop`(Lyra 桌面前端,Wails 薄壳 + 插件化内核 + 协议驱动)对 3 个有代表性的 GUI AI 应用:
> **AionUi**(Electron 桌面 agent,TS)· **Cherry Studio**(Electron LLM 工作台,TS)· **Cline**(VS Code 扩展 + headless SDK,TS)。
>
> **方法**:源码级核实(非文档/记忆),各 peer 经其桌面源码(`~/Desktop/<name>`)第一手核对,带 file 证据。基线截至 **2026-06-19**。
> **后端/引擎能力**(agent loop / 工具 / 沙箱 / 持久化…)的对比见 [`RUNTIME_COMPARISON.md`](RUNTIME_COMPARISON.md);本篇只谈**桌面 GUI 形态**:引擎位置、插件化、状态、聊天 UI、审批 UI、设计系统。
> **方法论**:对照 [`../desktop/CLAUDE.md`](../desktop/CLAUDE.md)("Kernel 不长肉 / 不内嵌 runtime / 协议驱动")与项目"薄核 / 库优于框架 / 不堆料"立场裁决"该不该学"。

---

## 0. TL;DR

`app/desktop` 的三条立身之本——**(1) 纯协议分离**(不内嵌引擎,经 JSON-RPC 连独立 runtime,且这是**唯一**模式);**(2) Kernel 不长肉**(路由/布局/渲染/命令/快捷键/主题/设置/审批全是插件贡献);**(3) 单一克制设计语言**(Radix + 自研 token + cards-on-canvas/no-lines/dark-first)——在这三条上**领先全部三个对比对象**:

- **引擎位置**:Cherry 把引擎**焊在 Electron main 进程**(lyra 的反面);Cline 把引擎放 **extension host**(但用双 gRPC 边界 + 新 SDK 的 hub 做了解耦先例);**只有 AionUi 真正分离引擎**(spawn 独立 `aioncore` 二进制 over HTTP+WS)——和 lyra 同路,但 AionUi *打包并托管* 后端,lyra *连接已存在的*独立 runtime,更彻底。**cline 的新 SDK hub(RuntimeHost + WebSocket envelope ≈ JSON-RPC)验证了 lyra 的方向,但那是它的 alternate path,不是 primary。**
- **插件化**:**只有 lyra 和 AionUi 是真插件架构**;Cherry/Cline 都**没有第三方插件 loader**(功能是硬编码页面 + 数据型 skills/rules/MCP)。lyra 的 "kernel 不长肉" 比 AionUi 更彻底(连核心 UI 都是插件贡献)。
- **设计纯度**:Cherry(antd + Tailwind + styled-components + 自研 Radix kit 四套共存)和 Cline(VSCode-toolkit + shadcn/Radix + heroui + styled-components 混合)都背着**多套样式系统并存的债**;lyra 的单一 Radix + token 设计语言最自洽。

**校准**:几个本以为是 gap 的,lyra **其实已有基础**——HTML artifact 渲染(`HtmlArtifact.tsx`)、审批设置面板(`settings/approvals/`)、虚拟化栈、tab 多会话、Zustand 分层。所以 desktop 侧真正要做的是**扩展/增强**(把 artifact 扩到 PDF/Office、把审批面板做成 auto-approve matrix),而非补齐空白。

---

## 1. 速查矩阵

> ✅ 有且成熟 · 🟡 部分/弱 · ❌ 无 · ★ 标杆

| 维度 | AionUi | Cherry Studio | Cline | **app/desktop** |
|---|---|---|---|---|
| **外壳** | Electron + React 19 | Electron + React 19 | VS Code 扩展 + React webview | Wails(Go 壳)+ React/TS |
| **引擎位置** | ✅分离(spawn 二进制,HTTP+WS) | ❌内嵌 main 进程 | ❌内嵌 extension host(+hub 解耦先例) | ★**纯分离**(JSON-RPC 连独立 runtime,唯一模式) |
| **协议** | HTTP REST + 多路 WS(13400) | Electron IPC(无协议) | gRPC over postMessage(ProtoBus)+ HostBridge | ★Lyra Protocol(JSON-RPC,HTTP+SSE/inproc) |
| **第三方插件系统** | ✅(`aion-extension.json`+marketplace+iframe 沙箱) | ❌(硬编码页面+数据型扩展+mini-apps) | ❌(无 API;.clinerules/hooks/skills/MCP) | ★✅**Kernel 不长肉,全功能=插件(ExtensionPoint+contribute/selector)** |
| **状态管理** | Context+SWR+外部 store | Redux Toolkit(28 slice) | React Context(~50 useState,全 state fan-out) | ✅小 Zustand store 分层 |
| **streaming 状态** | ✅外部 store+批量 flush(不进响应式) | ✅overlay(不进 Redux) | 🟡全 state 序列化推送 | ✅getState 读、top-level 不订阅 |
| **聊天 UI 富度** | ✅markdown/code/mermaid/katex/tool-viz/reasoning | ✅+artifacts/RAG 引用 | ✅+checkpoints/focus-chain row | ✅markdown/code/tool-viz/reasoning |
| **artifact/canvas** | ★✅Preview 面板(HTML/MD/PDF/Office+inspect) | ✅HTML artifacts | ❌ | 🟡**已有 HTML artifact**(可扩展) |
| **消息列表虚拟化** | ❌**未虚拟化(风险)** | 🟡 | ✅Virtuoso | ✅有虚拟化栈 |
| **审批/HITL UI** | ✅inline 消息审批+session 缓存;无 plan gate | ✅AI-SDK approval gate+PermissionMode | ★plan/act 模式+auto-approve matrix | ✅**已有 approvals 面板**(可增强成 matrix) |
| **多会话** | 🟡无 tab(单活动+后台并发+搜索) | ✅tab+assistants/topics | 🟡history(单活动 task) | ✅tab 多会话+draft |
| **provider 集成** | 经后端(~30,前端不碰 key) | main 直连(20+,持 key) | host 直连(42,持 key) | ★经 runtime(前端不碰 key) |
| **设计系统** | Arco + UnoCSS + token + 主题 gallery | 🟡antd+Tailwind+styled+自研 Radix(4 套共存) | 🟡VSCode-token+shadcn+heroui+styled(混合) | ★Radix + 自研 token(单一语言:cards-on-canvas/no-lines) |
| **定位** | 功能最全的 cowork 平台 | LLM 工作台(coding 是 bolt-on) | IDE 内 coding agent | 协议驱动的 agent client |

---

## 2. 引擎位置 —— `app/desktop` 的"纯协议分离"最彻底,且被 peer 的新路径验证

这是 desktop 形态最核心的轴线。四家的分野:

- **Cherry Studio = 内嵌(lyra 的反面)**:agent loop + 所有 provider API 调用 + MCP + 内嵌的 claude-agent-sdk **全在 Electron main 进程**(`src/main/ai/streamManager/AiStreamManager.ts:runExecutionLoop`),renderer 只是 IPC 流的消费端。**UI 无法脱离引擎独立运行。**
- **Cline = 内嵌 + 解耦先例**:agent loop 在 extension host(`core/task/index.ts`),webview 经 **ProtoBus(gRPC over postMessage)** 订阅全量 state。**但** 它有意做了 `HostProvider`/`HostBridge` 抽象 + standalone gRPC server(`protobus-service.ts`,TCP 26040),让 core 能跑在 VS Code/JetBrains/headless;新 SDK(`sdk/packages/core/src/hub/`)更进一步:`RuntimeHost`(Local/Hub/Remote)+ WebSocket **command/reply/event envelope**(≈JSON-RPC)。
- **AionUi = 真分离**:desktop 包**无 agent loop**,spawn 独立 `aioncore` 二进制,GUI 全程经 **HTTP REST + 多路 WebSocket**(13400)驱动(`common/adapter/httpBridge.ts`),Electron IPC 只留给原生外壳操作。**架构上和 lyra 同路。**
- **`app/desktop` = 纯分离,且是唯一模式**:Wails Go 壳**不内嵌 runtime**,经 Lyra Protocol(JSON-RPC,HTTP+SSE / inprocess)连**已存在的**独立 runtime。

**裁决:lyra 走得最彻底。** AionUi 也分离,但它*打包并 lifecycle-管理*后端(spawn 二进制),且 Electron 进程里还留着 image-gen/WebUI/pet 等领域逻辑;lyra 连接一个独立部署的 runtime,壳里零引擎逻辑。Cline 的 hub/standalone 路径**正是可以拿来佐证 lyra 方向的先例**——但它是 cline 的 *alternate path*,mainline 仍是 engine-in-host;**lyra 把"薄 GUI + 引擎在独立 runtime + 协议通信"当成唯一姿态,这是与所有 peer 的根本差异化。** Cherry 是纯反面(焊死),是"别走的方向"的教材。

---

## 3. 插件化 / 可扩展性 —— `app/desktop` 的 "Kernel 不长肉" 最彻底

只有 **lyra 和 AionUi** 是真正的**第三方代码插件架构**:
- **AionUi**:`aion-extension.json` manifest(`onActivate`/`onDeactivate` 生命周期 + permissions + `contributes`)+ **AionHub marketplace**(tarball + SRI 索引,装/更新/卸)+ 扩展设置渲染在 **沙箱 iframe**。扩展能贡献 acpAdapters/assistants/MCP/skills/channels。
- **`app/desktop`**:**Kernel 不长肉** —— 路由/布局/内容渲染/命令/快捷键/主题/运行时事件处理/**设置面板/审批面板**全部由内置插件经 **typed ExtensionPoint + contribute(写)/selector(读)** 贡献,Kernel 自己只是命名 Slot + 几个共享 store。**比 AionUi 更彻底**:AionUi 的核心 UI 仍是硬编码、插件是外挂层;lyra 连自己的核心 UI 都是插件贡献(同一条扩展底座)。

Cherry/Cline **都没有第三方插件 loader**:
- Cherry:"扩展"=数据型(skills/agents/mini-apps),功能是硬编码页面;内部有 aiCore 中间件引擎但不对用户开放。
- Cline:无插件 API,扩展性靠 `.clinerules` + workflows + hooks + skills + MCP marketplace(都是数据/外部可执行,非 JS 插件);新 SDK 才有 `AgentPlugin`。

**裁决:lyra 的插件化是全场最彻底的,是核心差异化,不是要学的方向。** 唯一可借鉴:**AionUi 的 marketplace + iframe 沙箱**(第三方插件的分发与隔离)——但这是生态/分发层,等 lyra 真有第三方插件生态需求再做(YAGNI)。

---

## 4. 状态管理 —— "streaming 别进响应式 store" 是共识,`app/desktop` 站对了

各家状态库不同,但**有一个共同洞察:高频 streaming 状态不能进响应式全局 store**:
- **`app/desktop`**:小 Zustand store 分层;streaming 走 `getState()` 读、render top-level 不订阅(否则 列表 × token 流 = 上千次 selector/秒)。
- **AionUi**:Context + SWR(server-state)+ **手写外部 store + listener set**(`conversationRuntimeViewStore`),token 批量 flush(setTimeout/RAF)。
- **Cherry**:Redux Toolkit,但 **streaming 显式不进 Redux**,用 `ExecutionStreamCollector` overlay。
- **Cline = 反面教材**:webview 用 React Context,host 每次变更**把整个 `ExtensionState` 序列化成字符串 fan-out** 给 webview——长会话下是明显的性能负担。

**裁决:lyra(getState 不订阅)、AionUi(外部 store+批量)、Cherry(overlay)殊途同归,都把 streaming 挡在响应式 store 外——lyra 做对了。Cline 的全量序列化推送是要避免的方向。** 无可学项。

---

## 5. 聊天 UI —— `app/desktop` 已有基础,artifact 可扩展

聊天富度大家都不弱(markdown/code/mermaid/katex/tool-call 可视化/reasoning 折叠/多模态)。关键差异两点,且 lyra 都**已有基础**:

- **artifact / canvas 面板**:**AionUi 是标杆**——Preview 面板支持 HTML/Markdown/PDF/PPT/Excel/Office/图片的查看+编辑,还有 **HTML inspect-mode**(点选 DOM 元素);Cherry 有 HTML artifacts(iframe 预览)。**lyra 已有 `HtmlArtifact.tsx`(HTML artifact 渲染)** —— 不是空白,可**扩展**到 PDF/Office/canvas inspect。**这是 desktop 侧最值得做的一项**:agent 产出富内容(图表/文档/网页)时,带外预览面板比塞进消息流体验好得多。
- **消息列表虚拟化**:**AionUi 未虚拟化(`react-virtuoso` 装了却没用,公认的长会话风险)**;Cline 用 Virtuoso;**lyra 有虚拟化栈**(diagnostics 已用)。lyra 不在 AionUi 的坑里。

---

## 6. HITL / 审批 UI —— `app/desktop` 已有面板,可借 Cline 的 auto-approve matrix 增强

- **Cline = 标杆**:**plan/act 模式滑钮**(可各配一个 model)+ **auto-approve matrix**(read/edit/safe-commands/all-commands/browser/MCP 逐项开关 + max-requests 上限,默认 20)。
- **AionUi**:inline 消息审批(ProceedOnce/Always/AlwaysTool/AlwaysServer/Cancel)+ session 级"always"缓存;**无 plan 审批 gate**。
- **Cherry**:AI-SDK v6 approval gate + `PermissionMode(default/acceptEdits/bypass/plan)`。
- **`app/desktop`**:**已有 `settings/approvals/` 面板** + 协议驱动的审批(runtime 推 interrupt,前端渲染确认)。

**裁决:lyra 审批 UI 有基础,值得借 Cline 的 auto-approve matrix 把它增强**——逐工具/逐类别的自动批准开关 + 请求数上限。**这与 [`RUNTIME_COMPARISON.md`](RUNTIME_COMPARISON.md) §6 的"runtime 侧细粒度权限规则"是配套的**:runtime 提供规则引擎,desktop 提供配置这些规则的 matrix UI。两侧一起做才完整。

---

## 7. Provider 集成 —— `app/desktop` 的"前端不碰 key"是协议分离的红利

- **`app/desktop` / AionUi**:GUI **不直接调 LLM**,provider key + 模型调用都在后端(runtime / aioncore);前端只发"用哪个 provider+model"。
- **Cherry / Cline**:在 main 进程 / extension host **直连 provider**,**API key 持在 GUI 侧进程内**(cline 42 个 provider handler 各自 `new Anthropic({apiKey})`)。

**裁决:lyra 的"前端零 provider key、全经 runtime"是协议分离的安全红利**——key 只在后端,前端被攻破也不泄露凭证;且换 provider 是 runtime 的事,前端零改。Cherry/Cline 的前端直连是其内嵌架构的必然代价。lyra 领先,无可学项。

---

## 8. 设计系统 / 原生体验 —— `app/desktop` 单一语言最自洽

- **`app/desktop`**:**单一设计语言** —— Tailwind utility-first + Radix primitives(带交互/焦点/键盘/aria 的一律先用)+ 自研 token,设计法则克制且成文(cards-on-canvas、**no lines**(surface ladder 不用 1px 边线)、dark-first、借鉴 Linear/Vercel)。
- **AionUi**:Arco Design + UnoCSS + token + 主题 gallery(light/dark/system + 自定义 CSS)——干净,但绑定 Arco 组件库。
- **Cherry = 反面**:antd v5 + Tailwind + styled-components + 自研 `@cherrystudio/ui`(Radix)**四套样式系统共存**(v1→v2 迁移半成品),是明显的技术债。
- **Cline**:VSCode theme token(强 IDE-native)+ shadcn/Radix + heroui + `@vscode/webview-ui-toolkit` + styled-components **混合**——native 调色但组件层多套并存。

**裁决:lyra 的单一 Radix + token + 克制法则是全场最自洽的。** Cherry/Cline 的"多套样式共存"正是 lyra 法则(不引入完整 UI Kit、Radix first、不写新 .css)要避免的。Cline 的 VSCode-theme-native 是它依附 IDE 的产物,lyra 是独立 Wails 应用,不依附、有自己的设计语言——by-design 差异,不追。

---

## 9. 多会话 & 独特能力

**多会话**:lyra(tab 多会话 + draft)与 Cherry(tab + assistants/topics)是 tab 派;AionUi(单活动 + 后台并发 + 全文搜索)、Cline(单活动 task + history)是单活动派。**lyra 的 tab 多会话是体验优势。**

**peer 独特能力(多数是堆料 / 出 scope)**:AionUi 的 Team 多 agent 并排面板、cron、WebUI 远程访问、Telegram/飞书/钉钉/微信/企微 channel、桌面 pet、Office 生成;Cherry 的 RAG 知识库、paintings/translate/OCR/notes、mini-apps;Cline 的 checkpoints timeline、focus chain、MCP marketplace、worktree UI、双 host 边界。

**裁决**:这些里**真正值得 desktop 借鉴的极少**——大多是"产品堆料"(IM channel / pet / Office / OCR / 翻译 / 画图)或出 agent-client scope(RAG 知识库归 runtime/rag 模块)。**Team 多 agent 并排面板**(AionUi)在 lyra 真要做多 agent 可视化时可参考;**checkpoints timeline UI**(Cline,可视化 rollback 点)与 runtime 的影子 git checkpoint 配套时可参考。其余不追。

---

## 10. 该学什么 —— 批判性裁决

desktop 侧真正值得动手的只 2 项,且都是**扩展已有基础**而非补空白:

| 优先级 | 学什么 | 来源 | 怎么落地(lyra 方式) | 为什么值得 |
|---|---|---|---|---|
| **中** | **artifact 预览面板扩展** | AionUi / Cherry | 把现有 `HtmlArtifact.tsx` 扩成一个 **artifact 插件**:HTML/Markdown/(后续 PDF/图片)带外预览面板,走 ExtensionPoint 贡献一个 workspace slot;数据来源是工具产出的 artifact(呼应 runtime 篇若做 artifact-bearing tool) | agent 产出富内容时,带外预览比塞进消息流体验好;lyra 已有 HTML 基础,扩展成本低 |
| **中** | **auto-approve matrix(审批面板增强)** | Cline | 把现有 `settings/approvals/` 面板增强成逐工具/逐类别的自动批准开关 + max-requests 上限;**与 runtime 侧细粒度权限规则配套**(runtime 出规则引擎,desktop 出配置 UI) | lyra 审批 UI 有基础但偏简单;高自主运行需要可视化的自动批准配置。两侧一起做才完整 |
| **低** | 插件 marketplace + iframe 沙箱 / Team 多 agent 面板 / checkpoints timeline | AionUi / Cline | 等生态/功能需求触发再做 | 都是"有真实需求才值"的扩展点,非当前 gap |

**明确不学(堆料 / 反面 / by-design / 出 scope):**
- **内嵌引擎**(Cherry main / Cline host):lyra 的纯协议分离是核心差异化,绝不内嵌。
- **多套样式系统共存**(Cherry / Cline):违背 lyra 单一设计语言法则,是技术债教材。
- **VSCode-theme 依附**(Cline):lyra 是独立 Wails 应用,有自己的设计语言。
- **IM channel / 桌面 pet / Office 生成 / OCR / 翻译 / 画图 / mini-apps**(AionUi / Cherry):产品堆料,出 agent-client scope。
- **RAG 知识库 UI**(Cherry):知识/检索归 runtime 侧(`rag` 模块),不在桌面壳。
- **前端直连 provider**(Cherry / Cline):lyra 前端零 key、全经 runtime,是安全红利,不退回。

---

## 一句话定档

**`app/desktop` 在桌面 GUI 形态的三条根本轴线——纯协议分离(不内嵌引擎)、Kernel 不长肉(全功能插件化)、单一克制设计语言——上领先全部三个对比对象:Cherry 是焊死引擎 + 四套样式的反面,Cline 的 hub/standalone 路径反而验证了 lyra 方向(但只是它的 alternate path),AionUi 架构最接近(也分离引擎)却功能堆料且消息列表未虚拟化。lyra 的 artifact 渲染、审批面板、虚拟化、tab 多会话都已有基础,desktop 侧真正要做的是两项配套增强——artifact 预览面板 + auto-approve matrix(后者与 runtime 侧权限规则配套),其余 peer 独有项多为堆料或反面教材,继续巩固薄壳 + 插件化 + 协议驱动的差异化。**

---

*对比基线截至 2026-06-19。各 peer 能力经其桌面源码第一手核实。本篇对应后端引擎能力对比 [`RUNTIME_COMPARISON.md`](RUNTIME_COMPARISON.md)。*
