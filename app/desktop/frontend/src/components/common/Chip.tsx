import type { ReactNode } from "react";
import { Icon, type IconName } from "./Icon";

type Props = {
  icon?: IconName;
  children: ReactNode;
  /** Native tooltip — useful when the chip's text is truncated. */
  title?: string;
  onClose?: () => void;
};

// A compact rounded label used for composer attachments, file refs, etc.
//
// The close affordance (×) is hidden until the chip is hovered/focused — keeps
// a row of chips quiet visually, only surfacing the controls when the user
// reaches for them.
export function Chip({ icon, children, title, onClose }: Props) {
  return (
    <span className="composer-chip" title={title}>
      {icon && <Icon name={icon} size={11} />}
      <span className="chip-label">{children}</span>
      {onClose && (
        <span
          className="x"
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
