import { cn } from "@/lib/classNames";

/**
 * How much a change adds and removes — the one shape for it.
 *
 * It was spelled six times across four files (the diff view's card header and its
 * subtext, the changed-file list's header and its rows, the run summary), and only
 * two of the six set `tabular-nums`. So the figures did not line up down a list, let
 * alone between two panels showing the same change: a column of proportional digits
 * shifts on every row whose count has a different width.
 *
 * A binary file is not "0 added, 0 removed": it HAS no line counts, and a pair of
 * zeroes claims it was touched and changed nothing.
 *
 * One strength, no dimmed variant. A rolled-up figure on a directory row briefly had
 * one at 70% alpha so a container's total would not outrank a file's own — and alpha
 * on tinted text is contrast: it measured 3.08:1 against the pane at 12px, under the
 * 4.5:1 the WCAG gate holds. The distinction it was reaching for is already made by
 * the LABEL beside it, which is muted on a directory and full ink on a file.
 */
export function DiffStat({
  added,
  removed,
  binary,
  className,
}: {
  added: number;
  removed: number;
  /**
   * The word for "this has no line counts", supplied by the caller — which is also
   * how the caller says the content IS binary. A boolean would have needed a default
   * label here, and a label in a design-system atom is a word no catalog owns.
   *
   * `string` and NOT `ReactNode`, learned the hard way: `ReactNode` accepts `false`,
   * so a caller passing its own boolean flag through compiled fine and then took this
   * branch on every non-binary row, rendering an empty span where the figures should
   * have been. A type that cannot hold the wrong thing beats remembering not to.
   */
  binary?: string;
  className?: string;
}) {
  const base = cn("shrink-0 items-center gap-1.5 font-mono tabular-nums", className);

  if (binary !== undefined) {
    return <span className={cn("inline-flex text-fg-faint", base)}>{binary}</span>;
  }
  // Nothing measured yet, or nothing to measure. A dash holds the column so the
  // figures beside it stay in line, which a blank does not.
  if (added === 0 && removed === 0) {
    return (
      <span aria-hidden className={cn("inline-flex text-fg-faint", base)}>
        —
      </span>
    );
  }

  return (
    <span className={cn("inline-flex", base)}>
      {added > 0 && <span className="text-success">+{added}</span>}
      {removed > 0 && <span className="text-negative">−{removed}</span>}
    </span>
  );
}
