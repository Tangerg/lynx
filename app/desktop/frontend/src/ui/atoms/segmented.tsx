import { cn } from "@/lib/classNames";
import { TabsPrimitive } from "@/ui/primitives";

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

export function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  ariaLabel,
  mono = false,
  className,
}: SegmentedProps<T>) {
  return (
    <TabsPrimitive.Root
      value={String(value)}
      onValueChange={(v) => {
        const opt = options.find((o) => String(o.value) === v);
        if (opt) onChange(opt.value);
      }}
      /* A recessed well, so the selected segment reads as a chip physically
         lifted out of it rather than a lighter rectangle painted on top. The
         chip's own rim + top-edge highlight (--shadow-raised-chip) is what sells
         the lift; the well's inner shadow is what it lifts out of. */
      className={cn(
        "inline-flex w-fit items-center gap-0.5 rounded-md p-0.5",
        "border border-field bg-control shadow-[var(--shadow-well)]",
        className,
      )}
    >
      <TabsPrimitive.List aria-label={ariaLabel} className="contents" activateOnFocus>
        {options.map((opt) => (
          <TabsPrimitive.Tab
            key={String(opt.value)}
            value={String(opt.value)}
            className={cn(
              "h-6 rounded-xs border border-transparent bg-transparent px-2 text-ui-sm font-medium",
              "text-fg-muted transition-[background-color,border-color,box-shadow,color] duration-[120ms] ease-out",
              mono && "font-mono",
              "hover:text-fg",
              "data-[active]:border-field data-[active]:bg-canvas data-[active]:text-fg",
              "data-[active]:shadow-[var(--shadow-raised-chip)]",
              "focus-visible:outline-none",
            )}
          >
            {opt.label}
          </TabsPrimitive.Tab>
        ))}
      </TabsPrimitive.List>
    </TabsPrimitive.Root>
  );
}
