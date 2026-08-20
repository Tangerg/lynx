// Everything the composer textarea + its surrounding toolbar exposes
// to plugins: key bindings, attachment chips, placeholders, status chips,
// slash commands.

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
 * Shape of one chip rendered in the composer attachments row. Declared here
 * (the SDK) so plugins don't have to import from `components/`.
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
 * A plugin contribution that produces attachment chips. The composer renders
 * every source's chips (ordered by `order`) in their own row, independent of
 * the user's staged `images` (which render as separate thumbnails).
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

export interface ComposerSubmitModeDraft {
  /** Exact controlled textarea value at the submit boundary. */
  rawText: string;
  /** Trimmed typed text, before staged paste material is appended. */
  text: string;
  /** Complete textual body, including staged pasted text. */
  body: string;
  slash: { command: string; args: string } | null;
  hasImages: boolean;
  hasPastes: boolean;
}

export interface ComposerSubmitModeContext extends ComposerSubmitModeDraft {
  /** Commit the same history + clear transaction as an accepted normal send. */
  accept(): void;
  /** Clear without recording history, used when a command only arms a mode. */
  clear(): void;
}

/**
 * A plugin-owned execution mode for the existing composer draft.
 *
 * The composer remains the only draft and submit-pipeline owner. A mode may
 * claim one submit, but it must call `accept` only after its authoritative
 * command has committed; otherwise the draft remains available for recovery.
 */
export interface ComposerSubmitModeSpec {
  id: string;
  matches(draft: ComposerSubmitModeDraft): boolean;
  submit(context: ComposerSubmitModeContext): void;
}

/**
 * Plugin-contributed chip in the composer footer ("project · branch · mode").
 *
 * The component renders the chip body — typically a small `<button>` with
 * icon + label. The host provides no props; chips read state from stores
 * directly.
 */

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
