import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";

import type {
  EmbeddingRole,
  Provider,
  RuntimeConnection,
  UpdateProviderRequest,
  UtilityRole,
} from "@lyra/runtime-contract";

import {
  getEmbeddingRole,
  getUtilityRole,
  listModels,
  listProviders,
  runtimeQueryKeys,
  setEmbeddingRole,
  setUtilityRole,
  testProvider,
  updateProvider,
} from "../../runtime/runtimeQueries";
import { useLocalization, type Translate } from "../localization/Localization";

interface ProviderModelSettingsProps {
  connection: RuntimeConnection;
}

export function ProviderModelSettings(props: ProviderModelSettingsProps) {
  const { t } = useLocalization();
  const [query, setQuery] = useState("");
  const providers = useQuery({
    queryKey: runtimeQueryKeys.providers(props.connection),
    queryFn: ({ signal }) => listProviders(props.connection, signal),
    retry: 2,
  });
  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return (providers.data?.data ?? []).filter(
      (provider) =>
        needle === "" ||
        provider.id.toLowerCase().includes(needle) ||
        providerName(provider.id).toLowerCase().includes(needle),
    );
  }, [providers.data, query]);

  return (
    <>
      <section className="settings-section" aria-labelledby="model-roles-title">
        <header>
          <div>
            <h2 id="model-roles-title">{t("settings.provider.roles")}</h2>
            <p>{t("settings.provider.rolesDetail")}</p>
          </div>
        </header>
        <div className="model-role-grid">
          <ModelRoleEditor
            connection={props.connection}
            role="utility"
            providers={providers.data?.data ?? []}
          />
          <ModelRoleEditor
            connection={props.connection}
            role="embedding"
            providers={providers.data?.data ?? []}
          />
        </div>
      </section>
      <section
        className="settings-section"
        aria-labelledby="provider-settings-title"
      >
        <header className="provider-section-heading">
          <div>
            <h2 id="provider-settings-title">
              {t("settings.provider.connections")}
            </h2>
            <p>{t("settings.provider.connectionsDetail")}</p>
          </div>
          <label>
            <span className="sr-only">{t("settings.provider.filter")}</span>
            <input
              value={query}
              maxLength={80}
              placeholder={t("settings.provider.filterPlaceholder")}
              onChange={(event) => setQuery(event.currentTarget.value)}
            />
          </label>
        </header>
        {providers.isPending ? (
          <SettingsState>{t("settings.provider.loading")}</SettingsState>
        ) : providers.isError ? (
          <SettingsState
            action={t("settings.provider.tryAgain")}
            onAction={() => void providers.refetch()}
          >
            {messageOf(providers.error, t)}
          </SettingsState>
        ) : visible.length === 0 ? (
          <SettingsState>{t("settings.provider.noMatch")}</SettingsState>
        ) : (
          <div className="provider-card-list">
            {visible.map((provider) => (
              <ProviderCard
                key={provider.id}
                connection={props.connection}
                provider={provider}
              />
            ))}
          </div>
        )}
      </section>
    </>
  );
}

