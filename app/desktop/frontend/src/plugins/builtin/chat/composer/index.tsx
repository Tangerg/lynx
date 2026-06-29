// Built-in plugin: composer accessories — modes / placeholders /
// keymap / status chips / model picker / attach / send / kbd hint.
//
// Every piece is its own micro-plugin so a fork can drop or replace any
// single accessory without touching the rest.

import type { IconName } from "@/components/common";
import type { ApprovalModeValue } from "@/lib/data/queries";
import type { ReactNode } from "react";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useEffect, useRef } from "react";
import { submitComposer } from "@/components/chat/composer";
import { Icon, MENU_CONTENT_CLASSES, ProviderIcon, Tooltip } from "@/components/common";
import { APPROVAL_MODES } from "@/lib/agent/approvalModes";
import { setApprovalMode } from "@/lib/agent/approvalConfig";
import { imageFiles } from "@/lib/agent/composerInput";
import { rpcErrorText } from "@/lib/agent/errorCopy";
import { fmtTokens } from "@/lib/format";
import { useSelectedModel } from "@/lib/agent/useSelectedModel";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSession";
import { useChatSend } from "@/lib/agent/useChatSend";
import { submitPendingApproval } from "@/lib/agent/submitPendingApproval";
import { notifyError } from "@/lib/notify";
import { useApprovalMode, useModels, useProjects } from "@/lib/data/queries";
import { basename } from "@/lib/path";
import { t, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import {
  COMPOSER_KEY_BINDING,
  COMPOSER_PLACEHOLDER,
  COMPOSER_STATUS,
} from "@/plugins/sdk/kernelPoints";
import {
  useAgentAction,
  useAgentRunContextTokens,
  useAgentRunning,
  useAgentRunUsage,
  useAgentStore,
} from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useSessionStore } from "@/state/sessionStore";

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

// After a history recall swaps the textarea value (async, via the store), park
// the caret at the end on the next frame: repeated ↑ then steps further back
// (single-line entries) or navigates within a recalled multi-line entry before
// recalling the next one — the editor-like "history at the boundary" behavior.
function caretToEnd(ta: HTMLTextAreaElement): void {
  requestAnimationFrame(() => {
    const end = ta.value.length;
    ta.setSelectionRange(end, end);
  });
}

export const composerKeymap = definePlugin({
  name: "lyra.builtin.composer-keymap",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Enter",
      description: t("composer.key.sendDesc"),
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
      description: t("composer.key.approveDesc"),
      handler: ({ submit }) => {
        if (submitPendingApproval("approved")) return true;
        submit();
        return true;
      },
    });
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "Mod+Shift+Backspace",
      description: t("composer.key.declineDesc"),
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
      description: t("composer.key.stopDesc"),
      handler: () => {
        const sid = useSessionStore.getState().activeSessionId;
        const entry = useAgentStore.getState().sessions[sid];
        if (!entry?.view.run.running) return false;
        entry.stop?.();
        return true;
      },
    });
    // ↑/↓ recall previously-sent messages (shell-style), but only at the
    // composer's edge: ↑ recalls when the caret can't move further up (no
    // earlier line), ↓ when it can't move further down — otherwise the arrows
    // move within a multi-line draft as usual. A text selection falls through.
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "ArrowUp",
      description: t("composer.key.historyPrevDesc"),
      handler: ({ event }) => {
        const ta = event.target as HTMLTextAreaElement | null;
        if (!ta || ta.selectionStart !== ta.selectionEnd) return false;
        if (ta.value.slice(0, ta.selectionStart).includes("\n")) return false;
        if (!useComposerStore.getState().historyPrev()) return false;
        caretToEnd(ta);
        return true;
      },
    });
    host.extensions.contribute(COMPOSER_KEY_BINDING, {
      key: "ArrowDown",
      description: t("composer.key.historyNextDesc"),
      handler: ({ event }) => {
        const ta = event.target as HTMLTextAreaElement | null;
        if (!ta || ta.selectionStart !== ta.selectionEnd) return false;
        if (ta.value.slice(ta.selectionEnd).includes("\n")) return false;
        if (!useComposerStore.getState().historyNext()) return false;
        caretToEnd(ta);
        return true;
      },
    });
  },
});

