import type { Recipe } from "@lyra/runtime-contract";

export function slashRecipeQuery(text: string) {
  if (!text.startsWith("/")) return undefined;
  const query = text.slice(1);
  return /\s/.test(query) ? undefined : query.toLowerCase();
}

export function expandRecipeInvocation(text: string, recipes: Recipe[]) {
  const match = /^\/([A-Za-z0-9_-]+)(?:\s+([\s\S]*))?$/.exec(text.trim());
  if (match === null) return text;
  const recipe = recipes.find((candidate) => candidate.name === match[1]);
  if (recipe === undefined) return text;
  const argumentsText = (match[2] ?? "").trim();
  const argumentsByPosition =
    argumentsText === "" ? [] : argumentsText.split(/\s+/);
  return recipe.body
    .replaceAll("$ARGUMENTS", argumentsText)
    .replace(/\$([1-9])(?!\d)/g, (_placeholder, position: string) =>
      argumentsByPosition[Number(position) - 1] ?? "",
    );
}
