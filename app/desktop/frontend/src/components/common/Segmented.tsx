import { Tabs as BaseTabs } from "@base-ui/react/tabs";
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

export function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  ariaLabel,
  mono = false,
  className,
}: SegmentedProps<T>) {
  return (
    <BaseTabs.Root
      value={String(value)}
      onValueChange={(v) => {
        const opt = options.find((o) => String(o.value) === v);
        if (opt) onChange(opt.value);
      }}
      className={cn(
        "inline-flex w-fit items-center gap-1 rounded-md border border-line bg-surface-2 p-1",
        className,
      )}
    >
      <BaseTabs.List aria-label={ariaLabel} className="contents" activateOnFocus>
        {options.map((opt) => (
          <BaseTabs.Tab
            key={String(opt.value)}
            value={String(opt.value)}
            className={cn(
              "rounded-sm px-2.5 py-0.5 text-[12px] font-medium text-fg-muted transition-colors duration-150 hover:text-fg",
              mono && "font-mono",
              "data-[active]:bg-surface data-[active]:text-fg data-[active]:shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]",
              "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
            )}
          >
            {opt.label}
          </BaseTabs.Tab>
        ))}
      </BaseTabs.List>
    </BaseTabs.Root>
  );
}
