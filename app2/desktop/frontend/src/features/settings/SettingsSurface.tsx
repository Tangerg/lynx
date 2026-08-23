import { useEffect, useRef, useState } from "react";

import type { RuntimeConnection, WorkspaceRef } from "@lyra/runtime-contract";

import { useLocalization, type MessageKey } from "../localization/Localization";
import { MCPSettings } from "./MCPSettings";
import { ApprovalSettings } from "./ApprovalSettings";
import { ProviderModelSettings } from "./ProviderModelSettings";
import { ScheduleSettings } from "./ScheduleSettings";
import { HookSettings } from "./HookSettings";
import { UsageSettings } from "./UsageSettings";
import { AppearanceSettings } from "./AppearanceSettings";
import { RuntimeSettings } from "./RuntimeSettings";
import "./ShellSettings.css";

interface SettingsSurfaceProps {
	connection: RuntimeConnection;
	sessionId?: string;
	workspace?: WorkspaceRef;
	onClose: () => void;
	onOpenSession: (sessionId: string) => void;
	onRuntimeChanged: () => Promise<void>;
}

const settingsPages = [
	{ id: "appearance", icon: "◐", title: "settings.page.appearance.title", description: "settings.page.appearance.description" },
	{ id: "runtime", icon: "⇄", title: "settings.page.runtime.title", description: "settings.page.runtime.description" },
	{ id: "providers", icon: "◫", title: "settings.page.providers.title", description: "settings.page.providers.description" },
	{ id: "mcp", icon: "⌘", title: "settings.page.mcp.title", description: "settings.page.mcp.description" },
	{ id: "approvals", icon: "✓", title: "settings.page.approvals.title", description: "settings.page.approvals.description" },
	{ id: "schedules", icon: "◷", title: "settings.page.schedules.title", description: "settings.page.schedules.description" },
	{ id: "hooks", icon: "⌁", title: "settings.page.hooks.title", description: "settings.page.hooks.description" },
	{ id: "usage", icon: "∑", title: "settings.page.usage.title", description: "settings.page.usage.description" },
] as const satisfies ReadonlyArray<{
	id: string;
	icon: string;
	title: MessageKey;
	description: MessageKey;
}>;

type SettingsPage = (typeof settingsPages)[number]["id"];

export function SettingsSurface(props: SettingsSurfaceProps) {
	const { t } = useLocalization();
	const [page, setPage] = useState<SettingsPage>("appearance");
	const surface = useRef<HTMLElement>(null);
	const closeButton = useRef<HTMLButtonElement>(null);
	const close = useRef(props.onClose);
	close.current = props.onClose;

	useEffect(() => {
		closeButton.current?.focus();
		const handleDialogKey = (event: KeyboardEvent) => {
			if (event.key === "Escape") {
				event.preventDefault();
				close.current();
				return;
			}
			if (event.key !== "Tab") return;
			const controls = [
				...(surface.current?.querySelectorAll<HTMLElement>(
					'button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [href], [tabindex]:not([tabindex="-1"])',
				) ?? []),
			];
			const first = controls[0];
			const last = controls.at(-1);
			if (first === undefined || last === undefined) return;
			if (event.shiftKey && document.activeElement === first) {
				event.preventDefault();
				last.focus();
			} else if (!event.shiftKey && document.activeElement === last) {
				event.preventDefault();
				first.focus();
			}
		};
		window.addEventListener("keydown", handleDialogKey);
		return () => {
			window.removeEventListener("keydown", handleDialogKey);
			window.requestAnimationFrame(() => {
				const trigger = document.querySelector<HTMLButtonElement>(
					'button[data-settings-trigger="true"]',
				);
				trigger?.focus();
			});
		};
	}, []);
	const activePage = settingsPages.find((candidate) => candidate.id === page) ?? settingsPages[0];

	return (
		<section
			ref={surface}
			className="settings-surface"
			role="dialog"
			aria-modal="true"
			aria-labelledby="settings-title"
		>
			<aside className="settings-nav">
				<header>
					<span className="eyebrow">{t("settings.desktopBrand")}</span>
					<h2>{t("settings.title")}</h2>
				</header>
				<nav aria-label={t("settings.sections")}>
					{settingsPages.map((candidate) => (
						<button
							key={candidate.id}
							type="button"
							aria-current={page === candidate.id ? "page" : undefined}
							onClick={() => setPage(candidate.id)}
						>
							<span aria-hidden="true">{candidate.icon}</span>
							{t(candidate.title)}
						</button>
					))}
				</nav>
				<p>{t("settings.authorityNote")}</p>
			</aside>
			<div className="settings-content">
				<header className="settings-heading">
					<div>
						<span className="eyebrow">{t("settings.desktopSettings")}</span>
						<h1 id="settings-title">{t(activePage.title)}</h1>
						<p>{t(activePage.description)}</p>
					</div>
					<button
						ref={closeButton}
						className="settings-close"
						type="button"
						aria-label={t("settings.close")}
						onClick={props.onClose}
					>
						<span aria-hidden="true">×</span>
					</button>
				</header>
				<div className="settings-scroll">
					{page === "appearance" ? (
						<AppearanceSettings />
					) : page === "runtime" ? (
						<RuntimeSettings onRuntimeChanged={props.onRuntimeChanged} />
					) : page === "providers" ? (
						<ProviderModelSettings connection={props.connection} />
					) : page === "mcp" ? (
						<MCPSettings connection={props.connection} />
					) : page === "approvals" ? (
						<ApprovalSettings connection={props.connection} sessionId={props.sessionId} />
					) : page === "schedules" ? (
						<ScheduleSettings connection={props.connection} onOpenSession={props.onOpenSession} />
					) : page === "hooks" ? (
						<HookSettings connection={props.connection} workspace={props.workspace} />
					) : (
						<UsageSettings connection={props.connection} sessionId={props.sessionId} />
					)}
				</div>
			</div>
		</section>
	);
}
