import { useEffect, useRef, useState, type FormEvent } from "react";

import type {
  RuntimeConnection,
  ToolSpec,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import { invokeDiagnosticTool } from "../../runtime/runtimeQueries";
import { ResourceState } from "./ResourceState";

interface ToolCatalogWorkspaceProps {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  values?: ToolSpec[];
  pending: boolean;
  error: Error | null;
  onRetry(): void;
}

export function ToolCatalogWorkspace(props: ToolCatalogWorkspaceProps) {
  if (props.pending) return <ResourceState title="Loading diagnostic tools…" />;
  if (props.error) {
    return (
      <ResourceState
        title="Diagnostic tools could not be loaded"
        detail={messageOf(props.error)}
        action="Try again"
        onAction={props.onRetry}
      />
    );
  }
  if (!props.values || props.values.length === 0) {
    return (
      <ResourceState
        title="No direct tools available"
        detail="This Runtime does not expose any capability that is safe to invoke outside an Agent Run."
      />
    );
  }

  return (
    <div className="tool-catalog">
      <header className="tool-catalog-intro">
        <strong>Direct diagnostics</strong>
        <p>
          These read-only tools run against the current workspace without a
          session, model loop, or approval flow. Agent-only tools remain inside
          the Run that owns their authority.
        </p>
      </header>
      <div className="resource-card-list tool-card-list">
        {props.values.map((tool) => (
          <ToolCard
            key={`${props.connection.generation}:${props.workspace.path}:${tool.name}`}
            connection={props.connection}
            workspace={props.workspace}
            tool={tool}
          />
        ))}
      </div>
    </div>
  );
}

function ToolCard(props: {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  tool: ToolSpec;
}) {
  const active = useRef<AbortController | undefined>(undefined);
  const [draft, setDraft] = useState(() => exampleArguments(props.tool.name));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string>();
  const [result, setResult] = useState<string>();

  useEffect(
    () => () => {
      active.current?.abort();
      active.current = undefined;
    },
    [],
  );

  const invoke = async (event: FormEvent) => {
    event.preventDefault();
    if (pending) return;
    setError(undefined);
    setResult(undefined);
    let args: Record<string, unknown>;
    try {
      args = parseArguments(draft);
    } catch (cause) {
      setError(messageOf(cause));
      return;
    }
    active.current?.abort();
    const request = new AbortController();
    active.current = request;
    setPending(true);
    try {
      const value = await invokeDiagnosticTool(
        props.connection,
        props.workspace,
        props.tool.name,
        args,
        request.signal,
      );
      if (!request.signal.aborted) setResult(renderResult(value));
    } catch (cause) {
      if (!request.signal.aborted) setError(messageOf(cause));
    } finally {
      if (active.current === request) {
        active.current = undefined;
        setPending(false);
      }
    }
  };

  return (
    <article className="resource-card tool-card">
      <header>
        <div>
          <h4>{props.tool.name}</h4>
          <small title={props.workspace.path}>{props.workspace.path}</small>
        </div>
        <span className="resource-tag">{props.tool.safetyClass ?? "safe"}</span>
      </header>
      <p>{props.tool.description || "No description provided."}</p>
      <details>
        <summary>JSON Schema</summary>
        <pre>{JSON.stringify(props.tool.parameters ?? {}, null, 2)}</pre>
      </details>
      <form className="tool-invocation" onSubmit={(event) => void invoke(event)}>
        <label htmlFor={`tool-arguments-${props.tool.name}`}>Arguments</label>
        <textarea
          id={`tool-arguments-${props.tool.name}`}
          rows={5}
          maxLength={65_536}
          spellCheck={false}
          value={draft}
          onChange={(event) => {
            setDraft(event.currentTarget.value);
            setError(undefined);
          }}
        />
        <footer>
          <span>JSON object</span>
          <button type="submit" disabled={pending || draft.trim() === ""}>
            {pending ? "Running…" : "Run diagnostic"}
          </button>
        </footer>
      </form>
      {error ? (
        <p className="tool-invocation-error" role="alert">
          {error}
        </p>
      ) : null}
      {result === undefined ? null : (
        <section className="tool-invocation-result" aria-live="polite">
          <strong>Result</strong>
          <pre>{result}</pre>
        </section>
      )}
    </article>
  );
}

function parseArguments(value: string): Record<string, unknown> {
  const parsed: unknown = JSON.parse(value);
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error("Arguments must be one JSON object.");
  }
  return parsed as Record<string, unknown>;
}

function renderResult(value: unknown) {
  if (typeof value === "string") return value;
  const encoded = JSON.stringify(value, null, 2);
  return encoded ?? String(value);
}

function exampleArguments(name: string) {
  if (name === "read") {
    return JSON.stringify({ path: "README.md", max_lines: 120 }, null, 2);
  }
  if (name === "glob") {
    return JSON.stringify({ pattern: "**/*.go", max_results: 100 }, null, 2);
  }
  if (name === "grep") {
    return JSON.stringify(
      { pattern: "TODO", path: ".", max_results: 100 },
      null,
      2,
    );
  }
  return "{}";
}

function messageOf(error: unknown) {
  return error instanceof Error
    ? error.message
    : "The request could not be completed.";
}
