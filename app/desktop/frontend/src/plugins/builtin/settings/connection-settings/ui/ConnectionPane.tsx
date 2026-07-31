import { useState } from "react";
import { Button, StatusDot, TextField } from "@/ui";
import { useT, type Translate } from "@/lib/i18n";
import {
  applyRuntimeEndpoint,
  currentRuntimeEndpoint,
  resetRuntimeEndpoint,
  DEFAULT_RUNTIME_ENDPOINT,
  type RuntimeEndpointRejection,
} from "@/plugins/builtin/runtime/public/endpoint";
import { SettingRow, SettingsGroup } from "../../public";

function rejectionMessage(reason: RuntimeEndpointRejection, translate: Translate): string {
  switch (reason) {
    case "invalid_url":
      return translate("connection.error.invalidUrl");
    case "unsupported_scheme":
      return translate("connection.error.urlScheme");
  }
}

export function ConnectionPane() {
  const t = useT();
  const initial = currentRuntimeEndpoint();
  const [url, setUrl] = useState(initial);
  const [error, setError] = useState<string | null>(null);

  const trimmed = url.trim();
  const dirty = trimmed !== initial.trim();
  const isDefault = trimmed === DEFAULT_RUNTIME_ENDPOINT;

  const apply = () => {
    const result = applyRuntimeEndpoint(url);
    if (result.kind === "rejected") {
      setError(rejectionMessage(result.reason, t));
      return;
    }
    setUrl(result.endpoint);
    setError(null);
    if (result.changed) window.location.reload();
  };

  const reset = () => {
    const result = resetRuntimeEndpoint();
    if (result.kind === "rejected") {
      setError(rejectionMessage(result.reason, t));
      return;
    }
    setUrl(result.endpoint);
    setError(null);
    if (result.changed) window.location.reload();
  };

  return (
    <SettingsGroup>
      <SettingRow
        label={t("settings.connection.title")}
        sub={t("settings.connection.sub")}
        align="start"
      >
        <div className="grid gap-2">
          <label htmlFor="runtime-base-url" className="text-ui-md font-medium text-fg-muted">
            {t("settings.connection.url")}
          </label>
          <div className="flex items-center gap-2">
            <TextField
              id="runtime-base-url"
              type="text"
              size="lg"
              invalid={error !== null}
              aria-label={t("settings.connection.url")}
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  apply();
                  (e.target as HTMLInputElement).blur();
                }
              }}
              placeholder={DEFAULT_RUNTIME_ENDPOINT}
              className="flex-1"
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
            {dirty && (
              <Button type="button" size="sm" onClick={apply} className="h-9 shrink-0">
                {t("settings.connection.apply")}
              </Button>
            )}
          </div>
          {error ? (
            <div className="flex items-center gap-1.5 text-ui-sm text-negative">
              <StatusDot tone="err" />
              <span>{error}</span>
            </div>
          ) : null}
        </div>
      </SettingRow>
    </SettingsGroup>
  );
}
