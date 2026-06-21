import type { CSSProperties, ReactNode } from "react";

interface Props {
  children: ReactNode;
  trailing?: ReactNode;
  style?: CSSProperties;
}

// Section heading used for sidebar sections + workspace-view gutters.
// Sentence-case sans (the redesign pulls mono back to code / IDs only),
// keeping the "label voice" consistent across the app.
export function SectionLabel({ children, trailing, style }: Props) {
  return (
    <div
      className="flex items-center gap-2 px-3 pb-1.5 pt-3.5 font-sans text-[11px] font-semibold text-fg-faint"
      style={style}
    >
      <span>{children}</span>
      {trailing}
    </div>
  );
}
