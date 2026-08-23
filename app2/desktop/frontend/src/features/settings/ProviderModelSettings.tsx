import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";

import type {
	EmbeddingRole,
	Provider,
	RuntimeConnection,
	UpdateProviderRequest,
	UtilityRole,
} from "@lyra/runtime-contract";

import {
	getEmbeddingRole,
	getUtilityRole,
	listModels,
	listProviders,
	runtimeQueryKeys,
	setEmbeddingRole,
	setUtilityRole,
	testProvider,
	updateProvider,
} from "../../runtime/runtimeQueries";

interface ProviderModelSettingsProps {
	connection: RuntimeConnection;
}

export function ProviderModelSettings(props: ProviderModelSettingsProps) {
	const [query, setQuery] = useState("");
	const providers = useQuery({
		queryKey: runtimeQueryKeys.providers(props.connection),
		queryFn: ({ signal }) => listProviders(props.connection, signal),
		retry: 2,
	});
	const visible = useMemo(() => {
		const needle = query.trim().toLowerCase();
		return (providers.data?.data ?? []).filter(
			(provider) =>
				needle === "" ||
				provider.id.toLowerCase().includes(needle) ||
				providerName(provider.id).toLowerCase().includes(needle),
		);
	}, [providers.data, query]);

	return (
		<>
			<section className="settings-section" aria-labelledby="model-roles-title">
				<header>
					<div>
						<h2 id="model-roles-title">Model roles</h2>
						<p>Optional Runtime-wide models for maintenance and semantic indexing.</p>
					</div>
				</header>
				<div className="model-role-grid">
					<ModelRoleEditor connection={props.connection} role="utility" providers={providers.data?.data ?? []} />
					<ModelRoleEditor connection={props.connection} role="embedding" providers={providers.data?.data ?? []} />
				</div>
			</section>
			<section className="settings-section" aria-labelledby="provider-settings-title">
				<header className="provider-section-heading">
					<div>
						<h2 id="provider-settings-title">Provider connections</h2>
						<p>Secrets are write-only. Environment credentials remain read-only.</p>
					</div>
					<label>
						<span className="sr-only">Filter providers</span>
						<input value={query} maxLength={80} placeholder="Filter providers…" onChange={(event) => setQuery(event.currentTarget.value)} />
					</label>
				</header>
				{providers.isPending ? (
					<SettingsState>Loading providers…</SettingsState>
				) : providers.isError ? (
					<SettingsState action="Try again" onAction={() => void providers.refetch()}>{messageOf(providers.error)}</SettingsState>
				) : visible.length === 0 ? (
					<SettingsState>No providers match this filter.</SettingsState>
				) : (
					<div className="provider-card-list">
						{visible.map((provider) => (
							<ProviderCard key={provider.id} connection={props.connection} provider={provider} />
						))}
					</div>
				)}
			</section>
		</>
	);
}

