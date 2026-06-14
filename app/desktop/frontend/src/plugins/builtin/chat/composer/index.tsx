// Built-in plugin: composer accessories — modes / placeholders /
// keymap / status chips / model picker / attach / send / kbd hint.
//
// Every piece is its own micro-plugin so a fork can drop or replace any
// single accessory without touching the rest.

import type { IconName } from "@/components/common";
import type { ReactNode } from "react";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useEffect, useRef } from "react";
import { submitComposer } from "@/components/chat/composer";
import { Icon, ProviderIcon, Tooltip } from "@/components/common";
import { imageFiles } from "@/lib/agent/composerInput";
import { useSelectedModel } from "@/lib/agent/useSelectedModel";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSession";
import { useChatSend } from "@/lib/agent/useChatSend";
import { submitPendingApproval } from "@/lib/agent/submitPendingApproval";
import { useModels, useProjects } from "@/lib/data/queries";
import { basename } from "@/lib/path";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import {
  COMPOSER_KEY_BINDING,
  COMPOSER_MODE,
  COMPOSER_PLACEHOLDER,
  COMPOSER_STATUS,
} from "@/plugins/sdk/kernelPoints";
import { useAgentAction, useAgentSlice, useAgentStore } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useSessionStore } from "@/state/sessionStore";

export const composerModes = definePlugin({
  name: "lyra.builtin.composer-modes",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(COMPOSER_MODE, {
      id: "agent",
      // label/description are i18n keys — the render site (ModePicker) resolves
      // them via t(); a third-party plugin may pass a literal, which t() returns
      // unchanged. Keep them keys so a locale switch relabels live.
      label: "composer.mode.agent",
      icon: "spark",
      order: 0,
      description: "Runs tools, edits files, executes commands. Asks before risky actions.",
    });
    // id is the WIRE RunMode value (API.md: agent | chat | plan) — the driver
    // forwards the selection verbatim on runs.start, so the picker's promise
    // ("read-only, no tool calls") is enforced by the runtime, not just copy.
    host.extensions.contribute(COMPOSER_MODE, {
      id: "chat",
      label: "composer.mode.chat",
      icon: "chat",
      order: 1,
      description: "Read-only conversation. No tool calls, no file changes.",
    });
    host.extensions.contribute(COMPOSER_MODE, {
      id: "plan",
      label: "composer.mode.plan",
      icon: "list",
      order: 2,
      description: "Produces a plan first. Nothing runs until you switch to Agent.",
    });
  },
});

export const composerPlaceholders = definePlugin({
  name: "lyra.builtin.composer-placeholders",
  version: "1.0.0",
  setup({ host }) {
    // `text` is an i18n key — Composer resolves it via t() at render (so a
    // locale switch relabels). The "ask" hint reuses the shared fallback key.
    host.extensions.contribute(COMPOSER_PLACEHOLDER, {
      id: "ask",
      text: "composer.placeholder.fallback",
    });
    host.extensions.contribute(COMPOSER_PLACEHOLDER, {
      id: "debug",
      text: "composer.placeholder.debug",
    });
    host.extensions.contribute(COMPOSER_PLACEHOLDER, {
      id: "implement",
      text: "composer.placeholder.implement",
    });
    host.extensions.contribute(COMPOSER_PLACEHOLDER, {
      id: "refactor",
      text: "composer.placeholder.refactor",
    });
  },
});

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
    // ⌘↩ answers a pending HITL approval if one is open (the run is parked, so
    // there's nothing to send), otherwise it sends. ⇧⌘⌫ declines. Both fall
    // through (return false) when no approval is pending, so the keys keep their
    // normal meaning the rest of the time.
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Mod+Enter",
      description: "Approve a pending request, otherwise send",
      handler: ({ submit }) => {
        if (submitPendingApproval("approved")) return true;
        submit();
        return true;
      },
    });
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Mod+Shift+Backspace",
      description: "Decline a pending request",
      handler: () => submitPendingApproval("declined"),
    });
    // Esc stops the active run while the composer is focused — the Stop button's
    // "(Esc)" hint. Composer-scoped on purpose: a GLOBAL Escape would fight the
    // overlays (command palette / dropdowns) that own Esc. Returns false (lets
    // Esc fall through) when nothing is running. Reads the store imperatively —
    // keybinding handlers run outside React. Composer.tsx guards isComposing
    // before this lookup, so Esc-to-cancel an IME candidate never reaches here.
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Escape",
      description: "Stop the running response",
      handler: () => {
        const sid = useSessionStore.getState().activeSessionId;
        const entry = useAgentStore.getState().sessions[sid];
        if (!entry?.view.run.running) return false;
        entry.stop?.();
        return true;
      },
    });
  },
});

