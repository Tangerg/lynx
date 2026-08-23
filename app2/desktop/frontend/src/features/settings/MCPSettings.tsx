import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";

import type { MCPServer, MCPTool, RuntimeConnection } from "@lyra/runtime-contract";

import {
	authorizeMCPServer,
	createMCPServer,
	deleteMCPServer,
	listMCPServers,
	listMCPTools,
	reconnectMCPServer,
	runtimeQueryKeys,
	testMCPServer,
	updateMCPServer,
} from "../../runtime/runtimeQueries";
import { MCPConnectionFields } from "./MCPConnectionFields";
import {
	candidateFromDraft,
	draftFromServer,
	durableMCPServerSignature,
	mcpConnectionSummary,
	mcpDraftChanged,
	newMCPDraft,
	requestFromDraft,
	withToolPolicy,
	type MCPDraft,
} from "./mcpDraft";

interface MCPSettingsProps {
	connection: RuntimeConnection;
}

type Verdict = { tone: "ok" | "error"; message: string };

export function MCPSettings(props: MCPSettingsProps) {
	const servers = useQuery({
		queryKey: runtimeQueryKeys.mcpServers(props.connection),
		queryFn: ({ signal }) => listMCPServers(props.connection, signal),
		retry: 2,
	});

	return (
		<>
			<section className="settings-section" aria-labelledby="mcp-add-title">
				<header>
					<div>
						<h2 id="mcp-add-title">Add a server</h2>
						<p>Probe a complete candidate without persisting it, then add it when ready.</p>
					</div>
				</header>
				<NewMCPServer connection={props.connection} />
			</section>
			<section className="settings-section" aria-labelledby="mcp-connections-title">
				<header>
					<div>
						<h2 id="mcp-connections-title">Configured servers</h2>
						<p>Live status comes from Runtime; reconnect never guesses at success.</p>
					</div>
				</header>
				{servers.isPending ? (
					<SettingsState>Loading MCP servers…</SettingsState>
				) : servers.isError ? (
					<SettingsState action="Try again" onAction={() => void servers.refetch()}>
						{messageOf(servers.error)}
					</SettingsState>
				) : (servers.data?.data.length ?? 0) === 0 ? (
					<SettingsState>No MCP servers are configured yet.</SettingsState>
				) : (
					<div className="mcp-server-list">
						{servers.data?.data.map((server) => (
							<MCPServerCard key={server.name} connection={props.connection} server={server} />
						))}
					</div>
				)}
			</section>
		</>
	);
}

function NewMCPServer(props: MCPSettingsProps) {
	const queryClient = useQueryClient();
	const [draft, setDraft] = useState<MCPDraft>(newMCPDraft);
	const [verdict, setVerdict] = useState<Verdict>();
	const testController = useRef<AbortController | undefined>(undefined);

	useEffect(() => () => testController.current?.abort(), []);
	const updateDraft = (next: MCPDraft) => {
		testController.current?.abort();
		setVerdict(undefined);
		setDraft(next);
	};
	const create = useMutation({
		mutationFn: () => createMCPServer(props.connection, candidateFromDraft(draft)),
		onSuccess: () => {
			setDraft(newMCPDraft());
			setVerdict(undefined);
			void queryClient.invalidateQueries({ queryKey: runtimeQueryKeys.mcp(props.connection) });
		},
	});
	const test = useMutation({
		mutationFn: () => {
			testController.current?.abort();
			const controller = new AbortController();
			testController.current = controller;
			return testMCPServer(props.connection, candidateFromDraft(draft), controller.signal);
		},
		onMutate: () => setVerdict(undefined),
		onSuccess: (result) => setVerdict(
			result.ok
				? { tone: "ok", message: "Candidate connected successfully." }
				: { tone: "error", message: problemMessage(result.error) },
		),
	});

	return (
		<article className="mcp-editor mcp-editor-new">
			<MCPConnectionFields draft={draft} onChange={updateDraft} includeName />
			<footer className="mcp-editor-actions">
				<div aria-live="polite">
					{verdict ? <span className="provider-verdict" data-tone={verdict.tone}>{verdict.message}</span> : null}
					{test.isError && !isAbortError(test.error) ? <span className="settings-inline-error">{messageOf(test.error)}</span> : null}
					{create.isError ? <span className="settings-inline-error">{messageOf(create.error)}</span> : null}
				</div>
				<div>
					<button className="secondary-action" type="button" disabled={test.isPending || create.isPending} onClick={() => test.mutate()}>
						{test.isPending ? "Testing…" : "Test candidate"}
					</button>
					<button className="primary-action" type="button" disabled={create.isPending || test.isPending} onClick={() => create.mutate()}>
						{create.isPending ? "Adding…" : "Add server"}
					</button>
				</div>
			</footer>
		</article>
	);
}

