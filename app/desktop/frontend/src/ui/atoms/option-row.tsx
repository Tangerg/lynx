import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { Pressable, type PressableProps } from "./pressable";

/**
 * A row in a floating list — a menu item, a command-palette result, a suggestion in
 * the composer's @-file or slash picker.
 *
 * It had been written five times, once inside this ring and four times outside it, and
 * the copies disagreed on every value: radius (`sm` / `md`), inset (2 / 2.5), gap
 * (2 / 2.5), and the type step. The part that actually showed was the last one — three
 * different mechanisms answer "which row is the keyboard on" (Base UI sets
 * `data-highlighted`, cmdk sets `aria-selected` and `data-selected`, a hand-driven
 * listbox knows its own index) and each copy had wired a different wash to whichever one
 * it used.
 *
 * All three are handled here, so which behaviour library a consumer needs cannot change
 * how selection looks — and a list driven by cmdk needs no `selected` prop, because the
 * attributes it already sets are two of the three.
 */
export const floatingRowStyles = cva(
  [
    "w-full items-center gap-2 rounded-[var(--shape-sm)] border-0 bg-transparent px-2 text-left",
    "text-ui-md text-fg outline-none transition-colors",
    // Hover first so a selected row that is also hovered stays selected.
    "hover:bg-hover",
    "aria-selected:bg-selected data-[highlighted]:bg-hover data-[selected]:bg-selected",
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
     *
     * Passing it also makes the row an `option`: `role="option"` REQUIRES
     * `aria-selected`, and the two belong to whoever knows the answer. Split across the
     * boundary — role at the call site, state in here — the pair reads as incomplete to
     * anything checking statically, and a call site that set the role and forgot the
     * state would be silently wrong to a screen reader.
     *
     * Leave unset where a behaviour library owns selection: cmdk sets `aria-selected`
     * and `data-selected` itself, and passing `undefined` here must not overwrite them.
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
      {...(selected ? { "data-selected": "" } : {})}
      className={cn(floatingRowStyles({ layout, size }), className)}
    />
  );
}
