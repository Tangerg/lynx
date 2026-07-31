import type { ComponentPropsWithRef } from "react";

export type TextAreaPrimitiveProps = ComponentPropsWithRef<"textarea">;

export function TextAreaPrimitive(props: TextAreaPrimitiveProps) {
  return <textarea {...props} />;
}
