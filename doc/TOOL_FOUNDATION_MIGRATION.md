# Tool 基础契约迁移

本次是 v0 阶段的 breaking change：共享工具契约已从具体实现模块 `tools` 下沉到独立的
`tool` 模块。仓内消费方已一次性迁移，不提供 alias、deprecated wrapper 或双轨兼容层。

## 新的所有权边界

| 能力 | 新 owner |
|---|---|
| `Tool`、`WrappingTool`、`Capability` | `github.com/Tangerg/lynx/tool` |
| `Registry` 及其 sentinel errors | `github.com/Tangerg/lynx/tool` |
| `New`、`Config` 与具体 shell/fs/http/web/skill 工具 | `github.com/Tangerg/lynx/tools` |

依赖方向固定为：应用装配 → `tools` / `agent` / `mcp` / `a2a` → `tool` → `core` → stdlib。
`agent`、`mcp`、`a2a` 只依赖共享契约，不再因工具实现集合的依赖增长而被动膨胀。

## 消费方迁移

将契约 import 从：

```go
import "github.com/Tangerg/lynx/tools"

var values []tools.Tool
registry, err := tools.NewRegistry(values...)
```

改为：

```go
import "github.com/Tangerg/lynx/tool"

var values []tool.Tool
registry, err := tool.NewRegistry(values...)
```

需要 typed function adapter 时同时导入两个模块：

```go
import (
    "github.com/Tangerg/lynx/tool"
    "github.com/Tangerg/lynx/tools"
)

executable, err := tools.New(tools.Config{Name: "lookup"}, lookup)
registry, err := tool.NewRegistry(executable)
```

外部实现只需继续满足 `Definition() chat.ToolDefinition` 与
`Call(context.Context, string) (string, error)`；方法本身没有变化，变化的是接口的 import path。
