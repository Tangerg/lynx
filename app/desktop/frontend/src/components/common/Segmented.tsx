import { StudioTabs } from "@/components/agent-studio";
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
    <StudioTabs.Root
      value={String(value)}
      onValueChange={(v) => {
        const opt = options.find((o) => String(o.value) === v);
        if (opt) onChange(opt.value);
      }}
      className={cn(
        "inline-flex w-fit items-center gap-0.5 rounded-[8px] bg-surface-2 p-0.5 shadow-[inset_0_0_0_0.5px_var(--color-field)]",
        className,
      )}
    >
      <StudioTabs.List aria-label={ariaLabel} className="contents" activateOnFocus>
        {options.map((opt) => (
          <StudioTabs.Tab
            key={String(opt.value)}
            value={String(opt.value)}
            className={cn(
              "h-6 rounded-[6px] border-0 bg-transparent px-2.5 text-[12px] font-medium text-fg-muted transition-[background-color,color,box-shadow] duration-[120ms] ease-out hover:text-fg",
              mono && "font-mono",
              "data-[active]:bg-canvas data-[active]:text-fg data-[active]:shadow-[inset_0_0_0_0.5px_var(--color-field)]",
              "focus-visible:outline-none",
            )}
          >
            {opt.label}
          </StudioTabs.Tab>
        ))}
      </StudioTabs.List>
    </StudioTabs.Root>
  );
}
