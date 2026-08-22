// Command palette entries + global keyboard shortcuts.

/**
 * A palette-invokable action. Surfaced in the Cmd+K command palette and
 * (eventually) any context-menu / button that wants to invoke it by id.
 *
 * Distinct from slash commands (which run from the composer when the user
 * types `/<cmd>`). Both can coexist for the same action — register both
 * if you want it reachable from both UIs.
 */
export interface CommandSpec {
  /** Stable id. */
  id: string;
  /**
   * Display label — a catalog key, resolved where the command renders.
   *
   * NOT resolved text: a command is registered once, and nothing re-registers on
   * a language switch, so a `t(...)` here froze the label in whatever locale the
   * app booted in while the view titles beside it (which are keys) updated. A key
   * the catalog doesn't have renders as itself, which also keeps identifiers
   * visible instead of silently rendering an empty label.
   */
  label: string;
  /** Short explanation shown below the label — a catalog key, as `label` is. */
  description?: string;
  /** Icon name. */
  icon?: string;
  /** Group header in the palette — a catalog key, as `label` is. */
  group?: string;
  /** Extra search aliases — appears in the label match but isn't displayed. */
  keywords?: string[];
  /** Key combo this command is bound to, e.g. "Mod+N" (Cmd on Mac, Ctrl
   *  elsewhere). The palette renders it as a platform glyph; `global-keymap`
   *  binds the global commands by reading this — one source, no glyph/combo drift. */
  combo?: string;
  /** Sort hint within the group. Lower comes first. */
  order?: number;
  /**
   * What to do. Optional `args` are forwarded by `host.commands.execute(id,
   * …args)` (cross-plugin invocation, VSCode-style); palette / shortcut
   * triggers pass none, so most commands take zero params.
   */
  run: (...args: unknown[]) => void | Promise<void>;
}

/**
 * Handler invoked when the matching key combo is pressed. Receives the
 * raw event so handlers can decide whether to `preventDefault` (most do).
 *
 * Return value is ignored.
 */
export type ShortcutHandler = (event: KeyboardEvent) => void;

/**
 * A keyboard shortcut registration.
 *
 * `key` is a `KeyboardEvent.key` plus optional modifier prefixes joined by
 * `+`. Examples:
 *   - "Escape"
 *   - "Cmd+K"           (Mac ⌘)
 *   - "Ctrl+K"          (everywhere else)
 *   - "Mod+K"           (Cmd on Mac, Ctrl elsewhere — preferred)
 *   - "Shift+/"         (`?` on US keyboards)
 *   - "Mod+Shift+P"
 *
 * Matching is case-insensitive on the key name. If two plugins register
 * the same combo, the last one wins (with a warning) — same policy as the
 * other slots.
 */
export interface ShortcutSpec {
  /** Combo string, e.g. "Mod+K". */
  key: string;
  /** What to do. */
  handler: ShortcutHandler;
  /** Optional human-readable description for a future shortcuts cheat-sheet. */
  description?: string;
  /**
   * Whether to fire even when the active element is an `<input>`/`<textarea>`.
   * Defaults to false — most shortcuts shouldn't steal typing input.
   */
  allowInInputs?: boolean;
}