function ProviderCard(props: { connection: RuntimeConnection; provider: Provider }) {
	const queryClient = useQueryClient();
	const [baseURL, setBaseURL] = useState(props.provider.baseUrl ?? "");
	const [apiKey, setAPIKey] = useState("");
	const [clearStoredKey, setClearStoredKey] = useState(false);
	const [testResult, setTestResult] = useState<{ tone: "ok" | "error"; message: string }>();
	const baseChanged = baseURL.trim() !== (props.provider.baseUrl ?? "");
	const keyChanged = apiKey.trim() !== "" || clearStoredKey;
	const incompleteEndpoint =
		props.provider.requiresBaseUrl &&
		baseURL.trim() === "" &&
		!clearStoredKey &&
		(apiKey.trim() !== "" || props.provider.keySource === "stored");

	useEffect(() => {
		setBaseURL(props.provider.baseUrl ?? "");
		setAPIKey("");
		setClearStoredKey(false);
	}, [props.provider.baseUrl, props.provider.apiKeyMasked, props.provider.keySource]);

	const save = useMutation({
		mutationFn: () => {
			const request: UpdateProviderRequest = { provider: props.provider.id };
			if (baseChanged) {
				request.baseUrl = baseURL.trim() === ""
					? { type: "clear" }
					: { type: "set", value: baseURL.trim() };
			}
			if (clearStoredKey) request.apiKey = { type: "clear" };
			else if (apiKey.trim() !== "") request.apiKey = { type: "set", value: apiKey };
			return updateProvider(props.connection, request);
		},
		onSuccess: () => {
			setAPIKey("");
			setClearStoredKey(false);
			void queryClient.invalidateQueries({ queryKey: runtimeQueryKeys.providers(props.connection) });
			void queryClient.invalidateQueries({
				queryKey: [...runtimeQueryKeys.scope(props.connection), "models"],
			});
		},
	});
	const test = useMutation({
		mutationFn: () => testProvider(props.connection, props.provider.id),
		onMutate: () => setTestResult(undefined),
		onSuccess: (result) => setTestResult(
			result.ok
				? { tone: "ok", message: "Connection succeeded." }
				: { tone: "error", message: result.error?.detail || result.error?.type || "Connection failed." },
		),
	});
	const configured = props.provider.apiKeyMasked !== "" || props.provider.id === "ollama";

	return (
		<article className="provider-card" data-configured={configured || undefined}>
			<header>
				<div>
					<h3>{providerName(props.provider.id)}</h3>
					<code>{props.provider.id}</code>
				</div>
				<span>{configured ? "Configured" : "Not configured"}</span>
			</header>
			<div className="provider-fields">
				<label>
					<span>Base URL {props.provider.requiresBaseUrl ? <b>Required</b> : <small>Optional override</small>}</span>
					<input type="url" value={baseURL} maxLength={2048} placeholder={props.provider.requiresBaseUrl ? "https://…/v1" : "Use provider default"} onChange={(event) => setBaseURL(event.currentTarget.value)} />
				</label>
				<label>
					<span>API key <small>{props.provider.keySource === "env" ? "From environment · read-only" : props.provider.apiKeyMasked || "Not set"}</small></span>
					<input type="password" value={apiKey} maxLength={4096} autoComplete="off" placeholder={clearStoredKey ? "Stored key will be removed" : "Enter a replacement key"} disabled={clearStoredKey} onChange={(event) => setAPIKey(event.currentTarget.value)} />
				</label>
			</div>
			<footer>
				<div>
					{props.provider.keySource === "stored" ? (
						<button className="text-action danger" type="button" onClick={() => { setAPIKey(""); setClearStoredKey((current) => !current); }}>
							{clearStoredKey ? "Keep stored key" : "Remove stored key"}
						</button>
					) : null}
					{testResult ? <span className="provider-verdict" data-tone={testResult.tone}>{testResult.message}</span> : null}
					{test.isError ? <span className="provider-verdict" data-tone="error">{messageOf(test.error)}</span> : null}
				</div>
				<div>
					<button className="secondary-action" type="button" disabled={test.isPending || baseChanged || keyChanged} title={baseChanged || keyChanged ? "Save draft changes before testing" : undefined} onClick={() => test.mutate()}>
						{test.isPending ? "Testing…" : "Test"}
					</button>
					<button className="primary-action" type="button" disabled={save.isPending || incompleteEndpoint || (!baseChanged && !keyChanged)} onClick={() => save.mutate()}>
						{save.isPending ? "Saving…" : "Save"}
					</button>
				</div>
			</footer>
			{save.isError ? <p className="settings-inline-error" role="alert">{messageOf(save.error)}</p> : null}
			{incompleteEndpoint ? <p className="settings-inline-error" role="alert">A base URL is required while a stored key is present.</p> : null}
		</article>
	);
}

