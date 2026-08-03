import { TextArea } from "@/ui";

interface LinesFieldProps {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}

export function LinesField({ label, value, onChange, placeholder }: LinesFieldProps) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-ui-md font-medium text-fg">{label}</span>
      <TextArea
        size="sm"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        rows={2}
        aria-label={label}
        placeholder={placeholder}
      />
    </label>
  );
}
