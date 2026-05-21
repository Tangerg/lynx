import type { CSSProperties, ReactNode } from "react";

type Props = {
  children: ReactNode;
  trailing?: ReactNode;
  style?: CSSProperties;
};

// Uppercase + wide-tracking heading used for sidebar sections and workspace-view
// gutters. Keeps the "label voice" consistent across the app.
export function SectionLabel({ children, trailing, style }: Props) {
  return (
    <div className="side-section-head" style={style}>
      <span>{children}</span>
      {trailing}
    </div>
  );
}
