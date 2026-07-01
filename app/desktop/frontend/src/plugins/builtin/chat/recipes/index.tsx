// Built-in plugin: registers one /<name> slash command per discovered recipe.
//
// Recipes (workspace.recipes.list) are user-invoked prompt templates; this
// plugin turns each into a live slash command whose run handler expands the
// template and sends it as a turn. The set is dynamic — re-resolved for the
// active session's cwd (project recipes layer over global), so switching
// sessions swaps which project's recipes are available.
//
// Registration is reactive and decoupled (the composer's slash autocomplete +
// submit are data-driven over SLASH_COMMAND): no kernel code knows about
// recipes. The browse view (workspace "Recipes") is the read-only catalog of
// the same set.

import type { Disposable } from "@/plugins/sdk";
import type { Recipe } from "@/rpc";
import type { RecipesQuery, SidebarSession } from "@/lib/data/queries";
import { RECIPES_KEY, SESSIONS_KEY } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { definePlugin, lookupDataProvider } from "@/plugins/sdk";
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
import {
  getActiveSessionId,
  subscribeActiveSessionId,
} from "@/plugins/builtin/agent/public/session";

const RECIPE_SIGNATURE_FIELD_SEPARATOR = "\u0000";
const RECIPE_SIGNATURE_ROW_SEPARATOR = "\u0001";

// expandRecipe substitutes the body's placeholders with the slash args:
// $ARGUMENTS = the whole trailing text; $1..$9 = the whitespace-split
// positionals. A referenced positional the user didn't supply collapses to ""
// (a recipe may name more $N than were typed); other "$…" text is left as-is.
function expandRecipe(body: string, argStr: string): string {
  const trimmed = argStr.trim();
  const parts = trimmed.length ? trimmed.split(/\s+/) : [];
  return body
    .replaceAll("$ARGUMENTS", trimmed)
    .replace(/\$([1-9])(?!\d)/g, (_m, d: string) => parts[Number(d) - 1] ?? "");
}

// activeCwd resolves the active session's working directory from the sessions
// cache — the same cwd the browse view passes, so project recipes resolve.
function activeCwd(): string | undefined {
  const id = getActiveSessionId();
  if (!id) return undefined;
  const sessions = queryClient.getQueryData<SidebarSession[]>([SESSIONS_KEY]);
  return sessions?.find((s) => s.id === id)?.cwd;
}

// fetchRecipes reads through the same query cache key the browse view uses, so
// the two share one fetch. Routes through the registered DATA_PROVIDER (which
// already swallows an unsupported method → []), so an old runtime just yields
// no recipes.
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

export default definePlugin({
  name: "lyra.builtin.recipes-slash",
  version: "1.0.0",
  setup({ host }) {
    let dynamic: Disposable[] = [];
    let lastSig = "";

    const rebuild = (recipes: Recipe[]) => {
      // Skip when the resolved set is unchanged — re-list fires on every
      // sessions-cache update, but the slash set only changes on cwd switch.
      const sig = recipes
        .map((r) => `${r.name}${RECIPE_SIGNATURE_FIELD_SEPARATOR}${r.body}`)
        .join(RECIPE_SIGNATURE_ROW_SEPARATOR);
      if (sig === lastSig) return;
      lastSig = sig;
      for (const d of dynamic) d.dispose();
      dynamic = recipes.map((r) => {
        const label = r.description || r.name;
        return host.extensions.contribute(
          SLASH_COMMAND,
          {
            description: r.argumentHint ? `${label}  ${r.argumentHint}` : label,
            run: ({ args, send }) => send(expandRecipe(r.body, args)),
          },
          { key: r.name },
        );
      });
    };

    // Generation token: a fast session switch (cwdA → cwdB) fires two refreshes
    // keyed on different cwds (fetchRecipes isn't deduped across cwds), so the
    // SLOWER (older cwd) resolve could land last and rebuild the slash set from
    // the wrong project. Drop a resolve that a newer refresh superseded — same
    // guard the workspace-events watch uses (rebuild's content-signature guard
    // alone can't tell which cwd produced the result).
    let gen = 0;
    const refresh = () => {
      const my = ++gen;
      void fetchRecipes(activeCwd())
        .then((r) => {
          if (my === gen) rebuild(r);
        })
        .catch(() => {
          if (my === gen) rebuild([]);
        });
    };

    refresh();
    const unsubSession = subscribeActiveSessionId(refresh);
    // The sessions list may load (or a cwd change) after the active id is set —
    // re-resolve when the SESSIONS cache updates (own RECIPES writes are filtered
    // out by the key check, so this doesn't recurse).
    const unsubQuery = queryClient.getQueryCache().subscribe((ev) => {
      if (ev.query.queryKey[0] === SESSIONS_KEY) refresh();
    });

    return () => {
      unsubSession();
      unsubQuery();
      for (const d of dynamic) d.dispose();
    };
  },
});
