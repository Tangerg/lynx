// @file autocomplete (T2.3): detect an `@token` at the caret, fuzzy-match it
// against the workspace file list (workspace.files.list, recursive), and let the
// user pick a path that's spliced back into the composer text. The picker owns
// ↑/↓/Enter/Tab/Esc while open (Composer routes keydowns here before its normal
// keymap). The file list is fetched lazily (only once a mention opens) and
// cached per cwd by react-query.
//
// Hand-rolled rather than Base UI's Combobox (the §4 exemption, stated): a
// Combobox owns an input and treats its value as the query, and here the query is
// one `@token` inside a message that is otherwise free text — the input's value is
// the whole draft. What that exemption costs is the ARIA the primitive would have
// supplied, so the composer wires the combobox pattern by hand off these ids:
// `MENTION_LISTBOX_ID` on the popup, `mentionOptionId(i)` per row, and
// aria-activedescendant on the textarea.

/** The popup's element id — `aria-controls` points at it only while mounted. */
export const MENTION_LISTBOX_ID = "composer-mention-listbox";

/** Per-row element id — `aria-activedescendant` names the focused one. */
export function mentionOptionId(index: number): string {
  return `composer-mention-option-${index}`;
}

import { useCallback, useMemo, useState } from "react";
import { useWorkspaceListFiles } from "@/plugins/builtin/workspace/public/queries";
import { fuzzyFile } from "./fuzzyFile";

const MENTION_ROWS = 8; // visible suggestions
const FETCH_LIMIT = 2000; // recursive file-list cap fed to the fuzzy matcher

interface Mention {
  query: string;
  start: number; // index of the '@'
  end: number; // caret
}

/** The active `@token` at `caret`, or null. The '@' must start a token (string
 *  start or after whitespace) so "user@host" doesn't trigger; the token runs to
 *  the caret and contains no whitespace. */
export function activeMention(value: string, caret: number): Mention | null {
  let i = caret - 1;
  for (; i >= 0; i--) {
    const ch = value[i]!;
    if (ch === "@") break;
    if (/\s/.test(ch)) return null;
  }
  if (i < 0 || value[i] !== "@") return null;
  const before = value[i - 1];
  if (i > 0 && before !== undefined && !/\s/.test(before)) return null;
  return { query: value.slice(i + 1, caret), start: i, end: caret };
}

interface Args {
  value: string;
  caret: number;
  cwd: string | undefined;
  /** Replace the composer text and move the caret (Composer wires the textarea). */
  apply: (text: string, caret: number) => void;
}

export interface FileMentions {
  active: boolean;
  items: string[];
  index: number;
  setIndex: (i: number) => void;
  accept: (path: string) => void;
  /** Returns true (and the caller should preventDefault) when the picker
   *  consumed the key — only while open. */
  handleKeyDown: (e: { key: string; shiftKey: boolean }) => boolean;
}

export function useFileMentions({ value, caret, cwd, apply }: Args): FileMentions {
  const [selection, setSelection] = useState<{
    candidateKey: string;
    index: number;
  } | null>(null);
  // The '@' position a user dismissed with Esc — suppresses the popup for that
  // one mention until they move off it (a new '@' reopens).
  const [dismissedStart, setDismissedStart] = useState<number | null>(null);

  const mention = useMemo(() => activeMention(value, caret), [value, caret]);
  const open = mention !== null && mention.start !== dismissedStart;

  // Lazy + cached: the recursive list is fetched only once a mention is open.
  const { data: files } = useWorkspaceListFiles(
    open && cwd !== undefined ? { cwd, recursive: true, limit: FETCH_LIMIT } : undefined,
  );

  const items = useMemo(() => {
    if (!open || !mention || !files) return [];
    return fuzzyFile(
      mention.query,
      files.map((f) => f.path),
      MENTION_ROWS,
    );
  }, [open, mention, files]);

  // Selection belongs to one concrete candidate set. Deriving the visible
  // index from that identity resets it during render when the mention or
  // results change, without an effect and its one-frame stale selection.
  const candidateKey = [cwd, mention?.start, mention?.query, ...items].join("\0");
  const index =
    selection?.candidateKey === candidateKey && selection.index < items.length
      ? selection.index
      : 0;
  const setIndex = useCallback(
    (nextIndex: number) => {
      if (nextIndex < 0 || nextIndex >= items.length) return;
      setSelection({ candidateKey, index: nextIndex });
    },
    [candidateKey, items.length],
  );

  const active = open && items.length > 0;

  const accept = useCallback(
    (path: string) => {
      if (!mention) return;
      const insert = path + " ";
      apply(
        value.slice(0, mention.start) + insert + value.slice(mention.end),
        mention.start + insert.length,
      );
      setDismissedStart(null);
    },
    [mention, value, apply],
  );

  const handleKeyDown = useCallback(
    (e: { key: string; shiftKey: boolean }): boolean => {
      if (!active) return false;
      switch (e.key) {
        case "ArrowDown":
          setIndex((index + 1) % items.length);
          return true;
        case "ArrowUp":
          setIndex((index - 1 + items.length) % items.length);
          return true;
        case "Tab":
          accept(items[index] ?? items[0]!);
          return true;
        case "Enter":
          if (e.shiftKey) return false; // Shift+Enter still inserts a newline
          accept(items[index] ?? items[0]!);
          return true;
        case "Escape":
          if (mention) setDismissedStart(mention.start);
          return true;
        default:
          return false;
      }
    },
    [active, items, index, setIndex, accept, mention],
  );

  return { active, items, index, setIndex, accept, handleKeyDown };
}
