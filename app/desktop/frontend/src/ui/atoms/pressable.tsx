import type { ReactNode } from "react";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

export type PressableProps = Omit<ButtonPrimitiveProps, "children"> & {
  children?: ReactNode;
};

// A composite surface whose content owns its layout and visual language: rows,
// cards, swatches, previews and disclosure headers. Pressable contributes only
// button semantics and the primitive's accessibility baseline. Ordinary actions
// belong to Button, IconButton or TextButton, which intentionally own metrics
// and presentation.
export function Pressable({ children, ...props }: PressableProps) {
  return <ButtonPrimitive {...props}>{children}</ButtonPrimitive>;
}
