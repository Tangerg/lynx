import { useEffect, useMemo, useRef, type ReactNode } from "react";

import type {
  ContentBlock,
  InterruptResponse,
  Item,
  PendingInterruptSet,
  RunProgress,
  RunRef,
} from "@lyra/runtime-contract";

import { InterruptSetCard } from "./InterruptSetCard";
import { ToolDisclosure } from "./ToolDisclosure";

interface AgentNarrativeProps {
  sessionTitle: string;
  items: Item[];
  runs: RunRef[];
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
  children?: ReactNode;
}

interface NarrativeMaterial {
  runById: Map<string, RunRef>;
  itemsByRunId: Map<string, Item[]>;
  childRunsByItemId: Map<string, RunRef[]>;
  rootItems: Item[];
  orphanRuns: RunRef[];
}

export function AgentNarrative(props: AgentNarrativeProps) {
  const scroll = useRef<HTMLDivElement>(null);
  const followsTail = useRef(true);
  const material = useMemo(
    () => indexNarrative(props.items, props.runs),
    [props.items, props.runs],
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
    .join("|");

  useEffect(() => {
    if (!followsTail.current || scroll.current === null) return;
    const frame = window.requestAnimationFrame(() => {
      scroll.current?.scrollTo({ top: scroll.current.scrollHeight });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [materialVersion]);

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
      aria-busy={props.pending}
    >
      <div className="narrative-timeline">
        {props.streamError ? (
          <p className="stream-warning" role="status">
            Live updates paused: {props.streamError}. Durable material is being
            reloaded.
          </p>
        ) : null}
        {material.rootItems.length === 0 && material.orphanRuns.length === 0 ? (
          <section className="session-welcome">
            <span className="eyebrow">Ready</span>
            <h3>{props.sessionTitle || "Untitled session"}</h3>
            <p>
              Describe the work below. Lyra will keep the conversation,
              execution facts, and recovery state attached to this workspace.
            </p>
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
            integrity="The parent delegation is unavailable in this snapshot."
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
}

function MaterialItem(props: MaterialItemProps) {
  const run = props.material.runById.get(props.item.runId);
  const children = props.material.childRunsByItemId.get(props.item.id) ?? [];
  return (
    <NarrativeItem item={props.item} run={run}>
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
        />
      ))}
    </NarrativeItem>
  );
}

function NarrativeItem({
  item,
  run,
  children,
}: {
  item: Item;
  run?: RunRef;
  children?: ReactNode;
}) {
  const child = run?.parentRunId !== undefined;
  switch (item.type) {
    case "userMessage":
      return (
        <article className="narrative-item user-turn" data-child={child}>
          <ItemMeta label="You" item={item} run={run} />
          <Content content={item.content} />
        </article>
      );
    case "agentMessage": {
      const final = item.phase === "finalAnswer";
      return (
        <article
          className={`narrative-item agent-turn ${final ? "final-turn" : "work-turn"}`}
          data-child={child}
          data-running={item.status === "running"}
        >
          <ItemMeta label={final ? "Answer" : "Lyra"} item={item} run={run} />
          <Content content={item.content} />
          {item.status === "running" ? <TypingMark /> : null}
        </article>
      );
    }
    case "reasoning":
      return (
        <details
          className="narrative-item reasoning-turn"
          data-child={child}
          defaultOpen={item.status === "running"}
        >
          <summary>
            <span>Reasoning</span>
            <small>{item.status === "running" ? "working" : "complete"}</small>
          </summary>
          <NarrativeText text={item.redacted ? "Reasoning was redacted." : item.text ?? ""} />
          {item.status === "running" ? <TypingMark /> : null}
        </details>
      );
    case "toolCall":
      return <ToolDisclosure item={item} run={run}>{children}</ToolDisclosure>;
    case "question":
      return (
        <article className="narrative-item question-turn" data-child={child}>
          <ItemMeta label="Input needed" item={item} run={run} />
          {item.question?.fields.map((field, index) => (
            <p key={`${item.id}:${index}`}>{field.prompt}</p>
          ))}
        </article>
      );
    case "compaction":
      return (
        <aside className="narrative-boundary">
          Context compacted
          {item.droppedMessages
            ? ` · ${item.droppedMessages} messages condensed`
            : ""}
        </aside>
      );
    default:
      return null;
  }
}

interface DelegatedRunDisclosureProps {
  run: RunRef;
  material: NarrativeMaterial;
  ancestry: Set<string>;
  pending: boolean;
  cancelingRunId?: string;
  cancelError?: { runId: string; message: string };
  integrity?: string;
  onCancelRun(runId: string): Promise<void>;
}

function DelegatedRunDisclosure(props: DelegatedRunDisclosureProps) {
  if (props.ancestry.has(props.run.id)) {
    return (
      <p className="delegated-run-integrity" role="alert">
        Delegated run lineage contains a cycle at {shortIdentity(props.run.id)}.
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
      aria-label={`Delegated run ${props.run.id}`}
    >
      <header className="delegated-run-header">
        <span className="delegated-run-state" aria-hidden="true" />
        <span className="delegated-run-identity">
          <strong>Delegated run</strong>
          <small title={props.run.id}>
            {modelIdentity(props.run)} · {shortIdentity(props.run.id)}
          </small>
        </span>
        <span className="delegated-run-status">{runState(props.run)}</span>
        {active ? (
          <button
            className="delegated-run-cancel"
            type="button"
            disabled={props.pending}
            onClick={() =>
              void props.onCancelRun(props.run.id).catch(() => undefined)
            }
          >
            {canceling ? "Canceling…" : "Cancel"}
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
              ? "Waiting for delegated material…"
              : "No delegated material was recorded."}
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

function ItemMeta(props: { label: string; item: Item; run?: RunRef }) {
  const occurredAt = props.item.createdAt ?? props.item.startedAt;
  return (
    <header className="item-meta">
      <strong>{props.label}</strong>
      {props.run?.parentRunId ? <span>Delegated</span> : null}
      {occurredAt ? (
        <time dateTime={occurredAt}>{formatTime(occurredAt)}</time>
      ) : null}
    </header>
  );
}

function Content({ content = [] }: { content: ContentBlock[] | undefined }) {
  return (
    <div className="message-content">
      {content.map((block, index) =>
        block.type === "image" && block.mime && block.data ? (
          <img
            key={index}
            src={`data:${block.mime};base64,${block.data}`}
            alt="Attached visual"
            loading="lazy"
          />
        ) : (
          <NarrativeText key={index} text={block.text ?? ""} />
        ),
      )}
    </div>
  );
}

function NarrativeText({ text }: { text: string }) {
  if (text === "") return null;
  const blocks = text.split(/```/g);
  return (
    <>
      {blocks.map((block, index) =>
        index % 2 === 1 ? (
          <pre key={index} className="message-code">
            <code>{block.replace(/^\w+\n/, "")}</code>
          </pre>
        ) : (
          <p key={index}>{block}</p>
        ),
      )}
    </>
  );
}

function TypingMark() {
  return (
    <span className="typing-mark" aria-label="Lyra is responding">
      <i />
      <i />
      <i />
    </span>
  );
}

function LiveProgress({ progress }: { progress: RunProgress }) {
  const tokens = progress.usage
    ? (progress.usage.inputTokens ?? 0) + (progress.usage.outputTokens ?? 0)
    : undefined;
  return (
    <div className="live-progress" role="status">
      <span className="status-dot" aria-hidden="true" />
      <span>{progress.activity || "Agent is working"}</span>
      {progress.step ? <small>Step {progress.step}</small> : null}
      {tokens ? <small>{tokens.toLocaleString()} tokens</small> : null}
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

function indexNarrative(items: Item[], runs: RunRef[]): NarrativeMaterial {
  const runById = new Map(runs.map((run) => [run.id, run]));
  const itemById = new Map(items.map((item) => [item.id, item]));
  const itemsByRunId = new Map<string, Item[]>();
  for (const item of items) {
    const material = itemsByRunId.get(item.runId) ?? [];
    material.push(item);
    itemsByRunId.set(item.runId, material);
  }

  const childRunsByItemId = new Map<string, RunRef[]>();
  const orphanRuns: RunRef[] = [];
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
  };
}

function runState(run: RunRef) {
  return run.status === "finished"
    ? (run.outcome?.type ?? "finished")
    : (run.status ?? "unknown");
}

function modelIdentity(run: RunRef) {
  if (run.provider && run.model) return `${run.provider}/${run.model}`;
  return run.model ?? run.provider ?? "default model";
}

function shortIdentity(value: string) {
  return value.length > 12 ? `${value.slice(0, 8)}…${value.slice(-3)}` : value;
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(value));
}
