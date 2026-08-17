import { useState } from "react";
import { Button, Icon, ProviderIcon, TextField } from "@/ui";
import {
  type ProviderConfiguration,
  providerMutationWasRetired,
  useProviderMutationMaterialGeneration,
  useUpdateProvider,
  useTestProvider,
} from "../application/providerConfig";
import {
  initialProviderCredentialsDraft,
  providerCredentialsDirty,
  providerCredentialsInput,
  providerCredentialsValid,
} from "../application/providerDraft";
import { useT } from "@/lib/i18n";
import { useAsyncFeedback } from "../../public";
import { cn } from "@/lib/classNames";

export function ProviderRow({ p }: { p: ProviderConfiguration }) {
  const t = useT();
  const update = useUpdateProvider();
  const test = useTestProvider();
  const [draft, setDraft] = useState(() => initialProviderCredentialsDraft(p));
  const [saving, setSaving] = useState(false);
  const materialGeneration = useProviderMutationMaterialGeneration();
  const { feedback, reset, fail, run } = useAsyncFeedback(materialGeneration);

  const enabled = p.apiKeyMasked !== "";
  // Env keys are read-only at the source, but a typed key still overrides them.
  const fromEnv = p.keySource === "env";
  const hasStoredKey = p.keySource === "stored";
  const dirty = providerCredentialsDirty(p, draft);
  const valid = providerCredentialsValid(p, draft);

  const onSave = async () => {
    setSaving(true);
    reset(); // invalidate any in-flight test so its result can't overwrite the new key state
    try {
      const saved = await update(providerCredentialsInput(p, draft));
      setDraft(initialProviderCredentialsDraft(saved));
    } catch (err) {
      if (providerMutationWasRetired(err)) return;
      fail(err instanceof Error ? err.message : t("providers.error.save"));
    } finally {
      setSaving(false);
    }
  };

  const onClearKey = async () => {
    setSaving(true);
    reset();
    try {
      const saved = await update({ provider: p.id, apiKey: null });
      setDraft(initialProviderCredentialsDraft(saved));
    } catch (err) {
      if (providerMutationWasRetired(err)) return;
      fail(err instanceof Error ? err.message : t("providers.error.save"));
    } finally {
      setSaving(false);
    }
  };

  const onTest = () => run(() => test(p.id), t("providers.error.test"), providerMutationWasRetired);

  return (
    <div className="rounded-md px-3 py-3 transition-colors hover:bg-hover">
      <div className="grid grid-cols-[24px_minmax(0,1fr)_auto] items-center gap-3">
        <ProviderIcon provider={p.id} size="lg" />
        <div className="min-w-0">
          <div className="truncate text-ui-md font-medium capitalize text-fg">{p.id}</div>
        </div>
        <span
          title={fromEnv ? p.apiKeyMasked : undefined}
          className={cn(
            "rounded-pill px-2 py-0.5 font-mono text-ui-sm font-medium",
            fromEnv
              ? "bg-info-wash text-info"
              : enabled
                ? "bg-success-wash text-success"
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
        <TextField
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
        />
        <TextField
          type="text"
          aria-label={t("providers.baseUrl.aria", { provider: p.id })}
          value={draft.baseUrl}
          onChange={(e) => setDraft((value) => ({ ...value, baseUrl: e.target.value }))}
          placeholder={t("providers.baseUrl.placeholder")}
        />
      </div>

      <div className="mt-2.5 flex items-center gap-2">
        <Button variant="primary" size="sm" disabled={!dirty || !valid || saving} onClick={onSave}>
          {saving ? t("providers.saving") : t("providers.save")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={!enabled || feedback.state === "busy"}
          onClick={onTest}
        >
          {feedback.state === "busy" ? t("providers.testing") : t("providers.test")}
        </Button>
        {hasStoredKey && (
          <Button variant="ghost" size="sm" disabled={saving} onClick={onClearKey}>
            {t("providers.apiKey.clear")}
          </Button>
        )}

        {feedback.state === "ok" && (
          <span className="inline-flex items-center gap-1 text-ui-md text-success">
            <Icon name="check" size="sm" /> {t("providers.connectionOk")}
          </span>
        )}
        {feedback.state === "error" && (
          <span className="inline-flex min-w-0 items-center gap-1 text-ui-md text-negative">
            <Icon name="alert" size="sm" />
            <span className="truncate" title={feedback.reason}>
              {feedback.reason}
            </span>
          </span>
        )}
      </div>
    </div>
  );
}
