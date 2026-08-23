import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
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

const cronPresets = [
	{ label: "Every hour", value: "0 * * * *" },
	{ label: "Weekdays · 09:00", value: "0 9 * * 1-5" },
	{ label: "Daily · 09:00", value: "0 9 * * *" },
	{ label: "Monday · 09:00", value: "0 9 * * 1" },
] as const;

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
	const schedules = useInfiniteQuery({
		queryKey: runtimeQueryKeys.schedules(props.connection),
		queryFn: ({ pageParam, signal }) => listSchedules(props.connection, pageParam, signal),
		initialPageParam: undefined as string | undefined,
		getNextPageParam: (page) => page.nextCursor || undefined,
		retry: 2,
	});
	const values = schedules.data?.pages.flatMap((page) => page.data) ?? [];

	return (
		<>
			<CreateScheduleCard connection={props.connection} />
			<section className="settings-section" aria-labelledby="schedule-list-title">
				<header>
					<div>
						<h2 id="schedule-list-title">Recurring work</h2>
						<p>A due occurrence is persisted before launch, so restart recovery keeps one stable Session and Run identity.</p>
					</div>
					{schedules.data ? <span className="schedule-count">{values.length} loaded</span> : null}
				</header>
				{schedules.isPending ? (
					<ScheduleState>Loading schedules…</ScheduleState>
				) : schedules.isError ? (
					<ScheduleState action="Try again" onAction={() => void schedules.refetch()}>
						{messageOf(schedules.error)}
					</ScheduleState>
				) : values.length === 0 ? (
					<ScheduleState>No schedules yet. Create one above when recurring work is intentional.</ScheduleState>
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
							<button className="secondary-action schedule-load-more" type="button" disabled={schedules.isFetchingNextPage} onClick={() => void schedules.fetchNextPage()}>
								{schedules.isFetchingNextPage ? "Loading…" : "Load more schedules"}
							</button>
						) : null}
						{schedules.isFetchNextPageError ? <p className="settings-inline-error" role="alert">{messageOf(schedules.error)}</p> : null}
					</div>
				)}
			</section>
		</>
	);
}

function CreateScheduleCard(props: { connection: RuntimeConnection }) {
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
			if (draft.workspace.trim() !== "") request.workspace = { path: draft.workspace.trim() };
			return createSchedule(props.connection, request);
		},
		onSuccess: () => {
			setDraft(emptyDraft);
			void queryClient.invalidateQueries({ queryKey: runtimeQueryKeys.schedules(props.connection) });
		},
	});
	const submit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (draft.instructions.trim() === "" || draft.cron.trim() === "" || !selectionComplete) return;
		create.mutate();
	};

	return (
		<section className="settings-section" aria-labelledby="schedule-create-title">
			<header>
				<div>
					<h2 id="schedule-create-title">New schedule</h2>
					<p>Write self-contained instructions; every occurrence opens a clean Session.</p>
				</div>
			</header>
			<form className="schedule-editor schedule-editor-new" onSubmit={submit}>
				<ScheduleFields draft={draft} onChange={setDraft} titleId="new-schedule" />
				<footer className="schedule-actions">
					<span>Times use the Runtime host timezone.</span>
					<button
						className="primary-action"
						type="submit"
						disabled={create.isPending || draft.instructions.trim() === "" || draft.cron.trim() === "" || !selectionComplete}
					>
						{create.isPending ? "Creating…" : "Create schedule"}
					</button>
				</footer>
				{!selectionComplete ? <p className="settings-inline-error" role="alert">Provider and model must be set together, or both left empty.</p> : null}
				{create.isError ? <p className="settings-inline-error" role="alert">{messageOf(create.error)}</p> : null}
			</form>
		</section>
	);
}

