import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useMemo, useState, type FormEvent } from "react";

import type {
  CreateScheduleRequest,
  RuntimeConnection,
  Schedule,
  UpdateScheduleRequest,
} from "@lyra/runtime-contract";

import {
  createSchedule,
  deleteSchedule,
  listSchedules,
  runScheduleNow,
  runtimeQueryKeys,
  updateSchedule,
} from "../../runtime/runtimeQueries";
import {
  useLocalization,
  type MessageKey,
  type Translate,
} from "../localization/Localization";

type DateTimeFormatter = (
  value: Date,
  options?: Intl.DateTimeFormatOptions,
) => string;

const cronPresets = [
  { label: "settings.schedule.everyHour", value: "0 * * * *" },
  { label: "settings.schedule.weekdaysAt", value: "0 9 * * 1-5" },
  { label: "settings.schedule.dailyAt", value: "0 9 * * *" },
  { label: "settings.schedule.mondayAt", value: "0 9 * * 1" },
] as const satisfies ReadonlyArray<{ label: MessageKey; value: string }>;

interface ScheduleSettingsProps {
  connection: RuntimeConnection;
  onOpenSession: (sessionId: string) => void;
}

interface ScheduleDraft {
  title: string;
  instructions: string;
  workspace: string;
  provider: string;
  model: string;
  cron: string;
}

const emptyDraft: ScheduleDraft = {
  title: "",
  instructions: "",
  workspace: "",
  provider: "",
  model: "",
  cron: cronPresets[2].value,
};

export function ScheduleSettings(props: ScheduleSettingsProps) {
  const { t, formatNumber } = useLocalization();
  const schedules = useInfiniteQuery({
    queryKey: runtimeQueryKeys.schedules(props.connection),
    queryFn: ({ pageParam, signal }) =>
      listSchedules(props.connection, pageParam, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.nextCursor || undefined,
    retry: 2,
  });
  const values = schedules.data?.pages.flatMap((page) => page.data) ?? [];

  return (
    <>
      <CreateScheduleCard connection={props.connection} />
      <section
        className="settings-section"
        aria-labelledby="schedule-list-title"
      >
        <header>
          <div>
            <h2 id="schedule-list-title">
              {t("settings.schedule.recurringWork")}
            </h2>
            <p>{t("settings.schedule.recurringWorkDetail")}</p>
          </div>
          {schedules.data ? (
            <span className="schedule-count">
              {t("settings.schedule.loadedCount", {
                count: formatNumber(values.length),
              })}
            </span>
          ) : null}
        </header>
        {schedules.isPending ? (
          <ScheduleState>{t("settings.schedule.loading")}</ScheduleState>
        ) : schedules.isError ? (
          <ScheduleState
            action={t("settings.common.tryAgain")}
            onAction={() => void schedules.refetch()}
          >
            {messageOf(schedules.error, t)}
          </ScheduleState>
        ) : values.length === 0 ? (
          <ScheduleState>{t("settings.schedule.empty")}</ScheduleState>
        ) : (
          <div className="schedule-list">
            {values.map((schedule) => (
              <ScheduleCard
                key={schedule.id}
                connection={props.connection}
                schedule={schedule}
                onOpenSession={props.onOpenSession}
              />
            ))}
            {schedules.hasNextPage ? (
              <button
                className="secondary-action schedule-load-more"
                type="button"
                disabled={schedules.isFetchingNextPage}
                onClick={() => void schedules.fetchNextPage()}
              >
                {schedules.isFetchingNextPage
                  ? t("settings.common.loading")
                  : t("settings.schedule.loadMore")}
              </button>
            ) : null}
          </div>
        )}
      </section>
    </>
  );
}

function CreateScheduleCard(props: { connection: RuntimeConnection }) {
  const { t } = useLocalization();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<ScheduleDraft>(emptyDraft);
  const selectionComplete = paired(draft.provider, draft.model);
  const create = useMutation({
    mutationFn: () => {
      const request: CreateScheduleRequest = {
        title: draft.title.trim() || undefined,
        instructions: draft.instructions.trim(),
        cron: draft.cron.trim(),
        provider: draft.provider.trim() || undefined,
        model: draft.model.trim() || undefined,
      };
      if (draft.workspace.trim() !== "")
        request.workspace = { path: draft.workspace.trim() };
      return createSchedule(props.connection, request);
    },
    onSuccess: () => {
      setDraft(emptyDraft);
      void queryClient.invalidateQueries({
        queryKey: runtimeQueryKeys.schedules(props.connection),
      });
    },
  });
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (
      draft.instructions.trim() === "" ||
      draft.cron.trim() === "" ||
      !selectionComplete
    )
      return;
    create.mutate();
  };

  return (
    <section
      className="settings-section"
      aria-labelledby="schedule-create-title"
    >
      <header>
        <div>
          <h2 id="schedule-create-title">{t("settings.schedule.new")}</h2>
          <p>{t("settings.schedule.newDetail")}</p>
        </div>
      </header>
      <form className="schedule-editor schedule-editor-new" onSubmit={submit}>
        <ScheduleFields
          draft={draft}
          onChange={setDraft}
          titleId="new-schedule"
        />
        <footer className="schedule-actions">
          <span>{t("settings.schedule.timezoneNote")}</span>
          <button
            className="primary-action"
            type="submit"
            disabled={
              create.isPending ||
              draft.instructions.trim() === "" ||
              draft.cron.trim() === "" ||
              !selectionComplete
            }
          >
            {create.isPending
              ? t("settings.schedule.creating")
              : t("settings.schedule.create")}
          </button>
        </footer>
        {!selectionComplete ? (
          <p className="settings-inline-error" role="alert">
            {t("settings.schedule.providerModelPair")}
          </p>
        ) : null}
        {create.isError ? (
          <p className="settings-inline-error" role="alert">
            {messageOf(create.error, t)}
          </p>
        ) : null}
      </form>
    </section>
  );
}

