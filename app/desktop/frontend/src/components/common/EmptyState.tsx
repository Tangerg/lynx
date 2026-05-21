// EmptyState — used wherever a query returned no rows OR the surface has
// no contribution yet (Notifications when zero, Files when no diff is
// active, etc.). Single primitive so every empty surface reads the same.
//
// Intentionally minimal: icon + title + optional sub + optional action.
// Layout is centred vertically; caller decides how much height to give it.

import type { CSSProperties, ReactNode } from "react";
import { Icon, type IconName } from "./Icon";

type Props = {
  icon?: IconName;
  title: string;
  /** Secondary line — usually a short phrase explaining the empty state. */
  sub?: string;
  /** Optional CTA (button, link). Rendered below the sub text. */
  action?: ReactNode;
  /** Tighter / more breathing room. Defaults to "comfortable". */
  size?: "compact" | "comfortable";
  style?: CSSProperties;
};

export function EmptyState({ icon, title, sub, action, size = "comfortable", style }: Props) {
  return (
    <div className={`empty-state empty-${size}`} style={style}>
      {icon && (
        <div className="empty-icon">
          <Icon name={icon} size={size === "compact" ? 16 : 22} />
        </div>
      )}
      <div className="empty-title">{title}</div>
      {sub && <div className="empty-sub">{sub}</div>}
      {action && <div className="empty-action">{action}</div>}
    </div>
  );
}
