import type {
	MCPConnectionInput,
	MCPServer,
	MCPServerCandidate,
	UpdateMCPServerRequest,
} from "@lyra/runtime-contract";

export type MCPTransport = "streamableHttp" | "stdio";

export interface MCPDraft {
	name: string;
	enabled: boolean;
	description: string;
	transport: MCPTransport;
	url: string;
	authorization: string;
	clearAuthorization: boolean;
	headersJSON: string;
	clearHeaders: boolean;
	command: string;
	argsText: string;
	environmentJSON: string;
	clearEnvironment: boolean;
	dir: string;
	timeoutSeconds: string;
	disabledTools: string[];
	autoApproveTools: string[];
}

export function newMCPDraft(): MCPDraft {
	return {
		name: "",
		enabled: true,
		description: "",
		transport: "streamableHttp",
		url: "",
		authorization: "",
		clearAuthorization: false,
		headersJSON: "",
		clearHeaders: false,
		command: "",
		argsText: "",
		environmentJSON: "",
		clearEnvironment: false,
		dir: "",
		timeoutSeconds: "",
		disabledTools: [],
		autoApproveTools: [],
	};
}

export function draftFromServer(server: MCPServer): MCPDraft {
	return {
		...newMCPDraft(),
		name: server.name,
		enabled: server.status.type !== "disabled",
		description: server.description ?? "",
		transport: server.connection.type as MCPTransport,
		url: server.connection.url ?? "",
		command: server.connection.command ?? "",
		argsText: (server.connection.args ?? []).join("\n"),
		dir: server.connection.dir ?? "",
		timeoutSeconds: server.timeoutSeconds ? String(server.timeoutSeconds) : "",
		disabledTools: [...(server.disabledTools ?? [])],
		autoApproveTools: [...(server.autoApproveTools ?? [])],
	};
}

export function durableMCPServerSignature(server: MCPServer) {
	return JSON.stringify({
		enabled: server.status.type !== "disabled",
		description: server.description ?? "",
		connection: {
			...server.connection,
			headersMasked: Object.entries(server.connection.headersMasked ?? {}).sort(([left], [right]) => left.localeCompare(right)),
			envMasked: Object.entries(server.connection.envMasked ?? {}).sort(([left], [right]) => left.localeCompare(right)),
		},
		timeoutSeconds: server.timeoutSeconds ?? 0,
		disabledTools: server.disabledTools ?? [],
		autoApproveTools: server.autoApproveTools ?? [],
	});
}

export function candidateFromDraft(draft: MCPDraft): MCPServerCandidate {
	const timeoutSeconds = timeoutFromDraft(draft);
	return {
		name: required(draft.name, "A stable server name is required."),
		enabled: draft.enabled,
		...(draft.description.trim() === "" ? {} : { description: draft.description.trim() }),
		connection: connectionFromDraft(draft, true),
		...(timeoutSeconds === 0 ? {} : { timeoutSeconds }),
		...(draft.disabledTools.length === 0 ? {} : { disabledTools: draft.disabledTools }),
		...(draft.autoApproveTools.length === 0 ? {} : { autoApproveTools: draft.autoApproveTools }),
	};
}

export function requestFromDraft(server: MCPServer, draft: MCPDraft): UpdateMCPServerRequest {
	const request: UpdateMCPServerRequest = { server: server.name };
	const currentEnabled = server.status.type !== "disabled";
	if (draft.enabled !== currentEnabled) request.enabled = draft.enabled;
	if (draft.description.trim() !== (server.description ?? "")) request.description = draft.description.trim();
	const timeout = timeoutFromDraft(draft);
	if (timeout !== (server.timeoutSeconds ?? 0)) request.timeoutSeconds = timeout;
	if (!sameValues(draft.disabledTools, server.disabledTools ?? [])) request.disabledTools = draft.disabledTools;
	if (!sameValues(draft.autoApproveTools, server.autoApproveTools ?? [])) request.autoApproveTools = draft.autoApproveTools;
	if (connectionChanged(server, draft)) request.connection = connectionFromDraft(draft, false);
	return request;
}

export function mcpDraftChanged(server: MCPServer, draft: MCPDraft) {
	const currentTimeout = server.timeoutSeconds ? String(server.timeoutSeconds) : "";
	return draft.enabled !== (server.status.type !== "disabled") ||
		draft.description.trim() !== (server.description ?? "") ||
		draft.timeoutSeconds.trim() !== currentTimeout ||
		!sameValues(draft.disabledTools, server.disabledTools ?? []) ||
		!sameValues(draft.autoApproveTools, server.autoApproveTools ?? []) ||
		connectionChanged(server, draft);
}

