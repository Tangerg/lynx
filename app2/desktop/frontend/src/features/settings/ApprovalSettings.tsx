import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import type { ApprovalRule, RuntimeConnection } from "@lyra/runtime-contract";

import {
	forgetApprovalRule,
	getApprovalMode,
	listApprovalRules,
	runtimeQueryKeys,
	setApprovalMode,
} from "../../runtime/runtimeQueries";

type ApprovalMode = "safe" | "balanced" | "yolo";

const modes: Array<{
	id: ApprovalMode;
	name: string;
	description: string;
	badge: string;
}> = [
	{
		id: "safe",
		name: "Safe",
		description: "Confirm workspace writes, command execution, and network access.",
		badge: "Most review",
	},
	{
		id: "balanced",
		name: "Balanced",
		description: "Confirm command execution; allow ordinary writes and network tools.",
		badge: "Default",
	},
	{
		id: "yolo",
		name: "Yolo",
		description: "Allow ordinary effects without prompts. Catastrophic commands still require review.",
		badge: "Least review",
	},
];

interface ApprovalSettingsProps {
	connection: RuntimeConnection;
	sessionId?: string;
}

export function ApprovalSettings(props: ApprovalSettingsProps) {
	const queryClient = useQueryClient();
	const [confirmingRule, setConfirmingRule] = useState<string>();
	const modeKey = runtimeQueryKeys.approvalMode(props.connection);
	const mode = useQuery({
		queryKey: modeKey,
		queryFn: ({ signal }) => getApprovalMode(props.connection, signal),
		retry: 2,
	});
	const rulesKey = runtimeQueryKeys.approvalRules(
		props.connection,
		props.sessionId ?? "unselected",
	);
	const rules = useQuery({
		queryKey: rulesKey,
		queryFn: ({ signal }) =>
			listApprovalRules(props.connection, props.sessionId ?? "", signal),
		enabled: props.sessionId !== undefined,
		retry: 2,
	});
	const changeMode = useMutation({
		mutationFn: (next: ApprovalMode) => setApprovalMode(props.connection, next),
		onSuccess: (committed) => {
			queryClient.setQueryData(modeKey, committed);
			void queryClient.invalidateQueries({
				queryKey: runtimeQueryKeys.approvals(props.connection),
			});
		},
	});
	const forget = useMutation({
		mutationFn: (id: string) => forgetApprovalRule(props.connection, id),
		onSuccess: () => {
			setConfirmingRule(undefined);
			void queryClient.invalidateQueries({ queryKey: rulesKey });
		},
	});

	return (
		<>
			<section className="settings-section" aria-labelledby="approval-mode-title">
				<header>
					<div>
						<h2 id="approval-mode-title">Effect stance</h2>
						<p>The Runtime reads this setting at every tool effect, including Runs already in progress.</p>
					</div>
				</header>
				{mode.isPending ? (
					<ApprovalState>Loading approval mode…</ApprovalState>
				) : mode.isError ? (
					<ApprovalState action="Try again" onAction={() => void mode.refetch()}>
						{messageOf(mode.error)}
					</ApprovalState>
				) : (
					<div className="approval-mode-grid">
						{modes.map((candidate) => {
							const selected = mode.data?.mode === candidate.id;
							return (
								<button
									key={candidate.id}
									className="approval-mode-card"
									type="button"
									data-selected={selected || undefined}
									aria-pressed={selected}
									disabled={changeMode.isPending}
									onClick={() => changeMode.mutate(candidate.id)}
								>
									<span><strong>{candidate.name}</strong><small>{candidate.badge}</small></span>
									<p>{candidate.description}</p>
								</button>
							);
						})}
					</div>
				)}
				{changeMode.isError ? <p className="settings-inline-error" role="alert">{messageOf(changeMode.error)}</p> : null}
			</section>

			<section className="settings-section" aria-labelledby="approval-rules-title">
				<header>
					<div>
						<h2 id="approval-rules-title">Remembered decisions</h2>
						<p>Session rules take priority over project rules, then global rules. Equal conflicts deny.</p>
					</div>
					{rules.data ? <span className="approval-rule-count">{rules.data.rules.length} visible</span> : null}
				</header>
				{props.sessionId === undefined ? (
					<ApprovalState>Select a session in the Work Index to inspect its visible rules.</ApprovalState>
				) : rules.isPending ? (
					<ApprovalState>Loading remembered decisions…</ApprovalState>
				) : rules.isError ? (
					<ApprovalState action="Try again" onAction={() => void rules.refetch()}>
						{messageOf(rules.error)}
					</ApprovalState>
				) : rules.data.rules.length === 0 ? (
					<ApprovalState>No remembered decisions are visible to this session.</ApprovalState>
				) : (
					<div className="approval-rule-list">
						{rules.data.rules.map((rule) => (
							<ApprovalRuleCard
								key={rule.id}
								rule={rule}
								confirming={confirmingRule === rule.id}
								pending={forget.isPending && forget.variables === rule.id}
								onConfirm={() => setConfirmingRule(rule.id)}
								onCancel={() => setConfirmingRule(undefined)}
								onForget={() => forget.mutate(rule.id)}
							/>
						))}
					</div>
				)}
				{forget.isError ? <p className="settings-inline-error" role="alert">{messageOf(forget.error)}</p> : null}
			</section>
		</>
	);
}

function ApprovalRuleCard(props: {
	rule: ApprovalRule;
	confirming: boolean;
	pending: boolean;
	onConfirm(): void;
	onCancel(): void;
	onForget(): void;
}) {
	const subject = props.rule.subject === "" ? "Every invocation" : props.rule.subject;
	return (
		<article className="approval-rule-card" data-decision={props.rule.decision}>
			<header>
				<div>
					<span className="approval-rule-verdict">{props.rule.decision}</span>
					<span className="approval-rule-scope">{scopeName(props.rule.scope)}</span>
				</div>
				{props.confirming ? (
					<div className="approval-forget-confirm">
						<span>Forget this decision?</span>
						<button type="button" disabled={props.pending} onClick={props.onCancel}>Cancel</button>
						<button className="danger" type="button" disabled={props.pending} onClick={props.onForget}>{props.pending ? "Forgetting…" : "Forget"}</button>
					</div>
				) : (
					<button className="text-action danger" type="button" onClick={props.onConfirm}>Forget</button>
				)}
			</header>
			<div className="approval-rule-key">
				<strong>{props.rule.tool}</strong>
				<code title={subject}>{subject}</code>
			</div>
			{props.rule.dir ? <p title={props.rule.dir}>Project · {props.rule.dir}</p> : null}
		</article>
	);
}

function ApprovalState(props: { children: string; action?: string; onAction?: () => void }) {
	return (
		<div className="settings-state">
			<p>{props.children}</p>
			{props.action && props.onAction ? <button className="secondary-action" type="button" onClick={props.onAction}>{props.action}</button> : null}
		</div>
	);
}

function scopeName(scope: string) {
	switch (scope) {
	case "session": return "This session";
	case "project": return "This project";
	case "global": return "Everywhere";
	default: return scope;
	}
}

function messageOf(error: unknown) {
	return error instanceof Error ? error.message : "The Runtime request failed.";
}
