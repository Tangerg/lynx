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
import {
  refreshRuntimeServiceStatus,
  useRuntimeServiceStatus,
  type RuntimeServicePhase,
} from "@/plugins/builtin/runtime/public/serviceStatus";
import { SettingRow, SettingsGroup } from "../../public";

const STATUS_TONE: Record<RuntimeServicePhase, "ok" | "running" | "waiting" | "err"> = {
  checking: "running",
  reconnecting: "running",
  ready: "ok",
  degraded: "waiting",
  unhealthy: "err",
  unavailable: "err",
};

const STATUS_KEY: Record<RuntimeServicePhase, string> = {
  checking: "settings.connection.status.checking",
  reconnecting: "settings.connection.status.reconnecting",
  ready: "settings.connection.status.ready",
  degraded: "settings.connection.status.degraded",
  unhealthy: "settings.connection.status.unhealthy",
  unavailable: "settings.connection.status.unavailable",
};

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
  const service = useRuntimeServiceStatus();
  const [url, setUrl] = useState(initial);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

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
  };

  const reset = () => {
    const result = resetRuntimeEndpoint();
    if (result.kind === "rejected") {
      setError(rejectionMessage(result.reason, t));
      return;
    }
    setUrl(result.endpoint);
    setError(null);
  };

  const refresh = async () => {
    setRefreshing(true);
    try {
      await refreshRuntimeServiceStatus();
    } finally {
      setRefreshing(false);
    }
  };

  const unhealthyChecks = Object.entries(service.observation?.checks ?? {}).filter(
    ([, health]) => health !== "ready",
  );

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
          <div className="mt-1 rounded-md border border-border-subtle bg-surface-1 px-3 py-2.5">
            <div className="flex items-center justify-between gap-3" aria-live="polite">
              <div className="flex min-w-0 items-center gap-2 text-ui-md text-fg-muted">
                <StatusDot tone={STATUS_TONE[service.phase]} />
                <span>{t(STATUS_KEY[service.phase])}</span>
              </div>
              <Button
                type="button"
                variant="soft"
                size="sm"
                disabled={
                  refreshing || service.phase === "checking" || service.phase === "reconnecting"
                }
                onClick={() => void refresh()}
              >
                {t("settings.connection.status.refresh")}
              </Button>
            </div>
            {service.observation ? (
              <dl className="mt-2 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-ui-sm">
                <dt className="text-fg-faint">{t("settings.connection.status.server")}</dt>
                <dd className="truncate font-mono text-fg-muted">
                  {service.observation.server.name} {service.observation.server.version}
                </dd>
                <dt className="text-fg-faint">{t("settings.connection.status.protocol")}</dt>
                <dd className="truncate font-mono text-fg-muted">
                  {service.observation.protocolVersion}
                </dd>
                {unhealthyChecks.length > 0 ? (
                  <>
                    <dt className="text-fg-faint">{t("settings.connection.status.checks")}</dt>
                    <dd className="flex min-w-0 flex-wrap gap-x-2 font-mono text-warning">
                      {unhealthyChecks.map(([name, health]) => (
                        <span key={name}>
                          {name} <span className="text-fg-faint">{t(STATUS_KEY[health])}</span>
                        </span>
                      ))}
                    </dd>
                  </>
                ) : null}
              </dl>
            ) : null}
            {service.failure ? (
              <p className="mt-2 break-words text-ui-sm text-negative">
                {service.failure.reason === "timeout"
                  ? t("settings.connection.status.timeout")
                  : service.failure.detail}
              </p>
            ) : null}
          </div>
        </div>
      </SettingRow>
    </SettingsGroup>
  );
}
