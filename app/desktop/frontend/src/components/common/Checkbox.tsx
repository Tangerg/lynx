import { Checkbox as BaseCheckbox } from "@base-ui/react/checkbox";
import { cn } from "@/lib/utils";
import { Icon } from "./Icon";

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
    <BaseCheckbox.Root
      id={id}
      checked={checked}
      onCheckedChange={onCheckedChange}
      aria-label={ariaLabel}
      className={cn(
        "grid h-3.5 w-3.5 shrink-0 place-items-center rounded-xs border-[0.5px] border-fg-faint/25 bg-surface-2 transition-colors duration-150",
        "data-[checked]:border-accent data-[checked]:bg-accent",
        "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
        className,
      )}
    >
      <BaseCheckbox.Indicator>
        <Icon name="check" size={10} strokeWidth={3} className="text-on-accent" />
      </BaseCheckbox.Indicator>
    </BaseCheckbox.Root>
  );
}
