// Built-in plugin: composer accessories — modes / placeholders /
// keymap / status chips / model picker / attach / send / kbd hint.
//
// Every piece is its own micro-plugin so a fork can drop or replace any
// single accessory without touching the rest. Migration P6.3 moved
// these from raw className strings to Tailwind utilities + Radix
// primitives — see CLAUDE.md "Tailwind first" / "Radix first" rules.

import * as React from "react";
import * as Popover from "@radix-ui/react-popover";
import * as Tooltip from "@radix-ui/react-tooltip";
import { Icon, type IconName } from "@/components/common";
import { submitComposer } from "@/components/chat/submitComposer";
import { useSessions } from "@/lib/queries";
import { cn } from "@/lib/utils";
import { definePlugin, useCommands } from "@/plugins/sdk";
import { useAgentAction } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useSessionStore } from "@/state/sessionStore";

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
    host.composer.registerPlaceholder({ id: "ask",       text: "Ask, plan, or paste a stack trace…  /  to run a command" });
    host.composer.registerPlaceholder({ id: "debug",     text: "Paste a failing test output and I'll walk you through it." });
    host.composer.registerPlaceholder({ id: "implement", text: "Implement what? Describe the change and I'll plan + execute." });
    host.composer.registerPlaceholder({ id: "refactor",  text: "Point at code that smells; I'll suggest a refactor." });
  },
});

// ---- keymap --------------------------------------------------------------

export const composerKeymap = definePlugin({
  name: "lyra.builtin.composer-keymap",
  version: "1.0.0",
  setup({ host }) {
    host.composer.registerKeyBinding({
      key: "Enter",
      description: "Send message",
      handler: ({ submit, event }) => {
        if (event.shiftKey) return false; // Shift+Enter inserts a newline.
        submit();
        return true;
      },
    });
    host.composer.registerKeyBinding({
      key: "Mod+Enter",
      description: "Send message (override)",
      handler: ({ submit }) => { submit(); return true; },
    });
  },
});

// ---- status chips --------------------------------------------------------
//
// Values are still hard-coded strings (taken over verbatim from the
// original AgentClientPage props). Wiring them to live state (active
// project, current branch via git, …) is a follow-up that doesn't touch
// the registration API.

function Chip({
  icon, title, children,
}: { icon: IconName; title: string; children: React.ReactNode }) {
  return (
    <button
      type="button"
      title={title}
      className="group inline-flex h-6 items-center gap-1.5 rounded-sm border border-transparent bg-transparent px-2 pl-2.5 font-mono text-[11.5px] font-normal text-fg-muted tracking-tight whitespace-nowrap cursor-pointer transition-colors duration-150 hover:bg-surface-2 hover:text-fg"
    >
      <Icon name={icon} size={11} className="text-fg-faint shrink-0 group-hover:text-fg" />
      <span>{children}</span>
      <Icon name="more" size={10} className="text-fg-faint opacity-60 group-hover:text-fg-muted" />
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
  const activeId = useSessionStore((s) => s.activeSessionId);
  const active = sessions.find((s) => s.id === activeId) ?? sessions[0];
  const model = active?.model ?? "Sonnet";

  return (
    <button
      type="button"
      title="Switch model"
      className="mr-1 inline-flex h-[26px] shrink-0 items-center gap-1.5 rounded-full border border-transparent bg-transparent pl-1 pr-2.5 font-sans text-[12px] font-semibold text-fg whitespace-nowrap cursor-pointer transition-colors hover:bg-surface-2 hover:border-line"
    >
      <span className="grid h-5 w-5 shrink-0 place-items-center rounded-full bg-[linear-gradient(135deg,var(--color-accent)_0%,color-mix(in_oklab,var(--color-accent)_40%,#000)_100%)] text-on-accent font-semibold text-[11px]">
        {model.slice(0, 1)}
      </span>
      <span className="font-mono text-[11.5px] font-semibold tracking-[0.01em]">{model}</span>
      <Icon name="more" size={10} className="text-fg-faint opacity-70" />
    </button>
  );
}

function AttachButton() {
  return (
    <button
      type="button"
      title="Attach file"
      className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md border-0 bg-transparent text-fg-muted cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg"
    >
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
//
// Composer-local key handlers — registered by composerKeymap on the
// textarea, not on the global shortcut store. They don't appear in
// useCommands(), so we list them statically.

const STATIC_CHEATS: Array<{ combo: string; label: string }> = [
  { combo: "↵",     label: "Send message" },
  { combo: "⌘↵",    label: "Send message" },
  { combo: "⇧↵",    label: "New line" },
  { combo: "Esc",   label: "Unfocus composer" },
  { combo: "⌘1-9",  label: "Switch to tab N" },
];

function KeyHint() {
  // Dynamic rows: any palette command that advertises a `shortcut` string.
  const commands = useCommands();
  const dynamic = commands
    .filter((c) => c.shortcut)
    .map((c) => ({ combo: c.shortcut as string, label: c.label }));

  return (
    <Popover.Root>
      <Popover.Trigger
        className="hidden xl:inline-flex items-center gap-1 px-1 font-mono text-[11px] text-fg-faint cursor-default border-0 bg-transparent"
      >
        <span className="text-accent">⌘K</span> commands · <span className="text-accent">⌘↵</span> send
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          side="top"
          sideOffset={8}
          align="end"
          role="tooltip"
          className="z-50 min-w-[220px] rounded-md border border-line-soft bg-surface-2 p-2.5 shadow-lg animate-rise-in"
        >
          <div className="mb-1.5 font-mono text-[10px] font-semibold text-fg-faint">Shortcuts</div>
          {[...STATIC_CHEATS, ...dynamic].map((r, i) => (
            <div key={`${r.combo}:${i}`} className="grid grid-cols-[64px_1fr] items-center gap-2.5 py-0.5 text-[11.5px]">
              <kbd className="rounded-sm border border-line-soft bg-line px-1.5 text-center font-mono text-[11px] text-fg-soft">
                {r.combo}
              </kbd>
              <span className="text-fg-muted">{r.label}</span>
            </div>
          ))}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
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
    <Tooltip.Provider delayDuration={300}>
      <Tooltip.Root>
        <Tooltip.Trigger asChild>
          <button
            type="button"
            disabled={disabled}
            onClick={onClick}
            className={cn(
              "grid h-8 w-8 shrink-0 place-items-center rounded-full border-0 cursor-pointer transition-transform duration-150",
              disabled
                ? "bg-surface-3 text-fg-faint cursor-not-allowed"
                : "bg-accent text-on-accent hover:scale-105 active:scale-95",
            )}
          >
            <Icon name="send-arrow" size={14} strokeWidth={2.5} />
          </button>
        </Tooltip.Trigger>
        <Tooltip.Portal>
          <Tooltip.Content
            side="top"
            sideOffset={6}
            className="rounded-sm bg-surface-3 px-2 py-1 font-mono text-[11px] text-fg-soft shadow-md"
          >
            Send (⌘↵)
          </Tooltip.Content>
        </Tooltip.Portal>
      </Tooltip.Root>
    </Tooltip.Provider>
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
