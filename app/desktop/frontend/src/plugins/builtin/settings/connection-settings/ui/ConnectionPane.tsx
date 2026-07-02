import { useState } from "react";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import {
  applyRuntimeBaseUrl,
  currentRuntimeBaseUrl,
  resetRuntimeBaseUrl,
  RUNTIME_BASE_URL,
} from "../application/runtimeConnection";
import { SettingRow } from "../../SettingRow";

export function ConnectionPane() {
  const t = useT();
  const initial = currentRuntimeBaseUrl();
  const [url, setUrl] = useState(initial);
  const [error, setError] = useState<string | null>(null);

  const trimmed = url.trim();
  const dirty = trimmed !== initial.trim();
  const isDefault = trimmed === RUNTIME_BASE_URL;

  const apply = () => {
    const result = applyRuntimeBaseUrl(url);
    setUrl(result.url);
    setError(result.error);
  };

  const reset = () => {
    setUrl(resetRuntimeBaseUrl());
    setError(null);
  };

  return (
    <div>
      <SettingRow
        label={t("settings.connection.title")}
        sub={t("settings.connection.sub")}
        align="start"
      >
        <div className="grid gap-2">
          <label htmlFor="runtime-base-url" className="text-[12px] font-semibold text-fg-faint">
            {t("settings.connection.url")}
          </label>
          <div className="flex items-center gap-2">
            <input
              id="runtime-base-url"
              type="text"
              aria-label={t("settings.connection.url")}
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              onBlur={apply}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  apply();
                  (e.target as HTMLInputElement).blur();
                }
              }}
              placeholder={RUNTIME_BASE_URL}
              className={cn(
                "flex-1 h-9 rounded-md border-[0.5px] bg-surface px-3 font-mono text-[13px] text-fg outline-none",
                error
                  ? "border-negative focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-negative)_18%,transparent)]"
                  : "border-field focus:border-accent focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]",
              )}
              spellCheck={false}
            />
            {!isDefault && (
              <button
                type="button"
                onClick={reset}
                className="h-9 shrink-0 rounded-md bg-surface-2 px-3 font-sans text-[12.5px] text-fg-muted hover:bg-surface-3 hover:text-fg transition-colors"
              >
                {t("settings.connection.reset")}
              </button>
            )}
          </div>
          {error ? (
            <div className="text-[11.5px] text-negative">{error}</div>
          ) : dirty ? (
            <div className="text-[11.5px] text-fg-faint">
              {/* Inline hint — applied on blur or Enter. */}↵ to apply · click outside to apply
            </div>
          ) : null}
        </div>
      </SettingRow>
    </div>
  );
}
