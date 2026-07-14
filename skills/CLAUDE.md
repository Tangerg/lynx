# CLAUDE.md — skills module

> Agent Skills(agentskills.io)的**只读仓库层**:解析 / 校验 / 按需取用 `SKILL.md` 目录(YAML frontmatter + Markdown 指令 + 可选 bundled 资源)。是 tools 两层 SPI 里的 backend 层,与 shell / fs 的 Executor 同位;LLM 可调的 `skill` 工具是 `tools/skills` 的薄封装,不在此。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md)。规范字段细则以 agentskills.io 为准;具体符号以代码为准 —— 本则只讲宏观。

---

## 定位

- **纯基础能力层**:只把 skill 目录读出来 —— 解析、校验、按需取内容。**不执行脚本**(交给 agent 自己的 shell / fs 工具),**不认识 chat / tool**(工具封装在 tools/skills)。

## 架构心智

- **渐进式披露分三级、拆成两个接口**:列表(name + description,够判断相关性)→ 载入单个 skill 的完整指令 → 按需打开 bundled 资源文件。只用前两级的调用方依赖窄接口,需要资源的依赖扩展接口 —— 这是本模块的 ISP 落点。
- **零业务依赖**:不 import core / agent / chat,三级披露返回裸 Go 类型 —— 保证它稳居 DAG 底部。
- **规范即真理**:字段规则、name 必须等于目录名等以 agentskills.io 规范为准,校验一次性汇报全部违规。
- **只交内容,不执行**:脚本资源只把**内容**交出去,真正运行由 agent 的既有工具负责(KISS + 不重造 shell + 安全)。
- **路径不可逃逸**:资源打开锚定在 skill 目录内,`..` 穿透被拒 —— 这是信任边界。
- **坏 skill 不拖垮列表**:列表跳过非法项而不报错(与生态行为一致)。
- **懒读 per-call**:不缓存、不预扫,外部编辑即时可见;要缓存 / 预载是调用方的事。

## 模块特有反向不变量

- ❌ **import core / chat / 任何业务模块** —— 本模块是裸能力层,`tools.Tool` 适配封装在 `tools/skills`。
- ❌ **在本模块执行脚本 / 引沙箱 / Docker** —— 脚本执行是 agent 既有工具的事,不在这里重造。
- ❌ **强制 allowed-tools** —— 实验字段,只解析,交给愿意自己执行的调用方。
- ❌ **加缓存 / pubsub / 状态管理** —— YAGNI,本模块只读;有需要在调用方包。

## 改动前必看(波及面)

- **要改 LLM 看到的工具形态**(op / schema / 渲染):去 tools/skills,不在这里。
- **动 Source / ResourceSource 接口**:是消费方契约,tools/skills 与任何自定义实现都受影响。
- **规范升级**(agentskills.io 改字段):动 frontmatter 结构 + 校验,对照官方规范。
- **加新 backend**(远程 skill store / 嵌入式 FS):需要资源读取实现扩展接口,只需列表 / 载入实现窄接口。
