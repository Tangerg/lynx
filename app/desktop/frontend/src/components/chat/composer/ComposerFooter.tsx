// Composer footer — the row of small chips below the composer. Left side
// holds session context (working directory, execution mode, git branch);
// right side holds run telemetry (tokens / cost / run state) pushed there
// by `align: "end"`.
//
// Chips come from the plugin registry (the `lyra.composer.status` extension point);
// the footer is a pure container — no per-chip props.

import type { ComposerStatusSpec } from "@/plugins/sdk";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { COMPOSER_STATUS, useExtensionPoint } from "@/plugins/sdk";

function Chip({ it }: { it: ComposerStatusSpec }) {
  const Body = it.component;
  return (
    <PluginBoundary plugin={`composer-status:${it.id}`} label={`${it.id} chip`}>
      <Body />
    </PluginBoundary>
  );
}

export function ComposerFooter() {
  const items = useExtensionPoint(COMPOSER_STATUS);
  if (items.length === 0) return null;

  const start = items.filter((it) => it.align !== "end");
  const end = items.filter((it) => it.align === "end");

  return (
    <div className="relative z-[3] flex items-center gap-2 px-1 pb-0.5 pt-1.5">
      <div className="flex min-w-0 flex-wrap items-center gap-1">
        {start.map((it) => (
          <Chip key={it.id} it={it} />
        ))}
      </div>
      {end.length > 0 && (
        <div className="ml-auto flex shrink-0 items-center gap-2.5 text-[11px] text-fg-muted tabular-nums">
          {end.map((it) => (
            <Chip key={it.id} it={it} />
          ))}
        </div>
      )}
    </div>
  );
}