//
// cwd + branch read live session/project state. ExecModeChip is the one
// hold-out: the protocol has no approval-policy surface yet, so its hint
// stays a static placeholder until one exists.

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
// the footer light.
function IconChip({ icon, hint }: { icon: IconName; hint: string }) {
  return (
    <Tooltip label={hint}>
      <button
        type="button"
        aria-label={hint}
        className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm border-0 bg-transparent text-fg-faint transition-colors hover:bg-surface-2 hover:text-fg"
      >
        <Icon name={icon} size={12} />
      </button>
    </Tooltip>
  );
}

const ExecModeChip = () => {
  const t = useT();
  return <IconChip icon="shield" hint={t("composer.execMode.hint")} />;
};

// Where is this conversation working? Basename in the chip, full path in
// the tooltip. Hidden until the session (and its cwd) is known.
function CwdChip() {
  const cwd = useActiveSessionCwd();
  if (!cwd) return null;
  return (
    <Chip icon="folder" title={cwd}>
      {basename(cwd)}
    </Chip>
  );
}

// Live git branch of the active session's project (workspace.listProjects;
// checkout flows refresh it through the resync invalidation). Hidden when
// the cwd isn't a known project or has no branch.
function GitBranchChip() {
  const t = useT();
  const cwd = useActiveSessionCwd();
  const { data: projects } = useProjects();
  const branch = cwd ? projects?.find((p) => p.id === cwd)?.branch : undefined;
  if (!branch) return null;
  return (
    <Chip icon="branch" title={t("composer.gitBranch")}>
      {branch}
    </Chip>
  );
}

export const composerChips = definePlugin({
  name: "lyra.builtin.composer-chips",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(COMPOSER_STATUS, {
      id: "cwd",
      order: 0,
      component: CwdChip,
    });
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

// Model selector — lists the models of every enabled provider (brand-iconed);
// selection drives the next run's `provider` + `model` PAIR (read by the
// rpc-agent driver from composerStore). Models are identified by provider+id
// since the same model id can appear under more than one provider.
function ModelPicker() {
  const t = useT();
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
          aria-label={t("composer.switchModel")}
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

// Attach images — opens a file picker; the selected image/* files are read to
// base64 and staged in composerStore (paste / drop go through Composer). Gated
// by the selected model's `multimodal` capability (the backend also rejects a
// non-multimodal model with invalid_params, MULTIMODAL_IMAGE_INPUT §4).
function AttachButton() {
  const t = useT();
  const addImageFiles = useComposerStore((s) => s.addImageFiles);
  const inputRef = useRef<HTMLInputElement>(null);

  const canAttach = useSelectedModel()?.multimodal ?? false;

  return (
    <>
      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        multiple
        aria-label={t("composer.attachImage")}
        className="hidden"
        onChange={(e) => {
          const files = imageFiles(e.target.files);
          e.target.value = ""; // allow re-picking the same file
          if (files.length > 0) addImageFiles(files); // same store path as paste/drop
        }}
      />
      <Tooltip
        label={canAttach ? t("composer.attachImage") : t("composer.attachImage.unsupported")}
      >
        <button
          type="button"
          aria-label={t("composer.attachImage")}
          disabled={!canAttach}
          onClick={() => inputRef.current?.click()}
          className="inline-flex h-6.5 w-6.5 shrink-0 items-center justify-center rounded-full border-0 bg-transparent text-fg-faint transition-colors hover:bg-surface-2 hover:text-fg disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent"
        >
          <Icon name="image" size={15} />
        </button>
      </Tooltip>
    </>
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

function SendButton() {
  const t = useT();
  const value = useComposerStore((s) => s.value);
  const images = useComposerStore((s) => s.images);
  const clear = useComposerStore((s) => s.clear);
  const send = useChatSend();
  const stop = useAgentAction("stop");
  // While a run is streaming, the send affordance becomes a stop button —
  // one active run per session (§6.11), so there's nothing to send mid-run.
  const running = useAgentSlice((v) => v.run.running);

  if (running) {
    return (
      <Tooltip label={t("composer.action.stop")}>
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
  const disabled = !value.trim() && images.length === 0;
  const onClick = () => submitComposer({ value, clear, sendInput: send, images });

  return (
    <Tooltip label={t("composer.action.send")}>
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
