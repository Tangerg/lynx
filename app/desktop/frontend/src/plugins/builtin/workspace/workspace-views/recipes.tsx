// Built-in workspace view: "Recipes" — the prompt recipes discovered for the
// active session's cwd (workspace.recipes.list). Read-only catalog of the
// /<name> slash commands the recipes-slash plugin registers; mirrors the
// Skills view shape (recipes are skills' user-facing sibling).

import { DataView } from "@/components/common";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useWorkspaceRecipes } from "@/plugins/builtin/workspace/application/workspaceData";

function RecipesTab() {
  const t = useT();
  const cwd = useActiveSessionCwd();
  const { data, isLoading, isError } = useWorkspaceRecipes({ cwd });
  const recipes = data ?? [];

  return (
    <WorkspaceViewLayout
      icon="command"
      titleStrong
      title="recipes.title"
      sub={t("recipes.available", { count: recipes.length })}
      scrollClassName="py-1"
    >
      <DataView
        items={recipes}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={4}
        empty={{ icon: "command", title: t("recipes.empty.title"), sub: t("recipes.empty.sub") }}
      >
        {(rows) => (
          <div className="flex flex-col">
            {rows.map((r) => (
              <div key={`${r.source}:${r.name}`} className="px-4 py-2">
                <div className="flex items-center gap-2">
                  <span className="truncate font-mono text-[13px] font-semibold text-accent">
                    /{r.name}
                  </span>
                  {r.argumentHint && (
                    <span className="truncate font-mono text-[11px] text-fg-faint">
                      {r.argumentHint}
                    </span>
                  )}
                  {/* Layer the recipe came from — "project" (<cwd>/.lyra/recipes)
                      overrides "global" on a name collision. */}
                  <span className="ml-auto shrink-0 rounded-xs bg-surface-2 px-1.5 py-px font-mono text-[10px] text-fg-faint">
                    {r.scope}
                  </span>
                </div>
                {r.description && (
                  <div className="mt-0.5 text-[11.5px] leading-[1.45] text-fg-muted">
                    {r.description}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const recipesView = defineWorkspaceView({
  id: "recipes",
  title: "workspace.view.title.recipes",
  icon: "command",
  order: 46,
  splittable: true,
  component: RecipesTab,
});
