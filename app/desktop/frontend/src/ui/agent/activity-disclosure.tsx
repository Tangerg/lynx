import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { Children, useId } from "react";
import type { ActivityShell } from "@/lib/activityShell";
import { cn } from "@/lib/classNames";
import { Collapsible } from "@/ui/atoms/collapsible";
import { Pressable } from "@/ui/atoms/pressable";
import { ProgressBar } from "@/ui/atoms/progress-bar";
import { Icon, type IconName } from "@/ui/icons";

type ActivityTone = "neutral" | "warning" | "negative";

// What each shell is FOR is in @/lib/activityShell; what it is MADE OF is here.
//
//   line     No fill, no card, quieter glyph.
//   card     Fill and the card corner.
//   flagged  A card plus the tone's edge for "this is waiting on you".
//            Deliberately NOT a filled tone band
//            across the header, which would duplicate the status edge.

type ActivityLeading = { icon: IconName; leading?: never } | { icon?: never; leading: ReactNode };

// The gutter, and the column the row's text starts on — one entry, so the two cannot
// drift apart.
//
// ONE slot, for every row. A mark identifies the row; it is not a place to put content.
// Anything beyond "what kind of row am I" belongs in the label or trailing slot;
// varying the mark width would move labels out of alignment between rows.
// Codex puts the identity mark first and the disclosure chevron after the summary.
// The body therefore returns to the reading edge; a material preview owns its own
// inset and reasoning contributes its own aside rule.
const GUTTER = { cardSlot: "w-5" } as const;

type AgentActivityDisclosureProps = Omit<ComponentPropsWithoutRef<"div">, "children"> &
  ActivityLeading & {
    label: ReactNode;
    detail?: ReactNode;
    trailing?: ReactNode;
    actions?: ReactNode;
    open: boolean;
    onToggle: () => void;
    /** Keep the summary row visible while its own body scrolls past. For a
     *  disclosure that can hold more rows than fit — a tool group — where losing
     *  the header means losing what the rows below it belong to. */
    stickyHeader?: boolean;
    /** How far along the thing this row stands for is, drawn as a hairline across
     *  the row's full width rather than as a bar in the meta column.
     *
     *  Both references spend the whole width on it and no content columns: at that
     *  length "how much is left" is read from the fill's edge without reading a
     *  number, which is the one thing a standing bar has to do while being scrolled
     *  past. A 40px bar wedged between the label and the count could not — it was
     *  short enough that its own two states looked alike. `label` names it for
     *  assistive tech, which a bare percentage does not (see ProgressBar). */
    progress?: { value: number; label: string };
    toggleLabel?: string;
    tone?: ActivityTone;
    shell?: ActivityShell;
    children: ReactNode;
    contentClassName?: string;
  };

const TONE_CLASS: Record<ActivityTone, string> = {
  neutral: "text-fg-muted",
  warning: "text-warning",
  negative: "text-negative",
};

// A flagged row's edge. Neutral cannot reach here through any caller — a row with
// nothing to flag is a card — but the map is total so the type stays honest.
const FLAG_EDGE_CLASS: Record<ActivityTone, string> = {
  neutral: "border-field",
  warning: "border-warning-edge",
  negative: "border-negative-edge",
};

// The tint behind a framed glyph — the row's own tone at chip strength, so a
// failed row is legible before its label is read.
const TRAY_CLASS: Record<ActivityTone, string> = {
  neutral: "bg-surface-2",
  warning: "bg-warning-badge",
  negative: "bg-negative-badge",
};

/**
 * One activity disclosure for the Agent Narrative.
 *
 * Tool calls, reasoning, delegated Runs and plan progress share this grammar: a
 * summary row, a leading mark, optional sibling actions, and a disclosed body.
 * What differs between them is now DECLARED — `shell` for how much plane the row
 * claims, `tone` for what state it is in — so the differences live in one table
 * here rather than in five components' class strings.
 *
 * Domain state and commands stay with the feature components; this owns geometry,
 * interaction chrome and disclosure accessibility.
 */
