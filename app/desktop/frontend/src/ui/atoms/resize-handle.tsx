import type { SeparatorPrimitiveProps } from "@/ui/primitives";
import { SeparatorPrimitive } from "@/ui/primitives";

export type ResizeHandleProps = Omit<SeparatorPrimitiveProps, "orientation" | "role" | "tabIndex">;

export function ResizeHandle(props: ResizeHandleProps) {
  return <SeparatorPrimitive {...props} orientation="vertical" tabIndex={0} />;
}
