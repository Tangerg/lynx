// Built-in plugins for the composer surface — modes / placeholders /
// keymap / status chips / start-toolbar items / end-toolbar items / send.
//
// Each is still an independent plugin (so a user can replace any single
// piece — e.g. ship a real model picker without touching the mode
// toggles), but the implementations live together because they're small
// and conceptually adjacent.

import { Icon, type IconName } from "@/components/common";
import { submitComposer } from "@/components/chat/submitComposer";
import { useSessions } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";
import { useAgentAction } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useUIStore } from "@/state/uiStore";

// ---- modes ---------------------------------------------------------------

export const composerModes = definePlugin({
  name: "lyra.builtin.composer-modes",
  version: "1.0.0",
  setup({ host }) {
    host.composer.registerMode({ id: "agent", label: "Agent", icon: "spark", order: 0 });
    host.composer.registerMode({ id: "ask",   label: "Ask",   icon: "chat",  order: 1 });
    host.composer.registerMode({ id: "plan",  label: "Plan",  icon: "list",  order: 2 });
  },
});

// ---- placeholders --------------------------------------------------------

export const composerPlaceholders = definePlugin({
  name: "lyra.builtin.composer-placeholders",
  version: "1.0.0",
  setup({ host }) {
    host.composer.registerPlaceholder({
      id: "default",
      text: "Ask, plan, or paste a stack trace…  /  to run a command",
      weight: 4,
    });
    host.composer.registerPlaceholder({
      id: "plan",
      text: "What should I do next? Try /plan to draft a checklist.",
    });
    host.composer.registerPlaceholder({
      id: "search",
      text: "Look up something in the codebase — start with /search …",
    });
    host.composer.registerPlaceholder({
      id: "explain",
      text: "Paste an error or a snippet and I'll explain it.",
    });
  },
});

// ---- key bindings --------------------------------------------------------

export const composerKeymap = definePlugin({
  name: "lyra.builtin.composer-keymap",
  version: "1.0.0",
  setup({ host }) {
    // Plain Enter sends; Shift+Enter is intentionally NOT registered so the
    // browser's default newline behavior kicks in.
    host.composer.registerKeyBinding({
      key: "Enter",
      description: "Send the current message",
      handler: ({ submit }) => {
        submit();
        return true;
      },
    });
    // Mod+Enter mirrors Enter — the hint chip on the toolbar advertises
    // "⌘↵ send", so users who learned that combo elsewhere don't get a
    // dead key.
    host.composer.registerKeyBinding({
      key: "Mod+Enter",
      description: "Send the current message",
      handler: ({ submit }) => {
        submit();
        return true;
      },
    });
    // Escape blurs the textarea so the user can keyboard-navigate out of
    // the composer (e.g. to use Cmd+1..9 tab shortcuts which skip inputs
    // by default).
    host.composer.registerKeyBinding({
      key: "Escape",
      description: "Unfocus the composer",
      handler: ({ event }) => {
        (event.target as HTMLElement | null)?.blur();
        return true;
      },
    });
  },
});

// ---- status chips --------------------------------------------------------

// Values are still hard-coded strings (taken over verbatim from the
// original AgentClientPage props). Wiring them to live state (active
// project, current branch via git, …) is a follow-up that doesn't touch
// the registration API.
function Chip({
  icon, title, children,
}: { icon: IconName; title: string; children: React.ReactNode }) {
  return (
    <button className="cf-chip" title={title}>
      <Icon name={icon} size={11} />
      <span>{children}</span>
      <Icon name="more" size={10} />
    </button>
  );
}
const ProjectChip   = () => <Chip icon="folder" title="Working directory">fern-api</Chip>;
const ExecModeChip  = () => <Chip icon="shield" title="Execution mode">Workspace · Auto</Chip>;
const GitBranchChip = () => <Chip icon="branch" title="Git branch">feat/result-type</Chip>;

export const composerChips = definePlugin({
  name: "lyra.builtin.composer-chips",
  version: "1.0.0",
  setup({ host }) {
    host.composer.registerStatus({ id: "project",   order: 0, component: ProjectChip });
    host.composer.registerStatus({ id: "exec-mode", order: 1, component: ExecModeChip });
    host.composer.registerStatus({ id: "git-branch", order: 2, component: GitBranchChip });
  },
});

// ---- toolbar (start) -----------------------------------------------------

function ModelPicker() {
  const { data: sessions = [] } = useSessions();
  const activeId = useUIStore((s) => s.activeSessionId);
  const active = sessions.find((s) => s.id === activeId) ?? sessions[0];
  const model = active?.model ?? "Sonnet";

  return (
    <button className="composer-model" title="Switch model">
      <span className="cm-avatar">{model.slice(0, 1)}</span>
      <span className="cm-name">{model}</span>
      <Icon name="more" size={10} />
    </button>
  );
}

function AttachButton() {
  return (
    <button className="composer-tool-btn" title="Attach file">
      <Icon name={"paperclip" as IconName} size={13} />
    </button>
  );
}

export const composerToolbar = definePlugin({
  name: "lyra.builtin.composer-toolbar",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("composer.toolbar.start", { id: "model",  order: 0, component: ModelPicker });
    host.layout.register("composer.toolbar.start", { id: "attach", order: 1, component: AttachButton });
  },
});

// ---- toolbar (end): keyboard hint + send ---------------------------------

// Minimum cheat-sheet content. Kept terse — it's a hover discovery aid,
// not a manual. If we add more shortcuts later, organize by surface
// (composer / global / chat) so users can scan.
const CHEATSHEET: Array<{ combo: string; what: string }> = [
  { combo: "⌘↵",    what: "Send message" },
  { combo: "⇧↵",    what: "New line" },
  { combo: "Esc",   what: "Unfocus composer" },
  { combo: "⌘K",    what: "Command palette" },
  { combo: "⌘N",    what: "New tab" },
  { combo: "⌘W",    what: "Close current tab" },
  { combo: "⌘L",    what: "Focus composer" },
  { combo: "⌘B",    what: "Toggle sidebar" },
  { combo: "⌘1-9",  what: "Switch to tab N" },
  { combo: "⌘⇧L",   what: "Toggle theme" },
];

function KeyHint() {
  return (
    <div className="meta key-hint">
      <span className="accent">⌘K</span> commands · <span className="accent">⌘↵</span> send
      <div className="key-cheatsheet" role="tooltip">
        <div className="key-cheatsheet-title">Shortcuts</div>
        {CHEATSHEET.map((r) => (
          <div className="key-cheat-row" key={r.combo}>
            <kbd>{r.combo}</kbd>
            <span>{r.what}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export const composerHint = definePlugin({
  name: "lyra.builtin.composer-hint",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("composer.toolbar.end", { id: "kbd-hint", order: 0, component: KeyHint });
  },
});

function SendButton() {
  const value = useComposerStore((s) => s.value);
  const clear = useComposerStore((s) => s.clear);
  const send = useAgentAction("send");

  const disabled = !value.trim() || !send;
  const onClick = () => {
    if (!send) return;
    submitComposer({ value, clear, sendText: send });
  };

  return (
    <button
      className="send-btn"
      disabled={disabled}
      onClick={onClick}
      title="Send (⌘↵)"
    >
      <Icon name="send-arrow" size={14} strokeWidth={2.5} />
    </button>
  );
}

export const composerSend = definePlugin({
  name: "lyra.builtin.composer-send",
  version: "1.0.0",
  setup({ host }) {
    // Order chosen so the send button sits after the kbd hint.
    host.layout.register("composer.toolbar.end", { id: "send", order: 100, component: SendButton });
  },
});
