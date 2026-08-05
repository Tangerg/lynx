import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { Pressable, type PressableProps } from "./pressable";

/**
 * A row in a floating list — a menu item, a search result, a suggestion in the
 * composer's pickers.
 *
 * Two mechanisms answer "which row is the keyboard on": Base UI sets
 * `data-highlighted`, and a hand-driven listbox knows its own index and says so
 * through `selected`. Both wash the row here, so which behaviour library a consumer
 * needs cannot change how selection looks.
 */
export const floatingRowStyles = cva(
  [
    "w-full items-center gap-2 rounded-[var(--shape-sm)] border-0 bg-transparent px-2 text-left",
    "text-ui-md text-fg outline-none transition-colors",
    // Hover first so a selected row that is also hovered stays selected.
    "hover:bg-hover",
    "aria-selected:bg-selected data-[highlighted]:bg-hover",
  ].join(" "),
  {
    variants: {
      /**
       * A menu's rows align their columns with each other, so a menu is a grid and the
       * consumer names the template. A result row carries optional trailing pieces — a
       * group name, a shortcut — and a grid template cannot describe a cell that may not
       * be there.
       */
      layout: { grid: "grid", flex: "flex" },
      size: {
        /** Menu items: the densest, and the only height that is a token, because menu
         *  rows have to line up with the rest of the chrome. */
        sm: "min-h-[var(--menu-row-height)] py-px",
        /** One line of content. */
        md: "h-8",
        /** Two stacked lines — a label over a description. */
        lg: "min-h-9 py-1.5",
      },
    },
    defaultVariants: { layout: "grid", size: "md" },
  },
);

export type OptionRowProps = Omit<PressableProps, "aria-selected"> &
  VariantProps<typeof floatingRowStyles> & {
    /**
     * Marks the row the keyboard is on, for a listbox driving selection by hand.
     * Passing it also makes the row an `option`, because `role="option"` requires
     * `aria-selected` and a call site that set the role and forgot the state would be
     * silently wrong to a screen reader.
     *
     * Leave unset where a behaviour library owns selection — Base UI sets its own
     * attributes and `undefined` here must not overwrite them.
     */
    selected?: boolean;
  };

export function OptionRow({ layout, size, selected, className, ...props }: OptionRowProps) {
  return (
    <Pressable
      {...props}
      type={props.type ?? "button"}
      {...(selected === undefined
        ? {}
        : { role: props.role ?? "option", "aria-selected": selected })}
      className={cn(floatingRowStyles({ layout, size }), className)}
    />
  );
}
