import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

// Shared two-column row for Appearance settings: a 140px title/subtitle
// gutter on the left, the control on the right. `align` defaults to
// centered (single-line controls); pass "start" for multi-line bodies
// so the title pins to the top.
export function SettingRow({
  label,
  sub,
  align = "center",
  children,
}: {
  label: string;
  sub: string;
  align?: "start" | "center";
  children: ReactNode;
}) {
  return (
    <div
      className={cn(
        "grid grid-cols-[180px_1fr] gap-5 px-5 py-4",
        align === "start" ? "items-start" : "items-center",
      )}
    >
      <div>
        <div className="text-[14px] text-fg">{label}</div>
        <div className="mt-1 text-[12px] leading-[1.45] text-fg-muted">{sub}</div>
      </div>
      <div>{children}</div>
    </div>
  );
}
