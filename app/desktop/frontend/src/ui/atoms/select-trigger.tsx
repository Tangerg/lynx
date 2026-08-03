import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon } from "@/ui/icons";
import { Pressable, type PressableProps } from "./pressable";

/**
 * The field-shaped face of a dropdown that stands in for a select.
 *
 * A menu trigger takes its appearance from whatever is handed to `render` — a `Button`
 * for a toolbar control, an `IconButton` for an overflow menu. There was no component
 * for the third shape, a field, so the three settings panes that needed one spelled the
 * whole skin out and the three drifted apart: inset 3 / 2.5 / 3, gap 2 / 2 / 2.5, the
 * type step inside the trigger in two of them and on a child span in the third, and only
 * one of them handling `disabled`. The chevron was copied three times too, each time as
 * a rotated `more` glyph.
 *
 * Width stays at the call site — how much room a locale name needs and how much a font
 * family needs are different questions — and so does anything the caller wants in front
 * of the label, like a swatch.
 */
export interface SelectTriggerProps extends Omit<PressableProps, "children"> {
  /** The current value, as the user reads it. Truncates rather than wrapping. */
  label: ReactNode;
  /** Rendered before the label — a colour swatch, a provider mark. */
  leading?: ReactNode;
}

export function SelectTrigger({ label, leading, className, ...props }: SelectTriggerProps) {
  return (
    <Pressable
      {...props}
      type={props.type ?? "button"}
      className={cn(
        "inline-flex w-fit min-h-[var(--field-height-md)] items-center justify-between gap-2",
        "rounded-[var(--field-radius)] border-[length:var(--control-edge-width)] border-field",
        "bg-surface-2 px-2.5 py-1.5 text-left text-ui-md font-medium text-fg transition-colors",
        "hover:bg-surface-3 data-[popup-open]:bg-surface-3",
        "disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-surface-2",
        className,
      )}
    >
      {leading}
      <span className="min-w-0 flex-1 truncate">{label}</span>
      <Icon name="more" size="xs" className="shrink-0 -rotate-90 text-fg-faint" />
    </Pressable>
  );
}
