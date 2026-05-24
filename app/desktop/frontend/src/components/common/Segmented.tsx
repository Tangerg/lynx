import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

type Option<T extends string> = {
  value: T;
  label: ReactNode;
};

type Props<T extends string> = {
  options: Option<T>[];
  value: T;
  onChange: (v: T) => void;
};

// Two-or-three-state segmented control used for theme switch + composer
// modes. Active segment lifts to surface-3 against the surface-2 track.
export function Segmented<T extends string>({ options, value, onChange }: Props<T>) {
  return (
    <div className="inline-flex items-center gap-0.5 rounded-sm border border-line bg-surface-3 p-0.5">
      {options.map((o) => {
        const active = value === o.value;
        return (
          <button
            key={o.value}
            type="button"
            onClick={() => {
              if (!active) onChange(o.value);
            }}
            className={cn(
              "inline-flex h-6.5 flex-1 items-center justify-center gap-1.5 rounded-xs px-3 font-sans text-[11.5px] font-semibold transition-colors duration-150 ease-out",
              active
                ? "bg-surface-3 text-fg shadow-[0_1px_0_rgba(0,0,0,0.4),0_0_0_0.5px_rgba(255,255,255,0.04)_inset]"
                : "text-fg-muted hover:text-fg",
            )}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}
