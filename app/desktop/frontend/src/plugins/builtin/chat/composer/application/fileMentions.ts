// @file autocomplete (T2.3): detect an `@token` at the caret, fuzzy-match it
// against the workspace file list (workspace.listFiles, recursive), and let the
// user pick a path that's spliced back into the composer text. The picker owns
// ↑/↓/Enter/Tab/Esc while open (Composer routes keydowns here before its normal
// keymap). The file list is fetched lazily (only once a mention opens) and
// cached per cwd by react-query.

import { useCallback, useEffect, useMemo, useState } from "react";
import { useListFiles } from "@/lib/data/queries";
import { fuzzyFile } from "@/lib/fuzzyFile";

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
  const [index, setIndex] = useState(0);
  // The '@' position a user dismissed with Esc — suppresses the popup for that
  // one mention until they move off it (a new '@' reopens).
  const [dismissedStart, setDismissedStart] = useState<number | null>(null);

  const mention = useMemo(() => activeMention(value, caret), [value, caret]);
  const open = mention !== null && mention.start !== dismissedStart;

  // Lazy + cached: the recursive list is fetched only once a mention is open.
  const { data: files } = useListFiles(
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

  // Reset the highlighted row whenever the candidate set changes.
  useEffect(() => {
    setIndex(0);
  }, [mention?.query, files]);

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
          setIndex((i) => (i + 1) % items.length);
          return true;
        case "ArrowUp":
          setIndex((i) => (i - 1 + items.length) % items.length);
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
    [active, items, index, accept, mention],
  );

  return { active, items, index, setIndex, accept, handleKeyDown };
}