function MCPServerCard(props: MCPSettingsProps & { server: MCPServer }) {
	const queryClient = useQueryClient();
	const [draft, setDraft] = useState(() => draftFromServer(props.server));
	const [confirmDelete, setConfirmDelete] = useState(false);
	const [authorizationVerdict, setAuthorizationVerdict] = useState<Verdict>();
	const authorizationController = useRef<AbortController | undefined>(undefined);
	const configurationSignature = durableMCPServerSignature(props.server);
	const currentServer = useRef(props.server);
	currentServer.current = props.server;

	useEffect(() => {
		setDraft(draftFromServer(currentServer.current));
		setConfirmDelete(false);
		setAuthorizationVerdict(undefined);
	}, [configurationSignature]);
	useEffect(() => () => authorizationController.current?.abort(), []);

	const tools = useQuery({
		queryKey: runtimeQueryKeys.mcpTools(props.connection, props.server.name),
		queryFn: ({ signal }) => listMCPTools(props.connection, props.server.name, signal),
		enabled: props.server.status.type === "connected",
		retry: 1,
	});
	const changed = mcpDraftChanged(props.server, draft);
	const invalidate = () => queryClient.invalidateQueries({ queryKey: runtimeQueryKeys.mcp(props.connection) });
	const save = useMutation({
		mutationFn: () => updateMCPServer(props.connection, requestFromDraft(props.server, draft)),
		onSuccess: (committed) => {
			setDraft(draftFromServer(committed));
			void invalidate();
		},
	});
	const reconnect = useMutation({
		mutationFn: () => reconnectMCPServer(props.connection, props.server.name),
		onSuccess: () => void invalidate(),
	});
	const remove = useMutation({
		mutationFn: () => deleteMCPServer(props.connection, props.server.name),
		onSuccess: () => void invalidate(),
	});
	const authorize = useMutation({
		mutationFn: () => {
			authorizationController.current?.abort();
			const controller = new AbortController();
			authorizationController.current = controller;
			return authorizeMCPServer(props.connection, props.server.name, controller.signal);
		},
		onMutate: () => setAuthorizationVerdict(undefined),
		onSuccess: (attempt) => {
			setAuthorizationVerdict(
				attempt.status.type === "succeeded"
					? { tone: "ok", message: "Authorization completed." }
					: { tone: "error", message: attempt.status.type === "canceled" ? "Authorization was canceled." : problemMessage(attempt.status.error) },
			);
			void invalidate();
		},
	});
	const busy = save.isPending || reconnect.isPending || remove.isPending || authorize.isPending;

	return (
		<article className="mcp-editor" data-status={props.server.status.type}>
			<header className="mcp-server-heading">
				<div>
					<h3>{props.server.name}</h3>
					<code>{mcpConnectionSummary(props.server)}</code>
				</div>
				<ServerStatus server={props.server} />
			</header>
			<MCPConnectionFields
				draft={draft}
				onChange={setDraft}
				masked={{
					authorization: props.server.connection.authorizationMasked,
					headers: props.server.connection.headersMasked,
					environment: props.server.connection.envMasked,
				}}
			/>
			<ToolPolicies
				tools={tools.data?.data ?? []}
				pending={tools.isPending && props.server.status.type === "connected"}
				error={tools.error}
				draft={draft}
				onChange={setDraft}
			/>
			{props.server.status.type === "needsAuth" ? (
				<div className="mcp-auth-callout">
					<div>
						<strong>Interactive authorization required</strong>
						<p>{problemMessage(props.server.status.error)}</p>
					</div>
					<button className="primary-action" type="button" disabled={busy || changed} title={changed ? "Save changes before authorizing" : undefined} onClick={() => authorize.mutate()}>
						{authorize.isPending ? "Waiting for authorization…" : "Authorize"}
					</button>
				</div>
			) : null}
			{authorizationVerdict ? <p className="provider-verdict" data-tone={authorizationVerdict.tone} aria-live="polite">{authorizationVerdict.message}</p> : null}
			<footer className="mcp-editor-actions">
				<div>
					{confirmDelete ? (
						<span className="mcp-delete-confirm">
							Delete permanently?
							<button type="button" className="text-action" onClick={() => setConfirmDelete(false)}>Keep</button>
							<button type="button" className="text-action danger" disabled={remove.isPending} onClick={() => remove.mutate()}>{remove.isPending ? "Deleting…" : "Delete"}</button>
						</span>
					) : (
						<button className="text-action danger" type="button" disabled={busy} onClick={() => setConfirmDelete(true)}>Delete server</button>
					)}
					{mutationError(save.error, reconnect.error, remove.error, authorize.error) ? (
						<span className="settings-inline-error" role="alert">{mutationError(save.error, reconnect.error, remove.error, authorize.error)}</span>
					) : null}
				</div>
				<div>
					<button className="secondary-action" type="button" disabled={busy || changed || props.server.status.type === "disabled"} title={changed ? "Save draft changes before reconnecting" : undefined} onClick={() => reconnect.mutate()}>
						{reconnect.isPending ? "Reconnecting…" : "Reconnect"}
					</button>
					<button className="primary-action" type="button" disabled={busy || !changed} onClick={() => save.mutate()}>
						{save.isPending ? "Saving…" : "Save changes"}
					</button>
				</div>
			</footer>
		</article>
	);
}

