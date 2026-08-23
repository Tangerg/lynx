import type { MCPDraft, MCPTransport } from "./mcpDraft";
import { useLocalization } from "../localization/Localization";

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
	const { t } = useLocalization();
	const update = <Key extends keyof MCPDraft>(key: Key, value: MCPDraft[Key]) =>
		props.onChange({ ...props.draft, [key]: value });
	const maskedHeaders = Object.keys(props.masked?.headers ?? {}).sort();
	const maskedEnvironment = Object.keys(props.masked?.environment ?? {}).sort();

	return (
		<div className="mcp-fields">
			<div className="mcp-field-grid">
				{props.includeName ? (
					<label>
						<span>{t("settings.mcp.stableName")} <b>{t("settings.common.required")}</b></span>
						<input value={props.draft.name} maxLength={128} autoComplete="off" placeholder="filesystem-tools" onChange={(event) => update("name", event.currentTarget.value)} />
					</label>
				) : null}
				<label>
					<span>{t("settings.mcp.transport")}</span>
					<select value={props.draft.transport} onChange={(event) => update("transport", event.currentTarget.value as MCPTransport)}>
						<option value="streamableHttp">{t("settings.mcp.streamableHTTP")}</option>
						<option value="stdio">{t("settings.mcp.localStdio")}</option>
					</select>
				</label>
				<label>
					<span>{t("settings.mcp.timeout")} <small>{t("settings.mcp.timeoutDetail")}</small></span>
					<input type="number" min="0" max="3600" inputMode="numeric" value={props.draft.timeoutSeconds} placeholder="15" onChange={(event) => update("timeoutSeconds", event.currentTarget.value)} />
				</label>
				<label className="mcp-enabled-field">
					<span>{t("settings.mcp.lifecycle")}</span>
					<span className="mcp-switch-copy">
						<input type="checkbox" checked={props.draft.enabled} onChange={(event) => update("enabled", event.currentTarget.checked)} />
						{props.draft.enabled ? t("settings.mcp.connectAutomatically") : t("settings.mcp.keepDisabled")}
					</span>
				</label>
			</div>
			<label>
				<span>{t("settings.mcp.description")} <small>{t("settings.common.optional")}</small></span>
				<input value={props.draft.description} maxLength={4096} placeholder={t("settings.mcp.descriptionPlaceholder")} onChange={(event) => update("description", event.currentTarget.value)} />
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
	const { t } = useLocalization();
	return (
		<>
			<label>
				<span>{t("settings.mcp.endpointURL")} <b>{t("settings.common.required")}</b></span>
				<input type="url" value={props.draft.url} maxLength={8192} placeholder="https://tools.example.com/mcp" onChange={(event) => props.update("url", event.currentTarget.value)} />
			</label>
			<div className="mcp-secret-grid">
				<div className="mcp-secret-field">
					<label>
						<span>{t("settings.mcp.authorization")} <small>{props.masked?.authorization || t("settings.mcp.notStored")}</small></span>
						<input type="password" autoComplete="off" value={props.draft.authorization} disabled={props.draft.clearAuthorization} placeholder={props.draft.clearAuthorization ? t("settings.mcp.storedValueRemoved") : t("settings.mcp.optionalReplacement")} onChange={(event) => props.update("authorization", event.currentTarget.value)} />
					</label>
					{props.masked?.authorization ? <SecretClear checked={props.draft.clearAuthorization} label={t("settings.mcp.removeAuthorization")} onChange={(checked) => props.onChange({ ...props.draft, authorization: checked ? "" : props.draft.authorization, clearAuthorization: checked })} /> : null}
				</div>
				<div className="mcp-secret-field">
					<label>
						<span>{t("settings.mcp.headersJSON")} <small>{props.maskedHeaders.length ? t("settings.mcp.storedNames", { names: props.maskedHeaders.join(", ") }) : t("settings.common.optional")}</small></span>
						<textarea value={props.draft.headersJSON} disabled={props.draft.clearHeaders} spellCheck={false} placeholder={'{\n  "X-API-Key": "…"\n}'} onChange={(event) => props.update("headersJSON", event.currentTarget.value)} />
					</label>
					{props.maskedHeaders.length ? <SecretClear checked={props.draft.clearHeaders} label={t("settings.mcp.removeHeaders")} onChange={(checked) => props.onChange({ ...props.draft, headersJSON: checked ? "" : props.draft.headersJSON, clearHeaders: checked })} /> : null}
				</div>
			</div>
		</>
	);
}

function StdioFields(props: MCPConnectionFieldsProps & {
	maskedEnvironment: string[];
	update: <Key extends keyof MCPDraft>(key: Key, value: MCPDraft[Key]) => void;
}) {
	const { t } = useLocalization();
	return (
		<>
			<div className="mcp-field-grid mcp-field-grid-wide">
				<label>
					<span>{t("settings.mcp.command")} <b>{t("settings.common.required")}</b></span>
					<input value={props.draft.command} maxLength={4096} placeholder="npx" onChange={(event) => props.update("command", event.currentTarget.value)} />
				</label>
				<label>
					<span>{t("settings.mcp.workingDirectory")} <small>{t("settings.mcp.absolutePath")}</small></span>
					<input value={props.draft.dir} placeholder="/absolute/path" onChange={(event) => props.update("dir", event.currentTarget.value)} />
				</label>
			</div>
			<div className="mcp-secret-grid">
				<label>
					<span>{t("settings.mcp.arguments")} <small>{t("settings.mcp.onePerLine")}</small></span>
					<textarea value={props.draft.argsText} spellCheck={false} placeholder={'-y\n@scope/server'} onChange={(event) => props.update("argsText", event.currentTarget.value)} />
				</label>
				<div className="mcp-secret-field">
					<label>
						<span>{t("settings.mcp.environmentJSON")} <small>{props.maskedEnvironment.length ? t("settings.mcp.storedNames", { names: props.maskedEnvironment.join(", ") }) : t("settings.common.optional")}</small></span>
						<textarea value={props.draft.environmentJSON} disabled={props.draft.clearEnvironment} spellCheck={false} placeholder={'{\n  "TOKEN": "…"\n}'} onChange={(event) => props.update("environmentJSON", event.currentTarget.value)} />
					</label>
					{props.maskedEnvironment.length ? <SecretClear checked={props.draft.clearEnvironment} label={t("settings.mcp.removeEnvironment")} onChange={(checked) => props.onChange({ ...props.draft, environmentJSON: checked ? "" : props.draft.environmentJSON, clearEnvironment: checked })} /> : null}
				</div>
			</div>
		</>
	);
}

function SecretClear(props: { checked: boolean; label: string; onChange: (checked: boolean) => void }) {
	return <label className="mcp-secret-clear"><input type="checkbox" checked={props.checked} onChange={(event) => props.onChange(event.currentTarget.checked)} />{props.label}</label>;
}
