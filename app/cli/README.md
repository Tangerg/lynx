# Lyra CLI

`app/cli` is Lyra's terminal client. It provides an interactive oolong TUI and a deterministic one-shot command for scripts and pipelines.

The application currently runs against a scripted mock runtime. It prints that fact to stderr whenever a command opens the runtime; stdout therefore remains safe for pipes. No production agent, account, or remote service is contacted yet.

## Run it

From this directory:

```sh
go run .
go run . run "explain why this test is flaky"
go run . run --json "summarize the change" > run.ndjson
go run . run -f internal/client/run.go "review this file"
go run . sessions ls
go run . config show
```

The interactive client uses a stable agent shell: session and workspace identity at the top, selectable transcript content in the center, a compact live plan and run status near the bottom, and a framed multiline composer whose footer always shows the active model, effort, mode, and permission. Optional regions yield their space on constrained terminals instead of squeezing the conversation. It supports session switching, search, attachments, approval questions, runtime-option pickers, tool details, plugin inspection, and plugin reload/unload. `lyra completion <bash|zsh|fish|powershell>` generates shell completion.

Core terminal interactions are available from both the keyboard and mouse:

- `Enter` sends a prompt; `Shift+Enter` or `Alt+Enter` inserts a newline.
- `Tab` moves keyboard focus from the composer into the transcript. `Up`/`Down` select one retained entry, `Home`/`End` jump to the edges, `Left`/`Right` collapse or expand the selected tool, `Enter` toggles it, and `Alt+C` copies the selected block. `Tab` or `Space` returns to the composer; typing printable text returns there automatically.
- `PageUp` and `PageDown` move through the live transcript; `Ctrl+Home` and `Ctrl+End` jump to its bounds. Scrolling up suspends bottom-following while output continues.
- Click a tool-call header to expand or collapse that tool. The action commits only when press and release land on the same header, so a drag selection cannot accidentally change layout. Its colored rail and right-aligned state remain visible while details are folded; `Ctrl+O` expands or collapses all tool details.
- `Ctrl+P` opens the searchable command palette. With transcript focus, `?` is the local alternative. `/help` opens the same surface, so command discovery does not flood the transcript.

The shortcut row is contextual: idle runs emphasize send, sessions, and mode; active runs emphasize cancel, multiline input, and tool details; transcript focus exposes entry navigation and actions. Mouse, selection, transcript scrolling, and command shortcuts remain available while output streams.

## Architecture

The dependency direction follows clean architecture: product policy points inward; frameworks and transports stay at the edge.

| Layer | Packages | Responsibility |
| --- | --- | --- |
| Domain and ports | `internal/client`, `internal/settings` | Validated session, run, event, interaction, approval, and configuration contracts. `client.Runtime` is the complete backend port; consumers use its narrower interfaces. |
| Application orchestration | `internal/cmd`, `internal/reconnect` | Use-case sequencing, idempotent run control, reconnect policy, configuration precedence, and process exit behavior. |
| Adapters | `internal/client/mock`, `internal/attachment`, `internal/render`, `internal/terminal`, `internal/sideload` | Scripted backend, workspace-safe files, text/NDJSON output, oolong terminal UI, and out-of-process plugins. |
| Extension substrate | `internal/extensions` | Typed extension points, capability checks, dependency ordering, rollback, unload, and reload ownership. |
| Composition root | `internal/cmd/root.go` | Constructs Cobra, Viper, the runtime adapter, plugin sources, and terminal application. |

Architecture tests prevent the domain from importing Cobra, Viper, oolong, renderers, or adapters. Constructors create isolated command and UI graphs, so tests do not share mutable package state.

### Replacing the mock runtime

The mock is isolated behind [`client.Runtime`](internal/client/client.go). A real adapter must implement that port and preserve its documented contracts:

- validate returned snapshots and envelopes;
- make a non-empty `StartRun.RequestID` idempotent;
- make `ResumeRun` idempotent for the same interrupt and answer;
- replay `FollowRun` strictly after the requested cursor without changing the durable log;
- honor context cancellation for every operation and stream.

Inject the adapter through `cmd.NewRoot(runtime)`. The command tree, TUI, renderers, attachment handling, and extension system require no changes. The nil path in `cmd.NewRoot` is the single composition-root choice that installs `mock.New()` today. Help and completion do not open a stateful backend.

## Configuration

Precedence is:

1. command flags;
2. `LYRA_` environment variables;
3. one selected YAML file: `--config`, otherwise `./.lyra.yaml` when present, otherwise the user configuration file;
4. product defaults.

The user file is `lyra/config.yaml` under the operating system's user configuration directory. Nested environment keys use underscores, for example `LYRA_UI_TRANSCRIPT_RETAIN=80` and `LYRA_APPROVAL_REMEMBER=project`.

```yaml
model: mock-balanced
effort: medium
mode: build
permission: ask

approval:
  remember: none

ui:
  mouse: true
  notifications: true
  tool-details: false
  transcript-retain: 24
  reconnect-attempts: 4

plugins:
  directories:
    - /absolute/path/to/plugins
```

Run `lyra config show` to inspect the merged, validated value and `lyra config path` to identify the selected file.

## Sideloaded plugins

Each configured plugin directory, or one of its immediate child directories, may contain `lyra-plugin.json` and an executable entry. Schema and host API version are both `1`.

```json
{
  "schemaVersion": 1,
  "id": "example.tools",
  "version": "1.0.0",
  "apiVersion": 1,
  "requires": [],
  "capabilities": ["terminal.commands"],
  "entry": "bin/example-plugin",
  "contributes": {
    "commands": [
      {
        "name": "hello",
        "title": "greet the current session",
        "aliases": ["hi"],
        "takes": true,
        "timeoutSeconds": 10
      }
    ]
  }
}
```

The entry must be a regular executable inside its canonical plugin directory. Absolute paths, unsafe segments, backslash-separated paths, and symlink escapes are rejected. Manifests reject unknown fields and duplicate command spellings. A plugin can declare at most 128 commands, 16 aliases per command, and a timeout from 1 through 60 seconds.

Commands use one JSON request on stdin and one JSON response on stdout:

```json
{"protocol":1,"pluginId":"example.tools","command":"hello","argument":"Ada","workspace":"/work/project","sessionId":"session_123"}
```

```json
{"protocol":1,"message":"Hello, Ada"}
```

The host caps each output stream at 1 MiB, request JSON at 128 KiB, arguments at 64 KiB, and response messages at 4 KiB. It passes only a small platform-safe environment allowlist plus `LYRA_PLUGIN_PROTOCOL`, `LYRA_PLUGIN_ID`, and `LYRA_PLUGIN_COMMAND`; unrelated secrets are not inherited. Cancellation terminates the owned process tree on supported platforms.

## Development

The TUI is pinned to the published oolong `v0.8.0` modules. Keep the module set on one release to avoid mixing component contracts.

```sh
go mod tidy
go build ./...
go vet ./...
go test ./...
go test -race ./...
golangci-lint run ./...
```

The test suite includes domain invariant tests, in-memory Cobra tests, replay and fault-injection tests, architecture dependency checks, plugin lifecycle tests, NDJSON contract tests, and oolong PTY interaction/resize tests.