function ScheduleCard(props: {
	connection: RuntimeConnection;
	schedule: Schedule;
	onOpenSession: (sessionId: string) => void;
}) {
	const queryClient = useQueryClient();
	const [draft, setDraft] = useState(() => draftOf(props.schedule));
	const [confirmingDelete, setConfirmingDelete] = useState(false);
	useEffect(
		() => setDraft(draftOf(props.schedule)),
		[props.schedule.id, props.schedule.revision],
	);
	const changed = useMemo(() => scheduleChanged(props.schedule, draft), [draft, props.schedule]);
	const selectionComplete = paired(draft.provider, draft.model);
	const invalidate = () => queryClient.invalidateQueries({ queryKey: runtimeQueryKeys.schedules(props.connection) });
	const save = useMutation({
		mutationFn: () => updateSchedule(props.connection, updateRequest(props.schedule, draft)),
		onSuccess: () => void invalidate(),
		onError: () => void invalidate(),
	});
	const toggle = useMutation({
		mutationFn: () => updateSchedule(props.connection, {
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
			void queryClient.invalidateQueries({ queryKey: runtimeQueryKeys.sessions(props.connection) });
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
	const pending = save.isPending || toggle.isPending || runNow.isPending || remove.isPending;
	const failure = save.error ?? toggle.error ?? runNow.error ?? remove.error;

	return (
		<article className="schedule-editor" data-enabled={props.schedule.enabled || undefined}>
			<header className="schedule-card-heading">
				<div>
					<span className="schedule-status">{props.schedule.enabled ? "Enabled" : "Paused"}</span>
					<code>{props.schedule.id}</code>
				</div>
				<div>
					<button className="secondary-action" type="button" disabled={pending || changed} title={changed ? "Save or discard draft edits before firing" : undefined} onClick={() => runNow.mutate()}>
						{runNow.isPending ? "Starting…" : "Run now"}
					</button>
					<button className="secondary-action" type="button" disabled={pending || changed} title={changed ? "Save or discard draft edits first" : undefined} onClick={() => toggle.mutate()}>
						{toggle.isPending ? "Saving…" : props.schedule.enabled ? "Pause" : "Enable"}
					</button>
				</div>
			</header>
			<ScheduleFields draft={draft} onChange={setDraft} titleId={props.schedule.id} />
			<div className="schedule-facts">
				<span><b>Next</b>{formatTime(props.schedule.nextRunAt) ?? "Paused"}</span>
				<span><b>Last admitted</b>{formatTime(props.schedule.lastRunAt) ?? "Never"}</span>
				<span><b>Revision</b>{props.schedule.revision}</span>
			</div>
			<footer className="schedule-actions">
				{confirmingDelete ? (
					<div className="schedule-delete-confirm">
						<span>Delete this recurring schedule?</span>
						<button type="button" disabled={pending} onClick={() => setConfirmingDelete(false)}>Cancel</button>
						<button className="danger" type="button" disabled={pending} onClick={() => remove.mutate()}>{remove.isPending ? "Deleting…" : "Delete"}</button>
					</div>
				) : (
					<button className="text-action danger" type="button" disabled={pending} onClick={() => setConfirmingDelete(true)}>Delete</button>
				)}
				<div>
					{changed ? <button className="text-action" type="button" disabled={pending} onClick={() => setDraft(draftOf(props.schedule))}>Discard</button> : null}
					<button className="primary-action" type="button" disabled={pending || !changed || draft.instructions.trim() === "" || draft.cron.trim() === "" || !selectionComplete} onClick={() => save.mutate()}>
						{save.isPending ? "Saving…" : "Save changes"}
					</button>
				</div>
			</footer>
			{!selectionComplete ? <p className="settings-inline-error" role="alert">Provider and model must be set together, or both left empty.</p> : null}
			{failure ? <p className="settings-inline-error" role="alert">{messageOf(failure)}</p> : null}
		</article>
	);
}

function ScheduleFields(props: {
	draft: ScheduleDraft;
	onChange: (draft: ScheduleDraft) => void;
	titleId: string;
}) {
	const update = <Key extends keyof ScheduleDraft>(key: Key, value: ScheduleDraft[Key]) =>
		props.onChange({ ...props.draft, [key]: value });
	return (
		<div className="schedule-fields">
			<label className="schedule-field-title">
				<span>Title <small>Optional</small></span>
				<input value={props.draft.title} maxLength={512} placeholder="Scheduled task" onChange={(event) => update("title", event.currentTarget.value)} />
			</label>
			<label className="schedule-field-cron">
				<span>Five-field cron</span>
				<input value={props.draft.cron} maxLength={512} list={`cron-presets-${props.titleId}`} spellCheck={false} onChange={(event) => update("cron", event.currentTarget.value)} />
				<datalist id={`cron-presets-${props.titleId}`}>{cronPresets.map((preset) => <option key={preset.value} value={preset.value}>{preset.label}</option>)}</datalist>
			</label>
			<label className="schedule-field-instructions">
				<span>Instructions</span>
				<textarea value={props.draft.instructions} maxLength={65_536} rows={4} placeholder="Complete instructions for every Run…" onChange={(event) => update("instructions", event.currentTarget.value)} />
			</label>
			<label>
				<span>Workspace <small>Empty uses Runtime default</small></span>
				<input value={props.draft.workspace} maxLength={4096} placeholder="Runtime default" spellCheck={false} onChange={(event) => update("workspace", event.currentTarget.value)} />
			</label>
			<label>
				<span>Provider <small>Optional pair</small></span>
				<input value={props.draft.provider} maxLength={256} placeholder="Runtime default" spellCheck={false} onChange={(event) => update("provider", event.currentTarget.value)} />
			</label>
			<label>
				<span>Model <small>Optional pair</small></span>
				<input value={props.draft.model} maxLength={256} placeholder="Runtime default" spellCheck={false} onChange={(event) => update("model", event.currentTarget.value)} />
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

function updateRequest(value: Schedule, draft: ScheduleDraft): UpdateScheduleRequest {
	const request: UpdateScheduleRequest = { id: value.id, expectedRevision: value.revision };
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

function formatTime(value?: string): string | undefined {
	if (!value) return undefined;
	return new Intl.DateTimeFormat(undefined, {
		dateStyle: "medium",
		timeStyle: "short",
	}).format(new Date(value));
}

function ScheduleState(props: { children: string; action?: string; onAction?: () => void }) {
	return (
		<div className="settings-state">
			<p>{props.children}</p>
			{props.action && props.onAction ? <button className="secondary-action" type="button" onClick={props.onAction}>{props.action}</button> : null}
		</div>
	);
}

function messageOf(error: unknown) {
	return error instanceof Error ? error.message : "The Runtime request failed.";
}
