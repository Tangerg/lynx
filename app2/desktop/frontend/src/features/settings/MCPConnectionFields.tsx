import type { MCPDraft, MCPTransport } from "./mcpDraft";

interface MCPConnectionFieldsProps {
	draft: MCPDraft;
	onChange: (draft: MCPDraft) => void;
	includeName?: boolean;
	masked?: {
		authorization?: string;
		headers?: Record<string, string>;
		environment?: Record<string, string>;
	};
}

export function MCPConnectionFields(props: MCPConnectionFieldsProps) {
	const update = <Key extends keyof MCPDraft>(key: Key, value: MCPDraft[Key]) =>
		props.onChange({ ...props.draft, [key]: value });
	const maskedHeaders = Object.keys(props.masked?.headers ?? {}).sort();
	const maskedEnvironment = Object.keys(props.masked?.environment ?? {}).sort();

	return (
		<div className="mcp-fields">
			<div className="mcp-field-grid">
				{props.includeName ? (
					<label>
						<span>Stable name <b>Required</b></span>
						<input value={props.draft.name} maxLength={128} autoComplete="off" placeholder="filesystem-tools" onChange={(event) => update("name", event.currentTarget.value)} />
					</label>
				) : null}
				<label>
					<span>Transport</span>
					<select value={props.draft.transport} onChange={(event) => update("transport", event.currentTarget.value as MCPTransport)}>
						<option value="streamableHttp">Streamable HTTP</option>
						<option value="stdio">Local stdio</option>
					</select>
				</label>
				<label>
					<span>Timeout <small>seconds · 0 uses Runtime default</small></span>
					<input type="number" min="0" max="3600" inputMode="numeric" value={props.draft.timeoutSeconds} placeholder="15" onChange={(event) => update("timeoutSeconds", event.currentTarget.value)} />
				</label>
				<label className="mcp-enabled-field">
					<span>Lifecycle</span>
					<span className="mcp-switch-copy">
						<input type="checkbox" checked={props.draft.enabled} onChange={(event) => update("enabled", event.currentTarget.checked)} />
						{props.draft.enabled ? "Connect automatically" : "Keep disabled"}
					</span>
				</label>
			</div>
			<label>
				<span>Description <small>Optional</small></span>
				<input value={props.draft.description} maxLength={4096} placeholder="What this server contributes" onChange={(event) => update("description", event.currentTarget.value)} />
			</label>
			{props.draft.transport === "streamableHttp" ? (
				<HTTPFields {...props} maskedHeaders={maskedHeaders} update={update} />
			) : (
				<StdioFields {...props} maskedEnvironment={maskedEnvironment} update={update} />
			)}
		</div>
	);
}

function HTTPFields(props: MCPConnectionFieldsProps & {
	maskedHeaders: string[];
	update: <Key extends keyof MCPDraft>(key: Key, value: MCPDraft[Key]) => void;
}) {
	return (
		<>
			<label>
				<span>Endpoint URL <b>Required</b></span>
				<input type="url" value={props.draft.url} maxLength={8192} placeholder="https://tools.example.com/mcp" onChange={(event) => props.update("url", event.currentTarget.value)} />
			</label>
			<div className="mcp-secret-grid">
				<div className="mcp-secret-field">
					<label>
						<span>Authorization <small>{props.masked?.authorization || "Not stored"}</small></span>
						<input type="password" autoComplete="off" value={props.draft.authorization} disabled={props.draft.clearAuthorization} placeholder={props.draft.clearAuthorization ? "Stored value will be removed" : "Optional replacement value"} onChange={(event) => props.update("authorization", event.currentTarget.value)} />
					</label>
					{props.masked?.authorization ? <SecretClear checked={props.draft.clearAuthorization} label="Remove stored authorization" onChange={(checked) => props.onChange({ ...props.draft, authorization: checked ? "" : props.draft.authorization, clearAuthorization: checked })} /> : null}
				</div>
				<div className="mcp-secret-field">
					<label>
						<span>Headers JSON <small>{props.maskedHeaders.length ? `Stored: ${props.maskedHeaders.join(", ")}` : "Optional"}</small></span>
						<textarea value={props.draft.headersJSON} disabled={props.draft.clearHeaders} spellCheck={false} placeholder={'{\n  "X-API-Key": "…"\n}'} onChange={(event) => props.update("headersJSON", event.currentTarget.value)} />
					</label>
					{props.maskedHeaders.length ? <SecretClear checked={props.draft.clearHeaders} label="Remove all stored headers" onChange={(checked) => props.onChange({ ...props.draft, headersJSON: checked ? "" : props.draft.headersJSON, clearHeaders: checked })} /> : null}
				</div>
			</div>
		</>
	);
}

function StdioFields(props: MCPConnectionFieldsProps & {
	maskedEnvironment: string[];
	update: <Key extends keyof MCPDraft>(key: Key, value: MCPDraft[Key]) => void;
}) {
	return (
		<>
			<div className="mcp-field-grid mcp-field-grid-wide">
				<label>
					<span>Command <b>Required</b></span>
					<input value={props.draft.command} maxLength={4096} placeholder="npx" onChange={(event) => props.update("command", event.currentTarget.value)} />
				</label>
				<label>
					<span>Working directory <small>Absolute path</small></span>
					<input value={props.draft.dir} placeholder="/absolute/path" onChange={(event) => props.update("dir", event.currentTarget.value)} />
				</label>
			</div>
			<div className="mcp-secret-grid">
				<label>
					<span>Arguments <small>One per line</small></span>
					<textarea value={props.draft.argsText} spellCheck={false} placeholder={'-y\n@scope/server'} onChange={(event) => props.update("argsText", event.currentTarget.value)} />
				</label>
				<div className="mcp-secret-field">
					<label>
						<span>Environment JSON <small>{props.maskedEnvironment.length ? `Stored: ${props.maskedEnvironment.join(", ")}` : "Optional"}</small></span>
						<textarea value={props.draft.environmentJSON} disabled={props.draft.clearEnvironment} spellCheck={false} placeholder={'{\n  "TOKEN": "…"\n}'} onChange={(event) => props.update("environmentJSON", event.currentTarget.value)} />
					</label>
					{props.maskedEnvironment.length ? <SecretClear checked={props.draft.clearEnvironment} label="Remove stored environment" onChange={(checked) => props.onChange({ ...props.draft, environmentJSON: checked ? "" : props.draft.environmentJSON, clearEnvironment: checked })} /> : null}
				</div>
			</div>
		</>
	);
}

function SecretClear(props: { checked: boolean; label: string; onChange: (checked: boolean) => void }) {
	return <label className="mcp-secret-clear"><input type="checkbox" checked={props.checked} onChange={(event) => props.onChange(event.currentTarget.checked)} />{props.label}</label>;
}
