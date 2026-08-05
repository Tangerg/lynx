/**
 * The files this draft carries.
 *
 * An `@path` typed into the composer is a real attachment — the turn will be sent with
 * that file in it — and until now it was indistinguishable from prose: a token in the
 * middle of a sentence, in the same ink as the sentence. What you had attached was
 * something you re-read your own message to work out.
 *
 * Derived, never stored. The draft text is the single source of what is attached, so a
 * chip row computed from it cannot disagree with the message that gets sent; a second
 * list, kept in sync, could.
 */

/** Matches a token that starts a word — the same rule `activeMention` applies, so a
 *  chip appears for exactly what the picker would have completed, and `user@host`
 *  is not a file. */
const MENTION = /(^|\s)@(\S+)/g;

export interface DraftMention {
  /** The path as typed. */
  path: string;
  /** Where the `@` sits, so removing this chip removes THIS occurrence — the same
   *  file mentioned twice is two chips, and closing one must not close the other. */
  start: number;
  /** One past the last character of the token. */
  end: number;
}

export function draftMentions(value: string): DraftMention[] {
  const out: DraftMention[] = [];
  MENTION.lastIndex = 0;
  for (let match = MENTION.exec(value); match !== null; match = MENTION.exec(value)) {
    const lead = match[1]?.length ?? 0;
    const path = match[2] ?? "";
    if (path === "") continue;
    const start = match.index + lead;
    out.push({ path, start, end: start + 1 + path.length });
  }
  return out;
}

/**
 * The draft with one mention taken out, and the whitespace it leaves behind tidied.
 *
 * Removing the token alone leaves a double space where it was, which the next thing
 * the user types then sits after — so the seam is collapsed to a single space unless
 * the token was at an edge.
 */
export function removeMention(value: string, mention: DraftMention): string {
  const before = value.slice(0, mention.start);
  const after = value.slice(mention.end);
  if (before === "" || after === "") return (before + after).trim();
  return `${before.replace(/\s+$/, "")} ${after.replace(/^\s+/, "")}`;
}
