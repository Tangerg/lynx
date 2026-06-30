import * as RadixSwitch from "@radix-ui/react-switch";
import { cn } from "@/lib/utils";

interface SwitchProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  ariaLabel?: string;
  className?: string;
}

// On/off toggle on the Radix Switch primitive — `role="switch"` + keyboard +
// a11y for free (the right primitive for an enablement toggle, vs Checkbox
// which is for a selection in a set). Accent-filled track + sliding thumb when
// on, surface-2 + hairline when off — the same on=accent treatment Checkbox
// uses, so the two read as one toggle family.
export function Switch({ checked, onCheckedChange, disabled, ariaLabel, className }: SwitchProps) {
  return (
    <RadixSwitch.Root
      checked={checked}
      onCheckedChange={onCheckedChange}
      disabled={disabled}
      aria-label={ariaLabel}
      className={cn(
        "relative inline-flex h-4 w-7 shrink-0 items-center rounded-pill border transition-colors duration-150",
        "disabled:cursor-not-allowed disabled:opacity-50",
        "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
        checked ? "border-accent bg-accent" : "border-line bg-surface-2",
        className,
      )}
    >
      <RadixSwitch.Thumb
        className={cn(
          "block h-3 w-3 rounded-full bg-fg shadow-[var(--shadow-focus)] transition-transform duration-150",
          "translate-x-0.5 data-[state=checked]:translate-x-[14px] data-[state=checked]:bg-on-accent",
        )}
      />
    </RadixSwitch.Root>
  );
}
