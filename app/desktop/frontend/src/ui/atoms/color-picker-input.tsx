import type { ColorInputPrimitiveProps } from "@/ui/primitives";
import { ColorInputPrimitive } from "@/ui/primitives";

export type ColorPickerInputProps = Omit<ColorInputPrimitiveProps, "className">;

export function ColorPickerInput(props: ColorPickerInputProps) {
  return (
    <ColorInputPrimitive
      {...props}
      className="absolute inset-0 h-full w-full cursor-pointer opacity-0"
    />
  );
}
