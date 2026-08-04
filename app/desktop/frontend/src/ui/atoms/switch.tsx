// Track is exactly twice the thumb — the ratio a physical toggle has, and the
// travel then reads as the thumb crossing the track rather than sliding along it.
import { cn } from "@/lib/classNames";
import { SwitchPrimitive } from "@/ui/primitives";

interface SwitchProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  ariaLabel?: string;
  className?: string;
}

export function Switch({ checked, onCheckedChange, disabled, ariaLabel, className }: SwitchProps) {
  return (
    <SwitchPrimitive.Root
      checked={checked}
      onCheckedChange={onCheckedChange}
      disabled={disabled}
      aria-label={ariaLabel}
      className={cn(
        "relative inline-flex h-5 w-8 shrink-0 items-center rounded-pill border-[length:var(--control-edge-width)] transition-colors duration-[var(--dur-color)]",
        "disabled:cursor-not-allowed disabled:opacity-50",
        checked ? "border-accent bg-accent" : "border-field bg-sunken",
        className,
      )}
    >
      <SwitchPrimitive.Thumb
        className={cn(
          "block h-4 w-4 rounded-full bg-canvas shadow-[var(--shadow-control)] transition-transform duration-[var(--dur-fast)]",
          "translate-x-0.5 data-[checked]:translate-x-[14px] data-[checked]:bg-on-accent",
        )}
      />
    </SwitchPrimitive.Root>
  );
}
