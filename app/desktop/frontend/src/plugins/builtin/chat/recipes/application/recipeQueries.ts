import { createParameterizedDataQuery } from "@/lib/data/dataQuery";

export interface RecipeReadModel {
  name: string;
  description?: string;
  argumentHint?: string;
  body: string;
  scope: "project" | "global";
  source: string;
}

export interface RecipesQuery {
  cwd?: string;
}

export const RECIPES_KEY = "recipes";
export const useRecipes = createParameterizedDataQuery<RecipesQuery, RecipeReadModel[]>(
  RECIPES_KEY,
);
