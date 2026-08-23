import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import type {
	ModelUsage,
	RuntimeConnection,
	UsageBucket,
} from "@lyra/runtime-contract";

import {
	loadSessionUsage,
	loadUsageSummary,
	runtimeQueryKeys,
} from "../../runtime/runtimeQueries";

interface UsageSettingsProps {
	connection: RuntimeConnection;
	sessionId?: string;
}

export function UsageSettings(props: UsageSettingsProps) {
	const [sinceDays, setSinceDays] = useState(30);
	const summary = useQuery({
		queryKey: runtimeQueryKeys.usageSummary(props.connection, sinceDays),
		queryFn: ({ signal }) =>
			loadUsageSummary(props.connection, { sinceDays }, signal),
		retry: 2,
	});
	const session = useQuery({
		queryKey: runtimeQueryKeys.sessionUsage(
			props.connection,
			props.sessionId ?? "unselected",
		),
		queryFn: ({ signal }) =>
			loadSessionUsage(props.connection, props.sessionId ?? "", signal),
		enabled: props.sessionId !== undefined,
		retry: 2,
	});

	return (
		<div className="usage-settings">
			<section className="settings-section">
				<header>
					<div>
						<h2>Runtime usage</h2>
						<p>
							Finished Run facts grouped by their exact provider and model identity.
						</p>
					</div>
					<div className="usage-period" aria-label="Usage period">
						{[
							{ label: "7 days", value: 7 },
							{ label: "30 days", value: 30 },
							{ label: "All time", value: 0 },
						].map((period) => (
							<button
								key={period.value}
								type="button"
								aria-pressed={sinceDays === period.value}
								onClick={() => setSinceDays(period.value)}
							>
								{period.label}
							</button>
						))}
					</div>
				</header>
				{summary.isPending ? (
					<UsageState label="Loading usage…" />
				) : summary.isError ? (
					<UsageState
						label={messageOf(summary.error)}
						action="Retry"
						onAction={() => void summary.refetch()}
					/>
				) : summary.data ? (
					<>
						<div className="usage-metrics">
							<UsageMetric label="Tokens" value={formatTokens(totalTokens(summary.data.total))} />
							<UsageMetric label="Cost" value={formatCost(summary.data.total.costUsd)} />
							<UsageMetric label="Runs" value={(summary.data.runs ?? 0).toLocaleString()} />
							<UsageMetric label="Sessions" value={(summary.data.sessions ?? 0).toLocaleString()} />
						</div>
						<p className="usage-cost-note">
							Cost is shown only when every contributing Run has known pricing.
						</p>
						<UsageBreakdown title="Providers" values={summary.data.byProvider ?? []} />
						<UsageBreakdown title="Models" values={summary.data.byModel ?? []} />
						<UsageBreakdown title="Days" values={summary.data.byDay ?? []} />
					</>
				) : null}
			</section>

			<section className="settings-section">
				<header>
					<div>
						<h2>Selected session</h2>
						<p>All finished Runs currently owned by the mounted Session.</p>
					</div>
				</header>
				{props.sessionId === undefined ? (
					<UsageState label="Select a Session to inspect its usage." />
				) : session.isPending ? (
					<UsageState label="Loading Session usage…" />
				) : session.isError ? (
					<UsageState
						label={messageOf(session.error)}
						action="Retry"
						onAction={() => void session.refetch()}
					/>
				) : session.data ? (
					<div className="usage-metrics usage-session-metrics">
						<UsageMetric label="Tokens" value={formatTokens(totalTokens(session.data))} />
						<UsageMetric label="Cost" value={formatCost(session.data.costUsd)} />
						<UsageMetric
							label="Models"
							value={Object.keys(session.data.byModel ?? {}).length.toLocaleString()}
						/>
					</div>
				) : null}
			</section>
		</div>
	);
}

function UsageMetric(props: { label: string; value: string }) {
	return (
		<div>
			<span>{props.label}</span>
			<strong>{props.value}</strong>
		</div>
	);
}

function UsageBreakdown(props: { title: string; values: UsageBucket[] }) {
	if (props.values.length === 0) return null;
	return (
		<section className="usage-breakdown" aria-label={props.title}>
			<h3>{props.title}</h3>
			<div>
				{props.values.slice(0, 10).map((value) => (
					<article key={value.key}>
						<strong title={value.key}>{value.key}</strong>
						<span>{formatTokens(totalTokens(value))}</span>
						<span>{formatCost(value.costUsd)}</span>
						<small>{(value.runs ?? 0).toLocaleString()} runs</small>
					</article>
				))}
			</div>
		</section>
	);
}

function UsageState(props: {
	label: string;
	action?: string;
	onAction?: () => void;
}) {
	return (
		<div className="settings-state">
			<p>{props.label}</p>
			{props.action && props.onAction ? (
				<button type="button" onClick={props.onAction}>{props.action}</button>
			) : null}
		</div>
	);
}

function totalTokens(usage: ModelUsage): number {
	return (usage.inputTokens ?? 0) + (usage.outputTokens ?? 0);
}

function formatTokens(value: number): string {
	if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}m`;
	if (value >= 1_000) return `${(value / 1_000).toFixed(1)}k`;
	return value.toLocaleString();
}

function formatCost(value: number | undefined): string {
	return value === undefined ? "Unknown" : `$${value.toFixed(4)}`;
}

function messageOf(error: unknown): string {
	return error instanceof Error ? error.message : "Usage could not be loaded.";
}