function ScheduleCard(props: {
  connection: RuntimeConnection;
  schedule: Schedule;
  onOpenSession: (sessionId: string) => void;
}) {
  const { t, formatNumber, formatDateTime } = useLocalization();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState(() => draftOf(props.schedule));
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  useEffect(
    () => setDraft(draftOf(props.schedule)),
    [props.schedule.id, props.schedule.revision],
  );
  const changed = useMemo(
    () => scheduleChanged(props.schedule, draft),
    [draft, props.schedule],
  );
  const selectionComplete = paired(draft.provider, draft.model);
  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: runtimeQueryKeys.schedules(props.connection),
    });
  const save = useMutation({
    mutationFn: () =>
      updateSchedule(props.connection, updateRequest(props.schedule, draft)),
    onSuccess: () => void invalidate(),
    onError: () => void invalidate(),
  });
  const toggle = useMutation({
    mutationFn: () =>
      updateSchedule(props.connection, {
        id: props.schedule.id,
        expectedRevision: props.schedule.revision,
        enabled: !props.schedule.enabled,
      }),
    onSuccess: () => void invalidate(),
    onError: () => void invalidate(),
  });
  const runNow = useMutation({
    mutationFn: () => runScheduleNow(props.connection, props.schedule.id),
    onSuccess: (started) => {
      void queryClient.invalidateQueries({
        queryKey: runtimeQueryKeys.sessions(props.connection),
      });
      props.onOpenSession(started.sessionId);
    },
    onError: () => void invalidate(),
  });
  const remove = useMutation({
    mutationFn: () => deleteSchedule(props.connection, props.schedule.id),
    onSuccess: () => {
      setConfirmingDelete(false);
      void invalidate();
    },
  });
  const pending =
    save.isPending || toggle.isPending || runNow.isPending || remove.isPending;
  const failure = save.error ?? toggle.error ?? runNow.error ?? remove.error;

  return (
    <article
      className="schedule-editor"
      data-enabled={props.schedule.enabled || undefined}
    >
      <header className="schedule-card-heading">
        <div>
          <span className="schedule-status">
            {props.schedule.enabled
              ? t("settings.schedule.enabled")
              : t("settings.schedule.paused")}
          </span>
          <code>{props.schedule.id}</code>
        </div>
        <div>
          <button
            className="secondary-action"
            type="button"
            disabled={pending || changed}
            title={changed ? t("settings.schedule.saveBeforeRun") : undefined}
            onClick={() => runNow.mutate()}
          >
            {runNow.isPending
              ? t("settings.schedule.starting")
              : t("settings.schedule.runNow")}
          </button>
          <button
            className="secondary-action"
            type="button"
            disabled={pending || changed}
            title={
              changed ? t("settings.schedule.saveBeforeToggle") : undefined
            }
            onClick={() => toggle.mutate()}
          >
            {toggle.isPending
              ? t("settings.common.saving")
              : props.schedule.enabled
                ? t("settings.schedule.pause")
                : t("settings.schedule.enable")}
          </button>
        </div>
      </header>
      <ScheduleFields
        draft={draft}
        onChange={setDraft}
        titleId={props.schedule.id}
      />
      <div className="schedule-facts">
        <span>
          <b>{t("settings.schedule.next")}</b>
          {formatTime(props.schedule.nextRunAt, formatDateTime) ??
            t("settings.schedule.paused")}
        </span>
        <span>
          <b>{t("settings.schedule.lastAdmitted")}</b>
          {formatTime(props.schedule.lastRunAt, formatDateTime) ??
            t("settings.schedule.never")}
        </span>
        <span>
          <b>{t("settings.schedule.revision")}</b>
          {formatNumber(props.schedule.revision)}
        </span>
      </div>
      <footer className="schedule-actions">
        {confirmingDelete ? (
          <div className="schedule-delete-confirm">
            <span>{t("settings.schedule.deleteQuestion")}</span>
            <button
              type="button"
              disabled={pending}
              onClick={() => setConfirmingDelete(false)}
            >
              {t("settings.common.cancel")}
            </button>
            <button
              className="danger"
              type="button"
              disabled={pending}
              onClick={() => remove.mutate()}
            >
              {remove.isPending
                ? t("settings.common.deleting")
                : t("settings.common.delete")}
            </button>
          </div>
        ) : (
          <button
            className="text-action danger"
            type="button"
            disabled={pending}
            onClick={() => setConfirmingDelete(true)}
          >
            {t("settings.common.delete")}
          </button>
        )}
        <div>
          {changed ? (
            <button
              className="text-action"
              type="button"
              disabled={pending}
              onClick={() => setDraft(draftOf(props.schedule))}
            >
              {t("settings.common.discard")}
            </button>
          ) : null}
          <button
            className="primary-action"
            type="button"
            disabled={
              pending ||
              !changed ||
              draft.instructions.trim() === "" ||
              draft.cron.trim() === "" ||
              !selectionComplete
            }
            onClick={() => save.mutate()}
          >
            {save.isPending
              ? t("settings.common.saving")
              : t("settings.common.saveChanges")}
          </button>
        </div>
      </footer>
      {!selectionComplete ? (
        <p className="settings-inline-error" role="alert">
          {t("settings.schedule.providerModelPair")}
        </p>
      ) : null}
      {failure ? (
        <p className="settings-inline-error" role="alert">
          {messageOf(failure, t)}
        </p>
      ) : null}
    </article>
  );
}

