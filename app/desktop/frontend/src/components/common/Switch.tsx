import { Switch as BaseSwitch } from "@base-ui/react/switch";
import { cn } from "@/lib/utils";

interface SwitchProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  ariaLabel?: string;
  className?: string;
}

export function Switch({ checked, onCheckedChange, disabled, ariaLabel, className }: SwitchProps) {
  return (
    <BaseSwitch.Root
      checked={checked}
      onCheckedChange={onCheckedChange}
      disabled={disabled}
      aria-label={ariaLabel}
      className={cn(
        "relative inline-flex h-5 w-9 shrink-0 items-center rounded-pill border transition-colors duration-150",
        "disabled:cursor-not-allowed disabled:opacity-50",
        "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
        checked ? "border-accent bg-accent" : "border-line bg-surface-2",
        className,
      )}
    >
      <BaseSwitch.Thumb
        className={cn(
          "block h-4 w-4 rounded-full bg-canvas shadow-[0_1px_2px_rgb(0_0_0_/_0.25)] transition-transform duration-150",
          "translate-x-0.5 data-[checked]:translate-x-[18px] data-[checked]:bg-on-accent",
        )}
      />
    </BaseSwitch.Root>
  );
}