// Readable chip — icon + label, no affordance glyph. Used where the
// value itself carries meaning the user wants to read (e.g. branch name).
function Chip({ icon, title, children }: { icon: IconName; title: string; children: ReactNode }) {
  return (
    <span
      title={title}
      className="inline-flex items-center gap-1.5 text-[11.5px] text-fg-faint whitespace-nowrap transition-colors hover:text-fg-muted"
    >
      <Icon name={icon} size={11} className="shrink-0" />
      <span>{children}</span>
    </span>
  );
}

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

// The composer-side entry to the runtime's global approval stance
// (approval.setMode) — the same control as the Approvals settings pane, surfaced
// at the point of attention. The stance is read live at every tool gate, so a
// switch here lands on the next gated call, even mid-run. It is GLOBAL, not
// per-turn (like the model picker, it sticks until changed) — there is no
// per-run mode in the protocol by design. Hidden when the runtime predates the
// approval RPCs (getMode errors) or before the stance has loaded.
function ApprovalModeChip() {
  const t = useT();
  const { data: mode, isError } = useApprovalMode();
  if (isError || mode === undefined) return null;
  const current = APPROVAL_MODES.find((m) => m.value === mode) ?? APPROVAL_MODES[2]!;
  const onSelect = async (next: ApprovalModeValue) => {
    if (next === mode) return;
    try {
      await setApprovalMode(next);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("approvals.error.mode"));
    }
  };
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          aria-label={t("approvals.mode.aria")}
          className="inline-flex items-center gap-1.5 text-[11.5px] text-fg-faint whitespace-nowrap transition-colors hover:text-fg-muted data-[state=open]:text-fg"
        >
          <Icon
            name="shield"
            size={11}
            className={cn("shrink-0", mode === "yolo" ? "text-warning" : "")}
          />
          {/* Warn-tint the unprompted "Auto" stance so it can't run silently
              unnoticed; the other stances stay neutral. */}
          <span className={cn(mode === "yolo" && "text-warning")}>{t(current.labelKey)}</span>
          <Icon name="chevron-down" size={9} className="opacity-70" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="start"
          sideOffset={6}
          className={cn(MENU_CONTENT_CLASSES, "min-w-[248px]")}
        >
          {APPROVAL_MODES.map((m) => (
            <DropdownMenu.Item
              key={m.value}
              onSelect={() => void onSelect(m.value)}
              className="grid grid-cols-[minmax(0,1fr)_14px] items-start gap-2 rounded-sm px-2 py-1.5 outline-none data-[highlighted]:bg-surface-2"
            >
              <span className="min-w-0">
                <span className="block text-[12.5px] font-semibold text-fg">{t(m.labelKey)}</span>
                <span className="block text-[11.5px] leading-snug text-fg-muted">
                  {t(m.descKey)}
                </span>
              </span>
              {m.value === mode && <Icon name="check" size={12} className="mt-0.5 text-accent" />}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

// Color ramp for context-window occupancy: calm until ~70%, warn approaching
// the limit, alarm once compaction is imminent (>90%).
function ctxTone(pct: number): string {
  if (pct >= 90) return "text-negative";
  if (pct >= 70) return "text-warning";
  return "text-fg-faint";
}

// Live token + cost readout for the current/last run (RunState.usage, the
// cumulative-over-rounds total) PLUS the context-window occupancy (the latest
// round's prompt size over the served model's contextWindow — how full the
// window is right now, distinct from the summed usage). Hidden until a run has
// reported usage, so a fresh session shows nothing rather than a bare "↑0 ↓0".
// Cost shows only when the model is priced; the % only when the model's window
// is known — never a fabricated number.
function UsageChip() {
  const t = useT();
  const usage = useAgentRunUsage();
  const contextTokens = useAgentRunContextTokens();
  const model = useSelectedModel();
  if (usage.inputTokens + usage.outputTokens === 0) return null;
  const window = model?.contextWindow;
  const pct =
    contextTokens !== undefined && window
      ? Math.min(100, Math.round((contextTokens / window) * 100))
      : undefined;
  return (
    <span
      title={t("composer.usage.hint")}
      className="inline-flex items-center gap-1.5 text-[11.5px] text-fg-faint whitespace-nowrap tabular-nums"
    >
      <span>↑{fmtTokens(usage.inputTokens)}</span>
      <span>↓{fmtTokens(usage.outputTokens)}</span>
      {usage.costUsd !== undefined && <span>·&nbsp;${usage.costUsd.toFixed(2)}</span>}
      {pct !== undefined && (
        <span className={ctxTone(pct)} title={t("composer.usage.context")}>
          ·&nbsp;{pct}%
        </span>
      )}
    </span>
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
      id: "approval-mode",
      order: 1,
      component: ApprovalModeChip,
    });
    host.extensions.contribute(COMPOSER_STATUS, {
      id: "git-branch",
      order: 2,
      component: GitBranchChip,
    });
    host.extensions.contribute(COMPOSER_STATUS, {
      id: "usage",
      order: 3,
      component: UsageChip,
    });
  },
});

