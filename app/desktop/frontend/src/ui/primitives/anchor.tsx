import type { ComponentPropsWithRef } from "react";

export type AnchorPrimitiveProps = ComponentPropsWithRef<"a">;

export function AnchorPrimitive({ children, ...props }: AnchorPrimitiveProps) {
  return <a {...props}>{children}</a>;
}
