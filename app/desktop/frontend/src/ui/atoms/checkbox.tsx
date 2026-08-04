import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon } from "@/ui/icons";
import { CheckboxPrimitive } from "@/ui/primitives";

interface CheckboxProps {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  /** Visible caption. Also the control's accessible name — the box is wrapped in
   *  the label element, so the two can never drift apart. */
  label: ReactNode;
  disabled?: boolean;
  /** Layout only (`ml-auto`, `mt-1`); tone and size come from the atom. */
  className?: string;
}

// The box owns its label rather than documenting "remember to pair me with one".
// Wrapping associates the two implicitly, so there is no id to invent, no
// `htmlFor` to keep in sync, and no `aria-label` to fall out of step with the
// text the user can actually see.
export function Checkbox({ checked, onCheckedChange, label, disabled, className }: CheckboxProps) {
  return (
    <label
      className={cn(
        "inline-flex items-center gap-2 text-ui-md text-fg-muted select-none",
        disabled ? "cursor-not-allowed opacity-60" : "cursor-default",
        className,
      )}
    >
      <CheckboxPrimitive.Root
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
        className={cn(
          "grid h-[18px] w-[18px] shrink-0 place-items-center rounded-2xs border-[length:var(--control-edge-width)] border-field bg-canvas transition-colors duration-[var(--dur-color)]",
          "data-[checked]:border-accent data-[checked]:bg-accent",
        )}
      >
        <CheckboxPrimitive.Indicator>
          <Icon name="check" size="xs" className="text-on-accent" />
        </CheckboxPrimitive.Indicator>
      </CheckboxPrimitive.Root>
      <span>{label}</span>
    </label>
  );
}