// Model selector — lists the models of every enabled provider (brand-iconed);
// selection drives the next run's `provider` + `model` PAIR (read by the
// rpc-agent driver from composerStore). Models are identified by provider+id
// since the same model id can appear under more than one provider.
function ModelPicker() {
  const t = useT();
  const { data: models = [], isLoading } = useModels();
  const provider = useComposerStore((s) => s.provider);
  const model = useComposerStore((s) => s.model);
  const setModel = useComposerStore((s) => s.setModel);

  // Default to the first model once the list loads, so what's shown is what
  // the run actually sends (null only lingers while models are still loading).
  useEffect(() => {
    if (!model && models.length > 0) setModel(models[0]!.provider, models[0]!.id);
  }, [model, models, setModel]);

  if (models.length === 0) {
    // Cold start: reserve the trigger's footprint with a quiet placeholder so
    // the picker doesn't pop in and shove the toolbar once models resolve. A
    // genuinely empty list (no provider configured) still collapses to nothing
    // — the welcome / settings flow handles first-run setup.
    if (!isLoading) return null;
    return (
      <div
        className="inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full pl-1.5 pr-2.5 opacity-60"
        aria-hidden
      >
        <span className="h-4 w-4 rounded-full bg-surface-2" />
        <span className="h-3 w-16 rounded bg-surface-2" />
      </div>
    );
  }
  const selected = models.find((m) => m.provider === provider && m.id === model) ?? models[0]!;

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          aria-label={t("composer.switchModel")}
          className="inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border border-line/40 bg-transparent pl-1.5 pr-2.5 text-[12px] font-medium text-fg whitespace-nowrap transition-colors hover:bg-surface-2 data-[state=open]:bg-surface-2"
          data-slot="composer-model"
        >
          <ProviderIcon provider={selected.provider} size={16} />
          <span className="font-sans text-[12px] font-medium">{selected.label}</span>
          <Icon name="chevron-down" size={10} className="text-fg-faint opacity-70" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="start"
          sideOffset={6}
          className={cn(MENU_CONTENT_CLASSES, "min-w-[200px]")}
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
          className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-0 bg-transparent text-fg-muted transition-colors hover:bg-fg/[0.06] hover:text-fg active:scale-95 disabled:cursor-not-allowed disabled:opacity-25"
          data-slot="composer-attach"
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
  const running = useAgentRunning();

  // Mid-run, a message doesn't open a new turn — it STEERS the active one
  // (useChatSend → runs.steer). Surface that as an explicit Steer button while
  // there's text to send, so the capability isn't keyboard-only; fall back to
  // the Stop button when the composer is empty (Esc stops in either case).
  if (running) {
    if (value.trim()) {
      return (
        <Tooltip label={t("composer.action.steer")}>
          <button
            type="button"
            onClick={() => submitComposer({ value, clear, sendInput: send, images })}
            className="grid h-8 w-8 shrink-0 place-items-center rounded-full border-0 bg-accent text-on-accent transition-transform duration-150 active:scale-95"
            data-slot="composer-send"
          >
            <Icon name="send-arrow" size={14} strokeWidth={2.5} />
          </button>
        </Tooltip>
      );
    }
    return (
      <Tooltip label={t("composer.action.stop")}>
        <button
          type="button"
          disabled={!stop}
          onClick={() => stop?.()}
          className="grid h-8 w-8 shrink-0 place-items-center rounded-full border-0 bg-surface-3 text-fg-muted transition-colors duration-150 hover:bg-surface-4 hover:text-fg active:scale-95 disabled:cursor-not-allowed disabled:opacity-40"
          data-slot="composer-stop"
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
            ? "bg-transparent text-fg-faint/30 cursor-not-allowed"
            : "bg-fg text-on-fg active:scale-95",
        )}
        data-slot="composer-send"
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
