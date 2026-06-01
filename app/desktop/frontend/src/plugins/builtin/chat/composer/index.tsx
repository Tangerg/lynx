// Built-in plugin: composer accessories — modes / placeholders /
// keymap / status chips / model picker / attach / send / kbd hint.
//
// Every piece is its own micro-plugin so a fork can drop or replace any
// single accessory without touching the rest. Migration P6.3 moved
// these from raw className strings to Tailwind utilities + Radix
// primitives — see CLAUDE.md "Tailwind first" / "Radix first" rules.

import type { IconName } from "@/components/common";
import type { ReactNode } from "react";
import { submitComposer } from "@/components/chat/submitComposer";
import { Icon, Tooltip } from "@/components/common";
import { useChatSend } from "@/lib/agent/useChatSend";
import { useSessions } from "@/lib/data/queries";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import {
  COMPOSER_KEY_BINDING,
  COMPOSER_MODE,
  COMPOSER_PLACEHOLDER,
  COMPOSER_STATUS,
} from "@/plugins/sdk/kernelPoints";
import { useAgentAction, useAgentSlice } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useSessionStore } from "@/state/sessionStore";

// ---- modes ---------------------------------------------------------------

export const composerModes = definePlugin({
  name: "lyra.builtin.composer-modes",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(COMPOSER_MODE, {
      id: "agent",
      label: "Agent",
      icon: "spark",
      order: 0,
      description: "Runs tools, edits files, executes commands. Asks before risky actions.",
    });
    host.extensions.contribute(COMPOSER_MODE, {
      id: "ask",
      label: "Ask",
      icon: "chat",
      order: 1,
      description: "Read-only conversation. No tool calls, no file changes.",
    });
    host.extensions.contribute(COMPOSER_MODE, {
      id: "plan",
      label: "Plan",
      icon: "list",
      order: 2,
      description: "Produces a plan first. Nothing runs until you switch to Agent.",
    });
  },
});

// ---- placeholders --------------------------------------------------------

export const composerPlaceholders = definePlugin({
  name: "lyra.builtin.composer-placeholders",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(COMPOSER_PLACEHOLDER, {
      id: "ask",
      text: "Ask, plan, or paste a stack trace…  /  to run a command",
    });
    host.extensions.contribute(COMPOSER_PLACEHOLDER, {
      id: "debug",
      text: "Paste a failing test output and I'll walk you through it.",
    });
    host.extensions.contribute(COMPOSER_PLACEHOLDER, {
      id: "implement",
      text: "Implement what? Describe the change and I'll plan + execute.",
    });
    host.extensions.contribute(COMPOSER_PLACEHOLDER, {
      id: "refactor",
      text: "Point at code that smells; I'll suggest a refactor.",
    });
  },
});

// ---- keymap --------------------------------------------------------------

export const composerKeymap = definePlugin({
  name: "lyra.builtin.composer-keymap",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Enter",
      description: "Send message",
      handler: ({ submit, event }) => {
        if (event.shiftKey) return false; // Shift+Enter inserts a newline.
        submit();
        return true;
      },
    });
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Mod+Enter",
      description: "Send message (override)",
      handler: ({ submit }) => {
        submit();
        return true;
      },
    });
  },
});

// ---- status chips --------------------------------------------------------
//
// Values are still hard-coded strings (taken over verbatim from the
// original AgentClientPage props). Wiring them to live state (active
// project, current branch via git, …) is a follow-up that doesn't touch
// the registration API.

// Readable chip — icon + label, no affordance glyph. Used where the
// value itself carries meaning the user wants to read (e.g. branch name).
function Chip({ icon, title, children }: { icon: IconName; title: string; children: ReactNode }) {
  return (
    <span
      title={title}
      className="group inline-flex h-6 items-center gap-1.5 rounded-sm px-2 font-mono text-[11.5px] font-normal text-fg-muted tracking-tight whitespace-nowrap"
    >
      <Icon name={icon} size={11} className="text-fg-faint shrink-0" />
      <span>{children}</span>
    </span>
  );
}

