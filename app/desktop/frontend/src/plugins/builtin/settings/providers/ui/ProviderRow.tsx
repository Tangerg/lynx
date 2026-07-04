import { useState } from "react";
import { Icon, INPUT_FOCUS_RING, ProviderIcon } from "@/ui";
import {
  type ProviderConfig,
  useConfigureProvider,
  useTestProvider,
} from "../application/providerConfig";
import {
  initialProviderCredentialsDraft,
  providerCredentialsDirty,
  providerCredentialsInput,
} from "../application/providerDraft";
import { useT } from "@/lib/i18n";
import { useProbe } from "../../useProbe";
import { cn } from "@/lib/utils";

export function ProviderRow({ p }: { p: ProviderConfig }) {
  const t = useT();
  const configure = useConfigureProvider();
  const test = useTestProvider();
  const [draft, setDraft] = useState(() => initialProviderCredentialsDraft(p));
  const [saving, setSaving] = useState(false);
  const { probe, reset, fail, run } = useProbe();

  const enabled = p.apiKeyMasked !== "";
  // Env keys are read-only at the source, but a typed key still overrides them.
  const fromEnv = p.keySource === "env";
  const dirty = providerCredentialsDirty(p, draft);

  const onSave = async () => {
    setSaving(true);
    reset(); // invalidate any in-flight test so its result can't overwrite the new key state
    try {
      await configure(providerCredentialsInput(p, draft));
      setDraft((value) => ({ ...value, apiKey: "" }));
    } catch (err) {
      fail(err instanceof Error ? err.message : t("providers.error.save"));
    } finally {
      setSaving(false);
    }
  };

  const onTest = () => run(() => test(p.id), t("providers.error.test"));

  return (
    <div className="rounded-lg bg-canvas px-3 py-2.5">
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
          value={draft.apiKey}
          onChange={(e) => setDraft((value) => ({ ...value, apiKey: e.target.value }))}
          placeholder={
            fromEnv
              ? t("providers.apiKey.envPlaceholder")
              : enabled
                ? t("providers.apiKey.replace")
                : t("providers.apiKey.placeholder")
          }
          className={cn(
            "h-8 rounded-md border-[0.5px] border-field bg-surface px-2.5 font-mono text-[12px] text-fg outline-none placeholder:text-fg-faint",
            INPUT_FOCUS_RING,
          )}
        />
        <input
          type="text"
          aria-label={t("providers.baseUrl.aria", { provider: p.id })}
          value={draft.baseUrl}
          onChange={(e) => setDraft((value) => ({ ...value, baseUrl: e.target.value }))}
          placeholder={t("providers.baseUrl.placeholder")}
          className={cn(
            "h-8 rounded-md border-[0.5px] border-field bg-surface px-2.5 font-mono text-[12px] text-fg outline-none placeholder:text-fg-faint",
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
            "h-7 rounded-md border-[0.5px] px-3 text-[12px] font-semibold transition-colors",
            !enabled || probe.state === "busy"
              ? "cursor-not-allowed border-field text-fg-faint"
              : "border-field text-fg-muted hover:bg-surface-2 hover:text-fg",
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
