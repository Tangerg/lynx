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
 * actions, and an indented detail rail. Domain state and commands stay with
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
      className={cn("group/activity my-1 min-w-0", className)}
    >
      <div className="flex min-h-7 min-w-0 items-center">
        <Pressable
          id={triggerId}
          type="button"
          aria-expanded={open}
          aria-controls={panelId}
          aria-label={toggleLabel}
          onClick={onToggle}
          className="flex min-h-7 min-w-0 flex-1 items-center gap-1.5 rounded-md px-2 py-1 text-left transition-colors duration-[var(--dur-fast)] hover:bg-hover"
        >
          <span
            aria-hidden
            className={cn("grid size-4 shrink-0 place-items-center", TONE_CLASS[tone])}
          >
            {leading ?? (icon ? <Icon name={icon} size={13} /> : null)}
          </span>
          <span className="min-w-0 truncate text-ui-md font-medium text-fg">{label}</span>
          {detail != null && (
            <span className="min-w-0 truncate text-ui-sm leading-snug text-fg-muted">{detail}</span>
          )}
          <span className="min-w-1 flex-1" />
          {trailing != null && (
            <span className="flex shrink-0 items-center gap-1.5 text-ui-sm text-fg-muted">
              {trailing}
            </span>
          )}
          <Icon
            name="chevron-down"
            size={12}
            className={cn(
              "shrink-0 text-fg-faint transition-transform duration-[var(--dur-fast)]",
              !open && "-rotate-90",
            )}
          />
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
          className={cn("ml-4 border-l border-field py-1 pl-3", contentClassName)}
        >
          {children}
        </div>
      </Collapsible>
    </div>
  );
}
