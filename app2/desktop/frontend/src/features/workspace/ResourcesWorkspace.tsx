import { useQuery } from "@tanstack/react-query";

import type {
  AgentDoc,
  Recipe,
  RuntimeConnection,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import {
  listAgentDocs,
  listRecipes,
  runtimeQueryKeys,
} from "../../runtime/runtimeQueries";
import type { ResourceView, SkillView } from "./contextDockState";
import { SkillsWorkspace } from "./SkillsWorkspace";

interface ResourcesWorkspaceProps {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  skillsEnabled: boolean;
  view: ResourceView;
  skillView: SkillView;
  onViewChange(view: ResourceView): void;
  onSkillViewChange(view: SkillView): void;
}

export function ResourcesWorkspace(props: ResourcesWorkspaceProps) {
  const recipes = useQuery({
    queryKey: runtimeQueryKeys.workspaceRecipes(
      props.connection,
      props.workspace.path,
    ),
    queryFn: ({ signal }) =>
      listRecipes(props.connection, props.workspace, signal),
    retry: 2,
  });
  const agentDocs = useQuery({
    queryKey: runtimeQueryKeys.workspaceAgentDocs(
      props.connection,
      props.workspace.path,
    ),
    queryFn: ({ signal }) =>
      listAgentDocs(props.connection, props.workspace, signal),
    retry: 2,
  });

  return (
    <section className="resources-workspace" aria-label="Workspace resources">
      <nav className="resource-view-switch" aria-label="Resource types">
        <ResourceButton
          label="Skills"
          selected={props.view === "skills"}
          onSelect={() => props.onViewChange("skills")}
        />
        <ResourceButton
          label="Recipes"
          count={recipes.data?.data.length}
          selected={props.view === "recipes"}
          onSelect={() => props.onViewChange("recipes")}
        />
        <ResourceButton
          label="Agent docs"
          count={agentDocs.data?.data.length}
          selected={props.view === "agentDocs"}
          onSelect={() => props.onViewChange("agentDocs")}
        />
      </nav>
      {props.view === "skills" ? (
        <SkillsWorkspace
          connection={props.connection}
          workspace={props.workspace}
          enabled={props.skillsEnabled}
          view={props.skillView}
          onViewChange={props.onSkillViewChange}
        />
      ) : props.view === "recipes" ? (
        <RecipeCatalog
          values={recipes.data?.data}
          pending={recipes.isPending}
          error={recipes.error}
          onRetry={() => void recipes.refetch()}
        />
      ) : (
        <AgentDocCatalog
          values={agentDocs.data?.data}
          pending={agentDocs.isPending}
          error={agentDocs.error}
          onRetry={() => void agentDocs.refetch()}
        />
      )}
    </section>
  );
}

function ResourceButton(props: {
  label: string;
  count?: number;
  selected: boolean;
  onSelect(): void;
}) {
  return (
    <button
      type="button"
      aria-current={props.selected ? "page" : undefined}
      onClick={props.onSelect}
    >
      <span>{props.label}</span>
      {props.count === undefined ? null : <small>{props.count}</small>}
    </button>
  );
}

function RecipeCatalog(props: {
  values?: Recipe[];
  pending: boolean;
  error: Error | null;
  onRetry(): void;
}) {
  if (props.pending) return <ResourceState title="Discovering Recipes…" />;
  if (props.error) {
    return (
      <ResourceState
        title="Recipes could not be discovered"
        detail={messageOf(props.error)}
        action="Try again"
        onAction={props.onRetry}
      />
    );
  }
  if (!props.values || props.values.length === 0) {
    return (
      <ResourceState
        title="No Recipes available"
        detail="Add a slash-invocable Markdown recipe under .lyra/recipes."
      />
    );
  }
  return (
    <div className="resource-card-list">
      {props.values.map((recipe) => (
        <article className="resource-card" key={`${recipe.scope}:${recipe.name}`}>
          <header>
            <div>
              <h4>/{recipe.name}</h4>
              {recipe.argumentHint ? <small>{recipe.argumentHint}</small> : null}
            </div>
            <ResourceTag>{recipe.scope}</ResourceTag>
          </header>
          <p>{recipe.description || "No description provided."}</p>
          <details>
            <summary>View prompt</summary>
            <pre>{recipe.body}</pre>
          </details>
          <footer title={recipe.source}>{compactPath(recipe.source)}</footer>
        </article>
      ))}
    </div>
  );
}

function AgentDocCatalog(props: {
  values?: AgentDoc[];
  pending: boolean;
  error: Error | null;
  onRetry(): void;
}) {
  if (props.pending) return <ResourceState title="Resolving Agent docs…" />;
  if (props.error) {
    return (
      <ResourceState
        title="Agent docs could not be resolved"
        detail={messageOf(props.error)}
        action="Try again"
        onAction={props.onRetry}
      />
    );
  }
  if (!props.values || props.values.length === 0) {
    return (
      <ResourceState
        title="No Agent docs apply"
        detail="Add AGENTS.md at home, project root, or below the selected workspace."
      />
    );
  }
  return (
    <div className="resource-card-list">
      {props.values.map((document) => (
        <article className="resource-card agent-doc-card" key={document.path}>
          <header>
            <h4>{document.title || "AGENTS.md"}</h4>
            <ResourceTag>{document.scope}</ResourceTag>
          </header>
          <p title={document.path}>{document.path}</p>
        </article>
      ))}
    </div>
  );
}

function ResourceTag(props: { children: string }) {
  return <span className="resource-tag">{props.children}</span>;
}

function ResourceState(props: {
  title: string;
  detail?: string;
  action?: string;
  onAction?(): void;
}) {
  return (
    <div className="resource-state">
      <h4>{props.title}</h4>
      {props.detail ? <p>{props.detail}</p> : null}
      {props.action && props.onAction ? (
        <button type="button" onClick={props.onAction}>
          {props.action}
        </button>
      ) : null}
    </div>
  );
}

function compactPath(path: string) {
  const parts = path.split(/[\\/]/).filter(Boolean);
  return parts.length > 3 ? `…/${parts.slice(-3).join("/")}` : path;
}

function messageOf(error: unknown) {
  return error instanceof Error
    ? error.message
    : "The resource could not be loaded.";
}
