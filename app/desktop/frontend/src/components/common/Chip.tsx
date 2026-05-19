import type { ReactNode } from "react";
import { Icon, type IconName } from "./Icon";

type Props = {
  icon?: IconName;
  children: ReactNode;
  onClose?: () => void;
};

// A compact rounded label used for composer attachments, file refs, etc.
export function Chip({ icon, children, onClose }: Props) {
  return (
    <span className="composer-chip">
      {icon && <Icon name={icon} size={11} />}
      {children}
      {onClose && (
        <span className="x" onClick={onClose} role="button" aria-label="Remove">
          <Icon name="x" size={10} />
        </span>
      )}
    </span>
  );
}