function ScheduleFields(props: {
  draft: ScheduleDraft;
  onChange: (draft: ScheduleDraft) => void;
  titleId: string;
}) {
  const { t } = useLocalization();
  const update = <Key extends keyof ScheduleDraft>(
    key: Key,
    value: ScheduleDraft[Key],
  ) => props.onChange({ ...props.draft, [key]: value });
  return (
    <div className="schedule-fields">
      <label className="schedule-field-title">
        <span>
          {t("settings.schedule.title")}{" "}
          <small>{t("settings.common.optional")}</small>
        </span>
        <input
          value={props.draft.title}
          maxLength={512}
          placeholder={t("settings.schedule.defaultTitle")}
          onChange={(event) => update("title", event.currentTarget.value)}
        />
      </label>
      <label className="schedule-field-cron">
        <span>{t("settings.schedule.cron")}</span>
        <input
          dir="ltr"
          value={props.draft.cron}
          maxLength={512}
          list={`cron-presets-${props.titleId}`}
          spellCheck={false}
          onChange={(event) => update("cron", event.currentTarget.value)}
        />
        <datalist id={`cron-presets-${props.titleId}`}>
          {cronPresets.map((preset) => (
            <option key={preset.value} value={preset.value}>
              {t(preset.label)}
            </option>
          ))}
        </datalist>
      </label>
      <label className="schedule-field-instructions">
        <span>{t("settings.schedule.instructions")}</span>
        <textarea
          value={props.draft.instructions}
          maxLength={65_536}
          rows={4}
          placeholder={t("settings.schedule.instructionsPlaceholder")}
          onChange={(event) =>
            update("instructions", event.currentTarget.value)
          }
        />
      </label>
      <label>
        <span>
          {t("settings.schedule.workspace")}{" "}
          <small>{t("settings.schedule.emptyUsesDefault")}</small>
        </span>
        <input
          dir="ltr"
          value={props.draft.workspace}
          maxLength={4096}
          placeholder={t("settings.schedule.runtimeDefault")}
          spellCheck={false}
          onChange={(event) => update("workspace", event.currentTarget.value)}
        />
      </label>
      <label>
        <span>
          {t("settings.provider.provider")}{" "}
          <small>{t("settings.schedule.optionalPair")}</small>
        </span>
        <input
          dir="ltr"
          value={props.draft.provider}
          maxLength={256}
          placeholder={t("settings.schedule.runtimeDefault")}
          spellCheck={false}
          onChange={(event) => update("provider", event.currentTarget.value)}
        />
      </label>
      <label>
        <span>
          {t("settings.provider.model")}{" "}
          <small>{t("settings.schedule.optionalPair")}</small>
        </span>
        <input
          dir="ltr"
          value={props.draft.model}
          maxLength={256}
          placeholder={t("settings.schedule.runtimeDefault")}
          spellCheck={false}
          onChange={(event) => update("model", event.currentTarget.value)}
        />
      </label>
    </div>
  );
}

