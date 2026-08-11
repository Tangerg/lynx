# Lyra CLI

`app/cli` is Lyra's terminal agent. It provides an interactive oolong TUI and a deterministic one-shot command for scripts and pipelines.

The production path uses `app/runtime`'s public in-process `embedded` binding. There is no HTTP listener, JSON-RPC bridge, or copied protocol model: one process-owned runtime is opened lazily, shared by every command that needs it, and fully closed before the CLI exits. The scripted mock remains available only for deterministic tests and explicit UI demos.

## Run it

From this directory:

```sh
# From the Lynx worktree, app/runtime/config/config.yaml is discovered as the
# development fallback. The CLI defaults match it: DeepSeek / deepseek-v4-flash.
go run .
go run . run "explain why this test is flaky"
go run . run --json "summarize the change" > result.json
go run . run --output-format streaming-json "trace the change" > run.ndjson
go run . run -f internal/agent/run.go "review this file"
go run . sessions ls
go run . sessions ls --json
go run . approvals ls --session ses_demo_1 --json
go run . config show
```

Runtime durability and the user runtime `config.yaml` live under `$LYRA_HOME/runtime` (default `~/.lyra/runtime`). That user file has priority; when it is absent and Lyra is launched from this worktree, `app/runtime/config/config.yaml` is used as the development fallback. `LYRA_RUNTIME_CONFIG_DIR` selects another absolute config directory explicitly. The embedded binding exclusively leases the data directory until the CLI exits, so two CLI/desktop processes cannot open the same store concurrently; use a different absolute `LYRA_HOME` for an isolated instance. `LYRA_RUNTIME=mock` selects the scripted adapter explicitly for development and test automation.

`run` supports `--output-format text`, `json`, and `streaming-json`; `--json` is the single-result JSON shorthand, while `streaming-json` is an incremental NDJSON event stream. A result object reports `completed`, `failed`, `canceled`, `interrupted`, or `incomplete` and includes authoritative assistant text plus run metadata. Stdout stays machine-readable while diagnostics remain on stderr. Completed outcomes exit zero, failed or canceled outcomes exit non-zero, and a question leaves the run parked and prints the exact `--session` command needed to continue interactively. Process interruption uses the conventional exit status 130 for SIGINT and 143 for SIGTERM.

The interactive client uses a stable agent shell: session and workspace identity at the top, selectable transcript content in the center, a compact live plan and run status near the bottom, and a framed multiline composer whose footer shows the provider-qualified model and run limits. Optional regions yield their space on constrained terminals; at extreme sizes the shell reduces to transcript, status, and a one-row composer, then restores the full chrome without losing the draft or keyboard focus. It supports session switching, search, protocol-representable text/image attachments, approval questions, provider/model and global approval-mode pickers, tool details, plugin inspection, and plugin reload/unload. `lyra completion <bash|zsh|fish|powershell>` generates shell completion.

Core terminal interactions are available from both the keyboard and mouse:

- `Enter` sends a prompt while idle and queues a typed follow-up while a run or its cancellation is still settling. Queued prompts are session-scoped and drain in FIFO order only after the prior transport lifecycle finishes; with an empty composer, `Enter` promotes the next queued prompt and cancels the active turn first. `Shift+Enter` or `Alt+Enter` inserts a newline.
- `Ctrl+;` or `/queue` opens the bottom queue drawer; `Ctrl+G` is the portable fallback for terminals that cannot distinguish modified punctuation. Use `Up`/`Down` or `j`/`k` to select, `Enter` or `e` to edit, `x` to remove, `Shift+J`/`Shift+K` to reorder, and `s` or `Ctrl+Enter` to send the selected prompt next. Queue edits preserve attachments; mouse actions commit only when an undragged press and release land on the same control.
- `Ctrl+C` is stateful: it clears a non-empty draft first and cancels the active run only when the composer is empty. `Esc` cancels an active run immediately without discarding the draft; while idle, two presses within 800ms clear a prompt and save it to history. Press `Ctrl+Q` or `Ctrl+D` twice to quit.
- `Tab` moves keyboard focus from the composer into the transcript. `Up`/`Down` select one retained entry, `Home`/`End` jump to the edges, `Left`/`Right` collapse or expand the selected tool, `Enter` toggles it, and `Alt+C` copies the selected block. `Tab` or `Space` returns to the composer; typing printable text returns there automatically.
- `PageUp` and `PageDown` move through the live transcript; `Ctrl+Home` and `Ctrl+End` jump to its bounds. Scrolling up suspends bottom-following while output continues.
- Click a tool-call header to expand or collapse that tool. Running output streams into the same block, completion replaces provisional output with the runtime's authoritative result, and scrolling up never resumes bottom-following. Tools without output or a diff use a non-interactive marker instead of opening an empty panel. The action commits only when press and release land on the same header, so a drag selection cannot accidentally change layout. Its colored rail and right-aligned state remain visible while details are folded; `Ctrl+O` expands or collapses all available tool details.
- `Ctrl+P` opens the searchable command palette. With transcript focus, `?` is the local alternative. `/help` opens the same surface, so command discovery does not flood the transcript.
- `Ctrl+X` or `/shortcuts` opens a scrollable shortcut guide generated from the active keymaps, so customized bindings and their current contexts stay discoverable.
- Searchable session, command, provider-qualified model, and runtime approval-mode pickers support both keyboard selection and click-to-activate. A click commits on release over the same row; dragging only moves the selection.