function ProviderCard(props: {
  connection: RuntimeConnection;
  provider: Provider;
}) {
  const { t } = useLocalization();
  const queryClient = useQueryClient();
  const [baseURL, setBaseURL] = useState(props.provider.baseUrl ?? "");
  const [apiKey, setAPIKey] = useState("");
  const [clearStoredKey, setClearStoredKey] = useState(false);
  const [testResult, setTestResult] = useState<{
    tone: "ok" | "error";
    message: string;
  }>();
  const baseChanged = baseURL.trim() !== (props.provider.baseUrl ?? "");
  const keyChanged = apiKey.trim() !== "" || clearStoredKey;
  const incompleteEndpoint =
    props.provider.requiresBaseUrl &&
    baseURL.trim() === "" &&
    !clearStoredKey &&
    (apiKey.trim() !== "" || props.provider.keySource === "stored");

  useEffect(() => {
    setBaseURL(props.provider.baseUrl ?? "");
    setAPIKey("");
    setClearStoredKey(false);
  }, [
    props.provider.baseUrl,
    props.provider.apiKeyMasked,
    props.provider.keySource,
  ]);

  const save = useMutation({
    mutationFn: () => {
      const request: UpdateProviderRequest = { provider: props.provider.id };
      if (baseChanged) {
        request.baseUrl =
          baseURL.trim() === ""
            ? { type: "clear" }
            : { type: "set", value: baseURL.trim() };
      }
      if (clearStoredKey) request.apiKey = { type: "clear" };
      else if (apiKey.trim() !== "")
        request.apiKey = { type: "set", value: apiKey };
      return updateProvider(props.connection, request);
    },
    onSuccess: () => {
      setAPIKey("");
      setClearStoredKey(false);
      void queryClient.invalidateQueries({
        queryKey: runtimeQueryKeys.providers(props.connection),
      });
      void queryClient.invalidateQueries({
        queryKey: [...runtimeQueryKeys.scope(props.connection), "models"],
      });
    },
  });
  const test = useMutation({
    mutationFn: () => testProvider(props.connection, props.provider.id),
    onMutate: () => setTestResult(undefined),
    onSuccess: (result) =>
      setTestResult(
        result.ok
          ? { tone: "ok", message: t("settings.provider.connectionSucceeded") }
          : {
              tone: "error",
              message:
                result.error?.detail ||
                result.error?.type ||
                t("settings.provider.connectionFailed"),
            },
      ),
  });
  const configured =
    props.provider.apiKeyMasked !== "" || props.provider.id === "ollama";

  return (
    <article
      className="provider-card"
      data-configured={configured || undefined}
    >
      <header>
        <div>
          <h3>{providerName(props.provider.id)}</h3>
          <code>{props.provider.id}</code>
        </div>
        <span>
          {configured
            ? t("settings.provider.configured")
            : t("settings.provider.notConfigured")}
        </span>
      </header>
      <div className="provider-fields">
        <label>
          <span>
            {t("settings.provider.baseURL")}{" "}
            {props.provider.requiresBaseUrl ? (
              <b>{t("settings.common.required")}</b>
            ) : (
              <small>{t("settings.provider.optionalOverride")}</small>
            )}
          </span>
          <input
            dir="ltr"
            type="url"
            value={baseURL}
            maxLength={2048}
            placeholder={
              props.provider.requiresBaseUrl
                ? "https://…/v1"
                : t("settings.provider.useDefault")
            }
            onChange={(event) => setBaseURL(event.currentTarget.value)}
          />
        </label>
        <label>
          <span>
            {t("settings.provider.apiKey")}{" "}
            <small>
              {props.provider.keySource === "env"
                ? t("settings.provider.environmentReadOnly")
                : props.provider.apiKeyMasked || t("settings.provider.notSet")}
            </small>
          </span>
          <input
            dir="ltr"
            type="password"
            value={apiKey}
            maxLength={4096}
            autoComplete="off"
            placeholder={
              clearStoredKey
                ? t("settings.provider.keyRemoved")
                : t("settings.provider.replacementKey")
            }
            disabled={clearStoredKey}
            onChange={(event) => setAPIKey(event.currentTarget.value)}
          />
        </label>
      </div>
      <footer>
        <div>
          {props.provider.keySource === "stored" ? (
            <button
              className="text-action danger"
              type="button"
              onClick={() => {
                setAPIKey("");
                setClearStoredKey((current) => !current);
              }}
            >
              {clearStoredKey
                ? t("settings.provider.keepKey")
                : t("settings.provider.removeKey")}
            </button>
          ) : null}
          {testResult ? (
            <span className="provider-verdict" data-tone={testResult.tone}>
              {testResult.message}
            </span>
          ) : null}
          {test.isError ? (
            <span className="provider-verdict" data-tone="error">
              {messageOf(test.error, t)}
            </span>
          ) : null}
        </div>
        <div>
          <button
            className="secondary-action"
            type="button"
            disabled={test.isPending || baseChanged || keyChanged}
            title={
              baseChanged || keyChanged
                ? t("settings.provider.saveBeforeTest")
                : undefined
            }
            onClick={() => test.mutate()}
          >
            {test.isPending
              ? t("settings.provider.testing")
              : t("settings.provider.test")}
          </button>
          <button
            className="primary-action"
            type="button"
            disabled={
              save.isPending ||
              incompleteEndpoint ||
              (!baseChanged && !keyChanged)
            }
            onClick={() => save.mutate()}
          >
            {save.isPending
              ? t("settings.common.saving")
              : t("settings.common.save")}
          </button>
        </div>
      </footer>
      {save.isError ? (
        <p className="settings-inline-error" role="alert">
          {messageOf(save.error, t)}
        </p>
      ) : null}
      {incompleteEndpoint ? (
        <p className="settings-inline-error" role="alert">
          {t("settings.provider.baseURLRequired")}
        </p>
      ) : null}
    </article>
  );
}

