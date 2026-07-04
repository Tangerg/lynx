import { useState } from "react";
import { Button, FIELD_CLASSES, StatusDot } from "@/ui";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import {
  applyRuntimeBaseUrl,
  currentRuntimeBaseUrl,
  resetRuntimeBaseUrl,
  RUNTIME_BASE_URL,
} from "../application/runtimeConnection";
import { SettingRow } from "../../public";

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
          <label htmlFor="runtime-base-url" className="text-[12px] font-medium text-fg-muted">
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
                FIELD_CLASSES,
                "h-9 flex-1 px-3 text-[13px] text-fg",
                error && "border-negative focus:border-negative",
              )}
              spellCheck={false}
            />
            {!isDefault && (
              <Button
                type="button"
                variant="soft"
                size="sm"
                onClick={reset}
                className="h-9 shrink-0"
              >
                {t("settings.connection.reset")}
              </Button>
            )}
          </div>
          {error ? (
            <div className="flex items-center gap-1.5 text-[11.5px] text-negative">
              <StatusDot tone="err" />
              <span>{error}</span>
            </div>
          ) : dirty ? (
            <div className="text-[11.5px] text-fg-muted">↵ to apply · click outside to apply</div>
          ) : null}
        </div>
      </SettingRow>
    </div>
  );
}
