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

// Two-or-three-state segmented control used for theme switch + composer modes.
export function Segmented<T extends string>({ options, value, onChange }: Props<T>) {
  return (
    <div className="sp-segmented">
      {options.map((o) => (
        <button
          key={o.value}
          className={cn("sp-seg", value === o.value && "active")}
          onClick={() => { if (value !== o.value) onChange(o.value); }}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
