import { cn } from "@/lib/utils";
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
        <SliderPrimitive.Track className="relative h-1 grow rounded-full bg-surface-3">
          <SliderPrimitive.Indicator className="absolute h-full rounded-full bg-accent" />
        </SliderPrimitive.Track>
        <SliderPrimitive.Thumb
          getAriaLabel={() => ariaLabel}
          className="block h-3.5 w-3.5 rounded-full border-[0.5px] border-field bg-fg shadow-[0_1px_2px_rgb(0_0_0_/_0.25)] transition-transform focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
        />
      </SliderPrimitive.Control>
    </SliderPrimitive.Root>
  );
}
