import type { ReactNode } from "react";
import type { IconName } from "./Icon";
import { Icon } from "./Icon";
import { Tooltip } from "./Tooltip";

interface Props {
  icon?: IconName;
  children: ReactNode;
  /** Tooltip label shown on hover — useful when the chip's text is
   *  truncated. Renders via Radix Tooltip rather than the native title
   *  attribute (200ms snappier, works on focus). */
  title?: string;
  onClose?: () => void;
}

// A compact rounded label used for composer attachments, file refs, etc.
//
// The close affordance (×) is hidden until the chip is hovered/focused —
// keeps a row of chips quiet visually, only surfacing the controls when
// the user reaches for them. Tailwind `group` enables that hover-reveal.
export function Chip({ icon, children, title, onClose }: Props) {
  return (
    <Tooltip label={title}>
      <span className="group inline-flex items-center gap-1.5 rounded-full bg-surface-2 px-2 py-0.5 pl-2 pr-1 text-[11px] text-fg-muted font-mono">
        {icon && <Icon name={icon} size={11} />}
        <span className="max-w-[220px] truncate">{children}</span>
        {onClose && (
          <button
            type="button"
            className="grid h-5 w-5 place-items-center rounded-full border-0 bg-transparent text-fg-faint opacity-0 scale-90 transition-all group-hover:opacity-100 group-hover:scale-100 group-focus-within:opacity-100 hover:bg-line-soft hover:text-fg active:scale-90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent"
            onClick={onClose}
            aria-label="Remove"
          >
            <Icon name="x" size={10} />
          </button>
        )}
      </span>
    </Tooltip>
  );
}
