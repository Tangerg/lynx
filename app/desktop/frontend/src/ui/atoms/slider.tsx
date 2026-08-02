import { cn } from "@/lib/classNames";
import { SliderPrimitive } from "@/ui/primitives";

interface SliderProps {
  value: number;
  min?: number;
  max?: number;
  step?: number;
  onValueChange: (value: number) => void;
  ariaLabel: string;
  className?: string;
}

export function Slider({
  value,
  min = 0,
  max = 100,
  step = 1,
  onValueChange,
  ariaLabel,
  className,
}: SliderProps) {
  return (
    <SliderPrimitive.Root
      className={cn("relative flex h-4 touch-none select-none items-center", className ?? "w-36")}
      value={value}
      min={min}
      max={max}
      step={step}
      onValueChange={onValueChange}
    >
      <SliderPrimitive.Control className="relative flex h-4 grow items-center">
        <SliderPrimitive.Track className="relative h-1 grow rounded-full bg-sunken">
          <SliderPrimitive.Indicator className="absolute h-full rounded-full bg-accent" />
        </SliderPrimitive.Track>
        <SliderPrimitive.Thumb
          getAriaLabel={() => ariaLabel}
          className="block h-3.5 w-3.5 rounded-full bg-canvas shadow-[var(--shadow-control)] transition-transform"
        />
      </SliderPrimitive.Control>
    </SliderPrimitive.Root>
  );
}
