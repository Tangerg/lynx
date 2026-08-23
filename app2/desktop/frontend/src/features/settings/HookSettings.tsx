import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import type { HookInfo, RuntimeConnection, WorkspaceRef } from "@lyra/runtime-contract";

import {
	listHooks,
	runtimeQueryKeys,
	setHookTrust,
} from "../../runtime/runtimeQueries";
import { useLocalization, type Translate } from "../localization/Localization";

interface HookSettingsProps {
	connection: RuntimeConnection;
	workspace?: WorkspaceRef;
}

export function HookSettings(props: HookSettingsProps) {
	const { t, formatNumber } = useLocalization();
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
			if (!hooks.data?.projectRoot) throw new MissingProjectRootError();
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
				<header><div><h2 id="hook-settings-title">{t("settings.hook.effective")}</h2><p>{t("settings.hook.effectiveDetail")}</p></div></header>
				<HookState>{t("settings.hook.selectSession")}</HookState>
			</section>
		);
	}

	return (
		<>
			<section className="settings-section" aria-labelledby="hook-trust-title">
				<header>
					<div>
						<h2 id="hook-trust-title">{t("settings.hook.projectTrust")}</h2>
						<p>{t("settings.hook.projectTrustDetail")}</p>
					</div>
				</header>
				{hooks.isPending ? (
					<HookState>{t("settings.hook.resolving")}</HookState>
				) : hooks.isError ? (
					<HookState action={t("settings.common.tryAgain")} onAction={() => void hooks.refetch()}>{messageOf(hooks.error, t)}</HookState>
				) : (
					<div className="hook-trust-card" data-trusted={hooks.data.projectTrusted || undefined}>
						<div>
							<span className="hook-trust-status">{hooks.data.projectTrusted ? t("settings.hook.trusted") : t("settings.hook.notTrusted")}</span>
							<strong>{t(projectHooks.length === 1 ? "settings.hook.projectCountOne" : "settings.hook.projectCountMany", { count: formatNumber(projectHooks.length) })}</strong>
							<code title={hooks.data.projectRoot}>{hooks.data.projectRoot || t("settings.hook.noProjectRoot")}</code>
						</div>
						{hooks.data.projectRoot ? (
							hooks.data.projectTrusted ? (
								<button className="secondary-action" type="button" disabled={trust.isPending} onClick={() => trust.mutate(false)}>{trust.isPending ? t("settings.hook.revoking") : t("settings.hook.revoke")}</button>
							) : projectHooks.length === 0 ? null : confirmingTrust ? (
								<div className="hook-trust-confirm">
									<span>{t("settings.hook.reviewed")}</span>
									<button type="button" disabled={trust.isPending} onClick={() => setConfirmingTrust(false)}>{t("settings.common.cancel")}</button>
									<button className="danger" type="button" disabled={trust.isPending} onClick={() => trust.mutate(true)}>{trust.isPending ? t("settings.hook.trusting") : t("settings.hook.trustProject")}</button>
								</div>
							) : (
								<button className="primary-action" type="button" onClick={() => setConfirmingTrust(true)}>{t("settings.hook.reviewAndTrust")}</button>
							)
						) : null}
						{trust.isError ? <p className="settings-inline-error" role="alert">{messageOf(trust.error, t)}</p> : null}
					</div>
				)}
			</section>

			<section className="settings-section" aria-labelledby="hook-list-title">
				<header>
					<div>
						<h2 id="hook-list-title">{t("settings.hook.cascade")}</h2>
						<p>{t("settings.hook.cascadeDetail")}</p>
					</div>
					{hooks.data ? <span className="hook-count">{t(hooks.data.hooks.length === 1 ? "settings.hook.countOne" : "settings.hook.countMany", { count: formatNumber(hooks.data.hooks.length) })}</span> : null}
				</header>
				{hooks.isPending ? (
					<HookState>{t("settings.hook.loading")}</HookState>
				) : hooks.isError ? (
					<HookState>{messageOf(hooks.error, t)}</HookState>
				) : hooks.data.hooks.length === 0 ? (
					<HookState>{t("settings.hook.empty")}</HookState>
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
	const { t, formatNumber } = useLocalization();
	const action = props.hook.command ?? props.hook.inject ?? "";
	const actionKind = props.hook.command ? t("settings.hook.command") : t("settings.hook.inject");
	return (
		<article className="hook-card" data-active={props.hook.active || undefined}>
			<header>
				<div>
					<strong>{props.hook.event}</strong>
					<span>{props.hook.scope}</span>
					{props.hook.matcher ? <code>{props.hook.matcher}</code> : null}
				</div>
				<span className="hook-active-state">{props.hook.active ? t("settings.hook.active") : t("settings.hook.inert")}</span>
			</header>
			<div className="hook-action">
				<span>{actionKind}{props.hook.timeoutMillis ? t("settings.hook.timeout", { count: formatNumber(props.hook.timeoutMillis) }) : ""}</span>
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

class MissingProjectRootError extends Error {}

function messageOf(error: unknown, t: Translate) {
	if (error instanceof MissingProjectRootError) return t("settings.hook.missingProjectRoot");
	return error instanceof Error ? error.message : t("settings.common.requestFailed");
}
