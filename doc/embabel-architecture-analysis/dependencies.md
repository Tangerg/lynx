# 依赖与集成

> 0.4.0-SNAPSHOT · 父 POM `embabel-build-parent:0.1.13-SNAPSHOT`

## 1. Spring 生态

| 依赖 | 用途 |
|------|------|
| Spring Boot 3.x | 自动配置 / `@ConfigurationProperties` / 嵌入式服务器 |
| Spring AI | `ChatModel` · `VectorStore` · MCP Client · 重试包 |
| Spring MVC | HTTP 端点(A2A server / webmvc starter) |
| Spring Shell | 交互式 REPL(`starter-shell`) |
| Spring Security | MCP Server 保护(`embabel-agent-mcp-security`) |
| Spring Retry | `ActionQos` 的底层重试实现 |

## 2. LLM 提供方

**13 个** autoconfigure 模块,每个对应一个 starter:

| 提供方 | 模块前缀(autoconfigure + starter) | 特性 |
|--------|-----------------------------------|------|
| OpenAI | `embabel-agent-openai-*` | 2026 最新定价(#1568) |
| OpenAI Custom | `embabel-agent-openai-custom-*` | 兼容 Azure / 自建端点 |
| Anthropic | `embabel-agent-anthropic-*` | Extended thinking · Claude Haiku 4.5 / Sonnet 4.6 / Opus 4.7 |
| AWS Bedrock | `embabel-agent-bedrock-*` | Claude Bedrock · 更新 Haiku 4.5 Bedrock 变体 |
| Google Gemini | `embabel-agent-gemini-*` | 本地 API |
| Google GenAI | `embabel-agent-google-genai-*` | REST 接入 AI Studio |
| Mistral AI | `embabel-agent-mistral-ai-*` | — |
| DeepSeek | `embabel-agent-deepseek-*` | — |
| MiniMax | `embabel-agent-minimax-*` | — |
| Ollama | `embabel-agent-ollama-*` | 本地;支持 thinking |
| LM Studio | `embabel-agent-lmstudio-*` | 本地 GUI 模型管理器 |
| Docker Models | `embabel-agent-dockermodels-*` | 容器化本地模型 |
| ONNX | `embabel-agent-onnx-*` | **本地嵌入 / 重排**;DJL tokenizers,无 Python |

每个 provider 在 `resources/models/*-models.yml` 中声明模型目录(模型名 / 知识截止日期 / 定价)。

## 3. 协议

| 协议 | 模块 | 形态 |
|------|------|------|
| **MCP** Model Context Protocol | `embabel-agent-mcp/embabel-agent-mcpserver` | 服务端;把 `@Goal` 暴露为 MCP Resource / Tool(`McpToolExport`) |
| **MCP** 客户端 | Spring AI `mcp-client` | `embabel-agent-api` 直接依赖 |
| **A2A** Agent-to-Agent | `embabel-agent-a2a` | `a2a-java-sdk-spec` 0.3.2.Final;JSON-RPC 2.0 + SSE |

## 4. 领域工具库

| 库 | 版本 | 用途 |
|----|------|------|
| JavaParser | 3.26.2 | Java AST 解析(`embabel-agent-code`) |
| JGit | 7.0.1 | Git 操作 |
| ClassGraph | 4.8.181 | 类路径扫描 |
| Apache Tika | (parent 管理) | 1000+ 文档格式解析(`rag-tika`) |
| Apache Lucene | 9.11.1 | BM25 + 向量检索(`rag-lucene`) |
| ONNX Runtime | 1.22.0 | 本地模型推理(`embabel-agent-onnx`) |
| DJL HuggingFace Tokenizers | 0.32.0 | Rust 分词,无 Python 依赖 |

## 5. 通用库

| 库 | 用途 |
|----|------|
| Kotlin stdlib / reflect / coroutines | — |
| Jackson(含 Kotlin 模块 / YAML) | JSON / YAML 序列化 |
| Apache Commons Text | 字符串处理 |
| moby-names-generator | 趣味化进程名 |
| Netty | 已固定 4.1.132.Final(根 POM 明确 override,修 CVE) |
| SLF4J | 日志抽象 |

## 6. 可观测性

| 组件 | 作用 |
|------|------|
| Micrometer Tracing | 指标、Span API |
| OpenTelemetry API / Context / SDK | 分布式追踪 |
| OpenTelemetry semconv 1.29.0-alpha | 标准属性命名 |
| AspectJ(weaver / runtime) | `@Tracked` 切面 |

## 7. 测试

| 框架 | 用途 |
|------|------|
| JUnit 5 | 主测试框架 |
| Mockito + `@MockitoBean` | Java 侧 mock |
| MockK + SpringMockK | Kotlin 侧 mock |
| Spring Boot Test | Spring 上下文集成 |
| TestContainers | Docker 集成测试(#1601) |
| ArchUnit | 架构约束 |

## 8. 构建与质量

| 工具 | 用途 |
|------|------|
| Maven Surefire | 测试运行;支持重试 |
| Maven Failsafe | 集成测试(IT 后缀) |
| JaCoCo | 覆盖率;0.4.0 为 SonarCloud 兼容性做过修复(#1575) |
| SonarCloud | 质量门禁(`.sonarcloud.properties`) |
| SpotBugs + sb-contrib | 静态分析;`spotbugs-concurrency.xml` 专注并发问题 |

## 9. 版本策略

- 三层 BOM 堆叠:`embabel-common-dependencies` ← `embabel-common-test-dependencies` ← `embabel-agent-dependencies` ← 子模块
- 根 POM 显式 override:
  - `netty-bom` 4.1.132.Final(CVE 修复)
  - 其他版本一般交给 `embabel-build-parent` 统一管辖
- 父 POM `embabel-build-parent` 本身也在开发中(当前 `0.1.13-SNAPSHOT`),最近刚跟进升级(#1576)

## 10. 0.4.0 依赖层面的变化

| PR | 变化 |
|----|------|
| #1576 | 父 POM bump 到 `embabel-build:0.1.13-SNAPSHOT` |
| #1586 | README 加入 Maven Central 徽章 |
| #1601 | 引入 TestContainers 依赖(为 Docker 相关集成测试) |
| #1609 | `embabel-agent-byok` 子模块提取(之前内嵌在 common),以支持泛型化的 `ByokFactory<T>` |
| **skills 模块迁移** | `embabel-agent-skills` 从旧仓库迁入(保留历史);加入 BOM(`01b05330`) |
