// EmptyState — universal "nothing here yet" surface. Vertically
// centred; sizes via `size` prop ("compact" / "comfortable").

import type { VariantProps } from "class-variance-authority";
import type { CSSProperties, ReactNode } from "react";
import type { IconName } from "./Icon";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/utils";
import { Icon } from "./Icon";

const root = cva(
  "flex flex-col items-center justify-center text-center text-fg-faint select-none",
  {
    variants: {
      size: {
        compact: "gap-1.5 px-4 py-6",
        comfortable: "gap-2.5 px-5 py-12",
      },
    },
    defaultVariants: { size: "comfortable" },
  },
);

const iconWrap = cva("grid place-items-center rounded-full bg-surface-2 text-fg-muted", {
  variants: {
    size: {
      compact: "h-7 w-7",
      comfortable: "h-10 w-10",
    },
  },
  defaultVariants: { size: "comfortable" },
});

type Props = VariantProps<typeof root> & {
  icon?: IconName;
  title: string;
  /** Secondary line — usually a short phrase explaining the empty state. */
  sub?: string;
  /** Optional CTA (button, link). Rendered below the sub text. */
  action?: ReactNode;
  style?: CSSProperties;
};

export function EmptyState({ icon, title, sub, action, size, style }: Props) {
  return (
    <div className={root({ size })} style={style}>
      {icon && (
        <div className={iconWrap({ size })}>
          <Icon name={icon} size={size === "compact" ? 16 : 22} />
        </div>
      )}
      <div
        className={cn(
          "font-semibold tracking-tight text-fg-soft",
          size === "compact" ? "text-xs" : "text-[13px]",
        )}
      >
        {title}
      </div>
      {sub && <div className="max-w-[280px] text-[11.5px] leading-[1.5] text-fg-faint">{sub}</div>}
      {action && <div className="mt-1.5">{action}</div>}
    </div>
  );
}
