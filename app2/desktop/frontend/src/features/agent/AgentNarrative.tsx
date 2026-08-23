import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import type {
	FeedbackRating,
  InterruptResponse,
  Item,
  PendingInterruptSet,
  RunProgress,
	RunSummary,
	RestoreType,
} from "@lyra/runtime-contract";

import {
	useLocalization,
	type Translate,
} from "../localization/Localization";
import { InterruptSetCard } from "./InterruptSetCard";
import { ToolDisclosure } from "./ToolDisclosure";
import type { LiveToolOutput } from "./agentSessionTypes";
import { MarkdownContent } from "./content/MarkdownContent";
import { NarrativeContent } from "./content/NarrativeContent";
import {
	ariaKeyShortcuts,
	commandByID,
} from "../shell/commandCatalog";

interface AgentNarrativeProps {
  sessionTitle: string;
  items: Item[];
	runs: RunSummary[];
  liveToolOutputs: Record<string, LiveToolOutput>;
  interrupts: PendingInterruptSet[];
  progress?: RunProgress;
  pending: boolean;
  interruptPending: boolean;
  interruptError?: string;
  streamError?: string;
  cancelingRunId?: string;
  cancelError?: { runId: string; message: string };
  onResume(
    interruptSet: PendingInterruptSet,
    responses: InterruptResponse[],
    idempotencyKey: string,
  ): Promise<void>;
  onCancelRun(runId: string): Promise<void>;
	onFeedback(itemId: string, runId: string, rating: FeedbackRating): Promise<void>;
	hasOlderHistory: boolean;
	historyPending: boolean;
	historyError?: string;
	onLoadOlderHistory(): Promise<void>;
	onForkFrom(runId: string): Promise<void>;
	onRollback(runId: string, restoreType: RestoreType): Promise<void>;
  searchRequest: number;
  children?: ReactNode;
}

interface NarrativeMaterial {
	runById: Map<string, RunSummary>;
  itemsByRunId: Map<string, Item[]>;
	childRunsByItemId: Map<string, RunSummary[]>;
  rootItems: Item[];
	orphanRuns: RunSummary[];
  liveToolOutputs: Record<string, LiveToolOutput>;
}

