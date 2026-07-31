import type { ComponentProps, ComponentPropsWithRef } from "react";
import { Input } from "@base-ui/react/input";

export const InputPrimitive = Input;
export type InputPrimitiveProps = ComponentProps<typeof Input>;

export type FileInputPrimitiveProps = Omit<ComponentPropsWithRef<"input">, "type">;

export function FileInputPrimitive(props: FileInputPrimitiveProps) {
  return <input {...props} type="file" />;
}

export type ColorInputPrimitiveProps = Omit<ComponentPropsWithRef<"input">, "type">;

export function ColorInputPrimitive(props: ColorInputPrimitiveProps) {
  return <input {...props} type="color" />;
}
