import * as RadixSlider from "@radix-ui/react-slider";
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

// Single-value slider on the Radix primitive — keyboard + a11y for free.
// Styled to the surface/accent ladder: surface-3 track, accent fill, an
// fg-filled thumb (matches the rest of the appearance controls).
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
    <RadixSlider.Root
      className={cn("relative flex h-4 touch-none select-none items-center", className ?? "w-36")}
      value={[value]}
      min={min}
      max={max}
      step={step}
      onValueChange={(v) => onValueChange(v[0] ?? value)}
      aria-label={ariaLabel}
    >
      <RadixSlider.Track className="relative h-1 grow rounded-full bg-surface-3">
        <RadixSlider.Range className="absolute h-full rounded-full bg-accent" />
      </RadixSlider.Track>
      <RadixSlider.Thumb className="block h-3.5 w-3.5 rounded-full border border-line bg-fg shadow-sm transition-transform hover:scale-110 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent" />
    </RadixSlider.Root>
  );
}
