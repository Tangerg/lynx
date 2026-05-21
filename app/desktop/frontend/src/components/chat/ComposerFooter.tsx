// Composer footer — renders the row of small chips below the composer
// (working directory, execution mode, git branch, etc.).
//
// Chips come from the plugin registry (`host.composer.registerStatus`);
// the footer is a pure container — no per-chip props.

import { PluginBoundary } from "@/plugins/PluginBoundary";
import { useComposerStatus } from "@/plugins/sdk";

export function ComposerFooter() {
  const items = useComposerStatus();
  if (items.length === 0) return null;

  return (
    <div className="composer-footer">
      {items.map((it) => {
        const Body = it.component;
        return (
          <PluginBoundary key={it.id} plugin={`composer-status:${it.id}`} label={`${it.id} chip`}>
            <Body />
          </PluginBoundary>
        );
      })}
    </div>
  );
}