function ModelRoleEditor(props: {
  connection: RuntimeConnection;
  role: "utility" | "embedding";
  providers: Provider[];
}) {
  const { t } = useLocalization();
  const queryClient = useQueryClient();
  const key = runtimeQueryKeys.modelRole(props.connection, props.role);
  const role = useQuery({
    queryKey: key,
    queryFn: ({ signal }) =>
      props.role === "utility"
        ? getUtilityRole(props.connection, signal)
        : getEmbeddingRole(props.connection, signal),
  });
  const [provider, setProvider] = useState("");
  const [model, setModel] = useState("");
  const [dirty, setDirty] = useState(false);
  const available = props.providers.filter(
    (candidate) =>
      (candidate.apiKeyMasked !== "" || candidate.id === "ollama") &&
      (props.role === "utility" || candidate.embeddingCapable),
  );
  useEffect(() => {
    if (dirty || role.data === undefined) return;
    setProvider(role.data.provider ?? "");
    setModel(role.data.model ?? "");
  }, [dirty, role.data]);
  const models = useQuery({
    queryKey: runtimeQueryKeys.models(
      props.connection,
      provider || "unselected",
    ),
    queryFn: ({ signal }) => listModels(props.connection, provider, signal),
    enabled: provider !== "" && props.role === "utility",
    staleTime: 5 * 60_000,
    retry: 1,
  });
  const save = useMutation({
    mutationFn: () => {
      const value = { provider, model };
      return props.role === "utility"
        ? setUtilityRole(props.connection, value as UtilityRole)
        : setEmbeddingRole(props.connection, value as EmbeddingRole);
    },
    onSuccess: (committed) => {
      queryClient.setQueryData(key, committed);
      setDirty(false);
      void queryClient.invalidateQueries({ queryKey: key });
    },
  });
  const selectedProvider = available.find(
    (candidate) => candidate.id === provider,
  );
  const changed =
    provider !== (role.data?.provider ?? "") ||
    model !== (role.data?.model ?? "");

  return (
    <article className="model-role-card">
      <header>
        <div>
          <h3>
            {props.role === "utility"
              ? t("settings.provider.utilityModel")
              : t("settings.provider.embeddingModel")}
          </h3>
          <p>
            {props.role === "utility"
              ? t("settings.provider.utilityDetail")
              : t("settings.provider.embeddingDetail")}
          </p>
        </div>
        <span>
          {role.data?.model
            ? t("settings.provider.assigned")
            : t("settings.common.optional")}
        </span>
      </header>
      {role.isPending ? (
        <SettingsState>{t("settings.provider.loadingRole")}</SettingsState>
      ) : role.isError ? (
        <SettingsState>{messageOf(role.error, t)}</SettingsState>
      ) : (
        <>
          <label>
            <span>{t("settings.provider.provider")}</span>
            <select
              value={provider}
              onChange={(event) => {
                const next = event.currentTarget.value;
                const metadata = available.find(
                  (candidate) => candidate.id === next,
                );
                setProvider(next);
                setModel(
                  props.role === "embedding"
                    ? (metadata?.defaultEmbeddingModel ?? "")
                    : "",
                );
                setDirty(true);
              }}
            >
              <option value="">{t("settings.provider.notAssigned")}</option>
              {available.map((candidate) => (
                <option key={candidate.id} value={candidate.id}>
                  {providerName(candidate.id)}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span>{t("settings.provider.model")}</span>
            <input
              dir="ltr"
              value={model}
              list={`role-models-${props.role}`}
              disabled={provider === ""}
              maxLength={256}
              placeholder={
                props.role === "embedding" &&
                selectedProvider?.defaultEmbeddingModel === ""
                  ? t("settings.provider.deploymentOrModel")
                  : t("settings.provider.modelID")
              }
              onChange={(event) => {
                setModel(event.currentTarget.value);
                setDirty(true);
              }}
            />
            {props.role === "utility" ? (
              <datalist id={`role-models-${props.role}`}>
                {models.data?.data.map((candidate) => (
                  <option key={candidate.id} value={candidate.id} />
                ))}
              </datalist>
            ) : null}
          </label>
          <footer>
            {models.isError ? (
              <span className="settings-inline-error">
                {messageOf(models.error, t)}
              </span>
            ) : (
              <span />
            )}
            <button
              className="secondary-action"
              type="button"
              disabled={
                save.isPending ||
                !changed ||
                (provider === "") !== (model === "")
              }
              onClick={() => save.mutate()}
            >
              {save.isPending
                ? t("settings.common.saving")
                : provider === ""
                  ? t("settings.provider.clearRole")
                  : t("settings.provider.saveRole")}
            </button>
          </footer>
          {save.isError ? (
            <p className="settings-inline-error" role="alert">
              {messageOf(save.error, t)}
            </p>
          ) : null}
        </>
      )}
    </article>
  );
}

function SettingsState(props: {
  children: string;
  action?: string;
  onAction?: () => void;
}) {
  return (
    <div className="settings-state">
      <p>{props.children}</p>
      {props.action && props.onAction ? (
        <button
          className="secondary-action"
          type="button"
          onClick={props.onAction}
        >
          {props.action}
        </button>
      ) : null}
    </div>
  );
}

function providerName(value: string) {
  return value
    .split("-")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function messageOf(error: unknown, t: Translate) {
  return error instanceof Error
    ? error.message
    : t("settings.common.requestFailed");
}
