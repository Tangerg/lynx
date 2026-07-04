import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

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
