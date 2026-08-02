import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { useId } from "react";
import { cn } from "@/lib/classNames";
import { Collapsible } from "@/ui/atoms/collapsible";
import { Pressable } from "@/ui/atoms/pressable";
import { Icon, type IconName } from "@/ui/icons";

type ActivityTone = "neutral" | "warning" | "negative";

type ActivityLeading = { icon: IconName; leading?: never } | { icon?: never; leading: ReactNode };

type AgentActivityDisclosureProps = Omit<ComponentPropsWithoutRef<"div">, "children"> &
  ActivityLeading & {
    label: ReactNode;
    detail?: ReactNode;
    trailing?: ReactNode;
    actions?: ReactNode;
    open: boolean;
    onToggle: () => void;
    toggleLabel?: string;
    tone?: ActivityTone;
    children: ReactNode;
    contentClassName?: string;
  };

const TONE_CLASS: Record<ActivityTone, string> = {
  neutral: "text-fg-muted",
  warning: "text-warning",
  negative: "text-negative",
};

/**
 * One compact activity disclosure for the Agent Narrative.
 *
 * Tool calls, reasoning and delegated Runs share this presentation grammar:
 * one quiet summary row, a status-sized leading glyph, optional sibling
 * actions, and an inset detail plane. Domain state and commands stay with
 * their feature components; this primitive owns only geometry, interaction
 * chrome and disclosure accessibility.
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
  toggleLabel,
  tone = "neutral",
  children,
  className,
  contentClassName,
  ...props
}: AgentActivityDisclosureProps) {
  const triggerId = useId();
  const panelId = useId();

  return (
    <div
      {...props}
      data-slot="agent-activity-disclosure"
      data-tone={tone}
      className={cn(
        "group/activity my-1 min-w-0 overflow-hidden rounded-[var(--surface-card-radius)] bg-card",
        className,
      )}
    >
      <div className="flex min-h-7 min-w-0 items-center">
        <Pressable
          id={triggerId}
          type="button"
          aria-expanded={open}
          aria-controls={panelId}
          aria-label={toggleLabel}
          onClick={onToggle}
          className="flex min-h-8 min-w-0 flex-1 items-center gap-2 px-3 py-1.5 text-left transition-colors duration-[var(--dur-fast)] hover:bg-hover"
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
            className={cn("grid size-4 shrink-0 place-items-center", TONE_CLASS[tone])}
          >
            {leading ?? (icon ? <Icon name={icon} size="sm" /> : null)}
          </span>
          <span className="shrink-0 truncate text-ui-sm font-medium text-fg-muted">{label}</span>
          {detail != null && (
            // The thing acted on — a path, a pattern, a preview line. It takes the
            // remaining width because it is what the eye is scanning for past a
            // column of identical verbs. Which VOICE it speaks in stays with the
            // caller: a path is data and sets itself in mono, a sentence is not.
            <span className="min-w-0 flex-1 truncate text-ui-xs leading-snug text-fg">
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
        {actions != null && (
          <div className="flex shrink-0 items-center gap-0.5 pl-0.5">{actions}</div>
        )}
      </div>
      <Collapsible open={open}>
        <div
          id={panelId}
          role="region"
          aria-labelledby={triggerId}
          className={cn("bg-sunken px-3 py-2.5", contentClassName)}
        >
          {children}
        </div>
      </Collapsible>
    </div>
  );
}
