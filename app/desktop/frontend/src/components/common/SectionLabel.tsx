import type { CSSProperties, ReactNode } from "react";

interface Props {
  children: ReactNode;
  trailing?: ReactNode;
  style?: CSSProperties;
}

// Mono-eyebrow heading used for sidebar sections + workspace-view
// gutters. Keeps the "label voice" consistent across the app.
export function SectionLabel({ children, trailing, style }: Props) {
  return (
    <div
      className="flex items-center gap-2 px-3 pb-1.5 pt-3.5 font-mono text-[11px] font-semibold text-fg-faint"
      style={style}
    >
      <span>{children}</span>
      {trailing}
    </div>
  );
}
