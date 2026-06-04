// Built-in plugin: "Providers" settings pane. Lists the LLM providers the
// runtime has configured (providers.list) with their brand icon, type,
// base URL, and masked API key. Read-only for now — providers.configure /
// providers.test aren't served by the runtime yet (capability_not_negotiated),
// so the edit / test affordances land once those methods exist.

import { DataView, ProviderIcon } from "@/components/common";
import { useProviders } from "@/lib/data/queries";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";

function ProvidersPane() {
  const { data, isLoading, isError } = useProviders();
  const providers = data ?? [];

  return (
    <div>
      <DataView
        items={providers}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{
          icon: "spark",
          title: "No providers",
          sub: "Configure an LLM provider so the agent has a model to run.",
        }}
      >
        {(rows) => (
          <div className="flex flex-col gap-2">
            {rows.map((p) => (
              <div
                key={p.id}
                className="grid grid-cols-[24px_minmax(0,1fr)_auto] items-center gap-3 rounded-lg border border-line-soft bg-canvas px-3 py-2.5"
              >
                <ProviderIcon provider={p.type} size={20} />
                <div className="min-w-0">
                  <div className="text-[14px] font-semibold capitalize text-fg">{p.type}</div>
                  <div className="truncate font-mono text-[12px] text-fg-faint">
                    {p.baseUrl || p.id}
                  </div>
                </div>
                <span
                  className={
                    p.apiKeyMasked
                      ? "font-mono text-[12px] text-fg-muted"
                      : "text-[12px] text-fg-faint"
                  }
                >
                  {p.apiKeyMasked ? `key ${p.apiKeyMasked}` : "no key"}
                </span>
              </div>
            ))}
          </div>
        )}
      </DataView>

      <div className="mt-4 text-[13px] leading-[1.55] text-fg-muted">
        Editing providers and testing connections will land once the runtime exposes{" "}
        <code className="rounded-[3px] bg-surface-2 px-1.5 py-px font-mono text-fg">
          providers.configure
        </code>{" "}
        /{" "}
        <code className="rounded-[3px] bg-surface-2 px-1.5 py-px font-mono text-fg">
          providers.test
        </code>
        .
      </div>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.providers-pane",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SETTINGS_PANE, {
      id: "providers",
      label: "Providers",
      icon: "spark",
      order: 50,
      component: ProvidersPane,
    });
  },
});
