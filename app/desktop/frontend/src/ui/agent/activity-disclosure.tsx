import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { Children, useId } from "react";
import type { ActivityShell } from "@/lib/activityShell";
import { cn } from "@/lib/classNames";
import { Collapsible } from "@/ui/atoms/collapsible";
import { Pressable } from "@/ui/atoms/pressable";
import { Icon, type IconName } from "@/ui/icons";

type ActivityTone = "neutral" | "warning" | "negative";

// What each shell is FOR is in @/lib/activityShell; what it is MADE OF is here.
//
//   line     No fill, no card, quieter glyph.
//   card     Fill and the card corner.
//   flagged  A card plus the tone's edge — the answer HitlCardShell already gives
//            for "this is waiting on you". Deliberately NOT a filled tone band
//            across the header, which is what both references do: we would then
//            have three spellings of one boundary — that band, this edge, and
//            ApprovalCard's `bg-<tone>-wash` strip.

type ActivityLeading = { icon: IconName; leading?: never } | { icon?: never; leading: ReactNode };

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
        "group/activity min-w-0 overflow-clip",
        shell === "line"
          ? // A radius even with no fill: the hover wash needs a shape, and a
            // full-bleed rectangle sliding under the cursor is what makes a quiet
            // row read as a table cell.
            "rounded-[var(--shape-sm)]"
          : "rounded-[var(--surface-card-radius)] bg-card",
        shell === "flagged" && `border ${FLAG_EDGE_CLASS[tone]}`,
        className,
      )}
    >
      <div
        className={cn(
          "flex min-w-0 items-center",
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
          stickyHeader && "sticky top-0 z-1",
          stickyHeader && (shell === "line" ? "bg-canvas" : "bg-card"),
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
            "flex min-w-0 flex-1 items-center gap-2 px-3 py-1.5 text-left",
            "transition-colors duration-[var(--dur-color)] hover:bg-hover",
            shell === "line" ? "min-h-7" : "min-h-8",
          )}
        >
          {/* The disclosure arrow leads the row. It is the only control here, and
              a reader scanning a column of activity rows for the one to open
              should find every arrow on the same left edge instead of at the end
              of labels that all differ in length. */}
          <Icon
            name="chevron-down"
            size="xs"
            className={cn(
              "shrink-0 text-fg-faint transition-transform duration-[var(--dur-fast)]",
              !open && "-rotate-90",
            )}
          />
          <span
            aria-hidden
            className={cn(
              "grid shrink-0 place-items-center",
              // A framed icon gets the tray; a bare one gets a 16px box. But that box
              // is the shell's answer for ONE mark — a caller handing over its own
              // `leading` owns the size as well, and a folded wave's mark is a strip of
              // glyphs that the fixed box cropped to the first one and a half.
              framed
                ? `size-5 rounded-[var(--shape-sm)] ${TRAY_CLASS[tone]}`
                : leading
                  ? ""
                  : "size-4",
              // A quiet row's glyph is quiet too: at full ink, a column of reads
              // pulls the eye as hard as the one command that changed something.
              shell === "line" && tone === "neutral" ? "text-fg-faint" : TONE_CLASS[tone],
            )}
          >
            {leading ?? (icon ? <Icon name={icon} size={framed ? "xs" : "sm"} /> : null)}
          </span>
          {/* `truncate` needs the box to be allowed to shrink. Pinned at
              `shrink-0` it kept its full intrinsic width instead, so a long label
              — a whole shell command, say — ran past the card's rounded corner and
              was cut mid-glyph with no ellipsis. Shrinking is proportional to base
              width, so a short verb still holds its ground against a long detail. */}
          <span className="min-w-0 shrink truncate text-ui-sm font-medium text-fg-muted">
            {label}
          </span>
          {detail != null && (
            // The thing acted on — a path, a pattern, a preview line. It takes the
            // remaining width because it is what the eye is scanning for past a
            // column of identical verbs, and it stays one ink ABOVE the verb for the
            // same reason. But one ink below the answer: at full `text-fg` it was
            // darker than the prose it precedes, so a tool's file path outranked the
            // sentence the reader actually came for.
            <span className="min-w-0 flex-1 truncate text-ui-xs leading-snug text-fg-soft">
              {detail}
            </span>
          )}
          <span className="min-w-1 flex-1" />
          {trailing != null && (
            <span className="flex shrink-0 items-center gap-2 font-mono text-ui-2xs text-fg-faint tabular-nums">
              {trailing}
            </span>
          )}
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
      <Collapsible open={open}>
        <div
          id={panelId}
          role="region"
          aria-labelledby={triggerId}
          className={cn(
            // No fill on the body, either shell. It used to be `bg-sunken`, and
            // every preview that shows program output or JSON puts its own
            // `bg-sunken` well inside it — a well cut into a well, which reads
            // flat. The card is the ground; the wells inside it are the depth.
            shell === "line" ? "pb-1.5 pl-8 pr-1" : "px-3 pb-2.5",
            contentClassName,
          )}
        >
          {children}
        </div>
      </Collapsible>
    </div>
  );
}
