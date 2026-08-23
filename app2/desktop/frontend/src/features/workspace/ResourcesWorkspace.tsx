import { useQuery } from "@tanstack/react-query";

import type {
  AgentDoc,
  Recipe,
  RuntimeConnection,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import { useLocalization } from "../localization/Localization";
import {
  listAgentDocs,
  listDiagnosticTools,
  listRecipes,
  runtimeQueryKeys,
} from "../../runtime/runtimeQueries";
import type { ResourceView, SkillView } from "./contextDockState";
import { KnowledgeWorkspace } from "./KnowledgeWorkspace";
import { MemoryWorkspace } from "./MemoryWorkspace";
import { ResourceState } from "./ResourceState";
import { SkillsWorkspace } from "./SkillsWorkspace";
import { ToolCatalogWorkspace } from "./ToolCatalogWorkspace";

interface ResourcesWorkspaceProps {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  skillsEnabled: boolean;
  knowledgeEnabled: boolean;
  memoryEnabled: boolean;
  view: ResourceView;
  skillView: SkillView;
  onViewChange(view: ResourceView): void;
  onSkillViewChange(view: SkillView): void;
}

export function ResourcesWorkspace(props: ResourcesWorkspaceProps) {
  const { t } = useLocalization();
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
  const tools = useQuery({
    queryKey: runtimeQueryKeys.tools(props.connection),
    queryFn: ({ signal }) => listDiagnosticTools(props.connection, signal),
    retry: 2,
  });

  return (
    <section
      className="resources-workspace"
      aria-label={t("resource.workspace")}
    >
      <nav className="resource-view-switch" aria-label={t("resource.types")}>
        <ResourceButton
          label={t("resource.skills")}
          selected={props.view === "skills"}
          onSelect={() => props.onViewChange("skills")}
        />
        <ResourceButton
          label={t("resource.tools")}
          count={tools.data?.data.length}
          selected={props.view === "tools"}
          onSelect={() => props.onViewChange("tools")}
        />
        <ResourceButton
          label={t("resource.recipes")}
          count={recipes.data?.data.length}
          selected={props.view === "recipes"}
          onSelect={() => props.onViewChange("recipes")}
        />
        <ResourceButton
          label={t("resource.agentDocs")}
          count={agentDocs.data?.data.length}
          selected={props.view === "agentDocs"}
          onSelect={() => props.onViewChange("agentDocs")}
        />
        <ResourceButton
          label={t("resource.knowledge")}
          selected={props.view === "knowledge"}
          onSelect={() => props.onViewChange("knowledge")}
        />
        <ResourceButton
          label={t("resource.memory")}
          selected={props.view === "memory"}
          onSelect={() => props.onViewChange("memory")}
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
      ) : props.view === "tools" ? (
        <ToolCatalogWorkspace
          connection={props.connection}
          workspace={props.workspace}
          values={tools.data?.data}
          pending={tools.isPending}
          error={tools.error}
          onRetry={() => void tools.refetch()}
        />
      ) : props.view === "recipes" ? (
        <RecipeCatalog
          values={recipes.data?.data}
          pending={recipes.isPending}
          error={recipes.error}
          onRetry={() => void recipes.refetch()}
        />
      ) : props.view === "agentDocs" ? (
        <AgentDocCatalog
          values={agentDocs.data?.data}
          pending={agentDocs.isPending}
          error={agentDocs.error}
          onRetry={() => void agentDocs.refetch()}
        />
      ) : props.view === "knowledge" ? (
        <KnowledgeWorkspace
          connection={props.connection}
          workspace={props.workspace}
          enabled={props.knowledgeEnabled}
        />
      ) : (
        <MemoryWorkspace
          connection={props.connection}
          workspace={props.workspace}
          enabled={props.memoryEnabled}
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
  const { formatNumber } = useLocalization();
  return (
    <button
      type="button"
      aria-current={props.selected ? "page" : undefined}
      onClick={props.onSelect}
    >
      <span>{props.label}</span>
      {props.count === undefined ? null : (
        <small>{formatNumber(props.count)}</small>
      )}
    </button>
  );
}

function RecipeCatalog(props: {
  values?: Recipe[];
  pending: boolean;
  error: Error | null;
  onRetry(): void;
}) {
  const { t } = useLocalization();
  if (props.pending) {
    return <ResourceState title={t("resource.discoveringRecipes")} />;
  }
  if (props.error) {
    return (
      <ResourceState
        title={t("resource.recipesFailed")}
        detail={messageOf(props.error, t("resource.loadingFailed"))}
        action={t("resource.tryAgain")}
        onAction={props.onRetry}
      />
    );
  }
  if (!props.values || props.values.length === 0) {
    return (
      <ResourceState
        title={t("resource.noRecipes")}
        detail={t("resource.noRecipesDetail")}
      />
    );
  }
  return (
    <div className="resource-card-list">
      {props.values.map((recipe) => (
        <article
          className="resource-card"
          key={`${recipe.scope}:${recipe.name}`}
        >
          <header>
            <div>
              <h4>/{recipe.name}</h4>
              {recipe.argumentHint ? (
                <small>{recipe.argumentHint}</small>
              ) : null}
            </div>
            <ResourceTag>{recipe.scope}</ResourceTag>
          </header>
          <p>{recipe.description || t("resource.noDescription")}</p>
          <details>
            <summary>{t("resource.viewPrompt")}</summary>
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
  const { t } = useLocalization();
  if (props.pending) {
    return <ResourceState title={t("resource.resolvingAgentDocs")} />;
  }
  if (props.error) {
    return (
      <ResourceState
        title={t("resource.agentDocsFailed")}
        detail={messageOf(props.error, t("resource.loadingFailed"))}
        action={t("resource.tryAgain")}
        onAction={props.onRetry}
      />
    );
  }
  if (!props.values || props.values.length === 0) {
    return (
      <ResourceState
        title={t("resource.noAgentDocs")}
        detail={t("resource.noAgentDocsDetail")}
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

function compactPath(path: string) {
  const parts = path.split(/[\\/]/).filter(Boolean);
  return parts.length > 3 ? `…/${parts.slice(-3).join("/")}` : path;
}

function messageOf(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}
