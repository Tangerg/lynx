import type { Recipe } from "@lyra/runtime-contract";

import { useLocalization } from "../localization/Localization";

interface RecipeSuggestionsProps {
  sessionId: string;
  recipes: Recipe[];
  activeIndex: number;
  onChoose(recipe: Recipe): void;
}

export function RecipeSuggestions(props: RecipeSuggestionsProps) {
  const { t } = useLocalization();
  return (
    <div
      id={`recipe-options-${props.sessionId}`}
      className="recipe-options"
      role="listbox"
      aria-label={t("recipe.listLabel")}
    >
      {props.recipes.map((recipe, index) => (
        <button
          id={`recipe-option-${props.sessionId}-${index}`}
          key={`${recipe.scope}:${recipe.name}`}
          type="button"
          role="option"
          aria-selected={index === props.activeIndex}
          onMouseDown={(event) => event.preventDefault()}
          onClick={() => props.onChoose(recipe)}
        >
          <span>
            <strong>/{recipe.name}</strong>
            {recipe.argumentHint ? <small>{recipe.argumentHint}</small> : null}
          </span>
          <p>{recipe.description || t("recipe.noDescription")}</p>
        </button>
      ))}
    </div>
  );
}
