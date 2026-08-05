import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";

/**
 * The heading that opens a section of a pane.
 *
 * Deliberately quieter than the rows it introduces: with regions separated by
 * tone instead of by lines, a label that competes with its own content turns
 * every section boundary into a second divider. Sentence case and a slightly
 * smaller step make it recognizable without adding the console-like texture of
 * tracked capitals.
 *
 * `trailing` is the count, progress or action the section is about. It belongs on
 * the label's own baseline because that is the one line in a section that is not
 * data; hanging it off the first row instead would make one row wear two jobs.
 */
export function SectionLabel({
  children,
  trailing,
  className,
}: {
  children: ReactNode;
  trailing?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        // `leading-tight` and not `leading-none`: the label truncates, and `truncate`
        // clips both axes. At a line box the height of the font size, the 2px the glyph
        // box needs below the baseline is outside it — so the `j` in "Projects" had its
        // tail shaved off. 1.15 is the ladder's tightest step that still contains its
        // own text.
        "flex min-w-0 items-center gap-2 px-2 pb-2 pt-2 font-sans text-ui-xs font-medium leading-tight text-fg-faint",
        className,
      )}
    >
      <span className="min-w-0 truncate">{children}</span>
      {trailing != null && (
        <span className="ml-auto flex shrink-0 items-center gap-1.5 normal-case tracking-normal">
          {trailing}
        </span>
      )}
    </div>
  );
}
