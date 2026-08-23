import type { Recipe } from "@lyra/runtime-contract";

interface RecipeSuggestionsProps {
  sessionId: string;
  recipes: Recipe[];
  activeIndex: number;
  onChoose(recipe: Recipe): void;
}

export function RecipeSuggestions(props: RecipeSuggestionsProps) {
  return (
    <div
      id={`recipe-options-${props.sessionId}`}
      className="recipe-options"
      role="listbox"
      aria-label="Recipes"
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
          <p>{recipe.description || "No description provided."}</p>
        </button>
      ))}
    </div>
  );
}
