// Shared action button for the stream-top banners (RunErrorBanner /
// CwdMissingBanner). `primary` renders the banner-tone-tinted emphasis
// variant; the secondary shape is neutral chrome. `focus-visible` (not
// `focus`) so mouse clicks don't trigger the keyboard ring.

import type { IconName } from "@/ui";
import { Icon } from "@/ui";
import { cn } from "@/lib/utils";

const FOCUS_RING =
  "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent";

const PRIMARY_TONE: Record<"negative" | "warning", string> = {
  negative: "border-negative/40 bg-negative/15 text-negative hover:bg-negative/25",
  warning: "border-warning/40 bg-warning/15 text-warning hover:bg-warning/25",
};

export function BannerAction({
  icon,
  label,
  onClick,
  primary,
  tone = "negative",
  disabled,
}: {
  icon?: IconName;
  label: string;
  onClick: () => void;
  primary?: boolean;
  /** The owning banner's severity — tints the primary variant. */
  tone?: "negative" | "warning";
  /** Inert + dimmed (e.g. a retry still counting down its backoff). */
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={cn(
        "inline-flex h-6 items-center gap-1 rounded-md border-[0.5px] px-2 font-sans text-[11.5px] transition-colors",
        primary
          ? cn("font-semibold", PRIMARY_TONE[tone])
          : "border-field bg-transparent text-fg-muted hover:bg-surface-2 hover:text-fg",
        "disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent",
        FOCUS_RING,
      )}
    >
      {icon && <Icon name={icon} size={11} />}
      <span>{label}</span>
    </button>
  );
}
