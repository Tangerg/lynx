import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import type { HookInfo, RuntimeConnection, WorkspaceRef } from "@lyra/runtime-contract";

import {
	listHooks,
	runtimeQueryKeys,
	setHookTrust,
} from "../../runtime/runtimeQueries";

interface HookSettingsProps {
	connection: RuntimeConnection;
	workspace?: WorkspaceRef;
}

export function HookSettings(props: HookSettingsProps) {
	const queryClient = useQueryClient();
	const [confirmingTrust, setConfirmingTrust] = useState(false);
	const key = runtimeQueryKeys.workspaceHooks(
		props.connection,
		props.workspace?.path ?? "unselected",
	);
	const hooks = useQuery({
		queryKey: key,
		queryFn: ({ signal }) => listHooks(props.connection, props.workspace ?? { path: "" }, signal),
		enabled: props.workspace !== undefined,
		retry: 2,
	});
	useEffect(() => setConfirmingTrust(false), [props.workspace?.path]);
	const trust = useMutation({
		mutationFn: (trusted: boolean) => {
			if (!hooks.data?.projectRoot) throw new Error("The Runtime did not report a project root.");
			return setHookTrust(props.connection, hooks.data.projectRoot, trusted);
		},
		onSuccess: () => {
			setConfirmingTrust(false);
			void queryClient.invalidateQueries({ queryKey: runtimeQueryKeys.hooks(props.connection) });
		},
	});
	const projectHooks = hooks.data?.hooks.filter((hook) => hook.scope === "project") ?? [];

	if (props.workspace === undefined) {
		return (
			<section className="settings-section" aria-labelledby="hook-settings-title">
				<header><div><h2 id="hook-settings-title">Effective hooks</h2><p>Hooks are resolved against an exact Session workspace.</p></div></header>
				<HookState>Select a session in the Work Index to review its global and project hooks.</HookState>
			</section>
		);
	}

	return (
		<>
			<section className="settings-section" aria-labelledby="hook-trust-title">
				<header>
					<div>
						<h2 id="hook-trust-title">Project trust</h2>
						<p>Global hooks belong to you and stay active. Cloned project hooks remain inert until this exact project root is trusted.</p>
					</div>
				</header>
				{hooks.isPending ? (
					<HookState>Resolving lifecycle hooks…</HookState>
				) : hooks.isError ? (
					<HookState action="Try again" onAction={() => void hooks.refetch()}>{messageOf(hooks.error)}</HookState>
				) : (
					<div className="hook-trust-card" data-trusted={hooks.data.projectTrusted || undefined}>
						<div>
							<span className="hook-trust-status">{hooks.data.projectTrusted ? "Trusted" : "Not trusted"}</span>
							<strong>{projectHooks.length} project hooks</strong>
							<code title={hooks.data.projectRoot}>{hooks.data.projectRoot || "No project root"}</code>
						</div>
						{hooks.data.projectRoot ? (
							hooks.data.projectTrusted ? (
								<button className="secondary-action" type="button" disabled={trust.isPending} onClick={() => trust.mutate(false)}>{trust.isPending ? "Revoking…" : "Revoke trust"}</button>
							) : projectHooks.length === 0 ? null : confirmingTrust ? (
								<div className="hook-trust-confirm">
									<span>I reviewed the commands and injections below.</span>
									<button type="button" disabled={trust.isPending} onClick={() => setConfirmingTrust(false)}>Cancel</button>
									<button className="danger" type="button" disabled={trust.isPending} onClick={() => trust.mutate(true)}>{trust.isPending ? "Trusting…" : "Trust project"}</button>
								</div>
							) : (
								<button className="primary-action" type="button" onClick={() => setConfirmingTrust(true)}>Review and trust</button>
							)
						) : null}
						{trust.isError ? <p className="settings-inline-error" role="alert">{messageOf(trust.error)}</p> : null}
					</div>
				)}
			</section>

			<section className="settings-section" aria-labelledby="hook-list-title">
				<header>
					<div>
						<h2 id="hook-list-title">Effective cascade</h2>
						<p>Commands are shown verbatim for audit. Inject actions add bounded context without spawning a process.</p>
					</div>
					{hooks.data ? <span className="hook-count">{hooks.data.hooks.length} hooks</span> : null}
				</header>
				{hooks.isPending ? (
					<HookState>Loading hook definitions…</HookState>
				) : hooks.isError ? (
					<HookState>{messageOf(hooks.error)}</HookState>
				) : hooks.data.hooks.length === 0 ? (
					<HookState>No global or project hooks apply to this workspace.</HookState>
				) : (
					<div className="hook-list">
						{hooks.data.hooks.map((hook, index) => <HookCard key={`${hook.source}:${hook.event}:${index}`} hook={hook} />)}
					</div>
				)}
			</section>
		</>
	);
}

function HookCard(props: { hook: HookInfo }) {
	const action = props.hook.command ?? props.hook.inject ?? "";
	const actionKind = props.hook.command ? "Command" : "Inject";
	return (
		<article className="hook-card" data-active={props.hook.active || undefined}>
			<header>
				<div>
					<strong>{props.hook.event}</strong>
					<span>{props.hook.scope}</span>
					{props.hook.matcher ? <code>{props.hook.matcher}</code> : null}
				</div>
				<span className="hook-active-state">{props.hook.active ? "Active" : "Inert"}</span>
			</header>
			<div className="hook-action">
				<span>{actionKind}{props.hook.timeoutMillis ? ` · ${props.hook.timeoutMillis} ms` : ""}</span>
				<pre>{action}</pre>
			</div>
			<footer title={props.hook.source}>{props.hook.source}</footer>
		</article>
	);
}

function HookState(props: { children: string; action?: string; onAction?: () => void }) {
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
