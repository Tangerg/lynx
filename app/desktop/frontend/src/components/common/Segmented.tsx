import * as ToggleGroup from "@radix-ui/react-toggle-group";
import { cn } from "@/lib/utils";

export interface SegmentedOption<T> {
  value: T;
  label: string;
}

interface SegmentedProps<T extends string | number> {
  value: T;
  options: SegmentedOption<T>[];
  onChange: (value: T) => void;
  ariaLabel: string;
  /** Render labels in tabular mono — for numeric segments (e.g. font size). */
  mono?: boolean;
  className?: string;
}

// Segmented control on the Radix ToggleGroup primitive (single-select) —
// roving-tabindex + arrow-key nav + proper radiogroup semantics for free,
// replacing the previous hand-rolled role="radiogroup" button rows. Values
// can be string or number; ToggleGroup keys on strings, so we serialize +
// map back. Visuals match DESIGN's segmented-control (surface-2 track, active
// segment lifts to surface + ink).
export function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  ariaLabel,
  mono = false,
  className,
}: SegmentedProps<T>) {
  return (
    <ToggleGroup.Root
      type="single"
      value={String(value)}
      // type="single" allows deselect (empty string) — ignore that so a
      // segmented control always has exactly one active segment.
      onValueChange={(v) => {
        if (v === "") return;
        const opt = options.find((o) => String(o.value) === v);
        if (opt) onChange(opt.value);
      }}
      aria-label={ariaLabel}
      className={cn(
        "inline-flex w-fit items-center gap-1 rounded-md border border-line bg-surface-2 p-1",
        className,
      )}
    >
      {options.map((opt) => (
        <ToggleGroup.Item
          key={String(opt.value)}
          value={String(opt.value)}
          className={cn(
            "rounded-sm px-2.5 py-0.5 text-[12px] font-medium text-fg-muted cursor-pointer transition-colors duration-150 hover:text-fg",
            mono && "font-mono",
            "data-[state=on]:bg-surface data-[state=on]:text-fg data-[state=on]:shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]",
            "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
          )}
        >
          {opt.label}
        </ToggleGroup.Item>
      ))}
    </ToggleGroup.Root>
  );
}
