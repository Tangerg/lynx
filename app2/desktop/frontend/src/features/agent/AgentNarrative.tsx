import { useEffect, useMemo, useRef, type ReactNode } from "react";

import type {
  ContentBlock,
  Item,
  RunProgress,
  RunRef,
} from "@lyra/runtime-contract";

interface AgentNarrativeProps {
  sessionTitle: string;
  items: Item[];
  runs: RunRef[];
  progress?: RunProgress;
  pending: boolean;
  streamError?: string;
  children?: ReactNode;
}

export function AgentNarrative(props: AgentNarrativeProps) {
  const scroll = useRef<HTMLDivElement>(null);
  const followsTail = useRef(true);
  const runById = useMemo(
    () => new Map(props.runs.map((run) => [run.id, run])),
    [props.runs],
  );
  const materialVersion = props.items
    .map((item) => `${item.id}:${item.status}:${itemTextLength(item)}`)
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
        {props.items.length === 0 ? (
          <section className="session-welcome">
            <span className="eyebrow">Ready</span>
            <h3>{props.sessionTitle || "Untitled session"}</h3>
            <p>
              Describe the work below. Lyra will keep the conversation,
              execution facts, and recovery state attached to this workspace.
            </p>
          </section>
        ) : (
          props.items.map((item) => (
            <NarrativeItem
              key={item.id}
              item={item}
              run={runById.get(item.runId)}
            />
          ))
        )}
        {props.children}
        {props.progress ? <LiveProgress progress={props.progress} /> : null}
      </div>
    </div>
  );
}

function NarrativeItem({ item, run }: { item: Item; run?: RunRef }) {
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
      return (
        <details className="narrative-item tool-turn" data-child={child}>
          <summary>
            <span>{item.tool?.name ?? "Tool"}</span>
            <small>{item.status}</small>
          </summary>
          <pre>{JSON.stringify(item.tool?.arguments ?? {}, null, 2)}</pre>
          {item.tool?.result !== undefined ? (
            <pre>{JSON.stringify(item.tool.result, null, 2)}</pre>
          ) : null}
          {item.error?.detail ? <p role="alert">{item.error.detail}</p> : null}
        </details>
      );
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

function formatTime(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(value));
}
