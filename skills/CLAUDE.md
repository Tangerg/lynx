# CLAUDE.md — skills module

> Read-only repository over **Agent Skills** (https://agentskills.io) — SKILL.md 目录的解析 / 校验 / 取用基础能力。LLM 可调的 `skill` 工具是 `tools/skills` 的薄封装,不在本模块。
> 项目级约定见 `../CLAUDE.md`。

---

## 一句话定位

把"一个装着 `SKILL.md`（YAML frontmatter + Markdown 指令）+ 可选 `references/` `assets/` `scripts/` 资源的目录"读出来的**纯基础能力层**。只解析、校验、按需取用内容;**不执行脚本**（agent 用自己的 bash/fs 工具跑），**不认识 `chat` / tool**（工具封装在 `tools/skills`）。是 tools 模块"两层 SPI"里的 backend 层 —— 与 `bash.Executor` / `fs.Executor` 同位。

## 技术栈

- Go 1.26.3
- `gopkg.in/yaml.v3` —— 解析 frontmatter（唯一外部依赖）
- **零业务依赖**：不 import `core` / `agent` / `lyra`，连 `chat.Tool` 都不碰 —— 渐进披露的三个能力是裸 Go 类型
- ~0.3k LOC

## 核心架构

渐进式披露（progressive disclosure）的三级，对应 `Source` 接口三方法：

- **`source.go`** —— `Source` 接口（消费方定义）+ `FS` 实现（任意 `fs.FS`，懒读，外部改动免刷新）
  - `List(ctx) []Summary` —— level 1：每个 skill 的 name + description（够 agent 判断相关性）
  - `Load(ctx, name) *Skill` —— level 2：单个 skill 的完整指令 body
  - `LoadResource(ctx, name, path) []byte` —— level 3：skill 目录下的 bundled 文件，按需读
- **`frontmatter.go`** —— `Frontmatter` 结构 + `Parse`（拆 `---` 围栏 / body）+ `Validate`（规范规则）
- **`skill.go`** —— `Skill`（`Frontmatter` + `Body`）/ `Summary` 类型
- **`errors.go`** —— sentinel（`ErrNoFrontmatter` / `ErrName*` / `ErrResourcePath` …）

## 关键接口/类型

- `Source` —— **接口在消费方（本包）定义**：真实目录、embedded FS、远程 store、测试 fake 都能满足。`FS` 是文件系统实现，`NewFS(fsys)` / `Dir(root)` 构造，`var _ Source = (*FS)(nil)` 编译期断言
- `Frontmatter` —— 规范字段：`name`（必填，须匹配目录名）/ `description`（必填）/ `license` / `compatibility` / `metadata` / `allowed-tools`。`Validate()` join 全部违规一次返回；`AllowedToolList()` 拆空格分隔串
- `Skill` —— `Frontmatter` 内嵌 + `Body`（Markdown 指令）；`Summary()` 投影成 metadata 视图
- `Parse([]byte) (Frontmatter, body, error)` —— 只拆不校验（校验单独 `Validate`）

## 强约定

- **规范即真理**：`name` 1-64 小写字母数字 + 单连字符（无首尾/连续连字符，正则 `^[a-z0-9]+(-[a-z0-9]+)*$`）、`description` 1-1024、`compatibility` ≤500、`name` 必须等于目录名（`Load` 强制 `ErrNameMismatch`）
- **不执行 `scripts/`**：本模块只把脚本**内容**通过 `LoadResource` 交出去;真正运行由 agent 的 bash/fs 工具负责（KISS + 不重复造 bash + 安全）
- **`allowed-tools` 解析不强制**：实验字段,各家都这么处理;`AllowedToolList()` 给愿意自己执行的 caller
- **路径不可逃逸**：`LoadResource` 用 `path.Join(name, resource)` + 前缀校验,`..` 穿透出 skill 目录 → `ErrResourcePath`;`validName` 挡住 name 里的 `/` `\` `..`
- **`List` 跳过非法项不报错**：非目录 / 无 `SKILL.md` / 校验失败的目录直接 skip,不让一个坏 skill 拖垮整张列表（同 ecosystem 行为）
- **懒读 per-call**：不缓存、不预扫;skills 数量少,外部编辑即时可见。要缓存/预载是 caller 的事

## 强反向不变量

- ❌ **import `core` / `chat` / 任何业务模块**：本模块是裸能力层,渐进披露三方法返回裸类型;`chat.Tool` 封装在 `tools/skills`
- ❌ **在本模块执行脚本 / 引沙箱 / Docker**（embabel 那套）：脚本执行是 agent 既有工具的事,不在这里重造
- ❌ **强制 `allowed-tools`**：实验字段,只解析
- ❌ **缓存 / pubsub / 状态管理**（crush Manager 那套）：YAGNI,本模块只读;有需要在 caller 包

## 关键目录

```
skills/
├── source.go       Source 接口 + FS 实现（List/Load/LoadResource）
├── frontmatter.go  Frontmatter + Parse + Validate
├── skill.go        Skill / Summary 类型
├── errors.go       sentinel
└── doc.go          包说明
```

## 常用命令

```bash
go build ./...
go test ./...   # fstest.MapFS 驱动 List/Load/LoadResource + Parse/Validate + 路径逃逸
```

## 修改任何东西之前

- **这是基础能力层,不是工具**：要改 LLM 看到的工具形态（op / schema / 渲染）去 `tools/skills`,不在这里
- **改 `Source` 接口**：是消费方契约,`tools/skills` + 任何自定义实现都受影响 —— 破坏性改动先咨询
- **加新 backend**（远程 skill store / embedded FS）：实现 `Source` 接口即可,`tools/skills.NewTool(yourSource)` 注入,不碰 FS
- **规范升级**（agentskills.io 改字段）：动 `Frontmatter` + `Validate`,对照 https://agentskills.io/specification
```
