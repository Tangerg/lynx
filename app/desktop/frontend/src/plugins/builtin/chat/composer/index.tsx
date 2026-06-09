// Built-in plugin: composer accessories — modes / placeholders /
// keymap / status chips / model picker / attach / send / kbd hint.
//
// Every piece is its own micro-plugin so a fork can drop or replace any
// single accessory without touching the rest. Migration P6.3 moved
// these from raw className strings to Tailwind utilities + Radix
// primitives — see CLAUDE.md "Tailwind first" / "Radix first" rules.

import type { IconName } from "@/components/common";
import type { ReactNode } from "react";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useEffect } from "react";
import { submitComposer } from "@/components/chat/composer";
import { Icon, ProviderIcon, Tooltip } from "@/components/common";
import { useChatSend } from "@/lib/agent/useChatSend";
import { useModels } from "@/lib/data/queries";
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
        className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm border-0 bg-transparent text-fg-faint transition-colors hover:bg-surface-2 hover:text-fg"
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

// Model selector — lists the models of every enabled provider (brand-iconed);
// selection drives the next run's `provider` + `model` PAIR (read by the
// rpc-agent driver from composerStore). Models are identified by provider+id
// since the same model id can appear under more than one provider.
function ModelPicker() {
  const { data: models = [] } = useModels();
  const provider = useComposerStore((s) => s.provider);
  const model = useComposerStore((s) => s.model);
  const setModel = useComposerStore((s) => s.setModel);

  // Default to the first model once the list loads, so what's shown is what
  // the run actually sends (null only lingers while models are still loading).
  useEffect(() => {
    if (!model && models.length > 0) setModel(models[0]!.provider, models[0]!.id);
  }, [model, models, setModel]);

  if (models.length === 0) return null; // no enabled provider yet — nothing to pick
  const selected = models.find((m) => m.provider === provider && m.id === model) ?? models[0]!;

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          aria-label="Switch model"
          className="mr-1 inline-flex h-6.5 shrink-0 items-center gap-1.5 rounded-full border border-transparent bg-transparent pl-1.5 pr-2.5 font-sans text-[12px] font-semibold text-fg whitespace-nowrap transition-colors hover:bg-surface-2 hover:border-line data-[state=open]:bg-surface-2 data-[state=open]:border-line"
        >
          <ProviderIcon provider={selected.provider} size={16} />
          <span className="font-mono text-[11.5px] font-semibold tracking-[0.01em]">
            {selected.label}
          </span>
          <Icon name="chevron-down" size={10} className="text-fg-faint opacity-70" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="start"
          sideOffset={6}
          className="z-50 min-w-[200px] overflow-hidden rounded-md border border-line-soft bg-surface p-1 shadow-lg animate-rise-in"
        >
          {models.map((m) => (
            <DropdownMenu.Item
              key={`${m.provider}:${m.id}`}
              onSelect={() => setModel(m.provider, m.id)}
              className="grid grid-cols-[16px_minmax(0,1fr)_14px] items-center gap-2 rounded-sm px-2 py-1.5 text-[12.5px] text-fg-muted outline-none data-[highlighted]:bg-surface-2 data-[highlighted]:text-fg"
            >
              <ProviderIcon provider={m.provider} size={16} />
              <span className="truncate">{m.label}</span>
              {m.provider === selected.provider && m.id === selected.id && (
                <Icon name="check" size={12} className="text-accent" />
              )}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

function AttachButton() {
  return (
    <Tooltip label="Attach file">
      <button
        type="button"
        aria-label="Attach file"
        className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md border-0 bg-transparent text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg"
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
          className="grid h-8 w-8 shrink-0 place-items-center rounded-full border-0 bg-surface-3 text-fg transition-transform duration-150 active:scale-95"
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
          "grid h-8 w-8 shrink-0 place-items-center rounded-full border-0 transition-transform duration-150",
          disabled
            ? "bg-surface-3 text-fg-faint cursor-not-allowed"
            : "bg-accent text-on-accent active:scale-95",
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
