import type { CSSProperties } from "react";
import type { VisualStyleSpec } from "@/plugins/sdk";
import { VISUAL_STYLE, useExtensionPoint } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { Icon, Pressable } from "@/ui";
import { SettingRow } from "../../public";
import { useVisualStylePreference } from "../application/appearancePreferences";

function StylePreview({ spec }: { spec: VisualStyleSpec }) {
  const floating = spec.traits.regions === "floating-card";
  const strong = spec.traits.regions === "tool-windows";
  const radius = spec.tokens["style-shape-lg"];
  const shellStyle = {
    background: spec.preview.sidebar,
    borderColor: spec.preview.edge,
    borderRadius: spec.traits.regions === "flush-panes" ? 2 : radius,
  } satisfies CSSProperties;

  return (
    <span
      aria-hidden
      className="relative flex h-14 w-full overflow-hidden border"
      style={shellStyle}
    >
      <span className="w-[23%] shrink-0" style={{ background: spec.preview.sidebar }}>
        <span className="mx-auto mt-2 block h-1 w-3/5 rounded-full bg-current opacity-15" />
        <span className="mx-auto mt-1.5 block h-1 w-2/5 rounded-full bg-current opacity-10" />
      </span>
      <span
        className={cn("relative flex min-w-0 flex-1 overflow-hidden", floating && "my-1 mr-1")}
        style={{
          background: spec.preview.canvas,
          borderRadius: floating ? radius : 0,
          boxShadow: floating ? `0 3px 10px ${spec.preview.edge}` : undefined,
          borderLeft: floating ? undefined : `1px solid ${spec.preview.edge}`,
        }}
      >
        <span className="flex min-w-0 flex-1 flex-col">
          <span
            className="h-3.5 shrink-0 border-b"
            style={{
              borderColor: spec.preview.edge,
              background: strong ? spec.preview.sidebar : undefined,
            }}
          />
          <span className="mx-auto mt-2 block h-1 w-1/2 rounded-full bg-current opacity-12" />
          <span className="mx-auto mt-1.5 block h-1 w-2/3 rounded-full bg-current opacity-8" />
        </span>
        <span
          className="w-[31%] shrink-0 border-l"
          style={{ background: spec.preview.dock, borderColor: spec.preview.edge }}
        >
          <span
            className="mx-auto mt-2 block h-1.5 w-1.5 rounded-full"
            style={{ background: spec.preview.accent }}
          />
        </span>
      </span>
    </span>
  );
}

export function VisualStyleSection() {
  const t = useT();
  const styles = useExtensionPoint(VISUAL_STYLE);
  const { visualStyle, setVisualStyle } = useVisualStylePreference();
  const resolvedStyle = styles.some((spec) => spec.id === visualStyle) ? visualStyle : "synara";

  return (
    <SettingRow label={t("settings.visualStyle")} sub={t("settings.visualStyle.sub")} align="start">
      <div className="grid grid-cols-2 gap-2">
        {styles.map((spec) => {
          const active = resolvedStyle === spec.id;
          return (
            <Pressable
              key={spec.id}
              aria-pressed={active}
              onClick={() => setVisualStyle(spec.id)}
              className={cn(
                "group min-w-0 rounded-[var(--surface-card-radius)] border-[var(--control-edge-width)] p-2",
                "transition-[background-color,border-color,scale] duration-[var(--dur-fast)] ease-[var(--ease-out)]",
                "hover:bg-hover active:scale-[var(--press-scale)]",
                active ? "border-accent bg-accent-wash" : "border-field bg-transparent",
              )}
            >
              <StylePreview spec={spec} />
              <span className="mt-2 flex min-w-0 items-center gap-1.5">
                <span className="min-w-0 flex-1 truncate text-ui-md font-medium text-fg">
                  {spec.label}
                </span>
                {active && <Icon name="check" size={12} className="shrink-0 text-accent" />}
              </span>
            </Pressable>
          );
        })}
      </div>
    </SettingRow>
  );
}
