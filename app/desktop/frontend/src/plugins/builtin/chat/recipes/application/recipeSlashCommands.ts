import type { Disposable, Host } from "@/plugins/sdk";
import type { Recipe } from "@/rpc";
import type { RecipesQuery, SidebarSession } from "@/lib/data/queries";
import { RECIPES_KEY, SESSIONS_KEY } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { lookupDataProvider } from "@/plugins/sdk";
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
import {
  getActiveSessionId,
  subscribeActiveSessionId,
} from "@/plugins/builtin/agent/public/session";

const RECIPE_SIGNATURE_FIELD_SEPARATOR = "\u0000";
const RECIPE_SIGNATURE_ROW_SEPARATOR = "\u0001";

function expandRecipe(body: string, argStr: string): string {
  const trimmed = argStr.trim();
  const parts = trimmed.length ? trimmed.split(/\s+/) : [];
  return body
    .replaceAll("$ARGUMENTS", trimmed)
    .replace(/\$([1-9])(?!\d)/g, (_match, digit: string) => parts[Number(digit) - 1] ?? "");
}

function activeCwd(): string | undefined {
  const id = getActiveSessionId();
  if (!id) return undefined;
  const sessions = queryClient.getQueryData<SidebarSession[]>([SESSIONS_KEY]);
  return sessions?.find((session) => session.id === id)?.cwd;
}

function fetchRecipes(cwd?: string): Promise<Recipe[]> {
  return queryClient.fetchQuery({
    queryKey: [RECIPES_KEY, { cwd }],
    staleTime: 60_000,
    queryFn: () => {
      const provider = lookupDataProvider<Recipe[], RecipesQuery>(RECIPES_KEY);
      return provider ? provider({ cwd }) : Promise.resolve<Recipe[]>([]);
    },
  });
}

function recipeSignature(recipes: Recipe[]): string {
  return recipes
    .map((recipe) => `${recipe.name}${RECIPE_SIGNATURE_FIELD_SEPARATOR}${recipe.body}`)
    .join(RECIPE_SIGNATURE_ROW_SEPARATOR);
}

export function installRecipeSlashCommands(host: Host): () => void {
  let dynamic: Disposable[] = [];
  let lastSignature = "";

  const rebuild = (recipes: Recipe[]) => {
    const signature = recipeSignature(recipes);
    if (signature === lastSignature) return;
    lastSignature = signature;
    for (const disposable of dynamic) disposable.dispose();
    dynamic = recipes.map((recipe) => {
      const label = recipe.description || recipe.name;
      return host.extensions.contribute(
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
    void fetchRecipes(activeCwd())
      .then((recipes) => {
        if (current === generation) rebuild(recipes);
      })
      .catch(() => {
        if (current === generation) rebuild([]);
      });
  };

  refresh();
  const unsubscribeSession = subscribeActiveSessionId(refresh);
  const unsubscribeQuery = queryClient.getQueryCache().subscribe((event) => {
    if (event.query.queryKey[0] === SESSIONS_KEY) refresh();
  });

  return () => {
    unsubscribeSession();
    unsubscribeQuery();
    for (const disposable of dynamic) disposable.dispose();
  };
}
