import type { FileInputPrimitiveProps } from "@/ui/primitives";
import { FileInputPrimitive } from "@/ui/primitives";

export type HiddenFileInputProps = Omit<FileInputPrimitiveProps, "className">;

export function HiddenFileInput(props: HiddenFileInputProps) {
  return <FileInputPrimitive {...props} className="hidden" />;
}
