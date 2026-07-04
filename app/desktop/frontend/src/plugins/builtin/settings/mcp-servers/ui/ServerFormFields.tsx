import { INPUT_FOCUS_RING } from "@/ui";
import { cn } from "@/lib/utils";

export const FIELD = cn(
  "h-8 w-full rounded-md border-[0.5px] border-field bg-surface px-2.5 font-mono text-[12px] text-fg outline-none placeholder:text-fg-faint",
  INPUT_FOCUS_RING,
);

export const TEXT_AREA = cn(
  "w-full resize-y rounded-md border-[0.5px] border-field bg-surface px-2.5 py-1.5 font-mono text-[12px] leading-[1.5] text-fg outline-none placeholder:text-fg-faint",
  INPUT_FOCUS_RING,
);

interface LinesFieldProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}

export function LinesField({ label, value, onChange, placeholder }: LinesFieldProps) {
  return (
    <label className="flex flex-col gap-1 text-[11px] text-fg-muted">
      {label}
      <textarea
        value={value}
        onChange={(event) => onChange(event.target.value)}
        rows={2}
        aria-label={label}
        placeholder={placeholder}
        className={TEXT_AREA}
      />
    </label>
  );
}
