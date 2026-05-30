// Everything the composer textarea + its surrounding toolbar exposes
// to plugins: key bindings, attachment chips, placeholders, mode toggles,
// status chips, slash commands.

import type { ComponentType } from "react";

/**
 * Context passed to a composer key binding handler. The handler can read
 * the current value, replace it, or invoke `submit` to send the pending
 * text. Returning `true` (or invoking `preventDefault` indirectly via
 * `submit`) tells the host to stop the browser default.
 */
export interface ComposerKeyContext {
  value: string;
  onChange: (next: string) => void;
  submit: () => void;
  event: KeyboardEvent;
}

export interface ComposerKeyBindingSpec {
  /** Combo string — same format as `host.shortcuts.register`. */
  key: string;
  description?: string;
  /** Return `true` to call `preventDefault` on the keypress. */
  handler: (ctx: ComposerKeyContext) => boolean | void;
}

/**
 * Shape of one chip rendered in the composer attachments row. Mirrors
 * `components/chat/Composer.tsx`'s `Attachment` type — declared here so
 * plugins don't have to import from `components/`.
 */
export interface ComposerAttachment {
  /** Display label, e.g. "src/api/auth.ts". */
  label: string;
  /** Optional icon glyph name. Defaults to "file" when omitted. */
  icon?: string;
  /**
   * Optional stable React-key id. When omitted the renderer falls back
   * to `${source.id}:${label}` — sufficient for the typical case where
   * each source emits unique labels.
   */
  id?: string;
}

/**
 * A plugin contribution that produces attachment chips. The kernel
 * merges the lists from every source (in `order`) ahead of any
 * user-added items stored in `useComposerStore.attachments`.
 *
 * `useAttachments` is a hook — plugins can derive the list from query
 * data ("recently edited files") or other stores.
 */
export interface ComposerAttachmentSourceSpec {
  id: string;
  /** Sort hint — lower comes first. Built-ins use 0..99. */
  order?: number;
  /** Hook that returns the current attachments. */
  useAttachments: () => ComposerAttachment[];
}

/**
 * One placeholder string for the composer textarea. Composer picks one at
 * mount via weighted random — `weight` defaults to 1, so a plugin can
 * register multiple to bias toward (or against) certain prompts.
 *
 * Useful for branding ("Ask Acme…") or seasonal nudges ("Try /lint on a
 * test file").
 */
export interface ComposerPlaceholderSpec {
  id: string;
  text: string;
  /** Selection weight — defaults to 1. Set to 0 to register but skip selection. */
  weight?: number;
}

/**
 * A composer mode toggle ("Agent" / "Ask" / "Plan" by default — plugins can
 * register more). The active mode is stored on `useComposerStore.mode` so
 * the conversation context (agent vs ask vs plan) can drive runtime
 * behaviour (e.g. a /plan command, or a stricter prompt prefix).
 *
 * Mode ids are free-form strings: built-ins use `agent`, `ask`, `plan`;
 * a third-party plugin could add `code`, `research`, etc.
 */
export interface ComposerModeSpec {
  id: string;
  label: string;
  icon?: string;
  /** Sort hint — lower comes first. Built-ins use 0..99. */
  order?: number;
  /** Optional tooltip; defaults to "${label} mode". */
  title?: string;
  /**
   * One-line capability blurb shown under the label in the mode dropdown
   * — e.g. "Read-only", "Plans first, then executes", "Runs tools".
   * The point is to make permissions/behaviour obvious before the user
   * picks a mode (UX review: "Composer mode 的语义不够可见").
   */
  description?: string;
}

/**
 * Plugin-contributed chip in the composer footer ("project · branch · mode").
 *
 * The component renders the chip body — typically a small `<button>` with
 * icon + label. The host provides no props; chips read state from stores
 * directly.
 */
export interface ComposerStatusSpec {
  id: string;
  /** Sort hint — lower comes first. Built-ins use 0..99. */
  order?: number;
  /**
   * Which side of the footer the chip sits on. "start" (default) =
   * left context chips (project / mode / branch); "end" = right-aligned
   * run telemetry (tokens / cost / run state).
   */
  align?: "start" | "end";
  /** The chip body. Receives no props. */
  component: ComponentType;
}

/**
 * Context passed to a slash command's `run` function.
 *
 * `send(text)` lets the command queue a real agent message after running
 * its local logic. Useful for commands like `/lint <file>` that first hit
 * a backend endpoint and then ask the agent to interpret the result.
 */
export interface SlashCommandRunCtx {
  args: string;
  send: (text: string) => void;
}

export interface SlashCommandSpec {
  /** Description shown in the autocomplete dropdown. */
  description: string;
  /**
   * Optional run handler. If absent, the command is a *hint only* — typing
   * it just shows the description; pressing Enter forwards the raw text as
   * a normal user message.
   */
  run?: (ctx: SlashCommandRunCtx) => void | Promise<void>;
}