export function AgentNarrative(props: AgentNarrativeProps) {
  const { t } = useLocalization();
  const scroll = useRef<HTMLDivElement>(null);
	const searchInput = useRef<HTMLInputElement>(null);
	const observedSearchRequest = useRef(props.searchRequest);
  const followsTail = useRef(true);
	const [search, setSearch] = useState("");
	const [activeMatch, setActiveMatch] = useState(0);
  const material = useMemo(
    () => indexNarrative(props.items, props.runs, props.liveToolOutputs),
    [props.items, props.liveToolOutputs, props.runs],
  );
  const materialVersion = props.items
    .map((item) => `${item.id}:${item.status}:${itemTextLength(item)}`)
    .concat(
      props.interrupts.map(
        (set) => `${set.rootRunId}:${set.createdAt}:${set.interrupts.length}`,
      ),
    )
    .concat(
      props.runs.map(
        (run) => `${run.id}:${run.status}:${run.outcome?.type ?? ""}`,
      ),
    )
    .concat(
      Object.entries(props.liveToolOutputs).map(
        ([itemId, output]) => `${itemId}:${output.text.length}`,
      ),
    )
    .join("|");
	const normalizedSearch = search.trim().toLocaleLowerCase();
	const searchMatches = useMemo(
		() =>
			normalizedSearch === ""
				? []
				: props.items.filter((item) =>
						itemSearchText(item).toLocaleLowerCase().includes(normalizedSearch),
					),
		[normalizedSearch, props.items],
	);
	const activeMatchID = searchMatches[activeMatch]?.id;

  useEffect(() => {
    if (!followsTail.current || scroll.current === null) return;
    const frame = window.requestAnimationFrame(() => {
      scroll.current?.scrollTo({ top: scroll.current.scrollHeight });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [materialVersion]);

	useEffect(() => {
		setActiveMatch(0);
	}, [normalizedSearch]);

	useEffect(() => {
		if (activeMatch < searchMatches.length) return;
		setActiveMatch(Math.max(0, searchMatches.length - 1));
	}, [activeMatch, searchMatches.length]);

	useEffect(() => {
		if (activeMatchID === undefined) return;
		const frame = window.requestAnimationFrame(() => {
			const target = [...(scroll.current?.querySelectorAll<HTMLElement>("[data-item-id]") ?? [])]
				.find((element) => element.dataset.itemId === activeMatchID);
			target?.scrollIntoView({
				block: "center",
				behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
					? "auto"
					: "smooth",
			});
		});
		return () => window.cancelAnimationFrame(frame);
	}, [activeMatchID]);

	useEffect(() => {
		if (observedSearchRequest.current === props.searchRequest) return;
		observedSearchRequest.current = props.searchRequest;
		searchInput.current?.focus();
		searchInput.current?.select();
	}, [props.searchRequest]);

	const moveMatch = (direction: -1 | 1) => {
		if (searchMatches.length === 0) {
			if (props.hasOlderHistory && !props.historyPending) {
				followsTail.current = false;
				void props.onLoadOlderHistory().catch(() => undefined);
			}
			return;
		}
		setActiveMatch((current) =>
			(current + direction + searchMatches.length) % searchMatches.length,
		);
	};

  const trackReader = () => {
    const element = scroll.current;
    if (element === null) return;
    followsTail.current =
      element.scrollHeight - element.scrollTop - element.clientHeight < 56;
  };

  return (
    <div
      className="narrative-scroll"
      ref={scroll}
      onScroll={trackReader}
	  aria-busy={props.pending || props.historyPending}
    >
      <div className="narrative-timeline">
		{props.items.length > 0 ? (
		  <div className="history-navigator">
			<label>
			  <span aria-hidden="true">⌕</span>
			  <span className="sr-only">{t("narrative.searchLoadedHistory")}</span>
			  <input
				ref={searchInput}
				type="search"
				value={search}
				placeholder={t("narrative.searchPlaceholder")}
				autoComplete="off"
				aria-keyshortcuts={ariaKeyShortcuts(
					commandByID("narrative.search").shortcut,
				)}
				onChange={(event) => setSearch(event.target.value)}
				onKeyDown={(event) => {
				  if (event.key === "Escape") {
					setSearch("");
					return;
				  }
				  if (event.key === "Enter") {
					event.preventDefault();
					moveMatch(event.shiftKey ? -1 : 1);
				  }
				}}
			  />
			  {search ? (
				<button
				  type="button"
				  aria-label={t("narrative.clearSearch")}
				  onClick={() => setSearch("")}
				>
				  ×
				</button>
			  ) : null}
			</label>
			{normalizedSearch !== "" ? (
			  <span className="history-match-count" aria-live="polite">
				{searchMatches.length === 0
				  ? t("narrative.noLoadedMatches")
				  : t("narrative.matchPosition", {
					  current: activeMatch + 1,
					  total: searchMatches.length,
					})}
			  </span>
			) : null}
			{searchMatches.length > 0 ? (
			  <span className="history-match-actions">
				<button type="button" aria-label={t("narrative.previousMatch")} onClick={() => moveMatch(-1)}>↑</button>
				<button type="button" aria-label={t("narrative.nextMatch")} onClick={() => moveMatch(1)}>↓</button>
			  </span>
			) : null}
			{props.hasOlderHistory || props.historyError ? (
			  <button
				className="history-load-older"
				type="button"
				disabled={props.historyPending}
				onClick={() => {
				  followsTail.current = false;
				  void props.onLoadOlderHistory().catch(() => undefined);
				}}
			  >
				{props.historyPending
				  ? t("narrative.loadingHistory")
				  : props.historyError
					? t("narrative.retryHistory")
					: normalizedSearch !== "" && searchMatches.length === 0
					  ? t("narrative.searchOlder")
					  : t("narrative.loadOlder")}
			  </button>
			) : null}
			{props.historyError ? <p role="alert">{props.historyError}</p> : null}
		  </div>
		) : null}
        {props.streamError ? (
          <p className="stream-warning" role="status">
            {t("narrative.streamPaused", { detail: props.streamError })}
          </p>
        ) : null}
        {material.rootItems.length === 0 && material.orphanRuns.length === 0 ? (
          <section className="session-welcome">
            <span className="eyebrow">{t("narrative.ready")}</span>
            <h3>{props.sessionTitle || t("narrative.untitledSession")}</h3>
            <p>{t("narrative.welcome")}</p>
          </section>
        ) : (
          material.rootItems.map((item) => (
            <MaterialItem
              key={item.id}
              item={item}
              material={material}
              ancestry={new Set<string>()}
              pending={props.interruptPending}
              cancelingRunId={props.cancelingRunId}
              cancelError={props.cancelError}
              onCancelRun={props.onCancelRun}
			  onFeedback={props.onFeedback}
			  searchQuery={normalizedSearch}
			  activeMatchID={activeMatchID}
			  onForkFrom={props.onForkFrom}
			  onRollback={props.onRollback}
            />
          ))
        )}
        {material.orphanRuns.map((run) => (
          <DelegatedRunDisclosure
            key={run.id}
            run={run}
            material={material}
            ancestry={new Set<string>()}
            pending={props.interruptPending}
            cancelingRunId={props.cancelingRunId}
            cancelError={props.cancelError}
            onCancelRun={props.onCancelRun}
			onFeedback={props.onFeedback}
			searchQuery={normalizedSearch}
			activeMatchID={activeMatchID}
			onForkFrom={props.onForkFrom}
			onRollback={props.onRollback}
            integrity={t("narrative.orphanDelegation")}
          />
        ))}
        {props.interrupts.map((interruptSet) => (
          <InterruptSetCard
            key={`${interruptSet.rootRunId}:${interruptSet.createdAt}`}
            interruptSet={interruptSet}
            pending={props.interruptPending}
            error={props.interruptError}
            onResume={props.onResume}
          />
        ))}
        {props.children}
        {props.progress ? <LiveProgress progress={props.progress} /> : null}
      </div>
    </div>
  );
}

interface MaterialItemProps {
  item: Item;
  material: NarrativeMaterial;
  ancestry: Set<string>;
  pending: boolean;
  cancelingRunId?: string;
  cancelError?: { runId: string; message: string };
  onCancelRun(runId: string): Promise<void>;
	onFeedback(itemId: string, runId: string, rating: FeedbackRating): Promise<void>;
	searchQuery: string;
	activeMatchID?: string;
	onForkFrom(runId: string): Promise<void>;
	onRollback(runId: string, restoreType: RestoreType): Promise<void>;
}

function MaterialItem(props: MaterialItemProps) {
  const run = props.material.runById.get(props.item.runId);
  const children = props.material.childRunsByItemId.get(props.item.id) ?? [];
	const historyBoundary =
		run?.parentRunId === undefined &&
		run?.status === "finished" &&
		props.item.createdAt === run.createdAt &&
		props.material.itemsByRunId
			.get(props.item.runId)
			?.find((item) => item.type === "userMessage")?.id === props.item.id;
  return (
    <NarrativeItem
      item={props.item}
      run={run}
      liveOutput={props.material.liveToolOutputs[props.item.id]}
	  historyBoundary={historyBoundary}
	  onFeedback={props.onFeedback}
	  searchQuery={props.searchQuery}
	  activeMatch={props.item.id === props.activeMatchID}
	  onForkFrom={props.onForkFrom}
	  onRollback={props.onRollback}
    >
      {children.map((child) => (
        <DelegatedRunDisclosure
          key={child.id}
          run={child}
          material={props.material}
          ancestry={props.ancestry}
          pending={props.pending}
          cancelingRunId={props.cancelingRunId}
          cancelError={props.cancelError}
          onCancelRun={props.onCancelRun}
		  onFeedback={props.onFeedback}
		  searchQuery={props.searchQuery}
		  activeMatchID={props.activeMatchID}
		  onForkFrom={props.onForkFrom}
		  onRollback={props.onRollback}
        />
      ))}
    </NarrativeItem>
  );
}

function NarrativeItem({
  item,
  run,
  children,
  liveOutput,
	historyBoundary,
	onFeedback,
	searchQuery,
	activeMatch,
	onForkFrom,
	onRollback,
}: {
  item: Item;
  run?: RunSummary;
  children?: ReactNode;
  liveOutput?: LiveToolOutput;
	historyBoundary: boolean;
	onFeedback(itemId: string, runId: string, rating: FeedbackRating): Promise<void>;
	searchQuery: string;
	activeMatch: boolean;
	onForkFrom(runId: string): Promise<void>;
	onRollback(runId: string, restoreType: RestoreType): Promise<void>;
}) {
  const { t } = useLocalization();
  const child = run?.parentRunId !== undefined;
  switch (item.type) {
    case "userMessage":
      return (
        <article
		  className="narrative-item user-turn"
		  data-child={child}
		  data-item-id={item.id}
		  data-search-match={activeMatch}
		>
          <ItemMeta label={t("narrative.you")} item={item} run={run} />
		  <NarrativeContent content={item.content} highlight={searchQuery} />
		  {historyBoundary ? (
			<SessionHistoryActions
			  runId={run.id}
			  onForkFrom={onForkFrom}
			  onRollback={onRollback}
			/>
		  ) : null}
        </article>
      );
    case "agentMessage": {
      const final = item.phase === "finalAnswer";
      return (
        <article
          className={`narrative-item agent-turn ${final ? "final-turn" : "work-turn"}`}
          data-child={child}
          data-running={item.status === "running"}
		  data-item-id={item.id}
		  data-search-match={activeMatch}
        >
          <ItemMeta
			label={final ? t("narrative.answer") : t("narrative.lyra")}
			item={item}
			run={run}
		  />
		  <NarrativeContent content={item.content} highlight={searchQuery} />
          {item.status === "running" ? <TypingMark /> : null}
		  {final && !child && run?.status === "finished" && item.status === "completed" ? (
			<FeedbackActions
			  itemId={item.id}
			  runId={run.id}
			  onFeedback={onFeedback}
			/>
		  ) : null}
        </article>
      );
    }
    case "reasoning":
      return (
        <details
          className="narrative-item reasoning-turn"
          data-child={child}
          defaultOpen={item.status === "running"}
		  data-item-id={item.id}
		  data-search-match={activeMatch}
        >
          <summary>
            <span>{t("narrative.reasoning")}</span>
            <small>
			  {item.status === "running"
				? t("narrative.working")
				: t("narrative.complete")}
			</small>
          </summary>
		  <div className="message-content">
			<MarkdownContent
			  source={item.redacted ? t("narrative.reasoningRedacted") : item.text ?? ""}
			  highlight={searchQuery}
			/>
		  </div>
          {item.status === "running" ? <TypingMark /> : null}
        </details>
      );
    case "toolCall":
      return (
        <ToolDisclosure
		  item={item}
		  run={run}
		  liveOutput={liveOutput}
		  searchMatch={activeMatch}
		>
          {children}
        </ToolDisclosure>
      );
    case "question":
      return (
        <article
		  className="narrative-item question-turn"
		  data-child={child}
		  data-item-id={item.id}
		  data-search-match={activeMatch}
		>
          <ItemMeta label={t("narrative.inputNeeded")} item={item} run={run} />
          {item.question?.fields.map((field, index) => (
			<p key={`${item.id}:${index}`}><HighlightedText text={field.prompt} query={searchQuery} /></p>
          ))}
        </article>
      );
    case "compaction":
      return (
        <aside
		  className="narrative-boundary"
		  data-item-id={item.id}
		  data-search-match={activeMatch}
		>
          {t("narrative.contextCompacted")}
          {item.droppedMessages
            ? ` · ${t("narrative.messagesCondensed", {
				count: item.droppedMessages,
			  })}`
            : ""}
        </aside>
      );
    default:
      return null;
  }
}

interface DelegatedRunDisclosureProps {
	run: RunSummary;
  material: NarrativeMaterial;
  ancestry: Set<string>;
  pending: boolean;
  cancelingRunId?: string;
  cancelError?: { runId: string; message: string };
  integrity?: string;
  onCancelRun(runId: string): Promise<void>;
	onFeedback(itemId: string, runId: string, rating: FeedbackRating): Promise<void>;
	searchQuery: string;
	activeMatchID?: string;
	onForkFrom(runId: string): Promise<void>;
	onRollback(runId: string, restoreType: RestoreType): Promise<void>;
}

function DelegatedRunDisclosure(props: DelegatedRunDisclosureProps) {
  const { t } = useLocalization();
  if (props.ancestry.has(props.run.id)) {
    return (
      <p className="delegated-run-integrity" role="alert">
        {t("narrative.delegationCycle", {
		  id: shortIdentity(props.run.id),
		})}
      </p>
    );
  }
  const ancestry = new Set(props.ancestry).add(props.run.id);
  const items = props.material.itemsByRunId.get(props.run.id) ?? [];
  const active =
    props.run.status === "running" || props.run.status === "waiting";
  const canceling = props.cancelingRunId === props.run.id;
  const error =
    props.cancelError?.runId === props.run.id
      ? props.cancelError.message
      : undefined;
  const outcomeDetail =
    props.run.outcome?.error?.detail ?? props.run.outcome?.detail;
  return (
    <section
      className="delegated-run"
      data-status={runState(props.run)}
      aria-label={t("narrative.delegatedRunLabel", { id: props.run.id })}
    >
      <header className="delegated-run-header">
        <span className="delegated-run-state" aria-hidden="true" />
        <span className="delegated-run-identity">
          <strong>{t("narrative.delegatedRun")}</strong>
          <small title={props.run.id}>
            {modelIdentity(props.run, t)} · {shortIdentity(props.run.id)}
          </small>
        </span>
        <span className="delegated-run-status">
		  {runStateLabel(runState(props.run), t)}
		</span>
        {active ? (
          <button
            className="delegated-run-cancel"
            type="button"
            disabled={props.pending}
            onClick={() =>
              void props.onCancelRun(props.run.id).catch(() => undefined)
            }
          >
            {canceling ? t("narrative.canceling") : t("narrative.cancel")}
          </button>
        ) : null}
      </header>
      {props.integrity ? (
        <p className="delegated-run-integrity" role="status">
          {props.integrity}
        </p>
      ) : null}
      {error ? (
        <p className="delegated-run-error" role="alert">
          {error}
        </p>
      ) : null}
      <div className="delegated-run-material">
        {items.length === 0 ? (
          <p className="delegated-run-empty">
            {active
              ? t("narrative.waitingDelegatedMaterial")
              : t("narrative.noDelegatedMaterial")}
          </p>
        ) : (
          items.map((item) => (
            <MaterialItem
              key={item.id}
              item={item}
              material={props.material}
              ancestry={ancestry}
              pending={props.pending}
              cancelingRunId={props.cancelingRunId}
              cancelError={props.cancelError}
              onCancelRun={props.onCancelRun}
			  onFeedback={props.onFeedback}
			  searchQuery={props.searchQuery}
			  activeMatchID={props.activeMatchID}
			  onForkFrom={props.onForkFrom}
			  onRollback={props.onRollback}
            />
          ))
        )}
      </div>
      {outcomeDetail ? (
        <p className="delegated-run-outcome">{outcomeDetail}</p>
      ) : null}
    </section>
  );
}

function FeedbackActions(props: {
	itemId: string;
	runId: string;
	onFeedback(itemId: string, runId: string, rating: FeedbackRating): Promise<void>;
}) {
	const { t } = useLocalization();
	const [pending, setPending] = useState<FeedbackRating>();
	const [selected, setSelected] = useState<FeedbackRating>();
	const [error, setError] = useState<string>();
	const submit = async (rating: FeedbackRating) => {
		if (pending !== undefined || selected !== undefined) return;
		setPending(rating);
		setError(undefined);
		try {
			await props.onFeedback(props.itemId, props.runId, rating);
			setSelected(rating);
		} catch (failure) {
			setError(
				failure instanceof Error
					? failure.message
					: t("narrative.feedbackSaveFailed"),
			);
		} finally {
			setPending(undefined);
		}
	};
	return (
		<footer className="feedback-actions" aria-label={t("narrative.rateAnswer")}>
			<span>
			  {selected === undefined
				? t("narrative.wasHelpful")
				: t("narrative.feedbackSaved")}
			</span>
			<button
				type="button"
				aria-label={t("narrative.helpful")}
				aria-pressed={selected === "positive"}
				disabled={pending !== undefined || selected !== undefined}
				onClick={() => void submit("positive")}
			>
				<span aria-hidden="true">↑</span> {t("narrative.helpful")}
			</button>
			<button
				type="button"
				aria-label={t("narrative.needsWork")}
				aria-pressed={selected === "negative"}
				disabled={pending !== undefined || selected !== undefined}
				onClick={() => void submit("negative")}
			>
				<span aria-hidden="true">↓</span> {t("narrative.needsWork")}
			</button>
			{error ? <p role="alert">{error}</p> : null}
		</footer>
	);
}

function ItemMeta(props: { label: string; item: Item; run?: RunSummary }) {
  const { formatDateTime, t } = useLocalization();
  const occurredAt = props.item.createdAt ?? props.item.startedAt;
  return (
    <header className="item-meta">
      <strong>{props.label}</strong>
      {props.run?.parentRunId ? <span>{t("narrative.delegated")}</span> : null}
      {occurredAt ? (
        <time dateTime={occurredAt}>
		  {formatDateTime(new Date(occurredAt), {
			hour: "numeric",
			minute: "2-digit",
		  })}
		</time>
      ) : null}
    </header>
  );
}

function SessionHistoryActions(props: {
	runId: string;
	onForkFrom(runId: string): Promise<void>;
	onRollback(runId: string, restoreType: RestoreType): Promise<void>;
}) {
	const { t } = useLocalization();
	const [pending, setPending] = useState(false);
	const [error, setError] = useState<string>();
	const run = async (action: () => Promise<void>) => {
		if (pending) return;
		setPending(true);
		setError(undefined);
		try {
			await action();
		} catch (failure) {
			setError(
				failure instanceof Error
					? failure.message
					: t("narrative.historyActionFailed"),
			);
		} finally {
			setPending(false);
		}
	};
	return (
		<div className="session-history-actions">
			<button
				type="button"
				disabled={pending}
				onClick={() => void run(() => props.onForkFrom(props.runId))}
			>
				{t("narrative.forkHere")}
			</button>
			<details>
				<summary>{t("narrative.rewind")}</summary>
				<div>
					<button
						type="button"
						disabled={pending}
						onClick={() =>
							void run(() => props.onRollback(props.runId, "history"))
						}
					>
						{t("narrative.rewindHistory")}
					</button>
					<button
						type="button"
						disabled={pending}
						onClick={() =>
							void run(() => props.onRollback(props.runId, "files"))
						}
					>
						{t("narrative.rewindFiles")}
					</button>
					<button
						type="button"
						disabled={pending}
						onClick={() =>
							void run(() => props.onRollback(props.runId, "both"))
						}
					>
						{t("narrative.rewindBoth")}
					</button>
				</div>
			</details>
			{error ? <p role="alert">{error}</p> : null}
		</div>
	);
}

function HighlightedText(props: { text: string; query: string }) {
	if (props.query === "") return props.text;
	const source = props.text.toLocaleLowerCase();
	const query = props.query.toLocaleLowerCase();
	const fragments: ReactNode[] = [];
	let cursor = 0;
	let match = source.indexOf(query);
	while (match >= 0) {
		if (match > cursor) fragments.push(props.text.slice(cursor, match));
		fragments.push(
			<mark key={`${match}:${fragments.length}`}>
				{props.text.slice(match, match + query.length)}
			</mark>,
		);
		cursor = match + query.length;
		match = source.indexOf(query, cursor);
	}
	if (cursor < props.text.length) fragments.push(props.text.slice(cursor));
	return fragments.length === 0 ? props.text : fragments;
}

function itemSearchText(item: Item): string {
	const content = (item.content ?? []).map((block) => block.text ?? "");
	const prompts = item.question?.fields.map((field) => field.prompt) ?? [];
	return [
		item.type,
		item.text ?? "",
		item.summary ?? "",
		item.tool?.name ?? "",
		item.error?.detail ?? "",
		...content,
		...prompts,
	].join("\n");
}

function TypingMark() {
  const { t } = useLocalization();
  return (
    <span className="typing-mark" aria-label={t("narrative.responding")}>
      <i />
      <i />
      <i />
    </span>
  );
}

function LiveProgress({ progress }: { progress: RunProgress }) {
  const { formatNumber, t } = useLocalization();
  const tokens = progress.usage
    ? (progress.usage.inputTokens ?? 0) + (progress.usage.outputTokens ?? 0)
    : undefined;
  return (
    <div className="live-progress" role="status">
      <span className="status-dot" aria-hidden="true" />
      <span>{progress.activity || t("narrative.agentWorking")}</span>
      {progress.step ? (
		<small>{t("narrative.step", { step: progress.step })}</small>
	  ) : null}
      {tokens ? (
		<small>{t("narrative.tokens", { count: formatNumber(tokens) })}</small>
	  ) : null}
    </div>
  );
}

function itemTextLength(item: Item) {
  return (
    (item.text?.length ?? 0) +
    (item.content ?? []).reduce(
      (total, block) => total + (block.text?.length ?? block.data?.length ?? 0),
      0,
    )
  );
}

function indexNarrative(
  items: Item[],
	runs: RunSummary[],
  liveToolOutputs: Record<string, LiveToolOutput>,
): NarrativeMaterial {
  const runById = new Map(runs.map((run) => [run.id, run]));
  const itemById = new Map(items.map((item) => [item.id, item]));
  const itemsByRunId = new Map<string, Item[]>();
  for (const item of items) {
    const material = itemsByRunId.get(item.runId) ?? [];
    material.push(item);
    itemsByRunId.set(item.runId, material);
  }

	const childRunsByItemId = new Map<string, RunSummary[]>();
	const orphanRuns: RunSummary[] = [];
  for (const run of runs) {
    if (run.parentRunId === undefined) continue;
    const parent = runById.get(run.parentRunId);
    const owner = run.spawnedByItemId
      ? itemById.get(run.spawnedByItemId)
      : undefined;
    if (
      parent === undefined ||
      owner === undefined ||
      owner.runId !== parent.id ||
      owner.type !== "toolCall" ||
      owner.tool?.name !== "delegate_task"
    ) {
      orphanRuns.push(run);
      continue;
    }
    const siblings = childRunsByItemId.get(owner.id) ?? [];
    siblings.push(run);
    childRunsByItemId.set(owner.id, siblings);
  }

  return {
    runById,
    itemsByRunId,
    childRunsByItemId,
    rootItems: items.filter((item) => {
      const run = runById.get(item.runId);
      return run === undefined || run.parentRunId === undefined;
    }),
    orphanRuns,
    liveToolOutputs,
  };
}

function runState(run: RunSummary) {
  return run.status === "finished"
    ? (run.outcome?.type ?? "finished")
    : (run.status ?? "unknown");
}

function modelIdentity(run: RunSummary, t: Translate) {
  if (run.provider && run.model) return `${run.provider}/${run.model}`;
  return run.model ?? run.provider ?? t("narrative.defaultModel");
}

function runStateLabel(state: string, t: Translate) {
  switch (state) {
    case "finished":
      return t("narrative.status.finished");
    case "running":
      return t("narrative.status.running");
    case "waiting":
      return t("narrative.status.waiting");
    case "completed":
      return t("narrative.status.completed");
    case "failed":
      return t("narrative.status.failed");
    case "canceled":
      return t("narrative.status.canceled");
    case "timedOut":
      return t("narrative.status.timedOut");
    case "maxSteps":
      return t("narrative.status.maxSteps");
    case "maxBudget":
      return t("narrative.status.maxBudget");
    case "lost":
      return t("narrative.status.lost");
    default:
      return t("narrative.status.unknown");
  }
}

function shortIdentity(value: string) {
  return value.length > 12 ? `${value.slice(0, 8)}…${value.slice(-3)}` : value;
}
