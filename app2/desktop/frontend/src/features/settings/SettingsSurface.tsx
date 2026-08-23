import { useEffect, useRef, useState } from "react";

import type { RuntimeConnection, WorkspaceRef } from "@lyra/runtime-contract";

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

type SettingsPage =
	| "appearance"
	| "runtime"
	| "providers"
	| "mcp"
	| "approvals"
	| "schedules"
	| "hooks"
	| "usage";

const pageCopy: Record<SettingsPage, { title: string; description: string }> = {
	appearance: {
		title: "Appearance",
		description: "Choose a durable theme and accent without changing application semantics.",
	},
	runtime: {
		title: "Runtime connection",
		description: "Switch between the supervised local Runtime and one verified remote deployment.",
	},
	providers: {
		title: "Models & providers",
		description: "Connect model providers and assign optional Runtime-wide model roles.",
	},
	mcp: {
		title: "MCP servers",
		description: "Own external tool connections, authorization, and tool-level trust explicitly.",
	},
	approvals: {
		title: "Approval policy",
		description: "Choose the live effect stance and manage remembered decisions visible to this session.",
	},
	schedules: {
		title: "Schedules",
		description: "Create recurring Runs with durable cadence, explicit workspace intent, and recoverable firing.",
	},
	hooks: {
		title: "Lifecycle hooks",
		description: "Review user and project automation before deciding which project hooks may execute.",
	},
	usage: {
		title: "Usage",
		description: "Inspect authoritative terminal Run usage without inventing prices the Runtime does not know.",
	},
};

export function SettingsSurface(props: SettingsSurfaceProps) {
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
					'button[aria-label="Open settings"]',
				);
				trigger?.focus();
			});
		};
	}, []);

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
					<span className="eyebrow">Lyra Desktop</span>
					<h2>Settings</h2>
				</header>
				<nav aria-label="Settings sections">
					<button
						type="button"
						aria-current={page === "appearance" ? "page" : undefined}
						onClick={() => setPage("appearance")}
					>
						<span aria-hidden="true">◐</span>
						Appearance
					</button>
					<button
						type="button"
						aria-current={page === "runtime" ? "page" : undefined}
						onClick={() => setPage("runtime")}
					>
						<span aria-hidden="true">⇄</span>
						Runtime connection
					</button>
					<button
						type="button"
						aria-current={page === "providers" ? "page" : undefined}
						onClick={() => setPage("providers")}
					>
						<span aria-hidden="true">◫</span>
						Models & providers
					</button>
					<button
						type="button"
						aria-current={page === "mcp" ? "page" : undefined}
						onClick={() => setPage("mcp")}
					>
						<span aria-hidden="true">⌘</span>
						MCP servers
					</button>
					<button
						type="button"
						aria-current={page === "approvals" ? "page" : undefined}
						onClick={() => setPage("approvals")}
					>
						<span aria-hidden="true">✓</span>
						Approval policy
					</button>
					<button
						type="button"
						aria-current={page === "schedules" ? "page" : undefined}
						onClick={() => setPage("schedules")}
					>
						<span aria-hidden="true">◷</span>
						Schedules
					</button>
					<button
						type="button"
						aria-current={page === "hooks" ? "page" : undefined}
						onClick={() => setPage("hooks")}
					>
						<span aria-hidden="true">⌁</span>
						Lifecycle hooks
					</button>
					<button
						type="button"
						aria-current={page === "usage" ? "page" : undefined}
						onClick={() => setPage("usage")}
					>
						<span aria-hidden="true">∑</span>
						Usage
					</button>
				</nav>
				<p>Appearance stays local. Runtime state remains the authority after every mutation.</p>
			</aside>
			<div className="settings-content">
				<header className="settings-heading">
					<div>
						<span className="eyebrow">Desktop settings</span>
						<h1 id="settings-title">{pageCopy[page].title}</h1>
						<p>{pageCopy[page].description}</p>
					</div>
					<button
						ref={closeButton}
						className="settings-close"
						type="button"
						aria-label="Close settings"
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
