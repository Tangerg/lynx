// Shared action button for the stream-top banners (RunErrorBanner /
// CwdMissingBanner). `primary` renders the banner-tone-tinted emphasis
// variant; the secondary shape is neutral chrome.

import type { IconName } from "@/ui";
import { Button, Icon } from "@/ui";

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
    <Button
      size="xs"
      variant={primary ? "tonal" : "soft"}
      tone={primary ? tone : undefined}
      onClick={onClick}
      disabled={disabled}
    >
      {icon && <Icon name={icon} size="xs" />}
      <span>{label}</span>
    </Button>
  );
}