function ModelRoleEditor(props: {
	connection: RuntimeConnection;
	role: "utility" | "embedding";
	providers: Provider[];
}) {
	const queryClient = useQueryClient();
	const key = runtimeQueryKeys.modelRole(props.connection, props.role);
	const role = useQuery({
		queryKey: key,
		queryFn: ({ signal }) => props.role === "utility"
			? getUtilityRole(props.connection, signal)
			: getEmbeddingRole(props.connection, signal),
	});
	const [provider, setProvider] = useState("");
	const [model, setModel] = useState("");
	const [dirty, setDirty] = useState(false);
	const available = props.providers.filter(
		(candidate) =>
			(candidate.apiKeyMasked !== "" || candidate.id === "ollama") &&
			(props.role === "utility" || candidate.embeddingCapable),
	);
	useEffect(() => {
		if (dirty || role.data === undefined) return;
		setProvider(role.data.provider ?? "");
		setModel(role.data.model ?? "");
	}, [dirty, role.data]);
	const models = useQuery({
		queryKey: runtimeQueryKeys.models(props.connection, provider || "unselected"),
		queryFn: ({ signal }) => listModels(props.connection, provider, signal),
		enabled: provider !== "" && props.role === "utility",
		staleTime: 5 * 60_000,
		retry: 1,
	});
	const save = useMutation({
		mutationFn: () => {
			const value = { provider, model };
			return props.role === "utility"
				? setUtilityRole(props.connection, value as UtilityRole)
				: setEmbeddingRole(props.connection, value as EmbeddingRole);
		},
		onSuccess: (committed) => {
			queryClient.setQueryData(key, committed);
			setDirty(false);
			void queryClient.invalidateQueries({ queryKey: key });
		},
	});
	const selectedProvider = available.find((candidate) => candidate.id === provider);
	const changed = provider !== (role.data?.provider ?? "") || model !== (role.data?.model ?? "");

	return (
		<article className="model-role-card">
			<header>
				<div>
					<h3>{props.role === "utility" ? "Utility model" : "Embedding model"}</h3>
					<p>{props.role === "utility" ? "Background curation and maintenance." : "Semantic codebase indexing."}</p>
				</div>
				<span>{role.data?.model ? "Assigned" : "Optional"}</span>
			</header>
			{role.isPending ? (
				<SettingsState>Loading role…</SettingsState>
			) : role.isError ? (
				<SettingsState>{messageOf(role.error)}</SettingsState>
			) : (
				<>
					<label>
						<span>Provider</span>
						<select value={provider} onChange={(event) => {
							const next = event.currentTarget.value;
							const metadata = available.find((candidate) => candidate.id === next);
							setProvider(next);
							setModel(props.role === "embedding" ? metadata?.defaultEmbeddingModel ?? "" : "");
							setDirty(true);
						}}>
							<option value="">Not assigned</option>
							{available.map((candidate) => <option key={candidate.id} value={candidate.id}>{providerName(candidate.id)}</option>)}
						</select>
					</label>
					<label>
						<span>Model</span>
						<input value={model} list={`role-models-${props.role}`} disabled={provider === ""} maxLength={256} placeholder={props.role === "embedding" && selectedProvider?.defaultEmbeddingModel === "" ? "Deployment or model id" : "Model id"} onChange={(event) => { setModel(event.currentTarget.value); setDirty(true); }} />
						{props.role === "utility" ? (
							<datalist id={`role-models-${props.role}`}>{models.data?.data.map((candidate) => <option key={candidate.id} value={candidate.id} />)}</datalist>
						) : null}
					</label>
					<footer>
						{models.isError ? <span className="settings-inline-error">{messageOf(models.error)}</span> : <span />}
						<button className="secondary-action" type="button" disabled={save.isPending || !changed || ((provider === "") !== (model === ""))} onClick={() => save.mutate()}>
							{save.isPending ? "Saving…" : provider === "" ? "Clear role" : "Save role"}
						</button>
					</footer>
					{save.isError ? <p className="settings-inline-error" role="alert">{messageOf(save.error)}</p> : null}
				</>
			)}
		</article>
	);
}

function SettingsState(props: { children: string; action?: string; onAction?: () => void }) {
	return (
		<div className="settings-state">
			<p>{props.children}</p>
			{props.action && props.onAction ? <button className="secondary-action" type="button" onClick={props.onAction}>{props.action}</button> : null}
		</div>
	);
}

function providerName(value: string) {
	return value
		.split("-")
		.map((part) => part.charAt(0).toUpperCase() + part.slice(1))
		.join(" ");
}

function messageOf(error: unknown) {
	return error instanceof Error ? error.message : "The Runtime request failed.";
}
