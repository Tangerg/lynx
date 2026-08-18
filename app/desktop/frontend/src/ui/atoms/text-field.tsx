import type { IconSize } from "@/lib/iconScale";
import type { VariantProps } from "class-variance-authority";
import { cva } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { Icon } from "@/ui/icons";
import { Button } from "./button";
import {
  InputPrimitive,
  TextAreaPrimitive,
  type InputPrimitiveProps,
  type TextAreaPrimitiveProps,
} from "@/ui/primitives";

// `variant` answers one question: who draws the edge?
//   boxed — this control does. A real border, because a text field is a fixed
//           control and the edge rule gives those a border rather than a shadow
//           ring (an input has to read as editable while at rest).
//   bare  — its container already did (a search bar, the composer surface, a
//           chip row). Metrics stay off in that case: the container set the
//           height, and a second one here would fight it.
const EDGE = {
  boxed:
    "rounded-[var(--field-radius)] border-[length:var(--control-edge-width)] border-field bg-canvas focus:border-field-strong",
  bare: "border-0 bg-transparent",
} as const;

// Invalid has to reach the eye through whichever edge the variant owns:
// recolour the border where there is one, add a ring where there isn't.
const INVALID = {
  boxed: "border-negative focus:border-negative",
  bare: "outline outline-1 outline-negative",
} as const;

// The type step is here and not per size: a field is large by height and inset, not by
// type. Only the composer's textarea steps up, and it does so because what is typed there
// is read back as a message.
const BASE =
  "w-full min-w-0 text-ui-md text-fg outline-none transition-colors placeholder:text-fg-faint " +
  "disabled:cursor-not-allowed disabled:opacity-60";

const SHARED_VARIANTS = {
  variant: EDGE,
  font: { mono: "font-mono", sans: "font-sans" },
  invalid: { true: "", false: "" },
} as const;

const INVALID_COMPOUNDS = [
  { variant: "boxed", invalid: true, class: INVALID.boxed },
  { variant: "bare", invalid: true, class: INVALID.bare },
] as const;

const inputStyles = cva(BASE, {
  variants: {
    ...SHARED_VARIANTS,
    // Height and inset only — the step lives in BASE.
    size: { sm: "", md: "", lg: "" },
  },
  compoundVariants: [
    { variant: "boxed", size: "sm", class: "h-[var(--field-height-sm)] px-2" },
    { variant: "boxed", size: "md", class: "h-[var(--field-height-md)] px-2.5" },
    { variant: "boxed", size: "lg", class: "h-[var(--field-height-lg)] px-3" },
    ...INVALID_COMPOUNDS,
  ],
  defaultVariants: { variant: "boxed", size: "md", font: "mono", invalid: false },
});

// A textarea has no height to set — `rows` and the resize handle own that — so
// its size step is the inset alone: `sm` for form rows, `md` for the prose and
// memory editors, which are read as much as typed into.
//
// `prose` is the composer's: what you type there is the message, so it has to be
// set at the size the message will be read at. It carries no inset of its own
// because the composer's own density tokens place the editor inside its card.
const textAreaStyles = cva(`${BASE} resize-y leading-body`, {
  variants: {
    ...SHARED_VARIANTS,
    size: {
      sm: "px-2.5 py-1.5",
      md: "px-3 py-2",
      prose: "text-prose leading-prose",
    },
    // Grow with what is typed, up to whatever `max-h` the caller sets. The engine
    // does this natively; the alternative every codebase reaches for first is an
    // effect that writes `height:auto`, reads `scrollHeight` and writes a pixel
    // back — a forced reflow on every keystroke, and a pixel cap that stops
    // tracking the type ladder the moment the user changes their text size.
    // Off by default: a form textarea wants the height its `rows` asked for.
    autosize: { true: "field-sizing-content resize-none", false: "" },
  },
  compoundVariants: [...INVALID_COMPOUNDS],
  defaultVariants: {
    variant: "boxed",
    size: "md",
    font: "mono",
    invalid: false,
    autosize: false,
  },
});

type FieldVariants = VariantProps<typeof inputStyles>;

export type TextFieldProps = Omit<InputPrimitiveProps, "size" | "className"> &
  FieldVariants & { className?: string };

export function TextField({ variant, size, font, invalid, className, ...props }: TextFieldProps) {
  return (
    <InputPrimitive
      {...props}
      data-slot="text-field"
      data-variant={variant ?? "boxed"}
      className={cn(inputStyles({ variant, size, font, invalid }), className)}
    />
  );
}

// `TextField` goes through Base UI's Input (a Field control, so it carries the
// field state contract and wires itself up if a caller ever puts it inside a
// `Field.Root`). Base UI ships no textarea part, and its control types every
// handler against `HTMLInputElement`, so routing one through it would cost a
// cast and buy nothing: a textarea's focus, keyboard and aria behaviour are
// entirely native. This is the documented Base-UI-first exemption, not an
// oversight.
export type TextAreaProps = Omit<TextAreaPrimitiveProps, "className"> &
  VariantProps<typeof textAreaStyles> & { className?: string };

export function TextArea({
  variant,
  size,
  font,
  invalid,
  autosize,
  className,
  ...props
}: TextAreaProps) {
  return (
    <TextAreaPrimitive
      {...props}
      className={cn(textAreaStyles({ variant, size, font, invalid, autosize }), className)}
    />
  );
}

// A search is a box with a magnifier in it, and the box has to grow the focus
// edge for the whole composite rather than for the input alone — which is why
// this is one component and not an instruction to wrap TextField in a div. It
// had been rediscovered three times, each reaching for a different edge (the
// field classes, a shadow ring, a literal border), so the affordance read
// slightly differently in each corner of the app.
const SEARCH_BOX = {
  sm: "h-[var(--field-height-sm)] gap-1.5 px-2",
  md: "h-[var(--field-height-md)] gap-1.5 px-2.5",
  lg: "h-[var(--field-height-lg)] gap-2 px-3",
} as const;

const SEARCH_GLYPH: Record<keyof typeof SEARCH_BOX, IconSize> = { sm: "xs", md: "sm", lg: "md" };

export type SearchFieldProps = Omit<TextFieldProps, "variant" | "invalid" | "size"> & {
  size?: keyof typeof SEARCH_BOX;
  /** Renders the clear affordance. Its label is the accessible name. */
  onClear?: () => void;
  clearLabel?: string;
};

export function SearchField({
  size = "md",
  font = "sans",
  onClear,
  clearLabel,
  className,
  ...props
}: SearchFieldProps) {
  return (
    <label
      className={cn(
        "flex items-center text-fg-muted focus-within:text-fg",
        EDGE.boxed,
        "focus-within:border-field-strong",
        SEARCH_BOX[size],
        className,
      )}
    >
      <Icon name="search" size={SEARCH_GLYPH[size]} className="shrink-0" />
      <TextField {...props} type="search" variant="bare" font={font} size={size} />
      {onClear && props.value !== "" && (
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onClear}
          aria-label={clearLabel}
          className="-mr-1 shrink-0"
        >
          <Icon name="x" size="xs" />
        </Button>
      )}
    </label>
  );
}
