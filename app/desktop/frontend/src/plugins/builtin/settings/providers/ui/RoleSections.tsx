import type { ReactNode } from "react";
import { useState } from "react";
import { DropdownMenu, Icon, ProviderIcon } from "@/ui";
import {
  type ProviderConfig,
  setEmbeddingRole,
  setUtilityRole,
  useEmbeddingModelConfig,
  useUtilityModelConfig,
} from "../application/providerConfig";
import { useT } from "@/lib/i18n";

const triggerClass =
  "inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border-[0.5px] border-field bg-canvas pl-2 pr-2.5 text-[12px] font-medium text-fg whitespace-nowrap transition-colors hover:bg-fg/[0.04] data-[popup-open]:bg-fg/[0.04]";

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
    <div className="flex flex-col gap-3 rounded-[14px] bg-surface p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1">
          <span className="text-[13px] font-medium text-fg">{title}</span>
          <span className="text-[13px] leading-snug text-fg-muted">{description}</span>
        </div>
        {children}
      </div>
      {note}
      {error && <p className="text-[12px] leading-snug text-negative">{error}</p>}
    </div>
  );
}

// Global utility model: turn-boundary maintenance can run on a cheaper model;
// empty means "use the main turn model".
export function UtilityModelSection() {
  const t = useT();
  const { role, modelOptions, selected, isSet } = useUtilityModelConfig();
  const [error, setError] = useState<string | null>(null);

  const pick = async (next: { provider: string; model: string } | null): Promise<void> => {
    setError(null);
    const res = await setUtilityRole(next ?? {});
    if (!res.ok) setError(res.error ?? t("providers.utility.error"));
  };

  return (
    <RoleSectionShell
      title={t("providers.utility.title")}
      description={t("providers.utility.desc")}
      error={error}
    >
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <button
              type="button"
              aria-label={t("providers.utility.title")}
              className={triggerClass}
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
              <Icon name="chevron-down" size={10} className="text-fg-muted" />
            </button>
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
            {!isSet && <Icon name="check" size={12} className="text-accent" />}
          </DropdownMenu.Item>
          {modelOptions.map((m) => (
            <DropdownMenu.Item
              key={`${m.provider}:${m.id}`}
              onClick={() => void pick({ provider: m.provider, model: m.id })}
              className={itemClass}
            >
              <ProviderIcon provider={m.provider} size={16} />
              <span className="truncate">{m.label}</span>
              {role?.provider === m.provider && role?.model === m.id && (
                <Icon name="check" size={12} className="text-accent" />
              )}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </RoleSectionShell>
  );
}

// Global embedding model for @codebase indexing; empty disables semantic search.
export function EmbeddingModelSection() {
  const t = useT();
  const { role, capableProviders, isSet } = useEmbeddingModelConfig();
  const [error, setError] = useState<string | null>(null);

  const pick = async (p: ProviderConfig | null): Promise<void> => {
    setError(null);
    const res = await setEmbeddingRole(
      p ? { provider: p.id, model: p.defaultEmbeddingModel || "" } : {},
    );
    if (!res.ok) setError(res.error ?? t("providers.embedding.error"));
  };

  return (
    <RoleSectionShell
      title={t("providers.embedding.title")}
      description={t("providers.embedding.desc")}
      error={error}
      note={
        capableProviders.length === 0 ? (
          <p className="text-[12px] leading-snug text-fg-muted">{t("providers.embedding.none")}</p>
        ) : null
      }
    >
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <button
              type="button"
              aria-label={t("providers.embedding.title")}
              className={triggerClass}
            >
              {isSet && role?.provider ? (
                <>
                  <ProviderIcon provider={role.provider} size={14} />
                  <span className="max-w-[160px] truncate font-mono text-[11.5px]">
                    {role.model}
                  </span>
                </>
              ) : (
                <span className="text-fg-muted">{t("providers.embedding.off")}</span>
              )}
              <Icon name="chevron-down" size={10} className="text-fg-muted" />
            </button>
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
            {!isSet && <Icon name="check" size={12} className="text-accent" />}
          </DropdownMenu.Item>
          {capableProviders.map((p) => (
            <DropdownMenu.Item key={p.id} onClick={() => void pick(p)} className={itemClass}>
              <ProviderIcon provider={p.id} size={16} />
              <span className="truncate">
                {p.id}
                {p.defaultEmbeddingModel ? ` · ${p.defaultEmbeddingModel}` : ""}
              </span>
              {role?.provider === p.id && <Icon name="check" size={12} className="text-accent" />}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </RoleSectionShell>
  );
}
