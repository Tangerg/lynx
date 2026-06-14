// Built-in plugin: "Providers" settings pane. Lists every provider the
// runtime supports (providers.list — configured or not) and lets the user set
// each one's API key / base URL (providers.configure) and live-probe it
// (providers.test). `apiKeyMasked != ""` ⇔ enabled; enabling a provider
// surfaces its models in the composer picker (models.list is per-provider).

import type { ProviderInfo } from "@/lib/data/queries";
import { useRef, useState } from "react";
import { DataView, Icon, ProviderIcon } from "@/components/common";
import { useProviders } from "@/lib/data/queries";
import { useConfigureProvider, useTestProvider } from "@/lib/agent/useProviderConfig";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";

type Probe = { state: "idle" | "busy" } | { state: "ok" } | { state: "error"; reason: string };

function ProviderRow({ p }: { p: ProviderInfo }) {
  const configure = useConfigureProvider();
  const test = useTestProvider();
  const [apiKey, setApiKey] = useState("");
  const [baseUrl, setBaseUrl] = useState(p.baseUrl);
  const [saving, setSaving] = useState(false);
  const [probe, setProbe] = useState<Probe>({ state: "idle" });
  // Monotonic token guarding probe results: a Test fired against the OLD key
  // can resolve after a Save reset the row — without the token the stale
  // outcome would stamp itself over the new key's state. Save bumps it so
  // any in-flight probe's result is discarded.
  const probeSeq = useRef(0);

  const enabled = p.apiKeyMasked !== "";
  const dirty = apiKey.trim() !== "" || baseUrl !== p.baseUrl;

  const onSave = async () => {
    setSaving(true);
    probeSeq.current++;
    setProbe({ state: "idle" });
    try {
      await configure({ provider: p.id, apiKey: apiKey.trim() || undefined, baseUrl });
      setApiKey(""); // cleared from the field once persisted (it's masked server-side)
    } catch (err) {
      setProbe({ state: "error", reason: err instanceof Error ? err.message : "Save failed" });
    } finally {
      setSaving(false);
    }
  };

  const onTest = async () => {
    const token = ++probeSeq.current;
    setProbe({ state: "busy" });
    try {
      const r = await test(p.id);
      if (probeSeq.current !== token) return; // superseded by a save / newer test
      setProbe(r.ok ? { state: "ok" } : { state: "error", reason: r.error ?? "Test failed" });
    } catch (err) {
      if (probeSeq.current !== token) return;
      setProbe({ state: "error", reason: err instanceof Error ? err.message : "Test failed" });
    }
  };

  return (
    <div className="rounded-lg border border-line-soft bg-canvas px-3 py-2.5">
      <div className="grid grid-cols-[24px_minmax(0,1fr)_auto] items-center gap-3">
        <ProviderIcon provider={p.id} size={20} />
        <div className="min-w-0">
          <div className="truncate text-[14px] font-semibold capitalize text-fg">{p.id}</div>
        </div>
        <span
          className={cn(
            "rounded-full px-2 py-0.5 text-[11px] font-medium",
            enabled ? "bg-accent/15 text-accent" : "bg-surface-2 text-fg-faint",
          )}
        >
          {enabled ? `key ${p.apiKeyMasked}` : "not configured"}
        </span>
      </div>

      <div className="mt-2.5 grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-2">
        <input
          type="password"
          aria-label={`${p.id} API key`}
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder={enabled ? "Replace API key…" : "API key"}
          className="h-8 rounded-md border border-line-soft bg-surface px-2.5 font-mono text-[12px] text-fg outline-none placeholder:text-fg-faint focus:border-accent"
        />
        <input
          type="text"
          aria-label={`${p.id} base URL`}
          value={baseUrl}
          onChange={(e) => setBaseUrl(e.target.value)}
          placeholder="Base URL (optional override)"
          className="h-8 rounded-md border border-line-soft bg-surface px-2.5 font-mono text-[12px] text-fg outline-none placeholder:text-fg-faint focus:border-accent"
        />
      </div>

      <div className="mt-2 flex items-center gap-2">
        <button
          type="button"
          disabled={!dirty || saving}
          onClick={onSave}
          className={cn(
            "h-7 rounded-md px-3 text-[12px] font-semibold transition-colors",
            !dirty || saving
              ? "cursor-not-allowed bg-surface-2 text-fg-faint"
              : "bg-accent text-on-accent hover:opacity-90",
          )}
        >
          {saving ? "Saving…" : "Save"}
        </button>
        <button
          type="button"
          disabled={!enabled || probe.state === "busy"}
          onClick={onTest}
          className={cn(
            "h-7 rounded-md border px-3 text-[12px] font-semibold transition-colors",
            !enabled || probe.state === "busy"
              ? "cursor-not-allowed border-line-soft text-fg-faint"
              : "border-line text-fg-muted hover:bg-surface-2 hover:text-fg",
          )}
        >
          {probe.state === "busy" ? "Testing…" : "Test"}
        </button>

        {probe.state === "ok" && (
          <span className="inline-flex items-center gap-1 text-[12px] text-accent">
            <Icon name="check" size={13} /> Connection OK
          </span>
        )}
        {probe.state === "error" && (
          <span className="inline-flex min-w-0 items-center gap-1 text-[12px] text-negative">
            <Icon name="alert" size={13} />
            <span className="truncate" title={probe.reason}>
              {probe.reason}
            </span>
          </span>
        )}
      </div>
    </div>
  );
}

function ProvidersPane() {
  const { data, isLoading, isError } = useProviders();

  return (
    <DataView
      items={data}
      isLoading={isLoading}
      isError={isError}
      skeletonCount={3}
      empty={{
        icon: "spark",
        title: "No providers",
        sub: "The runtime reports no supported LLM providers.",
      }}
    >
      {(rows) => (
        <div className="flex flex-col gap-2">
          {rows.map((p) => (
            <ProviderRow key={p.id} p={p} />
          ))}
        </div>
      )}
    </DataView>
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
