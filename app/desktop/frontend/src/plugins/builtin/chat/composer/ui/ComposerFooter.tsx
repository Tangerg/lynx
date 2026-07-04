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

  return (
    <div
      className="mt-1 flex flex-wrap items-center gap-x-1.5 gap-y-1 pt-0.5"
      data-slot="composer-chips"
    >
      {items.map((it, i) => (
        <span key={it.id} className="inline-flex items-center">
          {i > 0 && (
            <span className="mr-1.5 text-fg-faint/35 select-none" aria-hidden>
              ·
            </span>
          )}
          <Chip it={it} />
        </span>
      ))}
    </div>
  );
}
