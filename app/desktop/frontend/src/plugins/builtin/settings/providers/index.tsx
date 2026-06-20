// Built-in plugin: "Providers" settings pane. Lists every provider the
// runtime supports (providers.list — configured or not) and lets the user set
// each one's API key / base URL (providers.configure) and live-probe it
// (providers.test). `apiKeyMasked != ""` ⇔ enabled; enabling a provider
// surfaces its models in the composer picker (models.list is per-provider).

import type { ProviderInfo } from "@/lib/data/queries";
import { useRef, useState } from "react";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import {
  DataView,
  Icon,
  INPUT_FOCUS_RING,
  MENU_CONTENT_CLASSES,
  ProviderIcon,
} from "@/components/common";
import { useModels, useProviders, useUtilityRole } from "@/lib/data/queries";
import {
  setUtilityRole,
  useConfigureProvider,
  useTestProvider,
} from "@/lib/agent/useProviderConfig";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";

type Probe = { state: "idle" | "busy" } | { state: "ok" } | { state: "error"; reason: string };

function ProviderRow({ p }: { p: ProviderInfo }) {
  const t = useT();
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
  // Key picked up from the environment (read-only): the row stays editable so a
  // typed key overrides it (stored > env), but the badge + placeholder say so.
  const fromEnv = p.keySource === "env";
  const dirty = apiKey.trim() !== "" || baseUrl !== p.baseUrl;

  const onSave = async () => {
    setSaving(true);
    probeSeq.current++;
    setProbe({ state: "idle" });
    try {
      await configure({ provider: p.id, apiKey: apiKey.trim() || undefined, baseUrl });
      setApiKey(""); // cleared from the field once persisted (it's masked server-side)
    } catch (err) {
      setProbe({
        state: "error",
        reason: err instanceof Error ? err.message : t("providers.error.save"),
      });
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
      setProbe(
        r.ok ? { state: "ok" } : { state: "error", reason: r.error ?? t("providers.error.test") },
      );
    } catch (err) {
      if (probeSeq.current !== token) return;
      setProbe({
        state: "error",
        reason: err instanceof Error ? err.message : t("providers.error.test"),
      });
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
          title={fromEnv ? p.apiKeyMasked : undefined}
          className={cn(
            "rounded-full px-2 py-0.5 text-[11px] font-medium",
            fromEnv
              ? "bg-accent/12 text-accent"
              : enabled
                ? "bg-success/12 text-success"
                : "bg-surface-2 text-fg-faint",
          )}
        >
          {fromEnv
            ? t("providers.fromEnv")
            : enabled
              ? t("providers.key", { masked: p.apiKeyMasked })
              : t("providers.notConfigured")}
        </span>
      </div>

      <div className="mt-2.5 grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-2">
        <input
          type="password"
          aria-label={t("providers.apiKey.aria", { provider: p.id })}
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder={
            fromEnv
              ? t("providers.apiKey.envPlaceholder")
              : enabled
                ? t("providers.apiKey.replace")
                : t("providers.apiKey.placeholder")
          }
          className={cn(
            "h-8 rounded-md border border-line-soft bg-surface px-2.5 font-mono text-[12px] text-fg outline-none placeholder:text-fg-faint",
            INPUT_FOCUS_RING,
          )}
        />
        <input
          type="text"
          aria-label={t("providers.baseUrl.aria", { provider: p.id })}
          value={baseUrl}
          onChange={(e) => setBaseUrl(e.target.value)}
          placeholder={t("providers.baseUrl.placeholder")}
          className={cn(
            "h-8 rounded-md border border-line-soft bg-surface px-2.5 font-mono text-[12px] text-fg outline-none placeholder:text-fg-faint",
            INPUT_FOCUS_RING,
          )}
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
          {saving ? t("providers.saving") : t("providers.save")}
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
          {probe.state === "busy" ? t("providers.testing") : t("providers.test")}
        </button>

        {probe.state === "ok" && (
          <span className="inline-flex items-center gap-1 text-[12px] text-accent">
            <Icon name="check" size={13} /> {t("providers.connectionOk")}
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

// Global "utility model" picker: the cheaper model the runtime runs its
// turn-boundary maintenance work (compaction / extraction / titling) on,
// instead of the headline model. Empty = use the main model. Sits atop the
// per-provider rows since it's one global choice across providers; its options
// come from whichever providers are configured (models.list per enabled one).
function UtilityModelSection() {
  const t = useT();
  const { data: role } = useUtilityRole();
  const { data: models = [] } = useModels();
  const [error, setError] = useState<string | null>(null);

  const isSet = Boolean(role?.model);
  const selected = isSet
    ? (models.find((m) => m.provider === role?.provider && m.id === role?.model) ?? null)
    : null;

  const pick = async (next: { provider: string; model: string } | null): Promise<void> => {
    setError(null);
    const res = await setUtilityRole(next ?? {});
    if (!res.ok) setError(res.error ?? t("providers.utility.error"));
  };

  return (
    <div className="flex flex-col gap-2 rounded-lg bg-surface-2 p-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="text-[12.5px] font-semibold text-fg">
            {t("providers.utility.title")}
          </span>
          <span className="text-[11.5px] leading-snug text-fg-faint">
            {t("providers.utility.desc")}
          </span>
        </div>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              type="button"
              aria-label={t("providers.utility.title")}
              className="inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border border-line bg-surface pl-2 pr-2.5 text-[12px] font-semibold text-fg whitespace-nowrap transition-colors hover:bg-surface-3 data-[state=open]:bg-surface-3"
            >
              {isSet && selected ? (
                <>
                  <ProviderIcon provider={selected.provider} size={14} />
                  <span className="max-w-[160px] truncate font-mono text-[11.5px]">
                    {selected.label}
                  </span>
                </>
              ) : (
                <span className="text-fg-muted">{t("providers.utility.main")}</span>
              )}
              <Icon name="chevron-down" size={10} className="text-fg-faint opacity-70" />
            </button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content
              align="end"
              sideOffset={6}
              className={cn(MENU_CONTENT_CLASSES, "max-h-[320px] min-w-[220px] overflow-y-auto")}
            >
              <DropdownMenu.Item
                onSelect={() => void pick(null)}
                className="grid grid-cols-[16px_minmax(0,1fr)_14px] items-center gap-2 rounded-sm px-2 py-1.5 text-[12.5px] text-fg-muted outline-none data-[highlighted]:bg-surface-2 data-[highlighted]:text-fg"
              >
                <span />
                <span className="truncate">{t("providers.utility.main")}</span>
                {!isSet && <Icon name="check" size={12} className="text-accent" />}
              </DropdownMenu.Item>
              {models.map((m) => (
                <DropdownMenu.Item
                  key={`${m.provider}:${m.id}`}
                  onSelect={() => void pick({ provider: m.provider, model: m.id })}
                  className="grid grid-cols-[16px_minmax(0,1fr)_14px] items-center gap-2 rounded-sm px-2 py-1.5 text-[12.5px] text-fg-muted outline-none data-[highlighted]:bg-surface-2 data-[highlighted]:text-fg"
                >
                  <ProviderIcon provider={m.provider} size={16} />
                  <span className="truncate">{m.label}</span>
                  {role?.provider === m.provider && role?.model === m.id && (
                    <Icon name="check" size={12} className="text-accent" />
                  )}
                </DropdownMenu.Item>
              ))}
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>
      {error && <p className="text-[11px] leading-snug text-negative">{error}</p>}
    </div>
  );
}

function ProvidersPane() {
  const t = useT();
  const { data, isLoading, isError } = useProviders();

  return (
    <div className="flex flex-col gap-3">
      <UtilityModelSection />
      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{
          icon: "spark",
          title: t("providers.empty"),
          sub: t("providers.empty.sub"),
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
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.providers-pane",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SETTINGS_PANE, {
      id: "providers",
      label: "settings.pane.providers",
      icon: "spark",
      order: 50,
      component: ProvidersPane,
    });
  },
});
