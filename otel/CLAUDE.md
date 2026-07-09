# CLAUDE.md — otel module

> 把 OTel 三驾马车(Traces / Metrics / Logs)的 **dev sink 统一成 `log/slog`**:三个 exporter,每个信号一份。本机开发 / 单进程后端看着方便用,不是生产 exporter。
> 项目级法则见 [`../CLAUDE.md`](../CLAUDE.md);完整可观测性规约见 [`../doc/OBSERVABILITY.md`](../doc/OBSERVABILITY.md)。具体符号 / 依赖版本以代码为准 —— 本则只讲宏观。

---

## 定位

- **三驾马车都落到 slog**:span、metric、OTel log record 各有一个 exporter 写成 slog record。
- **为什么走 OTel 而不是直接写 slog**:可替换性 —— dev 用 slog 看着方便,生产把每个 exporter 换成 OTLP(→ 云端 tracing / logging),**业务代码零改**。这是 vendor-neutral 的意义。
- **Logs 也是一等 OTel 信号**:应用经 contrib 的 `otelslog` bridge 把 slog 喂进 LoggerProvider,再由本模块的 log exporter 落地 —— 不是"绕开 OTel 直接打日志"。

## 架构心智

- **三个 exporter 都实现 OTel SDK 的标准接口**,不自造接口:各信号一份,互不耦合。
- **log handler 不在本模块**:用 contrib 的 `otelslog`(slog handler → LoggerProvider),本模块只提供它下游的 log exporter。
- **组合根一次性绑定**:startup 设全局三 provider + 把 slog 默认 handler 换成 bridge + W3C propagator,之后 `otel.Tracer` / `otel.Meter` 直接用,零 DI。
- **dev 优先的取舍**:export 永远返回 nil(不让落地失败污染业务流)、同步 flush(不批量缓冲)、error span 升级到 error 级别。
- **attribute 原样转、key 去品牌**:semconv 有就用,否则裸 domain 名,不带项目前缀(instrumentation scope 名保留库路径 —— 那是库标识,不是数据)。

## 模块特有反向不变量

- ❌ **在本模块做 OTLP / Jaeger / Zipkin exporter** —— 那是生产 exporter,直接用 OTel 官方 contrib。本模块的定位就是"本机一行一个 span 看着方便"。

## 改动前必看(波及面)

- 要接生产后端不动这里,换组合根绑定的 exporter 即可 —— 全链路的观测规约见 [`../doc/OBSERVABILITY.md`](../doc/OBSERVABILITY.md)。