function draftOf(value: Schedule): ScheduleDraft {
  return {
    title: value.title,
    instructions: value.instructions,
    workspace: value.workspace?.path ?? "",
    provider: value.provider ?? "",
    model: value.model ?? "",
    cron: value.cron,
  };
}

function scheduleChanged(value: Schedule, draft: ScheduleDraft): boolean {
  const current = draftOf(value);
  const normalized: ScheduleDraft = {
    title: draft.title.trim() || "Scheduled task",
    instructions: draft.instructions.trim(),
    workspace: draft.workspace.trim(),
    provider: draft.provider.trim(),
    model: draft.model.trim(),
    cron: draft.cron.trim(),
  };
  return (Object.keys(current) as Array<keyof ScheduleDraft>).some(
    (key) => current[key] !== normalized[key],
  );
}

function updateRequest(
  value: Schedule,
  draft: ScheduleDraft,
): UpdateScheduleRequest {
  const request: UpdateScheduleRequest = {
    id: value.id,
    expectedRevision: value.revision,
  };
  const title = draft.title.trim() || "Scheduled task";
  const instructions = draft.instructions.trim();
  const cron = draft.cron.trim();
  const workspace = draft.workspace.trim();
  const provider = draft.provider.trim();
  const model = draft.model.trim();
  if (title !== value.title) request.title = title;
  if (instructions !== value.instructions) request.instructions = instructions;
  if (cron !== value.cron) request.cron = cron;
  if (workspace !== (value.workspace?.path ?? "")) {
    if (workspace === "") request.workspaceMode = "default";
    else request.workspace = { path: workspace };
  }
  if (provider !== (value.provider ?? "") || model !== (value.model ?? "")) {
    request.provider = provider;
    request.model = model;
  }
  return request;
}

function paired(provider: string, model: string): boolean {
  return (provider.trim() === "") === (model.trim() === "");
}

function formatTime(
  value: string | undefined,
  formatDateTime: DateTimeFormatter,
): string | undefined {
  if (!value) return undefined;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return undefined;
  return formatDateTime(date, {
    dateStyle: "medium",
    timeStyle: "short",
  });
}

function ScheduleState(props: {
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

function messageOf(error: unknown, t: Translate) {
  return error instanceof Error
    ? error.message
    : t("settings.common.requestFailed");
}
