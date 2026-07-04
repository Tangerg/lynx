import { FIELD_CLASSES } from "@/ui";
import { cn } from "@/lib/utils";

export const FIELD = cn(FIELD_CLASSES, "h-8 w-full px-2.5 text-fg placeholder:text-fg-faint");

export const TEXT_AREA = cn(
  FIELD_CLASSES,
  "w-full resize-y px-2.5 py-1.5 leading-[1.5] text-fg placeholder:text-fg-faint",
);

interface LinesFieldProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}

export function LinesField({ label, value, onChange, placeholder }: LinesFieldProps) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-[13px] font-medium text-fg">{label}</span>
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
