import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { Bot, ChevronDown, X } from "lucide-react";

import type { Model, RuntimeConnection, Session } from "@lyra/runtime-contract";

import { useLocalization } from "../localization/Localization";
import {
  listModels,
  listProviders,
  runtimeQueryKeys,
} from "../../runtime/runtimeQueries";
import { Icon } from "../shell/Icon";

interface SessionModelPickerProps {
  connection: RuntimeConnection;
  session: Session;
  disabled: boolean;
  onChange(provider: string, model: string): Promise<unknown>;
}

export function SessionModelPicker(props: SessionModelPickerProps) {
  const { t } = useLocalization();
  const root = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [provider, setProvider] = useState(props.session.provider ?? "");
  const [error, setError] = useState<string>();
  const [savingModel, setSavingModel] = useState<string>();
  const providers = useQuery({
    queryKey: runtimeQueryKeys.providers(props.connection),
    queryFn: ({ signal }) => listProviders(props.connection, signal),
    staleTime: 60_000,
  });
  const configuredProviders = useMemo(
    () =>
      (providers.data?.data ?? []).filter(
        (candidate) =>
          candidate.apiKeyMasked !== "" || candidate.id === "ollama",
      ),
    [providers.data],
  );

  useEffect(() => {
    if (!open) return;
    setProvider(
      (current) =>
        props.session.provider || current || configuredProviders[0]?.id || "",
    );
  }, [configuredProviders, open, props.session.provider]);

  useEffect(() => {
    if (!open) return;
    const close = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.isComposing || event.keyCode === 229)
        return;
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
    };
    document.addEventListener("pointerdown", close);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", close);
      document.removeEventListener("keydown", escape);
    };
  }, [open]);

  const models = useQuery({
    queryKey: runtimeQueryKeys.models(
      props.connection,
      provider || "unselected",
    ),
    queryFn: ({ signal }) => listModels(props.connection, provider, signal),
    enabled: open && provider !== "",
    staleTime: 5 * 60_000,
    retry: 1,
  });
  const choose = async (model: Model) => {
    if (
      model.provider === props.session.provider &&
      model.id === props.session.model
    ) {
      setOpen(false);
      return;
    }
    setError(undefined);
    setSavingModel(model.id);
    try {
      await props.onChange(model.provider, model.id);
      setOpen(false);
    } catch (cause) {
      setError(messageOf(cause, t("model.catalogUnavailable")));
    } finally {
      setSavingModel(undefined);
    }
  };

  return (
    <div className="session-model-picker" ref={root}>
      <button
        className="composer-tool model-picker-trigger"
        type="button"
        disabled={props.disabled}
        aria-haspopup="dialog"
        aria-expanded={open}
        title={
          props.session.model
            ? `${props.session.provider} / ${props.session.model}`
            : t("model.chooseStored")
        }
        onClick={() => {
          setError(undefined);
          setOpen((current) => !current);
        }}
      >
        <Icon glyph={Bot} size="sm" />
        {props.session.model || t("model.choose")}
        <Icon glyph={ChevronDown} size="xs" />
      </button>
      {open ? (
        <section
          className="model-picker-popover"
          role="dialog"
          aria-label={t("model.choose")}
        >
          <header>
            <div>
              <strong>{t("model.sessionModel")}</strong>
              <p>{t("model.explicitPair")}</p>
            </div>
            <button
              type="button"
              aria-label={t("model.closePicker")}
              onClick={() => setOpen(false)}
            >
              <Icon glyph={X} size="sm" />
            </button>
          </header>
          {providers.isPending ? (
            <ModelPickerState>{t("model.loadingProviders")}</ModelPickerState>
          ) : providers.isError ? (
            <ModelPickerState error>
              {messageOf(providers.error, t("model.catalogUnavailable"))}
            </ModelPickerState>
          ) : configuredProviders.length === 0 ? (
            <ModelPickerState>{t("model.noProvider")}</ModelPickerState>
          ) : (
            <>
              <nav aria-label={t("model.configuredProviders")}>
                {configuredProviders.map((candidate) => (
                  <button
                    key={candidate.id}
                    type="button"
                    aria-current={
                      candidate.id === provider ? "page" : undefined
                    }
                    onClick={() => {
                      setError(undefined);
                      setProvider(candidate.id);
                    }}
                  >
                    {providerName(candidate.id)}
                  </button>
                ))}
              </nav>
              <div className="model-picker-list">
                {models.isPending ? (
                  <ModelPickerState>
                    {t("model.loadingModels")}
                  </ModelPickerState>
                ) : models.isError ? (
                  <ModelPickerState error>
                    {messageOf(models.error, t("model.catalogUnavailable"))}
                  </ModelPickerState>
                ) : models.data?.data.length === 0 ? (
                  <ModelPickerState>{t("model.noModels")}</ModelPickerState>
                ) : (
                  models.data?.data.map((model) => (
                    <button
                      key={`${model.provider}:${model.id}`}
                      type="button"
                      data-selected={
                        model.provider === props.session.provider &&
                        model.id === props.session.model
                      }
                      disabled={savingModel !== undefined}
                      onClick={() => void choose(model)}
                    >
                      <span>
                        <strong>{model.displayName || model.id}</strong>
                        <small>{model.id}</small>
                      </span>
                      <ModelFacts model={model} />
                      <b>
                        {savingModel === model.id
                          ? t("model.saving")
                          : t("model.select")}
                      </b>
                    </button>
                  ))
                )}
              </div>
            </>
          )}
          {error ? (
            <p className="model-picker-error" role="alert">
              {error}
            </p>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}

function ModelFacts({ model }: { model: Model }) {
  const { t } = useLocalization();
  const facts = [
    model.contextWindow
      ? t("model.contextWindow", { count: formatTokens(model.contextWindow) })
      : undefined,
    model.capabilities?.reasoning ? t("model.reasoning") : undefined,
    model.capabilities?.multimodal ? t("model.images") : undefined,
    model.capabilities?.toolUse ? t("model.tools") : undefined,
  ].filter((value): value is string => value !== undefined);
  return <small>{facts.join(" · ") || t("model.catalogEntry")}</small>;
}

function ModelPickerState(props: { children: string; error?: boolean }) {
  return (
    <p className="model-picker-state" data-error={props.error || undefined}>
      {props.children}
    </p>
  );
}

function providerName(value: string) {
  return value
    .split("-")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function formatTokens(value: number) {
  return value >= 1_000_000
    ? `${(value / 1_000_000).toFixed(1)}m`
    : value >= 1_000
      ? `${Math.round(value / 1_000)}k`
      : String(value);
}

function messageOf(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}