export function withToolPolicy(draft: MCPDraft, name: string, policy: string): MCPDraft {
	const disabledTools = draft.disabledTools.filter((candidate) => candidate !== name);
	const autoApproveTools = draft.autoApproveTools.filter((candidate) => candidate !== name);
	if (policy === "disabled") disabledTools.push(name);
	if (policy === "autoApprove") autoApproveTools.push(name);
	return { ...draft, disabledTools: disabledTools.sort(), autoApproveTools: autoApproveTools.sort() };
}

export function mcpConnectionSummary(server: MCPServer) {
	return server.connection.type === "stdio"
		? [server.connection.command, ...(server.connection.args ?? [])].filter(Boolean).join(" ")
		: server.connection.url;
}

function connectionChanged(server: MCPServer, draft: MCPDraft) {
	if (draft.transport !== server.connection.type) return true;
	if (draft.authorization.trim() !== "" || draft.clearAuthorization || draft.headersJSON.trim() !== "" || draft.clearHeaders || draft.environmentJSON.trim() !== "" || draft.clearEnvironment) return true;
	return draft.transport === "streamableHttp"
		? draft.url.trim() !== (server.connection.url ?? "")
		: draft.command.trim() !== (server.connection.command ?? "") ||
			draft.dir.trim() !== (server.connection.dir ?? "") ||
			!sameValues(argumentLines(draft.argsText), server.connection.args ?? []);
}

function connectionFromDraft(draft: MCPDraft, candidate: boolean): MCPConnectionInput {
	if (draft.transport === "streamableHttp") {
		const authorization = draft.clearAuthorization
			? { type: "clear" }
			: draft.authorization.trim() === "" ? undefined : { type: "set", value: draft.authorization };
		const headers = draft.clearHeaders
			? { type: "clear" }
			: draft.headersJSON.trim() === "" ? undefined : { type: "set", value: parseSecretObject(draft.headersJSON, "headers") };
		return {
			type: "streamableHttp",
			url: required(draft.url, "An endpoint URL is required."),
			...((candidate && authorization?.type === "clear") || authorization === undefined ? {} : { authorization }),
			...((candidate && headers?.type === "clear") || headers === undefined ? {} : { headers }),
		};
	}
	const environment = draft.clearEnvironment
		? { type: "clear" }
		: draft.environmentJSON.trim() === "" ? undefined : { type: "set", value: parseSecretObject(draft.environmentJSON, "environment") };
	return {
		type: "stdio",
		command: required(draft.command, "A stdio command is required."),
		args: argumentLines(draft.argsText),
		...(draft.dir.trim() === "" ? {} : { dir: draft.dir.trim() }),
		...((candidate && environment?.type === "clear") || environment === undefined ? {} : { env: environment }),
	};
}

function timeoutFromDraft(draft: MCPDraft) {
	if (draft.timeoutSeconds.trim() === "") return 0;
	const value = Number(draft.timeoutSeconds);
	if (!Number.isInteger(value) || value < 0 || value > 3600) throw new Error("Timeout must be a whole number from 0 to 3600 seconds.");
	return value;
}

function parseSecretObject(source: string, label: string): Record<string, string> {
	let value: unknown;
	try {
		value = JSON.parse(source);
	} catch {
		throw new Error(`${capitalize(label)} must be valid JSON.`);
	}
	if (value === null || Array.isArray(value) || typeof value !== "object") throw new Error(`${capitalize(label)} must be a JSON object.`);
	const entries = Object.entries(value);
	if (entries.length === 0 || entries.some(([name, entry]) => name === "" || typeof entry !== "string")) {
		throw new Error(`${capitalize(label)} must contain non-empty names with string values.`);
	}
	return Object.fromEntries(entries) as Record<string, string>;
}

function argumentLines(value: string) {
	return value === "" ? [] : value.replaceAll("\r\n", "\n").split("\n");
}

function sameValues(left: string[], right: string[]) {
	return left.length === right.length && left.every((value, index) => value === right[index]);
}

function required(value: string, message: string) {
	const normalized = value.trim();
	if (normalized === "") throw new Error(message);
	return normalized;
}

function capitalize(value: string) {
	return value.charAt(0).toUpperCase() + value.slice(1);
}
