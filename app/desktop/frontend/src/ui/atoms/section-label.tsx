import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";

/**
 * The heading that opens a section of a pane.
 *
 * Deliberately the quietest text in the app rather than the loudest: with regions
 * separated by tone instead of by lines, a label that competes with its own
 * content turns every section boundary into a second divider. Small, tracked-out
 * caps read as a label at a glance without ever being mistaken for a row — which
 * is what lets a pane hold three or four sections with no rules drawn between
 * them.
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
        "flex min-w-0 items-center gap-2 px-2 pb-1.5 pt-2 font-sans text-ui-2xs font-medium uppercase leading-none tracking-[0.06em] text-fg-faint",
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
