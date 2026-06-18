import * as RadixCheckbox from "@radix-ui/react-checkbox";
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

// Checkbox on the Radix primitive — replaces the native <input type=checkbox>.
// 14px square, xs radius; accent fill + check glyph when checked, surface-2 +
// hairline when not. Keyboard + a11y handled by Radix.
export function Checkbox({ checked, onCheckedChange, id, ariaLabel, className }: CheckboxProps) {
  return (
    <RadixCheckbox.Root
      id={id}
      checked={checked}
      onCheckedChange={(c) => onCheckedChange(c === true)}
      aria-label={ariaLabel}
      className={cn(
        "grid h-3.5 w-3.5 shrink-0 place-items-center rounded-xs border border-line bg-surface-2 transition-colors duration-150",
        "data-[state=checked]:border-accent data-[state=checked]:bg-accent",
        "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
        className,
      )}
    >
      <RadixCheckbox.Indicator>
        <Icon name="check" size={10} strokeWidth={3} className="text-on-accent" />
      </RadixCheckbox.Indicator>
    </RadixCheckbox.Root>
  );
}
