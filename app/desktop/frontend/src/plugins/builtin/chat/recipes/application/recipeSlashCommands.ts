import type { Disposable, Contributor } from "@/plugins/sdk";
import { queryClient } from "@/lib/queryClient";
import { lookupDataProvider } from "@/plugins/sdk";
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
import type { AgentSessionPorts } from "@/plugins/builtin/agent/public/ports";
import {
  AGENT_SESSIONS_KEY,
  activeSessionWorkspaceSelection,
  subscribeAgentSessionProjection,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import { RECIPES_KEY, type RecipesQuery } from "./recipeQueries";

const RECIPE_SIGNATURE_FIELD_SEPARATOR = "\u0000";
const RECIPE_SIGNATURE_ROW_SEPARATOR = "\u0001";

interface Recipe {
  name: string;
  description?: string;
  argumentHint?: string;
  body: string;
}

function expandRecipe(body: string, argStr: string): string {
  const trimmed = argStr.trim();
  const parts = trimmed.length ? trimmed.split(/\s+/) : [];
  return body
    .replaceAll("$ARGUMENTS", trimmed)
    .replace(/\$([1-9])(?!\d)/g, (_match, digit: string) => parts[Number(digit) - 1] ?? "");
}

export function recipeWorkspaceQuery(
  activeSessionId: string,
  sessions: readonly AgentSessionSummary[] | undefined,
): RecipesQuery | undefined {
  const selection = activeSessionWorkspaceSelection(activeSessionId, sessions);
  return selection.status === "ready" ? { cwd: selection.cwd } : undefined;
}

function sessionWorkspaceRevision(sessions: readonly AgentSessionSummary[] | undefined): string {
  return JSON.stringify(sessions?.map(({ id, workspace }) => [id, workspace.path]) ?? null);
}

function fetchRecipes(query: RecipesQuery): Promise<Recipe[]> {
  return queryClient.fetchQuery({
    queryKey: [RECIPES_KEY, query],
    staleTime: 60_000,
    queryFn: () => {
      const provider = lookupDataProvider<Recipe[], RecipesQuery>(RECIPES_KEY);
      return provider ? provider(query) : Promise.resolve<Recipe[]>([]);
    },
  });
}

function recipeSignature(recipes: Recipe[]): string {
  return recipes
    .map((recipe) => `${recipe.name}${RECIPE_SIGNATURE_FIELD_SEPARATOR}${recipe.body}`)
    .join(RECIPE_SIGNATURE_ROW_SEPARATOR);
}

export function installRecipeSlashCommands(
  ctx: Contributor,
  sessionPorts: AgentSessionPorts,
): () => void {
  let dynamic: Disposable[] = [];
  let lastSignature = "";

  const rebuild = (recipes: Recipe[]) => {
    const signature = recipeSignature(recipes);
    if (signature === lastSignature) return;
    lastSignature = signature;
    for (const disposable of dynamic) disposable.dispose();
    dynamic = recipes.map((recipe) => {
      const label = recipe.description || recipe.name;
      return ctx.contribute(
        SLASH_COMMAND,
        {
          description: recipe.argumentHint ? `${label}  ${recipe.argumentHint}` : label,
          run: ({ args, send }) => send(expandRecipe(recipe.body, args)),
        },
        { key: recipe.name },
      );
    });
  };

  let generation = 0;
  const refresh = () => {
    const current = ++generation;
    const sessions = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
    const query = recipeWorkspaceQuery(sessionPorts.activeSessionId(), sessions);
    // Remove commands from the previous project immediately. An active id whose
    // Session row has not arrived is not permission to fall back to the Runtime's
    // default workspace.
    if (!query) {
      rebuild([]);
      return;
    }
    void fetchRecipes(query)
      .then((recipes) => {
        if (current === generation) rebuild(recipes);
      })
      .catch(() => {
        if (current === generation) rebuild([]);
      });
  };

  refresh();
  const unsubscribeSession = sessionPorts.subscribeActiveSessionId(refresh);
  const unsubscribeQuery = subscribeAgentSessionProjection(sessionWorkspaceRevision, refresh);

  return () => {
    generation += 1;
    unsubscribeSession();
    unsubscribeQuery();
    for (const disposable of dynamic) disposable.dispose();
    dynamic = [];
    queryClient.removeQueries({ queryKey: [RECIPES_KEY], type: "inactive" });
  };
}