// Icon-only affordance — the value lives in the tooltip, the glyph keeps
// the footer light. Click reserved for future state toggles.
function IconChip({ icon, hint, onClick }: { icon: IconName; hint: string; onClick?: () => void }) {
  return (
    <Tooltip label={hint}>
      <button
        type="button"
        aria-label={hint}
        onClick={onClick}
        className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm border-0 bg-transparent text-fg-faint cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg"
      >
        <Icon name={icon} size={12} />
      </button>
    </Tooltip>
  );
}

const ExecModeChip = () => <IconChip icon="shield" hint="Execution mode · Workspace · Auto" />;
const GitBranchChip = () => (
  <Chip icon="branch" title="Git branch">
    feat/result-type
  </Chip>
);

export const composerChips = definePlugin({
  name: "lyra.builtin.composer-chips",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(COMPOSER_STATUS, {
      id: "exec-mode",
      order: 1,
      component: ExecModeChip,
    });
    host.extensions.contribute(COMPOSER_STATUS, {
      id: "git-branch",
      order: 2,
      component: GitBranchChip,
    });
  },
});

// ---- toolbar (start) -----------------------------------------------------

function ModelPicker() {
  const { data: sessions = [] } = useSessions();
  const activeId = useSessionStore((s) => s.activeSessionId);
  const active = sessions.find((s) => s.id === activeId) ?? sessions[0];
  const model = active?.model ?? "Sonnet";

  return (
    <Tooltip label="Switch model">
      <button
        type="button"
        aria-label="Switch model"
        className="mr-1 inline-flex h-6.5 shrink-0 items-center gap-1.5 rounded-full border border-transparent bg-transparent pl-1 pr-2.5 font-sans text-[12px] font-semibold text-fg whitespace-nowrap cursor-pointer transition-colors hover:bg-surface-2 hover:border-line"
      >
        <span className="grid h-5 w-5 shrink-0 place-items-center rounded-full bg-[linear-gradient(135deg,var(--color-accent)_0%,color-mix(in_oklab,var(--color-accent)_40%,#000)_100%)] text-on-accent font-semibold text-[11px]">
          {model.slice(0, 1)}
        </span>
        <span className="font-mono text-[11.5px] font-semibold tracking-[0.01em]">{model}</span>
        <Icon name="more" size={10} className="text-fg-faint opacity-70" />
      </button>
    </Tooltip>
  );
}

function AttachButton() {
  return (
    <Tooltip label="Attach file">
      <button
        type="button"
        aria-label="Attach file"
        className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md border-0 bg-transparent text-fg-muted cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg"
      >
        <Icon name={"paperclip" as IconName} size={13} />
      </button>
    </Tooltip>
  );
}

export const composerToolbar = definePlugin({
  name: "lyra.builtin.composer-toolbar",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("composer.toolbar.start", {
      id: "model",
      order: 0,
      component: ModelPicker,
    });
    host.layout.register("composer.toolbar.start", {
      id: "attach",
      order: 1,
      component: AttachButton,
    });
  },
});

// ---- toolbar (end): send -------------------------------------------------

function SendButton() {
  const value = useComposerStore((s) => s.value);
  const clear = useComposerStore((s) => s.clear);
  const send = useChatSend();
  const stop = useAgentAction("stop");
  // While a run is streaming, the send affordance becomes a stop button —
  // one active run per session (§6.11), so there's nothing to send mid-run.
  const running = useAgentSlice((v) => v.run.running);

  if (running) {
    return (
      <Tooltip label="Stop (Esc)">
        <button
          type="button"
          disabled={!stop}
          onClick={() => stop?.()}
          className="grid h-8 w-8 shrink-0 place-items-center rounded-full border-0 cursor-pointer bg-surface-3 text-fg transition-transform duration-150 hover:scale-105 active:scale-95"
        >
          <Icon name="stop" size={13} />
        </button>
      </Tooltip>
    );
  }

  // Enabled whenever there's text — with no active session, send spins up a
  // draft (useChatSend), so the button works on the welcome screen too.
  const disabled = !value.trim();
  const onClick = () => submitComposer({ value, clear, sendText: send });

  return (
    <Tooltip label="Send (⌘↵)">
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
    </Tooltip>
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
