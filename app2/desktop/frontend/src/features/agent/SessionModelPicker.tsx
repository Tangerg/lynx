import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";

import type { Model, RuntimeConnection, Session } from "@lyra/runtime-contract";

import {
	listModels,
	listProviders,
	runtimeQueryKeys,
} from "../../runtime/runtimeQueries";

interface SessionModelPickerProps {
	connection: RuntimeConnection;
	session: Session;
	disabled: boolean;
	onChange(provider: string, model: string): Promise<unknown>;
}

export function SessionModelPicker(props: SessionModelPickerProps) {
	const root = useRef<HTMLDivElement>(null);
	const [open, setOpen] = useState(false);
	const [provider, setProvider] = useState(props.session.provider ?? "");
	const [error, setError] = useState<string>();
	const [savingModel, setSavingModel] = useState<string>();
	const providers = useQuery({
		queryKey: runtimeQueryKeys.providers(props.connection),
		queryFn: ({ signal }) => listProviders(props.connection, signal),
		staleTime: 60_000,
	});
	const configuredProviders = useMemo(
		() =>
			(providers.data?.data ?? []).filter(
				(candidate) =>
					candidate.apiKeyMasked !== "" || candidate.id === "ollama",
			),
		[providers.data],
	);

	useEffect(() => {
		if (!open) return;
		setProvider((current) =>
			props.session.provider ||
			current ||
			configuredProviders[0]?.id ||
			"",
		);
	}, [configuredProviders, open, props.session.provider]);

	useEffect(() => {
		if (!open) return;
		const close = (event: PointerEvent) => {
			if (!root.current?.contains(event.target as Node)) setOpen(false);
		};
		const escape = (event: KeyboardEvent) => {
			if (event.key === "Escape") setOpen(false);
		};
		document.addEventListener("pointerdown", close);
		document.addEventListener("keydown", escape);
		return () => {
			document.removeEventListener("pointerdown", close);
			document.removeEventListener("keydown", escape);
		};
	}, [open]);

	const models = useQuery({
		queryKey: runtimeQueryKeys.models(props.connection, provider || "unselected"),
		queryFn: ({ signal }) => listModels(props.connection, provider, signal),
		enabled: open && provider !== "",
		staleTime: 5 * 60_000,
		retry: 1,
	});
	const choose = async (model: Model) => {
		if (model.provider === props.session.provider && model.id === props.session.model) {
			setOpen(false);
			return;
		}
		setError(undefined);
		setSavingModel(model.id);
		try {
			await props.onChange(model.provider, model.id);
			setOpen(false);
		} catch (cause) {
			setError(messageOf(cause));
		} finally {
			setSavingModel(undefined);
		}
	};

	return (
		<div className="session-model-picker" ref={root}>
			<button
				className="composer-tool model-picker-trigger"
				type="button"
				disabled={props.disabled}
				aria-haspopup="dialog"
				aria-expanded={open}
				title={
					props.session.model
						? `${props.session.provider} / ${props.session.model}`
						: "Choose the model stored on this session"
				}
				onClick={() => {
					setError(undefined);
					setOpen((current) => !current);
				}}
			>
				<span aria-hidden="true">◇</span>
				{props.session.model || "Choose model"}
			</button>
			{open ? (
				<section className="model-picker-popover" role="dialog" aria-label="Choose model">
					<header>
						<div>
							<strong>Session model</strong>
							<p>Stored as an explicit provider and model pair.</p>
						</div>
						<button type="button" aria-label="Close model picker" onClick={() => setOpen(false)}>×</button>
					</header>
					{providers.isPending ? (
						<ModelPickerState>Loading configured providers…</ModelPickerState>
					) : providers.isError ? (
						<ModelPickerState error>{messageOf(providers.error)}</ModelPickerState>
					) : configuredProviders.length === 0 ? (
						<ModelPickerState>No provider is configured. Open Settings first.</ModelPickerState>
					) : (
						<>
							<nav aria-label="Configured providers">
								{configuredProviders.map((candidate) => (
									<button
										key={candidate.id}
										type="button"
										aria-current={candidate.id === provider ? "page" : undefined}
										onClick={() => {
											setError(undefined);
											setProvider(candidate.id);
										}}
									>
										{providerName(candidate.id)}
									</button>
								))}
							</nav>
							<div className="model-picker-list">
								{models.isPending ? (
									<ModelPickerState>Loading models…</ModelPickerState>
								) : models.isError ? (
									<ModelPickerState error>{messageOf(models.error)}</ModelPickerState>
								) : models.data?.data.length === 0 ? (
									<ModelPickerState>This provider returned no models.</ModelPickerState>
								) : (
									models.data?.data.map((model) => (
										<button
											key={`${model.provider}:${model.id}`}
											type="button"
											data-selected={
												model.provider === props.session.provider && model.id === props.session.model
											}
											disabled={savingModel !== undefined}
											onClick={() => void choose(model)}
										>
											<span>
												<strong>{model.displayName || model.id}</strong>
												<small>{model.id}</small>
											</span>
											<ModelFacts model={model} />
											<b>{savingModel === model.id ? "Saving…" : "Select"}</b>
										</button>
									))
								)}
							</div>
						</>
					)}
					{error ? <p className="model-picker-error" role="alert">{error}</p> : null}
				</section>
			) : null}
		</div>
	);
}

function ModelFacts({ model }: { model: Model }) {
	const facts = [
		model.contextWindow ? `${formatTokens(model.contextWindow)} ctx` : undefined,
		model.capabilities?.reasoning ? "reasoning" : undefined,
		model.capabilities?.multimodal ? "images" : undefined,
		model.capabilities?.toolUse ? "tools" : undefined,
	].filter((value): value is string => value !== undefined);
	return <small>{facts.join(" · ") || "Provider catalog model"}</small>;
}

function ModelPickerState(props: { children: string; error?: boolean }) {
	return <p className="model-picker-state" data-error={props.error || undefined}>{props.children}</p>;
}

function providerName(value: string) {
	return value
		.split("-")
		.map((part) => part.charAt(0).toUpperCase() + part.slice(1))
		.join(" ");
}

function formatTokens(value: number) {
	return value >= 1_000_000
		? `${(value / 1_000_000).toFixed(1)}m`
		: value >= 1_000
			? `${Math.round(value / 1_000)}k`
			: String(value);
}

function messageOf(error: unknown) {
	return error instanceof Error ? error.message : "The model catalog is unavailable.";
}
