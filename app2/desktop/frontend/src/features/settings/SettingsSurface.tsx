import { useEffect, useRef, useState } from "react";

import type { RuntimeConnection } from "@lyra/runtime-contract";

import { MCPSettings } from "./MCPSettings";
import { ApprovalSettings } from "./ApprovalSettings";
import { ProviderModelSettings } from "./ProviderModelSettings";
import { ScheduleSettings } from "./ScheduleSettings";

interface SettingsSurfaceProps {
	connection: RuntimeConnection;
	sessionId?: string;
	onClose: () => void;
	onOpenSession: (sessionId: string) => void;
}

type SettingsPage = "providers" | "mcp" | "approvals" | "schedules";

const pageCopy: Record<SettingsPage, { title: string; description: string }> = {
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
};

export function SettingsSurface(props: SettingsSurfaceProps) {
	const [page, setPage] = useState<SettingsPage>("providers");
	const closeButton = useRef<HTMLButtonElement>(null);
	const close = useRef(props.onClose);
	close.current = props.onClose;

	useEffect(() => {
		closeButton.current?.focus();
		const closeOnEscape = (event: KeyboardEvent) => {
			if (event.key !== "Escape") return;
			event.preventDefault();
			close.current();
		};
		window.addEventListener("keydown", closeOnEscape);
		return () => {
			window.removeEventListener("keydown", closeOnEscape);
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
			className="settings-surface"
			role="dialog"
			aria-modal="true"
			aria-labelledby="settings-title"
		>
			<aside className="settings-nav">
				<header>
					<span className="eyebrow">Lyra Runtime</span>
					<h2>Settings</h2>
				</header>
				<nav aria-label="Settings sections">
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
				</nav>
				<p>Secrets are write-only. Runtime state remains the authority after every mutation.</p>
			</aside>
			<div className="settings-content">
				<header className="settings-heading">
					<div>
						<span className="eyebrow">Runtime configuration</span>
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
					{page === "providers" ? (
						<ProviderModelSettings connection={props.connection} />
					) : page === "mcp" ? (
						<MCPSettings connection={props.connection} />
					) : page === "approvals" ? (
						<ApprovalSettings connection={props.connection} sessionId={props.sessionId} />
					) : (
						<ScheduleSettings connection={props.connection} onOpenSession={props.onOpenSession} />
					)}
				</div>
			</div>
		</section>
	);
}
