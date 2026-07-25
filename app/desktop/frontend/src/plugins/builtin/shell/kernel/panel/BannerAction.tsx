// Shared action button for the stream-top banners (RunErrorBanner /
// CwdMissingBanner). `primary` renders the banner-tone-tinted emphasis
// variant; the secondary shape is neutral chrome.

import type { IconName } from "@/ui";
import { Icon } from "@/ui";
import { cn } from "@/lib/utils";

// Rests at the tint, lifts to the chip weight on hover — the same two steps
// every other tonal surface uses, rather than this button's own third alpha.
const PRIMARY_TONE: Record<"negative" | "warning", string> = {
  negative: "bg-negative-wash text-negative hover:bg-negative-badge",
  warning: "bg-warning-wash text-warning hover:bg-warning-badge",
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
        "inline-flex h-6 items-center gap-1 rounded-md px-2 font-sans text-ui-sm transition-colors",
        primary
          ? cn("font-semibold", PRIMARY_TONE[tone])
          : "bg-canvas text-fg-soft hover:bg-surface-2 hover:text-fg",
        "disabled:cursor-not-allowed disabled:opacity-50",
        "focus-ring",
      )}
    >
      {icon && <Icon name={icon} size={11} />}
      <span>{label}</span>
    </button>
  );
}