function ToolPolicies(props: {
	tools: MCPTool[];
	pending: boolean;
	error: unknown;
	draft: MCPDraft;
	onChange: (draft: MCPDraft) => void;
}) {
	const names = useMemo(() => Array.from(new Set([
		...props.tools.map((tool) => tool.name),
		...props.draft.disabledTools,
		...props.draft.autoApproveTools,
	])).sort(), [props.draft.autoApproveTools, props.draft.disabledTools, props.tools]);
	if (props.pending) return <p className="mcp-tool-state">Loading tools…</p>;
	if (props.error && names.length === 0) return <p className="mcp-tool-state" data-error="true">{messageOf(props.error)}</p>;
	if (names.length === 0) return null;

	return (
		<section className="mcp-tool-policies" aria-label="Tool policies">
			<header>
				<div>
					<strong>Tool trust</strong>
					<p>Disabled tools stay hidden. Auto-approved tools may run without an approval prompt.</p>
				</div>
				<span>{names.length} tools</span>
			</header>
			{props.error ? <p className="mcp-tool-state" data-error="true">Live tools could not be refreshed. Stored policies remain editable.</p> : null}
			<div>
				{names.map((name) => {
					const policy = props.draft.disabledTools.includes(name)
						? "disabled"
						: props.draft.autoApproveTools.includes(name) ? "autoApprove" : "default";
					const tool = props.tools.find((candidate) => candidate.name === name);
					return (
						<label key={name}>
							<span><code>{name}</code>{tool?.description ? <small>{tool.description}</small> : null}</span>
							<select value={policy} onChange={(event) => props.onChange(withToolPolicy(props.draft, name, event.currentTarget.value))}>
								<option value="default">Ask when needed</option>
								<option value="disabled">Disabled</option>
								<option value="autoApprove">Auto-approve</option>
							</select>
						</label>
					);
				})}
			</div>
		</section>
	);
}

function ServerStatus({ server }: { server: MCPServer }) {
	const label = server.status.type === "connected" && server.status.toolCount !== undefined
		? `Connected · ${server.status.toolCount} tools`
		: statusLabel(server.status.type);
	return (
		<div className="mcp-status-block">
			<span className="mcp-status" data-status={server.status.type}><i aria-hidden="true" />{label}</span>
			{server.status.error ? <small>{problemMessage(server.status.error)}</small> : null}
		</div>
	);
}

function statusLabel(status: string) {
	return ({
		disabled: "Disabled",
		disconnected: "Disconnected",
		connecting: "Connecting",
		connected: "Connected",
		failed: "Connection failed",
		needsAuth: "Authorization required",
	} as Record<string, string>)[status] ?? status;
}

function problemMessage(problem: { type: string; detail?: string } | undefined) {
	if (problem?.detail) return problem.detail;
	return ({
		mcp_authorization_required: "This server requires interactive authorization.",
		mcp_authorization_failed: "Authorization did not complete.",
		mcp_dial_failed: "Runtime could not connect to this server.",
		timeout: "The server did not respond before the timeout.",
	} as Record<string, string>)[problem?.type ?? ""] ?? "The MCP operation did not succeed.";
}

function mutationError(...errors: unknown[]) {
	const error = errors.find(Boolean);
	return error === undefined ? "" : messageOf(error);
}

function isAbortError(error: unknown) {
	return error instanceof Error && error.name === "AbortError";
}

function SettingsState(props: { children: string; action?: string; onAction?: () => void }) {
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
