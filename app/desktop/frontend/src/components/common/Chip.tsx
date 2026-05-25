import type { ReactNode } from "react";
import type {IconName} from "./Icon";
import { Icon  } from "./Icon";

interface Props {
  icon?: IconName;
  children: ReactNode;
  /** Native tooltip — useful when the chip's text is truncated. */
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
    <span
      className="group inline-flex items-center gap-1.5 rounded-full bg-surface-2 px-2 py-0.5 pl-2 pr-1 text-[11px] text-fg-muted font-mono"
      title={title}
    >
      {icon && <Icon name={icon} size={11} />}
      <span className="max-w-[220px] truncate">{children}</span>
      {onClose && (
        <span
          className="grid h-5 w-5 place-items-center rounded-full text-fg-faint opacity-0 scale-90 cursor-pointer transition-all group-hover:opacity-100 group-hover:scale-100 group-focus-within:opacity-100 hover:bg-line-soft hover:text-fg active:scale-90"
          onClick={onClose}
          role="button"
          aria-label="Remove"
          tabIndex={0}
        >
          <Icon name="x" size={10} />
        </span>
      )}
    </span>
  );
}
