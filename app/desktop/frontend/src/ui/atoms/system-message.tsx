// SystemMessage — inline notice banner (info / warning / error / success).
// Tinted fill + matching foreground drawn from the semantic tokens; no border
// or inset ring, per the "no cheap lines" rule. Text and the leading icon share
// the variant color (Icon strokes with currentColor).

import type { VariantProps } from "class-variance-authority";
import type { ComponentProps, ReactNode } from "react";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { Icon, type IconName } from "@/ui/icons";
import { Button, type ButtonProps } from "./button";

const banner = cva("flex flex-row items-center gap-3 rounded-lg px-3 py-2", {
  variants: {
    variant: {
      info: "bg-info-wash text-info",
      warning: "bg-warning-wash text-warning",
      error: "bg-negative-wash text-negative",
      success: "bg-success-wash text-success",
    },
  },
  defaultVariants: { variant: "info" },
});

// The icon set has no info-/error-circle, so these are the nearest available
// glyphs; a consumer can override via the `icon` prop.
const DEFAULT_ICON: Record<NonNullable<VariantProps<typeof banner>["variant"]>, IconName> = {
  info: "question",
  warning: "alert",
  error: "x",
  success: "check",
};

export type SystemMessageProps = ComponentProps<"div"> &
  VariantProps<typeof banner> & {
    /** Override the per-variant leading icon. */
    icon?: IconName;
    /** Drop the leading icon entirely. */
    hideIcon?: boolean;
    /** Trailing call-to-action rendered as a scope Button. */
    action?: { label: string; onClick?: () => void; variant?: ButtonProps["variant"] };
    children: ReactNode;
  };

export function SystemMessage({
  variant = "info",
  icon,
  hideIcon = false,
  action,
  className,
  children,
  ...props
}: SystemMessageProps) {
  const iconName = icon ?? DEFAULT_ICON[variant ?? "info"];
  const role = variant === "error" || variant === "warning" ? "alert" : "status";

  return (
    <div role={role} className={cn(banner({ variant }), className)} {...props}>
      <div className="flex min-w-0 flex-1 flex-row items-start gap-2.5 leading-normal">
        {!hideIcon && (
          // h-[1lh] pins the icon box to one line's height so it aligns to the
          // first line of text, not the block's center, when copy wraps.
          <span className="flex h-[1lh] shrink-0 items-center justify-center">
            <Icon name={iconName} size="md" />
          </span>
        )}
        <div className="min-w-0 flex-1 text-ui-md">{children}</div>
      </div>
      {action && (
        <Button variant={action.variant ?? "soft"} size="sm" onClick={action.onClick}>
          {action.label}
        </Button>
      )}
    </div>
  );
}
