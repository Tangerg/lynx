import type { MarkdownReveal } from "./markdown/MarkdownMessage";

/**
 * The transcript's shared controls and presentation preferences.
 *
 * Deliberately holds NO session data. One instance is built above the transcript and
 * handed to every turn, so anything in here is compared against every turn's memo — a
 * field that changes while a run streams re-renders the entire transcript on every
 * token. Session facts travel per turn instead, as `TurnFacts`; that separation is why
 * the two are different parameters everywhere rather than one convenient object.
 */
export interface BlockCtx {
  onSelectTool: (id: string) => void;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  /** Mutually exclusive text presentation policy for every block in this turn. */
  textReveal: MarkdownReveal;
}