export function AgentActivityDisclosure({
  icon,
  leading,
  label,
  detail,
  trailing,
  actions,
  open,
  onToggle,
  stickyHeader,
  progress,
  toggleLabel,
  tone = "neutral",
  shell = "card",
  children,
  className,
  contentClassName,
  ...props
}: AgentActivityDisclosureProps) {
  const triggerId = useId();
  const panelId = useId();
  // A caller handing over its own `leading` owns that whole box — a plan's step
  // mark in a tray would be a mark inside a mark.
  const framed = shell !== "line" && icon !== undefined;

  return (
    <div
      {...props}
      data-slot="agent-activity-disclosure"
      data-tone={tone}
      data-shell={shell}
      className={cn(
        // No outer margin. What distance this row keeps from the one above it
        // depends on what that one WAS, which is a fact only the renderer walking
        // the sequence has (see renderUnitRhythm).
        // `clip`, not `hidden`. Both cut the same pixels, but `hidden` makes this
        // box a scroll container — which makes it the scrollport a `sticky`
        // descendant positions against, and a sticky header inside a box that
        // never scrolls simply does not stick. `clip` is not a scroll container,
        // so the corner is still clipped and `stickyHeader` below still works.
        "min-w-0 overflow-clip",
        shell === "line"
          ? // A radius even with no fill: the focus ring still needs a shape.
            "rounded-[var(--shape-sm)]"
          : "rounded-[var(--surface-card-radius)] bg-card",
        shell === "flagged" && `border ${FLAG_EDGE_CLASS[tone]}`,
        className,
      )}
    >
      <div
        className={cn(
          "group/activity-header flex min-w-0 items-center",
          // Opt-in, and deliberately not the default: a transcript where every
          // activity row stuck would pile a dozen headers at the top of the
          // reading column, each hiding the rows of the one above it. Only a
          // disclosure long enough to scroll past its own header wants this.
          //
          // The fill follows the SHELL, because a stuck header has to hide the
          // rows travelling under it and the shell decides what ground it sits
          // on: a `line` has no fill of its own, so it borrows the column's.
          // Hardcoding the column's ground would have put canvas over card the
          // first time a card shell asked for this.
          stickyHeader && ["sticky top-0 z-1", shell === "line" ? "bg-canvas" : "bg-card"],
        )}
      >
        <Pressable
          id={triggerId}
          type="button"
          aria-expanded={open}
          aria-controls={panelId}
          aria-label={toggleLabel}
          onClick={onToggle}
          className={cn(
            "flex min-w-0 flex-1 items-center text-left",
            shell === "line" ? "gap-1.5 py-0.5 pr-0" : "gap-3 py-1.5 pr-3",
            // A card insets its content from its own edge; a line has no edge, so it
            // starts on the column.
            shell === "line" ? "pl-0" : "pl-3",
            // The work narrative changes ink on hover; it does not paint a row
            // behind every invocation. Material belongs to the disclosed result.
            shell !== "line" && "transition-colors duration-[var(--dur-color)] hover:bg-hover",
            shell === "line" ? "min-h-5" : "min-h-8",
          )}
        >
          <span
            aria-hidden
            data-slot="agent-activity-mark"
            className={cn(
              "grid shrink-0 place-items-center",
              shell === "line" ? "h-4 w-4" : GUTTER.cardSlot,
              // A framed icon gets the tray; a bare one gets a 16px box. But that box
              // is the shell's answer for ONE mark — a caller handing over its own
              // `leading` owns the size as well, and a folded wave's mark is a strip of
              // glyphs that the fixed box cropped to the first one and a half.
              framed ? `h-5 rounded-[var(--shape-sm)] ${TRAY_CLASS[tone]}` : "h-4",
              // A quiet row's glyph is quiet too: at full ink, a column of reads
              // pulls the eye as hard as the one command that changed something.
              shell === "line" && tone === "neutral" ? "text-fg-faint" : TONE_CLASS[tone],
            )}
          >
            {/* A line keeps one bare identity mark; a card frames it. The same tool
                therefore keeps its shape when model state changes its shell, while
                tone remains responsible only for status. */}
            {leading ?? (icon ? <Icon name={icon} size="xs" /> : null)}
          </span>
          {/* `truncate` needs the box to be allowed to shrink. Pinned at
              `shrink-0` it kept its full intrinsic width instead, so a long label
              — a whole shell command, say — ran past the card's rounded corner and
              was cut mid-glyph with no ellipsis. Shrinking is proportional to base
              width, so a short verb still holds its ground against a long detail. */}
          <span
            data-slot="agent-activity-label"
            className="flex min-w-0 shrink items-center overflow-hidden text-ellipsis whitespace-nowrap text-ui-sm text-fg-muted group-hover/activity-header:text-fg"
          >
            {label}
          </span>
          {detail != null && (
            // The thing acted on — a path, a pattern, a preview line. It takes the
            // remaining width because it is what the eye is scanning for past a
            // column of identical verbs, and it stays one ink ABOVE the verb for the
            // same reason. But one ink below the answer: at full `text-fg` it was
            // darker than the prose it precedes, so a tool's file path outranked the
            // sentence the reader actually came for.
            <span className="flex min-w-0 flex-1 items-center overflow-hidden text-ellipsis whitespace-nowrap text-ui-sm leading-snug text-fg-muted group-hover/activity-header:text-fg">
              {detail}
            </span>
          )}
          {trailing != null && (
            <span className="flex shrink-0 items-center gap-1.5 font-mono text-ui-2xs text-fg-faint tabular-nums">
              {trailing}
            </span>
          )}
          {/* Codex keeps disclosure subordinate to the summary: it trails the
              text, appears on hover/focus, and remains visible while open. */}
          <span
            aria-hidden
            data-slot="agent-activity-chevron"
            className={cn(
              "flex shrink-0 text-fg-faint transition-[transform,opacity] duration-[var(--dur-fast)] group-focus-within/activity-header:opacity-100 group-hover/activity-header:opacity-100",
              open ? "opacity-100" : "-rotate-90 opacity-0",
            )}
          >
            <Icon name="chevron-down" size="xs" />
          </span>
        </Pressable>
        {/* The row's content is inset from the card edge by the trigger's own
            padding; the action rail sits OUTSIDE that trigger, so without its own
            it butted straight against the rounded corner and the card's
            `overflow-hidden` sliced the button in half. `Children.count` rather
            than a null check because an empty list is a perfectly ordinary thing
            for a caller to hand over and is not the same as "no rail" — rendered
            anyway it left a 2px stub at the edge of every settled row. */}
        {Children.count(actions) > 0 && (
          <div className="flex shrink-0 items-center gap-0.5 pl-0.5 pr-2">{actions}</div>
        )}
      </div>
      {/* Between the summary and the body, corner to corner: the card clips it, so
          on a closed row it reads as the card's own bottom edge and on an open one
          as the seam between what the row says and what it holds. Not `rounded-pill`
          — a 2px capsule inset from two rounded corners is three radii arguing. */}
      {progress && (
        <ProgressBar
          value={progress.value}
          label={progress.label}
          className="h-0.5 rounded-none"
          indicatorClassName="rounded-none"
        />
      )}
      <Collapsible open={open}>
        <div
          id={panelId}
          role="region"
          aria-labelledby={triggerId}
          className={cn(
            // No fill on the body, either shell. The card is the ground; previews
            // of program output or JSON provide the nested depth themselves.
            // A narrative line and its body share the reading edge. The expanded
            // shell/diff is the material child and owns its own inset; adding a
            // second 44px gutter here made every real result look nested twice.
            shell === "line" ? "pt-1.5 pb-1.5 pr-0" : "px-3 pb-2.5",
            contentClassName,
          )}
        >
          {children}
        </div>
      </Collapsible>
    </div>
  );
}
