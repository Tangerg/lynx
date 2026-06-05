# REFACTORING.md — lynx 重构要求

> 跨模块通用的**重构标尺 + 节奏**。设计哲学的"为什么"见 [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md);
> 项目级总约定见 [`CLAUDE.md`](CLAUDE.md)。本文件是"重构时**改什么、怎么改、按什么节奏**"的泛化清单,
> 适用**所有 sub-module**。全部抽象表述,**不绑定任何具体实现**——遇到对应场景时按本则判断。

---

## 0. 判据来源(用什么尺子)

- 对标 **go-sdk(尤其 `design/`)+ Go 标准库**:minimal、idiomatic、*accept interfaces / return structs*、小接口、options struct、future-proof、零值可用。
- **精修 ≠ 重写**:外科级、可逆、**在源头改对**,不在错的设计上叠补丁。参考业界只取思想、**不作命名锚**——名字恰好相同只因它独立评估下最优。
- **唯一允许背的"债"是"设计还没想清楚"本身**;绝不允许"明知更好、却为省事不改"。

## 1. 命名(名实相符)

- 名字必须与其**承载的数据 / 所做的事**一致;名不副实(类型名 ≠ 内容、方法名 ≠ 行为)就改。
- 字段名 == 序列化 tag;不一致时**优先改名而非将就 tag**。
- 消除 **package-name stutter**(`pkg.Pkg…`)。
- **文件名描述内容**:泛化 / Java 味文件名(`interface.go` / `impl.go` / `util.go` / `helper.go`)→ 按内容命名。文件重命名是包内操作,不改 API。
- 参考对应领域的事实标准词汇,但仅在它独立评估下最优时采用。

## 2. 注释

- 只解释 **why**,不解释 *what*(代码自身说明 what)。
- 删过期 / 迁移 / 误导注释;**改名或重构后同步清理所有引用**,不留陈旧指向。

## 3. 指针 vs 值

- **必然存在 + 小 + 只读 → 传值**(无意义的 nil 态加一次解引用是纯负担)。
- **真可选(nil 是有意义的状态、且被代码分支)→ 传指针**。
- 同一签名里值与指针并存,只要各自满足上述理由,就是**有原则的区分**,不是不一致。
- **不存储可由方法即时计算的派生状态**(冗余缓存字段删掉,改按需计算)。
- 例外:某些构造器返回**值**以保证 immutability——不要为统一而改成返指针(见各模块既定约定)。

## 4. nil 守卫(pointer-receiver 卫生)

- 指针接收者方法**在顶部自守 nil**,让调用方无需先判 `!= nil`。
- **仅加在 nil 真正可达的读访问器上**(返回零值 / sentinel)。
- **不加在** mutator、服务型行为方法、内部 helper 上——那里 nil 接收者是**构造 bug**,应当暴露而非静默 no-op。
- 已天然 nil-safe 的(返回常量、不碰接收者、经 nil-safe 委托、已有守卫)**不重复加**。

## 5. 自由函数 vs 方法(控制包作用域)

- 主参数是某**具体类型**、且只读它的包级私有函数 → 挂成该类型的**方法**(移出包作用域;编译器照样内联,零开销)。
- **保留为自由函数**:操作切片 / sealed interface 的(没有类型可挂,类比 `slices.*`)、`newXxx` 构造器、跨包类型的 helper(Go 无法给外部类型加方法)。

## 6. 卫语句 / 圈复杂度

- `if cond { 大段 }` + 尾部收尾 → `if !cond { return }` + 平铺;合并多层嵌套条件;把内聚分支抽成 helper。
- 但流式迭代器(`iter.Seq2` 等)的 `func→func→for→if !yield{return}` 是**结构性嵌套**,不是逻辑复杂度,**不动**。

## 7. 现代 Go(按各模块 go 版本)

- 用到当前 go 版本的现代特性,替代旧写法:`any`、`min/max`、`slices.*`/`maps.*`、`iter.Seq2`、`range int`/`range slice`、`time.Since`、`omitzero`(time/struct/而非失效的 omitempty)、类型化 atomic、`errors.AsType`(**仅当目标是 error 类型**)等。
- 用 use-modern-go skill 取当前版本指导;**只清真 straggler,不为改而改**(成熟代码往往已经很现代)。

## 8. 组织 / 就近原则

- **相关代码放一起,公共的下沉到公共处**,以提升阅读体验为目标。
- **god-file**(超行数阈值且混多个关注点)按职责拆成**同包多文件**(纯包内移动,不改 API、不动 import)。
- **大但内聚、单一职责的不拆**(如解析器 / 词法器 / 紧耦合的 builder 族)——拆了反而破坏内聚。

## 9. Go idiom 硬规则

- `errors.New` 优先于 `fmt.Errorf("常量")`;包装错误一律 `%w`(连 `panic` 路径也对齐)。
- 局部结果累加器用 `nil` slice 而非 `make([]T, 0)`;**字段恒非 nil 的既定 house-style 保留**。
- 构造器**出错返回 nil**,不返回"半成品 + error"。
- 重构途中发现的**真实 bug 顺手修**(独立 commit,与重构分开)。
- 禁推测性占位(`// TODO: 以后接` / stub interface)、死代码立即删。

## 10. 节奏与纪律(怎么推进)

1. **先深度审计**(grep / Explore / 读文件),不直接动手。
2. **分类**(命名 / 耦合 / 内聚 / SOLID / DRY / 现代 Go / 指针值 / nil / 组织)按 impact 排序。
3. **给批次方案** + 每项"动 vs 不动"权衡。
4. **破坏性或结构性改动先确认**:列 scope + **爆炸半径**(所有跨模块消费方)+ 备选方案,等用户拍板。
5. **每批一个可独立 revert 的 commit**;批与批之间 `go build && go vet && go test ./...` **全绿**;commit 后推送。
6. commit message 写清 **why**(含 audit 发现 + skip 理由)。
7. **承认 audit 误报**:深入看发现是 false positive 就 skip 并记录理由——正常,不是失败。

> 触发信号、Fowler 式重构清单(死代码 / 卫语句 / 查表 / 接口收窄 / 性能扫描等)、小型 vs 大型重构两档节奏,
> 详见 [`CLAUDE.md`](CLAUDE.md) 的「重构策略」与「Go idiom 纪律」段落。
