import { useState } from "react";
import { Button, FIELD_CLASSES, Icon, ProviderIcon } from "@/ui";
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
    <div className="rounded-md px-3 py-3 transition-colors hover:bg-fg/[0.04]">
      <div className="grid grid-cols-[24px_minmax(0,1fr)_auto] items-center gap-3">
        <ProviderIcon provider={p.id} size={20} />
        <div className="min-w-0">
          <div className="truncate text-[14px] font-medium capitalize text-fg">{p.id}</div>
        </div>
        <span
          title={fromEnv ? p.apiKeyMasked : undefined}
          className={cn(
            "rounded-pill px-2 py-0.5 font-mono text-[11px] font-medium",
            fromEnv
              ? "bg-info/10 text-info"
              : enabled
                ? "bg-success/10 text-success"
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
          className={cn(FIELD_CLASSES, "h-8 px-2.5 text-fg placeholder:text-fg-faint")}
        />
        <input
          type="text"
          aria-label={t("providers.baseUrl.aria", { provider: p.id })}
          value={draft.baseUrl}
          onChange={(e) => setDraft((value) => ({ ...value, baseUrl: e.target.value }))}
          placeholder={t("providers.baseUrl.placeholder")}
          className={cn(FIELD_CLASSES, "h-8 px-2.5 text-fg placeholder:text-fg-faint")}
        />
      </div>

      <div className="mt-2.5 flex items-center gap-2">
        <Button variant="primary" size="sm" disabled={!dirty || saving} onClick={onSave}>
          {saving ? t("providers.saving") : t("providers.save")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={!enabled || probe.state === "busy"}
          onClick={onTest}
        >
          {probe.state === "busy" ? t("providers.testing") : t("providers.test")}
        </Button>

        {probe.state === "ok" && (
          <span className="inline-flex items-center gap-1 text-[12px] text-success">
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
