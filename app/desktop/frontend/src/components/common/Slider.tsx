import { Slider as BaseSlider } from "@base-ui/react/slider";
import { cn } from "@/lib/utils";

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
    <BaseSlider.Root
      className={cn("relative flex h-4 touch-none select-none items-center", className ?? "w-36")}
      value={value}
      min={min}
      max={max}
      step={step}
      onValueChange={onValueChange}
    >
      <BaseSlider.Control className="relative flex h-4 grow items-center">
        <BaseSlider.Track className="relative h-1 grow rounded-full bg-surface-3">
          <BaseSlider.Indicator className="absolute h-full rounded-full bg-accent" />
        </BaseSlider.Track>
        <BaseSlider.Thumb
          getAriaLabel={() => ariaLabel}
          className="block h-3.5 w-3.5 rounded-full border border-field bg-fg shadow-[var(--shadow-focus)] transition-transform focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
        />
      </BaseSlider.Control>
    </BaseSlider.Root>
  );
}
