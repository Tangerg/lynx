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
        "grid grid-cols-[140px_1fr] gap-4 py-3",
        align === "start" ? "items-start" : "items-center",
      )}
    >
      <div>
        <div className="text-[16px] font-semibold text-fg">{label}</div>
        <div className="mt-0.5 text-[13px] text-fg-muted">{sub}</div>
      </div>
      <div>{children}</div>
    </div>
  );
}
