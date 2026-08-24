import type { ReactNode } from "react";
import { Button, DropdownMenu, Icon, ProviderIcon, Surface } from "@/ui";
import {
  type ProviderConfiguration,
  providerMutationWasRetired,
  setEmbeddingRole,
  setUtilityRole,
  useEmbeddingModelConfig,
  useProviderMutationMaterialGeneration,
  useUtilityModelConfig,
} from "../application/providerConfig";
import { useT } from "@/lib/i18n";
import { useAsyncFeedback } from "../../public";

const triggerClass =
  "inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border-[0.5px] border-field bg-canvas pl-2 pr-2.5 text-ui-md font-medium text-fg whitespace-nowrap transition-colors hover:bg-hover data-[popup-open]:bg-selected";

const itemClass = "grid-cols-[16px_minmax(0,1fr)_14px] px-2";

function RoleSectionShell({
  title,
  description,
  error,
  note,
  children,
}: {
  title: string;
  description: string;
  error?: string | null;
  note?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Surface className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1">
          <span className="text-ui-md font-medium text-fg">{title}</span>
          <span className="text-ui-md leading-snug text-fg-muted">{description}</span>
        </div>
        {children}
      </div>
      {note}
      {error && <p className="text-ui-md leading-snug text-negative">{error}</p>}
    </Surface>
  );
}

// Global utility model: turn-boundary maintenance can run on a cheaper model;
// empty means "use the main turn model".
export function UtilityModelSection() {
  const t = useT();
  const { role, modelOptions, selected, isSet, isAvailable, isError } = useUtilityModelConfig();
  const materialGeneration = useProviderMutationMaterialGeneration();
  const { feedback, run } = useAsyncFeedback(materialGeneration);
  const busy = feedback.state === "busy";

  const pick = (next: { provider: string; model: string } | null): Promise<void> =>
    run(() => setUtilityRole(next ?? {}), t("providers.utility.error"), providerMutationWasRetired);

  return (
    <RoleSectionShell
      title={t("providers.utility.title")}
      description={t("providers.utility.desc")}
      error={
        feedback.state === "error" ? feedback.reason : isError ? t("providers.models.error") : null
      }
      note={
        isSet && !isAvailable ? (
          <p className="text-ui-md leading-snug text-fg-muted">{t("providers.notConfigured")}</p>
        ) : null
      }
    >
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <Button
              type="button"
              variant="outline"
              size="md"
              press={false}
              disabled={busy}
              aria-label={t("providers.utility.title")}
              className={triggerClass}
            >
              {busy ? (
                <>
                  <Icon name="loop" size="xs" className="animate-spin text-fg-muted" />
                  <span className="text-fg-muted">{t("providers.saving")}</span>
                </>
              ) : isSet && role?.provider ? (
                <>
                  <ProviderIcon provider={role.provider} size="sm" />
                  <span className="max-w-[160px] truncate font-mono text-ui-sm">
                    {selected?.label ?? role.model}
                  </span>
                </>
              ) : (
                <span className="text-fg-muted">{t("providers.utility.main")}</span>
              )}
              {!busy && <Icon name="chevron-down" size="xs" className="text-fg-muted" />}
            </Button>
          }
        />
        <DropdownMenu.Content
          align="end"
          sideOffset={6}
          className="max-h-[320px] min-w-[220px] overflow-y-auto"
        >
          <DropdownMenu.Item onClick={() => void pick(null)} className={itemClass}>
            <span />
            <span className="truncate">{t("providers.utility.main")}</span>
            {!isSet && <Icon name="check" size="xs" className="text-accent" />}
          </DropdownMenu.Item>
          {modelOptions.map((m) => (
            <DropdownMenu.Item
              key={`${m.provider}:${m.id}`}
              onClick={() => void pick({ provider: m.provider, model: m.id })}
              className={itemClass}
            >
              <ProviderIcon provider={m.provider} size="md" />
              <span className="truncate">{m.label}</span>
              {role?.provider === m.provider && role?.model === m.id && (
                <Icon name="check" size="xs" className="text-accent" />
              )}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </RoleSectionShell>
  );
}

// Optional embedding model for agent-memory ranking; empty keeps keyword search.
export function EmbeddingModelSection() {
  const t = useT();
  const { role, capableProviders, isSet, isAvailable } = useEmbeddingModelConfig();
  const materialGeneration = useProviderMutationMaterialGeneration();
  const { feedback, run } = useAsyncFeedback(materialGeneration);
  const busy = feedback.state === "busy";

  const pick = (p: ProviderConfiguration | null): Promise<void> =>
    run(
      () => setEmbeddingRole(p ? { provider: p.id, model: p.defaultEmbeddingModel || "" } : {}),
      t("providers.embedding.error"),
      providerMutationWasRetired,
    );

  return (
    <RoleSectionShell
      title={t("providers.embedding.title")}
      description={t("providers.embedding.desc")}
      error={feedback.state === "error" ? feedback.reason : null}
      note={
        isSet && !isAvailable ? (
          <p className="text-ui-md leading-snug text-fg-muted">{t("providers.notConfigured")}</p>
        ) : capableProviders.length === 0 ? (
          <p className="text-ui-md leading-snug text-fg-muted">{t("providers.embedding.none")}</p>
        ) : null
      }
    >
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <Button
              type="button"
              variant="outline"
              size="md"
              press={false}
              disabled={busy}
              aria-label={t("providers.embedding.title")}
              className={triggerClass}
            >
              {busy ? (
                <>
                  <Icon name="loop" size="xs" className="animate-spin text-fg-muted" />
                  <span className="text-fg-muted">{t("providers.saving")}</span>
                </>
              ) : isSet && role?.provider ? (
                <>
                  <ProviderIcon provider={role.provider} size="sm" />
                  <span className="max-w-[160px] truncate font-mono text-ui-sm">{role.model}</span>
                </>
              ) : (
                <span className="text-fg-muted">{t("providers.embedding.off")}</span>
              )}
              {!busy && <Icon name="chevron-down" size="xs" className="text-fg-muted" />}
            </Button>
          }
        />
        <DropdownMenu.Content
          align="end"
          sideOffset={6}
          className="max-h-[320px] min-w-[220px] overflow-y-auto"
        >
          <DropdownMenu.Item onClick={() => void pick(null)} className={itemClass}>
            <span />
            <span className="truncate">{t("providers.embedding.off")}</span>
            {!isSet && <Icon name="check" size="xs" className="text-accent" />}
          </DropdownMenu.Item>
          {capableProviders.map((p) => (
            <DropdownMenu.Item key={p.id} onClick={() => void pick(p)} className={itemClass}>
              <ProviderIcon provider={p.id} size="md" />
              <span className="truncate">
                {p.id}
                {p.defaultEmbeddingModel ? ` · ${p.defaultEmbeddingModel}` : ""}
              </span>
              {role?.provider === p.id && <Icon name="check" size="xs" className="text-accent" />}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </RoleSectionShell>
  );
}
