import { cn } from "@/lib/utils";
import { Icon } from "@/ui/icons";
import { CheckboxPrimitive } from "@/ui/primitives";

interface CheckboxProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  /** Pair with a `<label htmlFor={id}>` so clicking the label toggles. */
  id?: string;
  ariaLabel?: string;
  className?: string;
}

export function Checkbox({ checked, onCheckedChange, id, ariaLabel, className }: CheckboxProps) {
  return (
    <CheckboxPrimitive.Root
      id={id}
      checked={checked}
      onCheckedChange={onCheckedChange}
      aria-label={ariaLabel}
      className={cn(
        "grid h-3.5 w-3.5 shrink-0 place-items-center rounded-xs border-[0.5px] border-field bg-surface-3 transition-colors duration-150",
        "data-[checked]:border-accent data-[checked]:bg-accent",
        "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
        className,
      )}
    >
      <CheckboxPrimitive.Indicator>
        <Icon name="check" size={10} strokeWidth={3} className="text-on-accent" />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  );
}
