import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";
import { Surface } from "@/ui";

/**
 * A settings form section with one outer edge.
 *
 * Child `SettingRow`s own only the separators between siblings. Keeping the
 * group edge here gives Appearance, Personalization, Connection, and plugin
 * panes the same form grammar without copying border/fill decisions.
 */
export function SettingsGroup({ className, children, ...props }: ComponentPropsWithoutRef<"div">) {
  return (
    <Surface
      {...props}
      inset="none"
      className={cn(
        "overflow-hidden border-[length:var(--control-edge-width)] border-field bg-transparent",
        className,
      )}
    >
      {children}
    </Surface>
  );
}