The shortcut row is contextual: idle runs emphasize send, sessions, and model selection; active runs emphasize cancel, multiline input, and tool details; transcript focus exposes entry navigation and actions. Mouse, selection, transcript scrolling, and command shortcuts remain available while output streams.

## Architecture

The dependency direction follows clean architecture: product policy points inward; frameworks and transports stay at the edge.

| Layer | Packages | Responsibility |
| --- | --- | --- |
| Domain and ports | `internal/agent`, `internal/settings` | Validated session, run, event, interaction, approval, and configuration contracts. `agent.Runtime` is the complete backend port; consumers use its narrower interfaces. |
| Application use cases | `internal/oneshot`, `internal/session`, `internal/promptqueue`, `internal/reconnect`, `internal/runrecovery` | Unattended run lifecycle, session opening, session-scoped follow-up ownership, bounded reconnects, and authoritative cold recovery. |
| Delivery adapters | `internal/cmd`, `internal/render`, `internal/terminal` | Cobra/Viper routing, text/JSON projections, and the oolong terminal UI. |
| Infrastructure adapters | `internal/runtimeembedded`, `internal/agent/mock`, `internal/attachment`, `internal/sideload` | Production embedded/protocol anti-corruption layer, scripted test runtime, workspace-safe files, and out-of-process plugins. |
| Extension substrate | `internal/extensions` | Typed extension points, capability checks, dependency ordering, rollback, unload, and reload ownership. |
| Composition root | `main.go` | Selects the runtime implementation, binds process streams and signals, and constructs the command tree. |

Architecture tests prevent the domain from importing Cobra, Viper, oolong, renderers, or adapters. Constructors create isolated command and UI graphs, so tests do not share mutable package state.

### Runtime boundary

[`internal/runtimeembedded`](internal/runtimeembedded) implements the consumer-owned [`agent.Runtime`](internal/agent/ports.go) port against `app/runtime/embedded` and `app/runtime/protocol`. The boundary preserves these contracts:

- assemble cold snapshots from session metadata, every durable Item (including its `runId` and running/completed/incomplete lifecycle), root Runs, the revisioned Plan state, and the complete pending Interrupt set;
- give every mutation a cryptographically strong idempotency key and return the Segment stream opened by `runs.start` or `runs.resume` atomically;
- subscribe to the exact `runId` + `segmentId`, carrying the opaque event checkpoint through transport metadata without parsing or ordering it;
- map waiting, finished, stale-segment, invalid-cursor, and replay-unavailable errors by identity so the client can switch to cold recovery;
- consume an entire pending interrupt set in one resume and preserve provider/model pairing plus run limits;
- negotiate the CLI's exact Run Protocol profile (`approval` and `question` interrupts, without `subagents` or `clientTools`) and reject an incompatible existing Run at the adapter boundary;
- map authoritative item/state events into the domain projection while treating item deltas as disposable previews whose completed Item remains the terminal source of truth;
- honor context cancellation for every operation and stream.

`main.go` is the composition root. It installs a lazy process-level owner; command help and static completion do not open databases or model clients. Protocol DTOs, request metadata, replay options, inline media, tool-result JSON, capability negotiation, and structured runtime errors are all confined to the adapter. The command tree, TUI, renderers, attachment handling, and extension system depend only on the CLI projection.

## Configuration

Precedence is:

1. command flags;
2. `LYRA_` environment variables;
3. one selected YAML file: `--config`, otherwise `./.lyra.yaml` when present, otherwise the user configuration file;
4. product defaults.

The user file is `lyra/config.yaml` under the operating system's user configuration directory. Nested environment keys use underscores, for example `LYRA_UI_TRANSCRIPT_RETAIN=80` and `LYRA_APPROVAL_REMEMBER=project`.

This CLI-settings file is separate from the embedded runtime's `$LYRA_HOME/runtime/config.yaml`. The built-in CLI selection is `deepseek` plus `deepseek-v4-flash`, matching the repository runtime config. The paired `LYRA_PROVIDER` and `LYRA_MODEL` variables override it; DeepSeek credentials may still be supplied through `DEEPSEEK_API_KEY`. `LYRA_HOME` and `LYRA_RUNTIME_CONFIG_DIR`, when set, must be absolute.

```yaml
provider: deepseek
model: deepseek-v4-flash

run:
  max-total-tokens: 0
  max-steps: 0
  max-budget-usd: 0

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

# Only overridden actions need to be listed. Spaces form key sequences.
keys:
  sessions: ["g s"]
  shortcuts: ["ctrl+x"]
```

Run `lyra config show` to inspect the merged, validated value and `lyra config path` to identify the selected file.
Configuration decoding is strict: unknown top-level or nested keys, missing effective actions, invalid chords, and duplicate bindings are rejected instead of silently falling back to an incomplete keymap. File overrides merge with the complete default action set. Completion-script generation is configuration-independent, so a broken local file cannot prevent shell setup or repair.

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

The TUI is pinned to the published oolong `v0.10.0` modules. Keep the module set on one release to avoid mixing component contracts.

```sh
go mod tidy
go build ./...
go vet ./...
go test ./...
go test -race ./...
golangci-lint run ./...
```

The test suite includes domain invariant tests, real in-process runtime lifecycle/catalog integration, protocol projection tests, in-memory Cobra tests, replay and fault-injection tests, architecture dependency checks, plugin lifecycle tests, JSON/NDJSON contract tests, concurrent stream-and-resize storms, binary-level SIGINT/SIGTERM exit checks, and an oolong PTY matrix covering xterm, screen/tmux-compatible, mouse-disabled, ASCII-locale, and VS Code on WSL terminal behavior.
